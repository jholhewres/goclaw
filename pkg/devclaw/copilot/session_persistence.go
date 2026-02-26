// session_persistence.go implements disk persistence for sessions using JSONL format.
package copilot

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	defaultSessionsDir = "./data/sessions"
)

// jsonlEntry is the JSON line format for each conversation entry.
type jsonlEntry struct {
	TS        string                 `json:"ts"`
	User      string                 `json:"user"`
	Assistant string                 `json:"assistant"`
	Meta      map[string]interface{} `json:"meta,omitempty"`
}

// CompactionEntry represents a compaction summary stored in the session.
type CompactionEntry struct {
	Type           string    `json:"type"`            // Always "compaction_summary"
	Summary        string    `json:"summary"`         // The LLM-generated summary
	CompactedAt    time.Time `json:"compacted_at"`    // When compaction occurred
	MessagesBefore int       `json:"messages_before"` // Count before compaction
	MessagesAfter  int       `json:"messages_after"`  // Count after compaction
}

// compactionEntry is the JSONL format for compaction entries.
type compactionEntryJSONL struct {
	Type           string    `json:"type"`
	Summary        string    `json:"summary"`
	CompactedAt    time.Time `json:"compacted_at"`
	MessagesBefore int       `json:"messages_before"`
	MessagesAfter  int       `json:"messages_after"`
}

// metaFile represents session metadata stored in .meta.json.
type metaFile struct {
	Channel      string       `json:"channel"`
	ChatID       string       `json:"chat_id"`
	Config       SessionConfig `json:"config"`
	ActiveSkills []string     `json:"active_skills"`
}

// SessionData holds all data needed to restore a session from disk.
type SessionData struct {
	ID           string
	Channel      string
	ChatID       string
	History      []ConversationEntry
	Facts        []string
	Config       SessionConfig
	ActiveSkills []string
}

// SessionPersistence handles saving and loading sessions to JSONL files.
type SessionPersistence struct {
	dir    string
	logger *slog.Logger
	mu     sync.RWMutex
	fileMu map[string]*sync.Mutex // per-session file lock
	mapMu  sync.Mutex
}

// NewSessionPersistence creates a SessionPersistence and ensures the directory exists.
func NewSessionPersistence(dir string, logger *slog.Logger) (*SessionPersistence, error) {
	if dir == "" {
		dir = defaultSessionsDir
	}
	if logger == nil {
		logger = slog.Default()
	}

	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("create sessions dir %q: %w", dir, err)
	}

	return &SessionPersistence{
		dir:    dir,
		logger: logger,
		fileMu: make(map[string]*sync.Mutex),
	}, nil
}

// sanitizeSessionID returns a filesystem-safe name (replaces / and : with _).
func sanitizeSessionID(sessionID string) string {
	s := sessionID
	s = strings.ReplaceAll(s, "/", "_")
	s = strings.ReplaceAll(s, ":", "_")
	return s
}

// fileMuFor returns the mutex for the given session ID.
func (p *SessionPersistence) fileMuFor(sessionID string) *sync.Mutex {
	sanitized := sanitizeSessionID(sessionID)
	p.mapMu.Lock()
	defer p.mapMu.Unlock()
	if m, ok := p.fileMu[sanitized]; ok {
		return m
	}
	m := &sync.Mutex{}
	p.fileMu[sanitized] = m
	return m
}

// SaveEntry appends one JSONL line for the given conversation entry.
func (p *SessionPersistence) SaveEntry(sessionID string, entry ConversationEntry) error {
	mu := p.fileMuFor(sessionID)
	mu.Lock()
	defer mu.Unlock()

	sanitized := sanitizeSessionID(sessionID)
	path := filepath.Join(p.dir, sanitized+".jsonl")

	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		p.logger.Error("failed to open session file for append", "session", sessionID, "err", err)
		return fmt.Errorf("open session file: %w", err)
	}
	defer func() {
		if closeErr := f.Close(); closeErr != nil {
			p.logger.Warn("failed to close session file", "session", sessionID, "err", closeErr)
		}
	}()

	je := jsonlEntry{
		TS:        entry.Timestamp.UTC().Format(time.RFC3339),
		User:      entry.UserMessage,
		Assistant: entry.AssistantResponse,
		Meta:      map[string]interface{}{},
	}
	data, err := json.Marshal(je)
	if err != nil {
		return fmt.Errorf("marshal entry: %w", err)
	}
	if _, err := f.Write(append(data, '\n')); err != nil {
		p.logger.Error("failed to write entry", "session", sessionID, "err", err)
		return fmt.Errorf("write entry: %w", err)
	}

	return nil
}

