package copilot

import (
	"context"
	"testing"
	"time"
)

func TestDBHubRateLimiter(t *testing.T) {
	limiter := &dbHubRateLimiter{
		lastCall:    make(map[string]time.Time),
		minInterval: 100 * time.Millisecond,
	}

	// First call should be allowed
	if !limiter.Allow("session1") {
		t.Error("first call should be allowed")
	}

	// Immediate second call should be blocked
	if limiter.Allow("session1") {
		t.Error("immediate second call should be blocked")
	}

	// Wait and try again
	time.Sleep(150 * time.Millisecond)
	if !limiter.Allow("session1") {
		t.Error("call after interval should be allowed")
	}

	// Different session should have its own limit
	if !limiter.Allow("session2") {
		t.Error("different session should be allowed")
	}
}

func TestDBHubRateLimiter_Concurrent(t *testing.T) {
	limiter := &dbHubRateLimiter{
		lastCall:    make(map[string]time.Time),
		minInterval: 50 * time.Millisecond,
	}

	// Test concurrent access
	done := make(chan bool)

	for i := 0; i < 10; i++ {
		go func(id string) {
			limiter.Allow(id)
			done <- true
		}(string(rune('a' + i)))
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}
}

func TestDBHubRateLimiter_Cleanup(t *testing.T) {
	limiter := &dbHubRateLimiter{
		lastCall:    make(map[string]time.Time),
		minInterval: 100 * time.Millisecond,
	}

	// Add entries with timestamps in the past to simulate old sessions
	limiter.mu.Lock()
	oldTime := time.Now().Add(-10 * time.Minute)
	for i := 0; i < 1000; i++ {
		limiter.lastCall[string(rune(i))] = oldTime
	}
	limiter.mu.Unlock()

	// This call should trigger cleanup since len > 1000
	limiter.Allow("trigger-cleanup")

	// The cleanup should have removed old entries (only recent ones remain)
	limiter.mu.Lock()
	count := len(limiter.lastCall)
	limiter.mu.Unlock()

	// After cleanup, most old entries should be removed, leaving only recent ones
	// The "trigger-cleanup" entry and any others from this test
	if count > 100 {
		t.Errorf("expected cleanup to remove old entries, got %d remaining", count)
	}
}

func TestExtractSessionID(t *testing.T) {
	// Without context value
	ctx := context.Background()
	if id := extractSessionID(ctx); id != "default" {
		t.Errorf("expected 'default', got '%s'", id)
	}

	// With context value
	ctx = context.WithValue(context.Background(), sessionIDKey{}, "test-session")
	if id := extractSessionID(ctx); id != "test-session" {
		t.Errorf("expected 'test-session', got '%s'", id)
	}

	// With empty value
	ctx = context.WithValue(context.Background(), sessionIDKey{}, "")
	if id := extractSessionID(ctx); id != "default" {
		t.Errorf("expected 'default' for empty value, got '%s'", id)
	}

	// With non-string value
	ctx = context.WithValue(context.Background(), sessionIDKey{}, 12345)
	if id := extractSessionID(ctx); id != "default" {
		t.Errorf("expected 'default' for non-string value, got '%s'", id)
	}
}

func TestDBHubRateLimiter_GlobalInstance(t *testing.T) {
	// Test that globalRateLimiter is properly initialized
	if globalRateLimiter == nil {
		t.Fatal("globalRateLimiter should be initialized")
	}

	if globalRateLimiter.lastCall == nil {
		t.Fatal("globalRateLimiter.lastCall should be initialized")
	}

	if globalRateLimiter.minInterval != 100*time.Millisecond {
		t.Errorf("expected minInterval 100ms, got %v", globalRateLimiter.minInterval)
	}
}

func TestDBHubRateLimiter_BurstThenWait(t *testing.T) {
	limiter := &dbHubRateLimiter{
		lastCall:    make(map[string]time.Time),
		minInterval: 50 * time.Millisecond,
	}

	// Burst of calls - only first should succeed
	allowed := 0
	for i := 0; i < 5; i++ {
		if limiter.Allow("burst-test") {
			allowed++
		}
	}

	if allowed != 1 {
		t.Errorf("expected 1 allowed in burst, got %d", allowed)
	}

	// Wait and try again
	time.Sleep(100 * time.Millisecond)
	if !limiter.Allow("burst-test") {
		t.Error("call after wait should be allowed")
	}
}

func TestDBHubRateLimiter_MultipleSessions(t *testing.T) {
	limiter := &dbHubRateLimiter{
		lastCall:    make(map[string]time.Time),
		minInterval: 100 * time.Millisecond,
	}

	sessions := []string{"session1", "session2", "session3"}

	// Each session should be allowed on first call
	for _, session := range sessions {
		if !limiter.Allow(session) {
			t.Errorf("session %s should be allowed on first call", session)
		}
	}

	// None should be allowed immediately after
	for _, session := range sessions {
		if limiter.Allow(session) {
			t.Errorf("session %s should be blocked on immediate second call", session)
		}
	}
}
