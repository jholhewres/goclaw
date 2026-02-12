// session.go implementa o gerenciamento de sessões isoladas por chat/grupo.
// Cada grupo ou DM possui sua própria sessão com memória, skills ativas
// e configurações independentes.
package copilot

import (
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// Session representa uma sessão isolada de conversa para um chat/grupo específico.
type Session struct {
	// ID é o identificador único da sessão (combinação de channel + chatID).
	ID string

	// Channel identifica o canal de origem (ex: "whatsapp", "discord").
	Channel string

	// ChatID é o identificador do grupo ou DM.
	ChatID string

	// Config contém configurações específicas desta sessão.
	Config SessionConfig

	// ActiveSkills lista as skills ativas nesta sessão.
	ActiveSkills []string

	// Facts são fatos de longo prazo extraídos e salvos para esta sessão.
	Facts []string

	// history é o histórico de mensagens da sessão.
	history []ConversationEntry

	// CreatedAt é o timestamp de criação da sessão.
	CreatedAt time.Time

	// LastActiveAt é o timestamp da última atividade.
	LastActiveAt time.Time

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
func (s *Session) AddMessage(userMsg, assistantResp string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.history = append(s.history, ConversationEntry{
		UserMessage:       userMsg,
		AssistantResponse: assistantResp,
		Timestamp:         time.Now(),
	})
	s.LastActiveAt = time.Now()
}

// RecentHistory retorna as últimas N entradas de conversa.
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
	s.Facts = append(s.Facts, fact)
}

// ClearHistory limpa o histórico de conversa mantendo fatos de longo prazo.
func (s *Session) ClearHistory() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.history = nil
}

// SessionStore gerencia sessões ativas, criando e recuperando por canal e chatID.
type SessionStore struct {
	sessions map[string]*Session
	logger   *slog.Logger
	mu       sync.RWMutex
}

// NewSessionStore cria um novo store de sessões.
func NewSessionStore(logger *slog.Logger) *SessionStore {
	if logger == nil {
		logger = slog.Default()
	}

	return &SessionStore{
		sessions: make(map[string]*Session),
		logger:   logger,
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

	// Double-check após adquirir write lock.
	if session, exists := ss.sessions[key]; exists {
		return session
	}

	session := &Session{
		ID:           key,
		Channel:      channel,
		ChatID:       chatID,
		Config:       SessionConfig{},
		ActiveSkills: []string{},
		Facts:        []string{},
		history:      []ConversationEntry{},
		CreatedAt:    time.Now(),
		LastActiveAt: time.Now(),
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

// sessionKey gera a chave única para uma sessão.
func sessionKey(channel, chatID string) string {
	return fmt.Sprintf("%s:%s", channel, chatID)
}