// SaveCompaction appends a compaction summary entry to the session file.
func (p *SessionPersistence) SaveCompaction(sessionID string, entry CompactionEntry) error {
	mu := p.fileMuFor(sessionID)
	mu.Lock()
	defer mu.Unlock()

	sanitized := sanitizeSessionID(sessionID)
	path := filepath.Join(p.dir, sanitized+".jsonl")

	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		p.logger.Error("failed to open session file for compaction", "session", sessionID, "err", err)
		return fmt.Errorf("open session file: %w", err)
	}
	defer func() {
		if closeErr := f.Close(); closeErr != nil {
			p.logger.Warn("failed to close session file", "session", sessionID, "err", closeErr)
		}
	}()

	// Store compaction entry with a special marker in Meta
	je := jsonlEntry{
		TS:   entry.CompactedAt.UTC().Format(time.RFC3339),
		User: "",
		Meta: map[string]interface{}{
			"type":            "compaction_summary",
			"summary":         entry.Summary,
			"compacted_at":    entry.CompactedAt.Format(time.RFC3339),
			"messages_before": entry.MessagesBefore,
			"messages_after":  entry.MessagesAfter,
		},
	}
	data, err := json.Marshal(je)
	if err != nil {
		return fmt.Errorf("marshal compaction entry: %w", err)
	}
	if _, err := f.Write(append(data, '\n')); err != nil {
		p.logger.Error("failed to write compaction entry", "session", sessionID, "err", err)
		return fmt.Errorf("write compaction entry: %w", err)
	}

	return nil
}

// LoadSession reads all entries and facts for a session.
func (p *SessionPersistence) LoadSession(sessionID string) ([]ConversationEntry, []string, error) {
	mu := p.fileMuFor(sessionID)
	mu.Lock()
	defer mu.Unlock()

	sanitized := sanitizeSessionID(sessionID)
	entries, err := p.readJSONL(filepath.Join(p.dir, sanitized+".jsonl"))
	if err != nil {
		return nil, nil, err
	}

	facts, err := p.readFacts(filepath.Join(p.dir, sanitized+".facts.json"))
	if err != nil {
		p.logger.Warn("failed to read facts, using empty", "session", sessionID, "err", err)
		facts = nil
	}

	return entries, facts, nil
}

// LoadAll scans the directory and restores all sessions from disk.
func (p *SessionPersistence) LoadAll() (map[string]*SessionData, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	entries, err := os.ReadDir(p.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]*SessionData{}, nil
		}
		return nil, fmt.Errorf("read sessions dir: %w", err)
	}

	seen := make(map[string]bool)
	result := make(map[string]*SessionData)

	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".jsonl") || strings.HasSuffix(name, ".facts.json") {
			continue
		}
		base := strings.TrimSuffix(name, ".jsonl")
		if base == "" {
			continue
		}
		if seen[base] {
			continue
		}
		seen[base] = true

		sd, err := p.loadSessionData(base)
		if err != nil {
			p.logger.Warn("failed to load session, skipping", "base", base, "err", err)
			continue
		}
		id := sd.ID
		if id == "" {
			id = base
			sd.ID = base
		}
		result[id] = sd
	}

	return result, nil
}

// loadSessionData loads a single session by its sanitized base name (no extension).
func (p *SessionPersistence) loadSessionData(sanitizedBase string) (*SessionData, error) {
	jsonlPath := filepath.Join(p.dir, sanitizedBase+".jsonl")
	factsPath := filepath.Join(p.dir, sanitizedBase+".facts.json")
	metaPath := filepath.Join(p.dir, sanitizedBase+".meta.json")

	entries, err := p.readJSONL(jsonlPath)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}

	facts, err := p.readFacts(factsPath)
	if err != nil && !os.IsNotExist(err) {
		p.logger.Warn("failed to read facts", "base", sanitizedBase, "err", err)
	}
	if facts == nil {
		facts = []string{}
	}

	channel, chatID := "", ""
	config := SessionConfig{}
	activeSkills := []string{}

	if b, err := os.ReadFile(metaPath); err == nil {
		var mf metaFile
		if json.Unmarshal(b, &mf) == nil {
			channel = mf.Channel
			chatID = mf.ChatID
			config = mf.Config
			activeSkills = mf.ActiveSkills
		}
	}
	// Fallback: reconstruct from sanitized base (channel_chatID -> channel:chatID)
	if channel == "" || chatID == "" {
		parts := strings.SplitN(sanitizedBase, "_", 2)
		if len(parts) >= 2 {
			channel, chatID = parts[0], parts[1]
		} else if len(parts) == 1 {
			channel = parts[0]
		}
	}

	sessionID := sessionKey(channel, chatID)
	if sessionID == ":" || sessionID == "" {
		sessionID = sanitizedBase
	}

	return &SessionData{
		ID:           sessionID,
		Channel:      channel,
		ChatID:       chatID,
		History:      entries,
		Facts:        facts,
		Config:       config,
		ActiveSkills: activeSkills,
	}, nil
}

