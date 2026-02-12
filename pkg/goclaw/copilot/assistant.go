// Package copilot implementa o orquestrador principal do AgentGo Copilot.
// Coordena canais, skills, scheduler, memória e segurança para processar
// mensagens de usuários e gerar respostas via LLM.
package copilot

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jholhewres/goclaw/pkg/goclaw/channels"
	"github.com/jholhewres/goclaw/pkg/goclaw/copilot/security"
	"github.com/jholhewres/goclaw/pkg/goclaw/scheduler"
	"github.com/jholhewres/goclaw/pkg/goclaw/skills"
)

// Assistant é o orquestrador principal do AgentGo Copilot.
// Coordena o fluxo completo: recepção → validação → contexto → execução → resposta.
type Assistant struct {
	// config contém as configurações do assistente.
	config *Config

	// channelMgr gerencia os canais de comunicação.
	channelMgr *channels.Manager

	// skillRegistry gerencia as skills disponíveis.
	skillRegistry *skills.Registry

	// scheduler gerencia tarefas agendadas.
	scheduler *scheduler.Scheduler

	// sessionStore gerencia sessões por chat/grupo.
	sessionStore *SessionStore

	// promptComposer monta o prompt em camadas.
	promptComposer *PromptComposer

	// inputGuard valida inputs antes do processamento.
	inputGuard *security.InputGuardrail

	// outputGuard valida outputs antes do envio.
	outputGuard *security.OutputGuardrail

	// logger para logs estruturados.
	logger *slog.Logger

	ctx    context.Context
	cancel context.CancelFunc
}

// New cria um novo Assistant com as dependências fornecidas.
// Se cfg for nil, usa a configuração padrão.
func New(cfg *Config, logger *slog.Logger) *Assistant {
	if cfg == nil {
		cfg = DefaultConfig()
	}
	if logger == nil {
		logger = slog.Default()
	}

	return &Assistant{
		config:         cfg,
		channelMgr:     channels.NewManager(logger.With("component", "channels")),
		skillRegistry:  skills.NewRegistry(logger.With("component", "skills")),
		sessionStore:   NewSessionStore(logger.With("component", "sessions")),
		promptComposer: NewPromptComposer(cfg),
		inputGuard:     security.NewInputGuardrail(cfg.Security.MaxInputLength, cfg.Security.RateLimit),
		outputGuard:    security.NewOutputGuardrail(),
		logger:         logger,
	}
}

// Start inicializa e inicia todos os subsistemas do assistente.
func (a *Assistant) Start(ctx context.Context) error {
	a.ctx, a.cancel = context.WithCancel(ctx)

	a.logger.Info("iniciando AgentGo Copilot",
		"name", a.config.Name,
		"model", a.config.Model,
	)

	// 1. Carrega skills de todos os loaders.
	if err := a.skillRegistry.LoadAll(a.ctx); err != nil {
		a.logger.Error("falha ao carregar skills", "error", err)
	}

	// 2. Inicia o gerenciador de canais (permite 0 canais no modo CLI).
	if err := a.channelMgr.Start(a.ctx); err != nil {
		return fmt.Errorf("falha ao iniciar canais: %w", err)
	}

	// 2.1. Inicia pruner de sessões inativas.
	a.sessionStore.StartPruner(a.ctx)

	// 3. Inicia o scheduler, se habilitado.
	if a.scheduler != nil {
		if err := a.scheduler.Start(a.ctx); err != nil {
			a.logger.Error("falha ao iniciar scheduler", "error", err)
		}
	}

	// 4. Inicia o loop principal de processamento de mensagens.
	go a.messageLoop()

	a.logger.Info("AgentGo Copilot iniciado com sucesso")
	return nil
}

// Stop encerra todos os subsistemas de forma graciosa.
func (a *Assistant) Stop() {
	a.logger.Info("encerrando AgentGo Copilot...")

	if a.cancel != nil {
		a.cancel()
	}

	// Encerra em ordem reversa de inicialização.
	if a.scheduler != nil {
		a.scheduler.Stop()
	}
	a.channelMgr.Stop()
	a.skillRegistry.ShutdownAll()

	a.logger.Info("AgentGo Copilot encerrado")
}

