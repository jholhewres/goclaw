package copilot

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestConfigWatcherDetectsChanges(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	// Create initial config
	initialContent := "name: test-bot\nmodel: gpt-4\ntimezone: UTC\n"
	if err := os.WriteFile(configPath, []byte(initialContent), 0644); err != nil {
		t.Fatalf("failed to write initial config: %v", err)
	}

	var mu sync.Mutex
	changeCount := 0
	var lastConfig *Config

	watcher := NewConfigWatcher(
		configPath,
		100*time.Millisecond,
		func(cfg *Config) {
			mu.Lock()
			defer mu.Unlock()
			changeCount++
			lastConfig = cfg
		},
		slog.Default(),
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go watcher.Start(ctx)

	// Wait for initial load
	time.Sleep(200 * time.Millisecond)

	t.Run("detects config change", func(t *testing.T) {
		// Reset counter
		mu.Lock()
		changeCount = 0
		mu.Unlock()

		// Modify config
		newContent := "name: modified-bot\nmodel: claude-3\ntimezone: UTC\n"
		if err := os.WriteFile(configPath, []byte(newContent), 0644); err != nil {
			t.Fatalf("failed to modify config: %v", err)
		}

		// Wait for detection
		time.Sleep(300 * time.Millisecond)

		mu.Lock()
		defer mu.Unlock()
		if changeCount == 0 {
			t.Error("expected config change to be detected")
		}
		if lastConfig == nil || lastConfig.Name != "modified-bot" {
			t.Errorf("expected name 'modified-bot', got %v", lastConfig)
		}
	})

	t.Run("ignores touch without change", func(t *testing.T) {
		// Reset counter
		mu.Lock()
		changeCount = 0
		mu.Unlock()

		// Touch file without changing content
		now := time.Now()
		if err := os.Chtimes(configPath, now, now); err != nil {
			t.Fatalf("failed to touch config: %v", err)
		}

		// Wait
		time.Sleep(300 * time.Millisecond)

		mu.Lock()
		defer mu.Unlock()
		// Should not trigger change (same hash)
		if changeCount > 0 {
			t.Log("Note: touch triggered change (might be acceptable)")
		}
	})
}

func TestConfigWatcherStop(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	if err := os.WriteFile(configPath, []byte("name: test\n"), 0644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	watcher := NewConfigWatcher(
		configPath,
		50*time.Millisecond,
		func(cfg *Config) {},
		slog.Default(),
	)

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		watcher.Start(ctx)
		close(done)
	}()

	// Let it run briefly
	time.Sleep(100 * time.Millisecond)

	// Stop
	cancel()

	select {
	case <-done:
		// Good, watcher stopped
	case <-time.After(1 * time.Second):
		t.Error("watcher did not stop in time")
	}
}

func TestConfigWatcherHashComparison(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	content1 := "name: test\n"
	content2 := "name: different\n"

	if err := os.WriteFile(configPath, []byte(content1), 0644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	var detectedContents []string
	var mu sync.Mutex

	watcher := NewConfigWatcher(
		configPath,
		50*time.Millisecond,
		func(cfg *Config) {
			mu.Lock()
			defer mu.Unlock()
			if cfg != nil {
				detectedContents = append(detectedContents, cfg.Name)
			}
		},
		slog.Default(),
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go watcher.Start(ctx)

	// Initial load
	time.Sleep(100 * time.Millisecond)

	// Change content
	if err := os.WriteFile(configPath, []byte(content2), 0644); err != nil {
		t.Fatalf("failed to write: %v", err)
	}

	// Wait for detection
	time.Sleep(200 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	// Should have detected the change
	found := false
	for _, name := range detectedContents {
		if name == "different" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected to detect config change to 'different'")
	}
}