func (p *SessionPersistence) readJSONL(path string) ([]ConversationEntry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var entries []ConversationEntry
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var je jsonlEntry
		if err := json.Unmarshal([]byte(line), &je); err != nil {
			p.logger.Warn("skip invalid jsonl line", "path", path, "line", line, "err", err)
			continue
		}
		ts, _ := time.Parse(time.RFC3339, je.TS)
		entries = append(entries, ConversationEntry{
			UserMessage:       je.User,
			AssistantResponse: je.Assistant,
			Timestamp:         ts,
		})
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan jsonl: %w", err)
	}
	return entries, nil
}

func (p *SessionPersistence) readFacts(path string) ([]string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var facts []string
	if err := json.Unmarshal(b, &facts); err != nil {
		return nil, fmt.Errorf("unmarshal facts: %w", err)
	}
	return facts, nil
}

// SaveFacts writes facts to {session_id}.facts.json.
func (p *SessionPersistence) SaveFacts(sessionID string, facts []string) error {
	mu := p.fileMuFor(sessionID)
	mu.Lock()
	defer mu.Unlock()

	sanitized := sanitizeSessionID(sessionID)
	path := filepath.Join(p.dir, sanitized+".facts.json")

	data, err := json.MarshalIndent(facts, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal facts: %w", err)
	}

	if err := os.WriteFile(path, data, 0600); err != nil {
		p.logger.Error("failed to write facts", "session", sessionID, "err", err)
		return fmt.Errorf("write facts: %w", err)
	}

	return nil
}

// SaveMeta persists session metadata (channel, chatID, config, activeSkills).
func (p *SessionPersistence) SaveMeta(sessionID, channel, chatID string, config SessionConfig, activeSkills []string) error {
	mu := p.fileMuFor(sessionID)
	mu.Lock()
	defer mu.Unlock()

	sanitized := sanitizeSessionID(sessionID)
	path := filepath.Join(p.dir, sanitized+".meta.json")

	mf := metaFile{
		Channel:      channel,
		ChatID:       chatID,
		Config:       config,
		ActiveSkills: activeSkills,
	}
	data, err := json.MarshalIndent(mf, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal meta: %w", err)
	}

	if err := os.WriteFile(path, data, 0600); err != nil {
		p.logger.Error("failed to write meta", "session", sessionID, "err", err)
		return fmt.Errorf("write meta: %w", err)
	}

	return nil
}

// DeleteSession removes the session's JSONL and facts files.
func (p *SessionPersistence) DeleteSession(sessionID string) error {
	sanitized := sanitizeSessionID(sessionID)
	for _, ext := range []string{".jsonl", ".facts.json", ".meta.json", ".jsonl.bak"} {
		path := filepath.Join(p.dir, sanitized+ext)
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			p.logger.Warn("failed to remove session file", "path", path, "err", err)
			// Continue removing other files
		}
	}
	return nil
}

// Rotate creates a .bak of the JSONL file and starts fresh.
func (p *SessionPersistence) Rotate(sessionID string, maxLines int) error {
	mu := p.fileMuFor(sessionID)
	mu.Lock()
	defer mu.Unlock()

	sanitized := sanitizeSessionID(sessionID)
	path := filepath.Join(p.dir, sanitized+".jsonl")
	bakPath := path + ".bak"

	entries, err := p.readJSONL(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	if len(entries) <= maxLines {
		return nil // No rotation needed.
	}

	keep := entries[len(entries)-maxLines:]
	if err := os.Rename(path, bakPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("rename to bak: %w", err)
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("create fresh file: %w", err)
	}
	defer f.Close()

	for _, e := range keep {
		je := jsonlEntry{
			TS:        e.Timestamp.UTC().Format(time.RFC3339),
			User:      e.UserMessage,
			Assistant: e.AssistantResponse,
			Meta:      map[string]interface{}{},
		}
		data, _ := json.Marshal(je)
		if _, err := f.Write(append(data, '\n')); err != nil {
			return fmt.Errorf("write rotated entry: %w", err)
		}
	}

	p.logger.Info("session rotated", "session", sessionID, "kept", len(keep), "removed", len(entries)-len(keep))
	return nil
}

// Close flushes any buffers. JSONL writes are unbuffered (direct write), so this is a no-op for now.
func (p *SessionPersistence) Close() error {
	return nil
}
