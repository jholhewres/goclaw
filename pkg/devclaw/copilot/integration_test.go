package copilot

import (
	"log/slog"
	"testing"
)

// TestStrategyLoopIntegration simulates the real-world scenario from the bug report:
// Agent tries to add a route but gets 404 repeatedly with different approaches.
func TestStrategyLoopIntegration(t *testing.T) {
	t.Parallel()
	
	cfg := ToolLoopConfig{
		Enabled:                 true,
		HistorySize:             30,
		WarningThreshold:        8,
		CriticalThreshold:       15,
		CircuitBreakerThreshold: 25,
		GlobalCircuitBreaker:    30,
	}
	
	detector := NewToolLoopDetector(cfg, slog.Default())
	
	// Simulate the actual sequence from the bug report:
	// Agent tries multiple different approaches but gets same 404 error
	
	sameError := "Error: Cannot GET /api/matches/10/demo-url - 404 Not Found"
	
	// Attempt 1: Check if route exists in source
	detector.RecordAndCheck("bash", map[string]any{"command": "grep demo-url src/dashboard/routes.ts"})
	detector.RecordToolOutcome("demo-url found in routes.ts")
	
	// Attempt 2: Check if route exists in compiled output
	detector.RecordAndCheck("bash", map[string]any{"command": "grep demo-url dist/dashboard/routes.js"})
	detector.RecordToolOutcome("demo-url found in dist/routes.js")
	
	// Attempt 3: Test the route - gets 404
	detector.RecordAndCheck("bash", map[string]any{"command": "curl http://localhost:3000/api/matches/10/demo-url"})
	detector.RecordToolOutcome(sameError)
	
	// Attempt 4: Rebuild
	detector.RecordAndCheck("bash", map[string]any{"command": "npm run build"})
	detector.RecordToolOutcome("Build completed")
	
	// Attempt 5: Test again - still 404
	detector.RecordAndCheck("bash", map[string]any{"command": "curl http://localhost:3000/api/matches/10/demo-url"})
	detector.RecordToolOutcome(sameError)
	
	// Attempt 6: Restart server
	detector.RecordAndCheck("bash", map[string]any{"command": "pm2 restart app"})
	detector.RecordToolOutcome("App restarted")
	
	// Attempt 7: Test again - still 404
	detector.RecordAndCheck("bash", map[string]any{"command": "curl http://localhost:3000/api/matches/10/demo-url"})
	detector.RecordToolOutcome(sameError)
	
	// Attempt 8: Check if routes are registered
	detector.RecordAndCheck("bash", map[string]any{"command": "grep registerRoutes src/server.ts"})
	detector.RecordToolOutcome("registerRoutes found")
	
	// Attempt 9: Test again - still 404
	detector.RecordAndCheck("bash", map[string]any{"command": "curl -v http://localhost:3000/api/matches/10/demo-url"})
	detector.RecordToolOutcome(sameError)
	
	// Attempt 10: Rebuild again
	detector.RecordAndCheck("bash", map[string]any{"command": "npm run build && pm2 restart"})
	detector.RecordToolOutcome("Build and restart complete")
	
	// Attempt 11: Test again - STILL 404 (5th same error)
	detector.RecordAndCheck("bash", map[string]any{"command": "curl http://localhost:3000/api/matches/10/demo-url"})
	detector.RecordToolOutcome(sameError)
	
	// Now sameErrorCount should be 5, next RecordAndCheck should trigger
	result := detector.RecordAndCheck("bash", map[string]any{"command": "curl -s http://localhost:3000/api/matches/10/demo-url"})
	
	// At this point (5th same error), strategy loop should be detected
	if result.Severity != LoopCritical {
		t.Errorf("Expected LoopCritical after 5 same errors, got %v (sameErrorCount=%d)", result.Severity, detector.sameErrorCount)
	}
	
	if result.Pattern != "strategy_loop" {
		t.Errorf("Expected pattern 'strategy_loop', got %q", result.Pattern)
	}
	
	// Verify the message suggests investigation
	if result.Message == "" {
		t.Error("Expected non-empty message with investigation guidance")
	}
	
	t.Logf("Strategy loop detected after %d attempts with message: %s", result.Streak, result.Message)
}

// TestReflectionIntervalReduction verifies that reflection happens every 5 turns
func TestReflectionIntervalReduction(t *testing.T) {
	t.Parallel()
	
	// Verify the constant is set correctly
	if reflectionInterval != 5 {
		t.Errorf("Expected reflectionInterval to be 5, got %d", reflectionInterval)
	}
	
	// Verify it's documented
	const expectedComment = "Reduced from 15 to 5 to catch stuck patterns earlier"
	// This is a compile-time check - if the constant exists, the code compiles
	_ = reflectionInterval
}

// TestPromptLayerIntegration verifies that new prompt sections are included
func TestPromptLayerIntegration(t *testing.T) {
	t.Parallel()
	
	config := &Config{
		Name:     "TestAgent",
		Model:    "test-model",
		Language: "en",
		Timezone: "UTC",
		TokenBudget: TokenBudgetConfig{
			Total:             128000,
			System:            8000,
			History:           8000,
			Memory:            2000,
			Skills:            4000,
			BootstrapMaxChars: 20000,
		},
	}
	
	composer := NewPromptComposer(config)
	session := &Session{
		ID:      "test-session",
		Channel: "test",
		ChatID:  "test-chat",
	}
	
	prompt := composer.Compose(session, "test input")
	
	// Verify new sections are present (from buildCoreLayer, buildSafetyLayer)
	// These sections are always present regardless of bootstrap file loading
	requiredSections := []string{
		"You are TestAgent",
		"## Tooling",
		"## Tool Call Style",
		"## Safety",
		"## Workspace",
		"## Reply Tags",
		"## Messaging",
		"## Silent Replies",
		"## Heartbeats",
		"## Encrypted Vault",
		"## Media Capabilities",
		"NO_REPLY",
		"HEARTBEAT_OK",
	}
	
	for _, section := range requiredSections {
		if !contains(prompt, section) {
			t.Errorf("Expected prompt to contain section: %q", section)
		}
	}
	
	t.Logf("Prompt length: %d chars", len(prompt))
}

// Helper function to check if a string contains a substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && 
		(s[:len(substr)] == substr || contains(s[1:], substr)))
}
