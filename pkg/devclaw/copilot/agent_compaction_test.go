package copilot

import (
	"context"
	"log/slog"
	"os"
	"strings"
	"testing"
)

func TestManagedCompaction(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	// Create mock LLM setup
	cfg := &Config{
		Model: "test-model",
		API: APIConfig{
			Provider: "openai",
			BaseURL:  "http://localhost:1234",
		},
	}
	llm := NewLLMClient(cfg, logger)
	agent := NewAgentRun(llm, nil, logger)

	// Simulate conversation with 15 messages (overflow simulation)
	var messages []chatMessage
	messages = append(messages, chatMessage{Role: "system", Content: "System Prompt Context"})
	messages = append(messages, chatMessage{Role: "user", Content: "Initial Request Goal"})

	for i := 1; i <= 13; i++ {
		if i%2 == 0 {
			messages = append(messages, chatMessage{Role: "user", Content: "User input " + string(rune(i))})
		} else {
			messages = append(messages, chatMessage{Role: "assistant", Content: "Assistant reply " + string(rune(i))})
		}
	}

	// Because we can't actually call LLM here, we'll test the pruning math
	compacted := agent.managedCompaction(context.Background(), messages)

	// In the logic:
	// length = 15
	// body count = 14
	// goal = body[0]
	// header = 1 (system)
	// middle = body[1 : len(body)-6] = body[1 : 14-6] = body[1 : 8] (7 messages)
	// recent = body[14-6:] = body[8:] (6 messages)
	// total = header (1) + goal(1) + summary_msg(1) + recent(6) = 9 messages

	if len(compacted) != 9 {
		t.Errorf("Expected managed compaction to yield 9 messages, got %d", len(compacted))
	}

	if compacted[0].Role != "system" {
		t.Errorf("Expected first message to be system, got %s", compacted[0].Role)
	}

	if compacted[1].Role != "user" || compacted[1].Content != "Initial Request Goal" {
		t.Errorf("Expected second message to be the goal")
	}

	summaryMsg, ok := compacted[2].Content.(string)
	if !ok || !strings.Contains(summaryMsg, "[System: The following is a summary") {
		t.Errorf("Expected third message to be the summary wrapper, got %v", compacted[2].Content)
	}
}

func TestAggressiveCompaction(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	cfg := &Config{
		Model: "test-model",
		API: APIConfig{
			Provider: "openai",
			BaseURL:  "http://localhost:1234",
		},
	}
	llm := NewLLMClient(cfg, logger)
	agent := NewAgentRun(llm, nil, logger)

	var messages []chatMessage
	messages = append(messages, chatMessage{Role: "system", Content: "System"})
	messages = append(messages, chatMessage{Role: "user", Content: "Goal"})

	for i := 0; i < 10; i++ {
		messages = append(messages, chatMessage{Role: "assistant", Content: "Test"})
	}

	// 1 system + 11 user/assistant = 12 total messages
	// aggressive keeps goal (body[0]) + summary + keepRecent (2)
	// Total expecting: 1 system + 1 goal + 1 summary + 2 recent = 5

	compacted := agent.aggressiveCompaction(context.Background(), messages)

	if len(compacted) != 5 {
		t.Errorf("Expected aggressive compaction to yield 5 messages, got %d", len(compacted))
	}

	summaryMsg, ok := compacted[2].Content.(string)
	if !ok || !strings.Contains(summaryMsg, "[System: Aggressive fallback compaction") {
		t.Errorf("Expected third message to be the aggressive summary wrapper")
	}
}
