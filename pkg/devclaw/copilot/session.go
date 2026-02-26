// session.go implementa o gerenciamento de sessões isoladas por chat/grupo.
// Cada grupo ou DM possui sua própria sessão com memória, skills ativas
// e configurações independentes.
package copilot

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
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

	// compactionSummaries armazena resumos de compaction para reconstrução de contexto.
	compactionSummaries []CompactionEntry

	// history é o histórico de mensagens da sessão.
	history []ConversationEntry

	// maxHistory é o limite máximo de entradas no histórico.
	maxHistory int

	// Token tracking (thread-safe via mu).
	totalPromptTokens     int
	totalCompletionTokens int
	totalRequests         int

	// CreatedAt é o timestamp de criação da sessão.
	CreatedAt time.Time

	// lastActiveAt é o timestamp da última atividade.
	lastActiveAt time.Time

	persistence SessionPersister

	mu sync.RWMutex
}

// SessionPersister is the interface for session persistence backends (JSONL or SQLite).
type SessionPersister interface {
	SaveEntry(sessionID string, entry ConversationEntry) error
	LoadSession(sessionID string) ([]ConversationEntry, []string, error)
	SaveFacts(sessionID string, facts []string) error
	SaveMeta(sessionID, channel, chatID string, config SessionConfig, activeSkills []string) error
	SaveCompaction(sessionID string, entry CompactionEntry) error
	DeleteSession(sessionID string) error
	Rotate(sessionID string, maxLines int) error
	LoadAll() (map[string]*SessionData, error)
	Close() error
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

	// ThinkingLevel controls extended thinking: "", "off", "low", "medium", "high".
	ThinkingLevel string `yaml:"thinking_level"`

	// Verbose enables narration of tool calls and internal steps.
	Verbose bool `yaml:"verbose"`
}

// ConversationEntry representa uma troca de mensagem na sessão.
type ConversationEntry struct {
	UserMessage       string
	AssistantResponse string
	Timestamp         time.Time
}

// AddMessage adiciona uma nova entrada de conversa à sessão.
// Aplica o limite de maxHistory, removendo mensagens antigas quando excedido.
// Persiste a entrada em disco se persistence estiver configurada.
func (s *Session) AddMessage(userMsg, assistantResp string) {
	entry := ConversationEntry{
		UserMessage:       userMsg,
		AssistantResponse: assistantResp,
		Timestamp:         time.Now(),
	}

	s.mu.Lock()
	s.history = append(s.history, entry)

	// Trim histórico se exceder o limite para evitar leak de memória.
	if s.maxHistory > 0 && len(s.history) > s.maxHistory {
		s.history = s.history[len(s.history)-s.maxHistory:]
	}

	s.lastActiveAt = time.Now()
	persistence := s.persistence
	s.mu.Unlock()

	if persistence != nil {
		if err := persistence.SaveEntry(s.ID, entry); err != nil {
			// Log is done inside SaveEntry; avoid holding lock during I/O
		}
	}
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
// Persiste os fatos em disco se persistence estiver configurada.
func (s *Session) AddFact(fact string) {
	s.mu.Lock()
	s.facts = append(s.facts, fact)
	facts := make([]string, len(s.facts))
	copy(facts, s.facts)
	persistence := s.persistence
	s.mu.Unlock()

	if persistence != nil {
		if err := persistence.SaveFacts(s.ID, facts); err != nil {
			// Log is done inside SaveFacts
		}
	}
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

// ClearFacts removes all session facts. Used by /reset.
func (s *Session) ClearFacts() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.facts = nil
}

// HistoryLen returns the number of entries in the session history.
func (s *Session) HistoryLen() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.history)
}

// AddTokenUsage records token usage from an LLM response. Thread-safe.
func (s *Session) AddTokenUsage(promptTokens, completionTokens int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.totalPromptTokens += promptTokens
	s.totalCompletionTokens += completionTokens
	s.totalRequests++
}

// GetTokenUsage returns a copy of the token usage. Thread-safe.
func (s *Session) GetTokenUsage() (promptTokens, completionTokens, requests int) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.totalPromptTokens, s.totalCompletionTokens, s.totalRequests
}

// ResetTokenUsage clears token counters. Thread-safe.
func (s *Session) ResetTokenUsage() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.totalPromptTokens = 0
	s.totalCompletionTokens = 0
	s.totalRequests = 0
}

// GetThinkingLevel returns the session thinking level. Thread-safe.
func (s *Session) GetThinkingLevel() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.config.ThinkingLevel
}

// SetThinkingLevel sets the session thinking level. Thread-safe.
func (s *Session) SetThinkingLevel(level string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.config.ThinkingLevel = level
}

