// session.go implementa o gerenciamento de sessões isoladas por chat/grupo.
// Cada grupo ou DM possui sua própria sessão com memória, skills ativas
// e configurações independentes.
package copilot

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// DefaultMaxHistory é o limite padrão de entradas no histórico por sessão.
const DefaultMaxHistory = 100

// DefaultSessionTTL é o tempo de inatividade antes de uma sessão ser removida.
const DefaultSessionTTL = 24 * time.Hour

// Session representa uma sessão isolada de conversa para um chat/grupo específico.
type Session struct {
	// ID é o identificador único da sessão (combinação de channel + chatID).
	ID string

	// Channel identifica o canal de origem (ex: "whatsapp", "discord").
	Channel string

	// ChatID é o identificador do grupo ou DM.
	ChatID string

	// config contém configurações específicas desta sessão.
	config SessionConfig

	// activeSkills lista as skills ativas nesta sessão.
	activeSkills []string

	// facts são fatos de longo prazo extraídos e salvos para esta sessão.
	facts []string

	// history é o histórico de mensagens da sessão.
	history []ConversationEntry

	// maxHistory é o limite máximo de entradas no histórico.
	maxHistory int

	// CreatedAt é o timestamp de criação da sessão.
	CreatedAt time.Time

	// lastActiveAt é o timestamp da última atividade.
	lastActiveAt time.Time

	mu sync.RWMutex
}

// SessionConfig contém configurações específicas de uma sessão.
type SessionConfig struct {
	// Trigger é a palavra-chave que ativa o copilot nesta sessão.
	Trigger string `yaml:"trigger"`

	// Language é o idioma preferido nesta sessão.
	Language string `yaml:"language"`

	// MaxTokens é o budget máximo de tokens para esta sessão.
	MaxTokens int `yaml:"max_tokens"`

	// Model é o modelo LLM a ser usado nesta sessão (pode ser diferente do padrão).
	Model string `yaml:"model"`

	// BusinessContext é o contexto de negócio/usuário para esta sessão.
	BusinessContext string `yaml:"business_context"`
}

// ConversationEntry representa uma troca de mensagem na sessão.
type ConversationEntry struct {
	UserMessage       string
	AssistantResponse string
	Timestamp         time.Time
}

// AddMessage adiciona uma nova entrada de conversa à sessão.
// Aplica o limite de maxHistory, removendo mensagens antigas quando excedido.
func (s *Session) AddMessage(userMsg, assistantResp string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.history = append(s.history, ConversationEntry{
		UserMessage:       userMsg,
		AssistantResponse: assistantResp,
		Timestamp:         time.Now(),
	})

	// Trim histórico se exceder o limite para evitar leak de memória.
	if s.maxHistory > 0 && len(s.history) > s.maxHistory {
		s.history = s.history[len(s.history)-s.maxHistory:]
	}

	s.lastActiveAt = time.Now()
}

// RecentHistory retorna as últimas N entradas de conversa (cópia thread-safe).
func (s *Session) RecentHistory(maxEntries int) []ConversationEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if len(s.history) <= maxEntries {
		result := make([]ConversationEntry, len(s.history))
		copy(result, s.history)
		return result
	}

	start := len(s.history) - maxEntries
	result := make([]ConversationEntry, maxEntries)
	copy(result, s.history[start:])
	return result
}

// AddFact adiciona um fato de longo prazo à sessão.
func (s *Session) AddFact(fact string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.facts = append(s.facts, fact)
}

// GetFacts retorna uma cópia thread-safe dos fatos da sessão.
func (s *Session) GetFacts() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]string, len(s.facts))
	copy(result, s.facts)
	return result
}

// GetActiveSkills retorna uma cópia thread-safe das skills ativas.
func (s *Session) GetActiveSkills() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]string, len(s.activeSkills))
	copy(result, s.activeSkills)
	return result
}

// SetActiveSkills define as skills ativas da sessão.
func (s *Session) SetActiveSkills(skills []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.activeSkills = make([]string, len(skills))
	copy(s.activeSkills, skills)
}