// ChannelManager retorna o gerenciador de canais para registro externo.
func (a *Assistant) ChannelManager() *channels.Manager {
	return a.channelMgr
}

// SkillRegistry retorna o registro de skills.
func (a *Assistant) SkillRegistry() *skills.Registry {
	return a.skillRegistry
}

// SetScheduler configura o scheduler do assistente.
func (a *Assistant) SetScheduler(s *scheduler.Scheduler) {
	a.scheduler = s
}

// messageLoop é o loop principal que processa mensagens de todos os canais.
func (a *Assistant) messageLoop() {
	for {
		select {
		case msg, ok := <-a.channelMgr.Messages():
			if !ok {
				return
			}
			go a.handleMessage(msg)

		case <-a.ctx.Done():
			return
		}
	}
}

// handleMessage processa uma mensagem individual seguindo o fluxo completo:
// trigger check → input validation → context build → agent execution → output validation → send.
func (a *Assistant) handleMessage(msg *channels.IncomingMessage) {
	start := time.Now()
	logger := a.logger.With(
		"channel", msg.Channel,
		"chat_id", msg.ChatID,
		"from", msg.From,
		"msg_id", msg.ID,
	)

	// 1. Verifica trigger (ex: @copilot).
	if !a.matchesTrigger(msg.Content) {
		return
	}

	logger.Info("mensagem recebida, processando...")

	// 2. Valida input (injection, rate limit, tamanho).
	if err := a.inputGuard.Validate(msg.From, msg.Content); err != nil {
		logger.Warn("input rejeitado", "error", err)
		a.sendReply(msg, fmt.Sprintf("Desculpe, não posso processar: %v", err))
		return
	}

	// 3. Carrega/cria sessão para este chat.
	session := a.sessionStore.GetOrCreate(msg.Channel, msg.ChatID)

	// 4. Monta o prompt com todas as camadas.
	prompt := a.promptComposer.Compose(session, msg.Content)

	// 5. Executa o agente com LLM.
	// TODO: Integrar com o agent executor real.
	response := a.executeAgent(a.ctx, prompt, session)

	// 6. Valida output (URLs, PII, fatos).
	if err := a.outputGuard.Validate(response); err != nil {
		logger.Warn("output rejeitado, aplicando fallback", "error", err)
		response = "Desculpe, encontrei um problema ao gerar a resposta. Pode reformular?"
	}

	// 7. Atualiza sessão com a conversa.
	session.AddMessage(msg.Content, response)

	// 8. Envia resposta.
	a.sendReply(msg, response)

	logger.Info("mensagem processada",
		"duration_ms", time.Since(start).Milliseconds(),
	)
}

// matchesTrigger verifica se a mensagem contém a palavra-chave de ativação.
func (a *Assistant) matchesTrigger(content string) bool {
	// TODO: Implementar matching mais sofisticado (regex, menção, DM direto).
	trigger := a.config.Trigger
	if trigger == "" {
		return true // Sem trigger = sempre responde (modo DM).
	}
	return len(content) >= len(trigger) && content[:len(trigger)] == trigger
}

// executeAgent executa o agente LLM com o prompt montado.
func (a *Assistant) executeAgent(_ context.Context, prompt string, _ *Session) string {
	// TODO: Integrar com o SDK AgentGo (agent.Run, tools, etc).
	_ = prompt
	return "AgentGo Copilot está em desenvolvimento. Em breve estarei funcional!"
}

// sendReply envia uma resposta para o canal de origem da mensagem.
func (a *Assistant) sendReply(original *channels.IncomingMessage, content string) {
	outMsg := &channels.OutgoingMessage{
		Content: content,
		ReplyTo: original.ID,
	}

	if err := a.channelMgr.Send(a.ctx, original.Channel, original.ChatID, outMsg); err != nil {
		a.logger.Error("falha ao enviar resposta",
			"channel", original.Channel,
			"chat_id", original.ChatID,
			"error", err,
		)
	}
}