// CompactHistory replaces the full history with a summary entry,
// keeping only the most recent entries. Returns the old entries for
// memory extraction.
func (s *Session) CompactHistory(summary string, keepRecent int) []ConversationEntry {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.history) <= keepRecent {
		return nil // Nothing to compact.
	}

	// Save old entries for memory extraction.
	cutoff := len(s.history) - keepRecent
	old := make([]ConversationEntry, cutoff)
	copy(old, s.history[:cutoff])

	// Replace old entries with a summary.
	recent := make([]ConversationEntry, keepRecent+1)
	recent[0] = ConversationEntry{
		UserMessage:       "[session compacted]",
		AssistantResponse: summary,
		Timestamp:         time.Now(),
	}
	copy(recent[1:], s.history[cutoff:])

	s.history = recent
	return old
}

// SessionStore gerencia sessões ativas, criando e recuperando por canal e chatID.
// Implementa pruning automático de sessões inativas.
type SessionStore struct {
	sessions    map[string]*Session
	sessionTTL  time.Duration
	logger      *slog.Logger
	mu          sync.RWMutex
	persistence SessionPersister
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

// SetPersistence configures disk persistence for sessions.
func (ss *SessionStore) SetPersistence(p SessionPersister) {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	ss.persistence = p
}

// GetOrCreate retorna a sessão existente ou cria uma nova para o canal e chatID.
// Se persistence estiver configurada, tenta carregar do disco antes de criar.
func (ss *SessionStore) GetOrCreate(channel, chatID string) *Session {
	key := sessionKey(channel, chatID)

	ss.mu.RLock()
	if session, exists := ss.sessions[key]; exists {
		ss.mu.RUnlock()
		return session
	}
	persistence := ss.persistence
	ss.mu.RUnlock()

	ss.mu.Lock()
	defer ss.mu.Unlock()

	// Double-check após adquirir write lock para evitar race.
	if session, exists := ss.sessions[key]; exists {
		return session
	}

	var session *Session

	if persistence != nil {
		entries, facts, loadErr := persistence.LoadSession(key)
		if loadErr == nil && (len(entries) > 0 || len(facts) > 0) {
			session = &Session{
				ID:           key,
				Channel:      channel,
				ChatID:       chatID,
				config:       SessionConfig{},
				activeSkills: []string{},
				facts:        facts,
				history:      entries,
				maxHistory:   DefaultMaxHistory,
				CreatedAt:    time.Now(),
				lastActiveAt: time.Now(),
			}
			ss.sessions[key] = session
			ss.logger.Info("sessão restaurada do disco",
				"channel", channel,
				"chat_id", chatID,
			)
			return session
		}
	}

	// Create new session
	session = &Session{
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
		persistence:  persistence,
	}

	if persistence != nil {
		if err := persistence.SaveMeta(key, channel, chatID, SessionConfig{}, nil); err != nil {
			ss.logger.Warn("failed to persist session meta", "channel", channel, "chat_id", chatID, "err", err)
		}
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

// SessionMeta holds read-only metadata for a session (for listing).
type SessionMeta struct {
	ID           string
	Channel      string
	ChatID       string
	MessageCount int
	CreatedAt    time.Time
	LastActiveAt time.Time
}

// ListSessions returns metadata for all sessions in the store.
func (ss *SessionStore) ListSessions() []SessionMeta {
	ss.mu.RLock()
	defer ss.mu.RUnlock()
	out := make([]SessionMeta, 0, len(ss.sessions))
	for _, s := range ss.sessions {
		s.mu.RLock()
		meta := SessionMeta{
			ID:           s.ID,
			Channel:      s.Channel,
			ChatID:       s.ChatID,
			MessageCount: len(s.history),
			CreatedAt:    s.CreatedAt,
			LastActiveAt: s.lastActiveAt,
		}
		s.mu.RUnlock()
		out = append(out, meta)
	}
	return out
}

// GetByID returns a session by its raw store key. Returns nil if not found.
func (ss *SessionStore) GetByID(id string) *Session {
	ss.mu.RLock()
	defer ss.mu.RUnlock()
	return ss.sessions[id]
}

// Delete removes a session by channel and chatID.
func (ss *SessionStore) Delete(channel, chatID string) bool {
	key := sessionKey(channel, chatID)
	ss.mu.Lock()
	defer ss.mu.Unlock()
	if _, exists := ss.sessions[key]; exists {
		delete(ss.sessions, key)
		ss.logger.Info("session deleted", "channel", channel, "chat_id", chatID)
		return true
	}
	return false
}

// DeleteByID removes a session by its hash ID.
func (ss *SessionStore) DeleteByID(id string) bool {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	if s, exists := ss.sessions[id]; exists {
		delete(ss.sessions, id)
		// Also delete from persistence if available.
		if ss.persistence != nil {
			ss.persistence.DeleteSession(id)
		}
		ss.logger.Info("session deleted by ID",
			"id", id, "channel", s.Channel, "chat_id", s.ChatID)
		return true
	}
	return false
}

// Export returns a portable representation of a session's history and metadata.
func (ss *SessionStore) Export(id string) *SessionExport {
	ss.mu.RLock()
	defer ss.mu.RUnlock()
	s, exists := ss.sessions[id]
	if !exists {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	export := &SessionExport{
		ID:        s.ID,
		Channel:   s.Channel,
		ChatID:    s.ChatID,
		Config:    s.config,
		Facts:     make([]string, len(s.facts)),
		CreatedAt: s.CreatedAt,
		Messages:  make([]ExportedMessage, 0, len(s.history)),
	}
	copy(export.Facts, s.facts)
	for _, entry := range s.history {
		export.Messages = append(export.Messages, ExportedMessage{
			User:      entry.UserMessage,
			Assistant: entry.AssistantResponse,
			Timestamp: entry.Timestamp,
		})
	}
	return export
}

// SessionExport is a portable representation of a session for backup/export.
type SessionExport struct {
	ID        string            `json:"id"`
	Channel   string            `json:"channel"`
	ChatID    string            `json:"chat_id"`
	Config    SessionConfig     `json:"config"`
	Facts     []string          `json:"facts"`
	CreatedAt time.Time         `json:"created_at"`
	Messages  []ExportedMessage `json:"messages"`
}

// ExportedMessage is a single message in an exported session.
type ExportedMessage struct {
	User      string    `json:"user"`
	Assistant string    `json:"assistant"`
	Timestamp time.Time `json:"timestamp"`
}

// RenameSession changes the ChatID of a session (e.g. for aliasing).
func (ss *SessionStore) RenameSession(oldID, newChannel, newChatID string) bool {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	s, exists := ss.sessions[oldID]
	if !exists {
		return false
	}
	newKey := sessionKey(newChannel, newChatID)
	if _, conflict := ss.sessions[newKey]; conflict {
		return false
	}
	delete(ss.sessions, oldID)
	s.mu.Lock()
	s.ID = newKey
	s.Channel = newChannel
	s.ChatID = newChatID
	s.mu.Unlock()
	ss.sessions[newKey] = s
	ss.logger.Info("session renamed", "old_id", oldID, "new_id", newKey)
	return true
}

// sessionKey gera a chave única para uma sessão.
// MakeSessionID generates a deterministic, opaque session ID from channel and chatID.
// The ID is a truncated SHA-256 hash, so no PII (phone numbers, etc.) leaks into
// file names, logs, or persisted job data.
// SessionKey is a structured session identifier that preserves the original
// channel, chatID, and optional branch components while providing a compact
// string form. This enables multi-agent routing: sessions can be found by
// channel, user, or branch without losing context.
type SessionKey struct {
	Channel string // "whatsapp", "discord", "webui", "subagent", etc.
	ChatID  string // Group JID, user JID, or chat UUID.
	Branch  string // Optional: sub-session branch (e.g. "topic-research", fork ID).
}

// String returns the canonical string form: "channel:chatID" or "channel:chatID:branch".
func (sk SessionKey) String() string {
	if sk.Branch != "" {
		return sk.Channel + ":" + sk.ChatID + ":" + sk.Branch
	}
	return sk.Channel + ":" + sk.ChatID
}

// Hash returns a compact hash suitable for map keys and persistence IDs.
func (sk SessionKey) Hash() string {
	h := sha256.Sum256([]byte(sk.String()))
	return hex.EncodeToString(h[:8])
}

// ParseSessionKey parses a "channel:chatID" or "channel:chatID:branch" string.
func ParseSessionKey(s string) SessionKey {
	parts := splitMax(s, ":", 3)
	switch len(parts) {
	case 3:
		return SessionKey{Channel: parts[0], ChatID: parts[1], Branch: parts[2]}
	case 2:
		return SessionKey{Channel: parts[0], ChatID: parts[1]}
	default:
		return SessionKey{ChatID: s}
	}
}

// splitMax splits s by sep into at most n parts.
func splitMax(s, sep string, n int) []string {
	if sep == "" || n <= 1 {
		return []string{s}
	}
	var parts []string
	for i := 0; i < n-1; i++ {
		idx := -1
		for j := 0; j < len(s); j++ {
			if s[j] == sep[0] {
				idx = j
				break
			}
		}
		if idx < 0 {
			break
		}
		parts = append(parts, s[:idx])
		s = s[idx+1:]
	}
	parts = append(parts, s)
	return parts
}

// MakeSessionID returns a compact hash-based session ID from channel and chatID.
// For backward compatibility, this is still used as the primary key.
func MakeSessionID(channel, chatID string) string {
	return SessionKey{Channel: channel, ChatID: chatID}.Hash()
}

func sessionKey(channel, chatID string) string {
	return MakeSessionID(channel, chatID)
}