// GetConfig retorna uma cópia thread-safe da configuração da sessão.
func (s *Session) GetConfig() SessionConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.config
}

// SetConfig atualiza a configuração da sessão.
func (s *Session) SetConfig(cfg SessionConfig) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.config = cfg
}

// LastActiveAt retorna o timestamp da última atividade (thread-safe).
func (s *Session) LastActiveAt() time.Time {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.lastActiveAt
}

// ClearHistory limpa o histórico de conversa mantendo fatos de longo prazo.
func (s *Session) ClearHistory() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.history = nil
}

// SessionStore gerencia sessões ativas, criando e recuperando por canal e chatID.
// Implementa pruning automático de sessões inativas.
type SessionStore struct {
	sessions   map[string]*Session
	sessionTTL time.Duration
	logger     *slog.Logger
	mu         sync.RWMutex
}

// NewSessionStore cria um novo store de sessões.
func NewSessionStore(logger *slog.Logger) *SessionStore {
	if logger == nil {
		logger = slog.Default()
	}

	return &SessionStore{
		sessions:   make(map[string]*Session),
		sessionTTL: DefaultSessionTTL,
		logger:     logger,
	}
}

// GetOrCreate retorna a sessão existente ou cria uma nova para o canal e chatID.
func (ss *SessionStore) GetOrCreate(channel, chatID string) *Session {
	key := sessionKey(channel, chatID)

	ss.mu.RLock()
	if session, exists := ss.sessions[key]; exists {
		ss.mu.RUnlock()
		return session
	}
	ss.mu.RUnlock()

	ss.mu.Lock()
	defer ss.mu.Unlock()

	// Double-check após adquirir write lock para evitar race.
	if session, exists := ss.sessions[key]; exists {
		return session
	}

	session := &Session{
		ID:           key,
		Channel:      channel,
		ChatID:       chatID,
		config:       SessionConfig{},
		activeSkills: []string{},
		facts:        []string{},
		history:      []ConversationEntry{},
		maxHistory:   DefaultMaxHistory,
		CreatedAt:    time.Now(),
		lastActiveAt: time.Now(),
	}

	ss.sessions[key] = session
	ss.logger.Info("nova sessão criada",
		"channel", channel,
		"chat_id", chatID,
	)

	return session
}

// Get retorna a sessão pelo canal e chatID, ou nil se não existir.
func (ss *SessionStore) Get(channel, chatID string) *Session {
	ss.mu.RLock()
	defer ss.mu.RUnlock()
	return ss.sessions[sessionKey(channel, chatID)]
}

// Count retorna o número de sessões ativas.
func (ss *SessionStore) Count() int {
	ss.mu.RLock()
	defer ss.mu.RUnlock()
	return len(ss.sessions)
}

// Prune remove sessões inativas há mais tempo que o TTL configurado.
// Deve ser chamado periodicamente (ex: via goroutine com ticker).
func (ss *SessionStore) Prune() int {
	ss.mu.Lock()
	defer ss.mu.Unlock()

	cutoff := time.Now().Add(-ss.sessionTTL)
	pruned := 0

	for key, session := range ss.sessions {
		if session.LastActiveAt().Before(cutoff) {
			delete(ss.sessions, key)
			pruned++
		}
	}

	if pruned > 0 {
		ss.logger.Info("sessões inativas removidas",
			"pruned", pruned,
			"remaining", len(ss.sessions),
		)
	}

	return pruned
}

// StartPruner inicia uma goroutine que executa Prune periodicamente.
// Para quando o contexto é cancelado.
func (ss *SessionStore) StartPruner(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(ss.sessionTTL / 2)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				ss.Prune()
			case <-ctx.Done():
				return
			}
		}
	}()
}

// sessionKey gera a chave única para uma sessão.
func sessionKey(channel, chatID string) string {
	return fmt.Sprintf("%s:%s", channel, chatID)
}
