package copilot

import (
	"testing"
	"time"
)

func TestCompactionEntry_Creation(t *testing.T) {
	t.Parallel()

	entry := CompactionEntry{
		Type:           "compaction_summary",
		Summary:        "Test summary of conversation",
		CompactedAt:    time.Now(),
		MessagesBefore: 50,
		MessagesAfter:  20,
	}

	if entry.Type != "compaction_summary" {
		t.Errorf("expected type 'compaction_summary', got %s", entry.Type)
	}
	if entry.Summary != "Test summary of conversation" {
		t.Errorf("expected summary 'Test summary of conversation', got %s", entry.Summary)
	}
	if entry.MessagesBefore != 50 {
		t.Errorf("expected 50 messages before, got %d", entry.MessagesBefore)
	}
	if entry.MessagesAfter != 20 {
		t.Errorf("expected 20 messages after, got %d", entry.MessagesAfter)
	}
}

func TestPersistCompactionSummary_NoPersistence(t *testing.T) {
	t.Parallel()

	logger := newTestLogger()
	a := &AgentRun{
		logger: logger,
	}

	// Should not panic when sessionPersistence is nil
	a.persistCompactionSummary("test summary", 50, 20)
}

func TestPersistCompactionSummary_EmptySummary(t *testing.T) {
	t.Parallel()

	logger := newTestLogger()
	a := &AgentRun{
		logger:               logger,
		sessionPersistence:   &mockSessionPersister{},
	}

	// Should not save empty summary
	a.persistCompactionSummary("", 50, 20)
}

// Mock implementations

type mockSessionPersister struct{}

func (m *mockSessionPersister) SaveEntry(sessionID string, entry ConversationEntry) error {
	return nil
}

func (m *mockSessionPersister) LoadSession(sessionID string) ([]ConversationEntry, []string, error) {
	return nil, nil, nil
}

func (m *mockSessionPersister) SaveFacts(sessionID string, facts []string) error {
	return nil
}

func (m *mockSessionPersister) SaveMeta(sessionID, channel, chatID string, config SessionConfig, activeSkills []string) error {
	return nil
}

func (m *mockSessionPersister) SaveCompaction(sessionID string, entry CompactionEntry) error {
	return nil
}

func (m *mockSessionPersister) DeleteSession(sessionID string) error {
	return nil
}

func (m *mockSessionPersister) Rotate(sessionID string, maxLines int) error {
	return nil
}

func (m *mockSessionPersister) LoadAll() (map[string]*SessionData, error) {
	return nil, nil
}

func (m *mockSessionPersister) Close() error {
	return nil
}
