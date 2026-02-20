package copilot

import (
	"sync"
	"testing"
	"time"
)

func TestParseSessionKey(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		channel string
		chatID  string
		branch  string
	}{
		{"two parts", "whatsapp:123456", "whatsapp", "123456", ""},
		{"three parts", "whatsapp:123456:topic", "whatsapp", "123456", "topic"},
		{"one part", "justchatid", "", "justchatid", ""},
		{"empty branch", "discord:abc:", "discord", "abc", ""},
		{"colons in chatid", "webui:user@host:branch", "webui", "user@host", "branch"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			sk := ParseSessionKey(tt.input)
			if sk.Channel != tt.channel {
				t.Errorf("Channel = %q, want %q", sk.Channel, tt.channel)
			}
			if sk.ChatID != tt.chatID {
				t.Errorf("ChatID = %q, want %q", sk.ChatID, tt.chatID)
			}
			if sk.Branch != tt.branch {
				t.Errorf("Branch = %q, want %q", sk.Branch, tt.branch)
			}
		})
	}
}

func TestSessionKey_String(t *testing.T) {
	t.Parallel()

	tests := []struct {
		sk   SessionKey
		want string
	}{
		{SessionKey{Channel: "whatsapp", ChatID: "123"}, "whatsapp:123"},
		{SessionKey{Channel: "whatsapp", ChatID: "123", Branch: "topic"}, "whatsapp:123:topic"},
		{SessionKey{ChatID: "only"}, ":only"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			t.Parallel()
			if got := tt.sk.String(); got != tt.want {
				t.Errorf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSessionKey_Hash_Deterministic(t *testing.T) {
	t.Parallel()
	sk := SessionKey{Channel: "whatsapp", ChatID: "123"}
	h1 := sk.Hash()
	h2 := sk.Hash()
	if h1 != h2 {
		t.Errorf("same key should produce same hash: %q != %q", h1, h2)
	}
	if h1 == "" {
		t.Error("hash should not be empty")
	}
}

func TestSessionKey_Hash_Different(t *testing.T) {
	t.Parallel()
	sk1 := SessionKey{Channel: "whatsapp", ChatID: "123"}
	sk2 := SessionKey{Channel: "whatsapp", ChatID: "456"}
	if sk1.Hash() == sk2.Hash() {
		t.Error("different keys should produce different hashes")
	}
}

func TestMakeSessionID(t *testing.T) {
	t.Parallel()
	id := MakeSessionID("whatsapp", "123")
	if id == "" {
		t.Error("MakeSessionID should return non-empty string")
	}
	// Same inputs should produce same ID.
	id2 := MakeSessionID("whatsapp", "123")
	if id != id2 {
		t.Error("MakeSessionID should be deterministic")
	}
	// Different inputs should produce different IDs.
	id3 := MakeSessionID("whatsapp", "456")
	if id == id3 {
		t.Error("different inputs should produce different IDs")
	}
}

// Session struct tests

func TestSessionAddMessage(t *testing.T) {
	s := &Session{
		ID:         "test-session",
		Channel:    "whatsapp",
		ChatID:     "5511999999999",
		maxHistory: 100,
	}

	t.Run("adds message to history", func(t *testing.T) {
		s.AddMessage("Hello", "Hi there!")

		if len(s.history) != 1 {
			t.Errorf("expected 1 history entry, got %d", len(s.history))
		}

		entry := s.history[0]
		if entry.UserMessage != "Hello" {
			t.Errorf("expected user message 'Hello', got %q", entry.UserMessage)
		}
		if entry.AssistantResponse != "Hi there!" {
			t.Errorf("expected assistant response 'Hi there!', got %q", entry.AssistantResponse)
		}
	})

	t.Run("updates lastActiveAt", func(t *testing.T) {
		before := s.lastActiveAt
		time.Sleep(10 * time.Millisecond)
		s.AddMessage("Another", "Response")

		if !s.lastActiveAt.After(before) {
			t.Error("expected lastActiveAt to be updated")
		}
	})
}

func TestSessionHistoryLimit(t *testing.T) {
	s := &Session{
		ID:         "test-session",
		maxHistory: 5,
	}

	t.Run("trims history when exceeding limit", func(t *testing.T) {
		for i := 0; i < 10; i++ {
			s.AddMessage("msg", "resp")
		}

		if len(s.history) != 5 {
			t.Errorf("expected history to be trimmed to 5, got %d", len(s.history))
		}
	})

	t.Run("keeps most recent messages", func(t *testing.T) {
		// History should contain messages 5-9 (0-indexed)
		// Since we added 10 and trimmed to 5, we keep the last 5
		if s.HistoryLen() != 5 {
			t.Errorf("expected 5 entries, got %d", s.HistoryLen())
		}
	})
}

func TestSessionRecentHistory(t *testing.T) {
	s := &Session{
		ID:         "test-session",
		maxHistory: 100,
	}

	// Add 10 messages
	for i := 0; i < 10; i++ {
		s.AddMessage("msg", "resp")
	}

	t.Run("returns all when maxEntries > history", func(t *testing.T) {
		recent := s.RecentHistory(20)
		if len(recent) != 10 {
			t.Errorf("expected 10 entries, got %d", len(recent))
		}
	})

	t.Run("returns last N entries", func(t *testing.T) {
		recent := s.RecentHistory(3)
		if len(recent) != 3 {
			t.Errorf("expected 3 entries, got %d", len(recent))
		}
	})

	t.Run("returns copy not reference", func(t *testing.T) {
		recent := s.RecentHistory(3)
		recent[0].UserMessage = "modified"

		// Original should be unchanged
		if s.history[len(s.history)-3].UserMessage == "modified" {
			t.Error("expected original history to be unchanged")
		}
	})
}

func TestSessionFacts(t *testing.T) {
	s := &Session{
		ID: "test-session",
	}

	t.Run("adds and retrieves facts", func(t *testing.T) {
		s.AddFact("User likes pizza")
		s.AddFact("User speaks Portuguese")

		facts := s.GetFacts()
		if len(facts) != 2 {
			t.Errorf("expected 2 facts, got %d", len(facts))
		}

		// Check that our facts are present
		factMap := make(map[string]bool)
		for _, f := range facts {
			factMap[f] = true
		}

		if !factMap["User likes pizza"] {
			t.Error("expected fact 'User likes pizza' to be present")
		}
		if !factMap["User speaks Portuguese"] {
			t.Error("expected fact 'User speaks Portuguese' to be present")
		}
	})

	t.Run("GetFacts returns copy", func(t *testing.T) {
		facts := s.GetFacts()
		facts[0] = "modified"

		original := s.GetFacts()
		if original[0] == "modified" {
			t.Error("expected original facts to be unchanged")
		}
	})

	t.Run("ClearFacts removes all facts", func(t *testing.T) {
		s.ClearFacts()
		facts := s.GetFacts()
		if len(facts) != 0 {
			t.Errorf("expected 0 facts after clear, got %d", len(facts))
		}
	})
}

func TestSessionActiveSkills(t *testing.T) {
	s := &Session{
		ID: "test-session",
	}

	t.Run("sets and gets active skills", func(t *testing.T) {
		s.SetActiveSkills([]string{"skill1", "skill2"})
		skills := s.GetActiveSkills()

		if len(skills) != 2 {
			t.Errorf("expected 2 skills, got %d", len(skills))
		}
	})

	t.Run("GetActiveSkills returns copy", func(t *testing.T) {
		skills := s.GetActiveSkills()
		skills[0] = "modified"

		original := s.GetActiveSkills()
		if original[0] == "modified" {
			t.Error("expected original skills to be unchanged")
		}
	})
}

func TestSessionConfig(t *testing.T) {
	s := &Session{
		ID: "test-session",
	}

	t.Run("sets and gets config", func(t *testing.T) {
		cfg := SessionConfig{
			Trigger:         "!bot",
			Language:        "pt-BR",
			MaxTokens:       4000,
			Model:           "gpt-4",
			BusinessContext: "Customer support",
			ThinkingLevel:   "medium",
			Verbose:         true,
		}

		s.SetConfig(cfg)
		got := s.GetConfig()

		if got.Trigger != cfg.Trigger {
			t.Errorf("expected trigger %q, got %q", cfg.Trigger, got.Trigger)
		}
		if got.Language != cfg.Language {
			t.Errorf("expected language %q, got %q", cfg.Language, got.Language)
		}
		if got.MaxTokens != cfg.MaxTokens {
			t.Errorf("expected maxTokens %d, got %d", cfg.MaxTokens, got.MaxTokens)
		}
		if got.Model != cfg.Model {
			t.Errorf("expected model %q, got %q", cfg.Model, got.Model)
		}
	})
}

func TestSessionTokenUsage(t *testing.T) {
	s := &Session{
		ID: "test-session",
	}

	t.Run("tracks token usage", func(t *testing.T) {
		s.AddTokenUsage(100, 50)
		s.AddTokenUsage(200, 75)

		prompt, completion, requests := s.GetTokenUsage()

		if prompt != 300 {
			t.Errorf("expected 300 prompt tokens, got %d", prompt)
		}
		if completion != 125 {
			t.Errorf("expected 125 completion tokens, got %d", completion)
		}
		if requests != 2 {
			t.Errorf("expected 2 requests, got %d", requests)
		}
	})

	t.Run("resets token usage", func(t *testing.T) {
		s.ResetTokenUsage()
		prompt, completion, requests := s.GetTokenUsage()

		if prompt != 0 || completion != 0 || requests != 0 {
			t.Error("expected all token usage to be reset to 0")
		}
	})
}

func TestSessionThinkingLevel(t *testing.T) {
	s := &Session{
		ID: "test-session",
		config: SessionConfig{
			ThinkingLevel: "low",
		},
	}

	t.Run("gets thinking level", func(t *testing.T) {
		level := s.GetThinkingLevel()
		if level != "low" {
			t.Errorf("expected thinking level 'low', got %q", level)
		}
	})

	t.Run("sets thinking level", func(t *testing.T) {
		s.SetThinkingLevel("high")
		level := s.GetThinkingLevel()
		if level != "high" {
			t.Errorf("expected thinking level 'high', got %q", level)
		}
	})
}

func TestSessionCompactHistory(t *testing.T) {
	s := &Session{
		ID:         "test-session",
		maxHistory: 100,
	}

	// Add 20 messages
	for i := 0; i < 20; i++ {
		s.AddMessage("msg", "resp")
	}

	t.Run("compacts history with summary", func(t *testing.T) {
		old := s.CompactHistory("Summary of old messages", 5)

		if len(old) != 15 {
			t.Errorf("expected 15 old entries returned, got %d", len(old))
		}

		// History should now have summary + 5 recent = 6 entries
		if s.HistoryLen() != 6 {
			t.Errorf("expected 6 entries after compact, got %d", s.HistoryLen())
		}

		// First entry should be the summary
		if s.history[0].UserMessage != "[session compacted]" {
			t.Errorf("expected summary entry, got %q", s.history[0].UserMessage)
		}
		if s.history[0].AssistantResponse != "Summary of old messages" {
			t.Errorf("expected summary text, got %q", s.history[0].AssistantResponse)
		}
	})

	t.Run("returns nil when nothing to compact", func(t *testing.T) {
		// Only 6 entries, keeping 5 means only 1 to remove
		// But if we try to keep more than we have, nothing happens
		old := s.CompactHistory("Another summary", 10)
		if old != nil {
			t.Error("expected nil when keeping more than history size")
		}
	})
}

func TestSessionClearHistory(t *testing.T) {
	s := &Session{
		ID: "test-session",
	}

	// Add some messages
	for i := 0; i < 5; i++ {
		s.AddMessage("msg", "resp")
	}

	t.Run("clears history", func(t *testing.T) {
		s.ClearHistory()
		if s.HistoryLen() != 0 {
			t.Errorf("expected 0 entries after clear, got %d", s.HistoryLen())
		}
	})
}

func TestSessionConcurrency(t *testing.T) {
	s := &Session{
		ID:         "test-session",
		maxHistory: 1000,
	}

	var wg sync.WaitGroup
	numOps := 100

	t.Run("concurrent AddMessage is safe", func(t *testing.T) {
		for i := 0; i < numOps; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				s.AddMessage("msg", "resp")
			}(i)
		}
		wg.Wait()

		if s.HistoryLen() != numOps {
			t.Errorf("expected %d entries, got %d", numOps, s.HistoryLen())
		}
	})

	t.Run("concurrent reads are safe", func(t *testing.T) {
		for i := 0; i < numOps; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_ = s.RecentHistory(10)
				_ = s.GetFacts()
				_ = s.GetActiveSkills()
				_, _, _ = s.GetTokenUsage()
			}()
		}
		wg.Wait()
	})
}

func TestSessionLastActiveAt(t *testing.T) {
	s := &Session{
		ID:           "test-session",
		lastActiveAt: time.Now().Add(-1 * time.Hour),
	}

	before := s.LastActiveAt()
	s.AddMessage("test", "test")
	after := s.LastActiveAt()

	if !after.After(before) {
		t.Error("expected LastActiveAt to be updated after AddMessage")
	}
}
