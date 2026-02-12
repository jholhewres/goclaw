// manager.go gerencia múltiplos canais de comunicação simultaneamente,
// fornecendo um ponto único de entrada para receber mensagens de todas
// as plataformas e rotear respostas para o canal correto.
package channels

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
)

// Manager orquestra múltiplos canais de comunicação, agregando mensagens
// recebidas em um único stream e roteando respostas.
type Manager struct {
	// channels armazena todos os canais registrados, indexados por nome.
	channels map[string]Channel

	// messages é o canal agregado que recebe mensagens de todos os canais.
	messages chan *IncomingMessage

	// logger para logs estruturados.
	logger *slog.Logger

	// listenWg sincroniza goroutines de escuta para shutdown seguro.
	listenWg sync.WaitGroup

	mu     sync.RWMutex
	ctx    context.Context
	cancel context.CancelFunc
}

// NewManager cria um novo gerenciador de canais com o logger fornecido.
func NewManager(logger *slog.Logger) *Manager {
	if logger == nil {
		logger = slog.Default()
	}

	return &Manager{
		channels: make(map[string]Channel),
		messages: make(chan *IncomingMessage, 256),
		logger:   logger,
	}
}

// Register adiciona um canal ao gerenciador. Deve ser chamado antes de Start.
func (m *Manager) Register(ch Channel) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	name := ch.Name()
	if _, exists := m.channels[name]; exists {
		return fmt.Errorf("channel %q already registered", name)
	}

	m.channels[name] = ch
	m.logger.Info("canal registrado", "channel", name)
	return nil
}

// Start conecta todos os canais registrados e começa a escutar mensagens.
// Canais que falharem na conexão são logados mas não impedem os demais.
// Retorna nil se pelo menos um canal conectou ou se nenhum canal foi registrado.
func (m *Manager) Start(ctx context.Context) error {
	m.ctx, m.cancel = context.WithCancel(ctx)

	// Snapshot dos canais sob lock para evitar race com Register.
	m.mu.RLock()
	snapshot := make(map[string]Channel, len(m.channels))
	for k, v := range m.channels {
		snapshot[k] = v
	}
	m.mu.RUnlock()

	if len(snapshot) == 0 {
		m.logger.Warn("nenhum canal registrado, rodando sem canais de mensagem")
		return nil
	}

	var connected int
	for name, ch := range snapshot {
		if err := ch.Connect(m.ctx); err != nil {
			m.logger.Error("falha ao conectar canal",
				"channel", name,
				"error", err,
			)
			continue
		}

		connected++
		m.logger.Info("canal conectado", "channel", name)

		// Escuta mensagens deste canal em goroutine rastreada pelo WaitGroup.
		m.listenWg.Add(1)
		go func(c Channel) {
			defer m.listenWg.Done()
			m.listenChannel(c)
		}(ch)
	}

	if connected == 0 {
		return fmt.Errorf("nenhum canal conectado com sucesso")
	}

	m.logger.Info("manager iniciado", "channels_connected", connected)
	return nil
}

// Stop desconecta todos os canais de forma graciosa.
// Aguarda todas as goroutines de escuta finalizarem antes de fechar o canal de mensagens.
func (m *Manager) Stop() {
	if m.cancel != nil {
		m.cancel()
	}

	// Aguarda goroutines de escuta finalizarem para evitar panic ao fechar o channel.
	m.listenWg.Wait()

	m.mu.RLock()
	defer m.mu.RUnlock()

	for name, ch := range m.channels {
		if err := ch.Disconnect(); err != nil {
			m.logger.Error("erro ao desconectar canal",
				"channel", name,
				"error", err,
			)
		}
	}

	close(m.messages)
	m.logger.Info("manager encerrado")
}

// Messages retorna o canal agregado de mensagens de todas as plataformas.
func (m *Manager) Messages() <-chan *IncomingMessage {
	return m.messages
}

// Send envia uma mensagem pelo canal especificado.
func (m *Manager) Send(ctx context.Context, channelName, to string, msg *OutgoingMessage) error {
	m.mu.RLock()
	ch, exists := m.channels[channelName]
	m.mu.RUnlock()

	if !exists {
		return fmt.Errorf("canal %q não encontrado", channelName)
	}

	if !ch.IsConnected() {
		return fmt.Errorf("canal %q desconectado", channelName)
	}

	return ch.Send(ctx, to, msg)
}

// Channel retorna um canal específico pelo nome.
func (m *Manager) Channel(name string) (Channel, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	ch, ok := m.channels[name]
	return ch, ok
}

// HealthAll retorna o status de saúde de todos os canais registrados.
func (m *Manager) HealthAll() map[string]HealthStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()

	statuses := make(map[string]HealthStatus, len(m.channels))
	for name, ch := range m.channels {
		statuses[name] = ch.Health()
	}
	return statuses
}

// HasChannels retorna true se há pelo menos um canal registrado.
func (m *Manager) HasChannels() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.channels) > 0
}

// listenChannel escuta mensagens de um canal e repassa ao stream agregado.
func (m *Manager) listenChannel(ch Channel) {
	for msg := range ch.Receive() {
		select {
		case m.messages <- msg:
		case <-m.ctx.Done():
			return
		}
	}
}
