// Package copilot – memory_tools.go implements the memory dispatcher tool.
// Uses dispatcher pattern to reduce tool count while maintaining functionality.
package copilot

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/jholhewres/devclaw/pkg/devclaw/copilot/memory"
)

// MemoryDispatcherConfig holds configuration for the memory dispatcher tool.
type MemoryDispatcherConfig struct {
	Store       *memory.FileStore
	SQLiteStore *memory.SQLiteStore
	Config      MemoryConfig
}

// RegisterMemoryTools registers the memory dispatcher tool.
// Uses a single tool with action parameter instead of multiple tools.
func RegisterMemoryTools(executor *ToolExecutor, cfg MemoryDispatcherConfig) {
	store := cfg.Store
	sqliteStore := cfg.SQLiteStore
	memCfg := cfg.Config

	// Build description based on available features
	desc := "Manage long-term memory with actions: save, search, list, index. " +
		"Use action='save' to remember facts/preferences, action='search' to find relevant memories, " +
		"action='list' to see recent entries, action='index' to rebuild search index."
	if sqliteStore != nil {
		desc += " Supports semantic search (vector + keyword hybrid)."
	}

	// Define schema with all possible parameters
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"action": map[string]any{
				"type":        "string",
				"enum":        []string{"save", "search", "list", "index"},
				"description": "Action to perform: save, search, list, index",
			},
			"content": map[string]any{
				"type":        "string",
				"description": "Content to save (for action='save')",
			},
			"category": map[string]any{
				"type":        "string",
				"description": "Category for saved content: 'fact', 'preference', 'event', 'summary' (for action='save')",
				"enum":        []string{"fact", "preference", "event", "summary"},
			},
			"query": map[string]any{
				"type":        "string",
				"description": "Search query (for action='search')",
			},
			"limit": map[string]any{
				"type":        "integer",
				"description": "Maximum results to return (for action='search' or 'list')",
			},
		},
		"required": []string{"action"},
	}

	executor.Register(
		MakeToolDefinition("memory", desc, schema),
		func(ctx context.Context, args map[string]any) (any, error) {
			action, _ := args["action"].(string)
			if action == "" {
				return nil, fmt.Errorf("action is required")
			}

			switch action {
			case "save":
				return handleMemorySave(ctx, store, sqliteStore, memCfg, args)
			case "search":
				return handleMemorySearch(ctx, store, sqliteStore, memCfg, args)
			case "list":
				return handleMemoryList(ctx, store, args)
			case "index":
				return handleMemoryIndex(ctx, sqliteStore, memCfg)
			default:
				return nil, fmt.Errorf("unknown action: %s (valid: save, search, list, index)", action)
			}
		},
	)
}

// handleMemorySave saves content to long-term memory.
func handleMemorySave(_ context.Context, store *memory.FileStore, sqliteStore *memory.SQLiteStore, cfg MemoryConfig, args map[string]any) (any, error) {
	content, _ := args["content"].(string)
	if content == "" {
		return nil, fmt.Errorf("content is required for save action")
	}

	// Validate category
	validCategories := map[string]bool{"fact": true, "preference": true, "event": true, "summary": true}
	category, _ := args["category"].(string)
	if category == "" {
		category = "fact"
	} else if !validCategories[category] {
		return nil, fmt.Errorf("invalid category: %s (valid: fact, preference, event, summary)", category)
	}

	err := store.Save(memory.Entry{
		Content:   content,
		Source:    "agent",
		Category:  category,
		Timestamp: time.Now(),
	})
	if err != nil {
		return nil, err
	}

	// Re-index the MEMORY.md file if SQLite memory is available.
	if sqliteStore != nil && cfg.Index.Auto {
		memDir := filepath.Join(filepath.Dir(cfg.Path), "memory")
		chunkCfg := memory.ChunkConfig{MaxTokens: cfg.Index.ChunkMaxTokens, Overlap: 100}
		if chunkCfg.MaxTokens <= 0 {
			chunkCfg.MaxTokens = 500
		}
		go func() {
			// Use timeout to prevent goroutine leak on shutdown
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			_ = sqliteStore.IndexMemoryDir(ctx, memDir, chunkCfg)
		}()
	}

	return fmt.Sprintf("Saved to memory [%s]: %s", category, content), nil
}

// handleMemorySearch searches long-term memory.
func handleMemorySearch(ctx context.Context, store *memory.FileStore, sqliteStore *memory.SQLiteStore, cfg MemoryConfig, args map[string]any) (any, error) {
	query, _ := args["query"].(string)
	if query == "" {
		return nil, fmt.Errorf("query is required for search action")
	}

	maxLimit := 100
	limit := 10
	if l, ok := args["limit"].(float64); ok && l > 0 {
		limit = int(l)
		if limit > maxLimit {
			limit = maxLimit
		}
	}

	// Try hybrid search first if SQLite is available.
	if sqliteStore != nil {
		// Build config for temporal decay and MMR
		decayCfg := memory.TemporalDecayConfig{
			Enabled:      cfg.Search.TemporalDecay.Enabled,
			HalfLifeDays: cfg.Search.TemporalDecay.HalfLifeDays,
		}
		mmrCfg := memory.MMRConfig{
			Enabled: cfg.Search.MMR.Enabled,
			Lambda:  cfg.Search.MMR.Lambda,
		}

		results, err := sqliteStore.HybridSearchWithOptions(
			ctx, query, limit, cfg.Search.MinScore,
			cfg.Search.HybridWeightVector, cfg.Search.HybridWeightBM25,
			decayCfg, mmrCfg,
		)
		if err == nil && len(results) > 0 {
			var sb strings.Builder
			sb.WriteString(fmt.Sprintf("Found %d memories (semantic search):\n\n", len(results)))
			for _, r := range results {
				text := r.Text
				if len(text) > 500 {
					text = text[:500] + "..."
				}
				sb.WriteString(fmt.Sprintf("- [%s] (score: %.2f) %s\n", r.FileID, r.Score, text))
			}
			return sb.String(), nil
		}
	}

	// Fallback to substring search.
	entries, err := store.Search(query, limit)
	if err != nil {
		return nil, err
	}

	if len(entries) == 0 {
		return "No memories found matching the query.", nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Found %d memories:\n\n", len(entries)))
	for _, e := range entries {
		sb.WriteString(fmt.Sprintf("- [%s] %s\n", e.Category, e.Content))
	}
	return sb.String(), nil
}

// handleMemoryList lists recent memories.
func handleMemoryList(_ context.Context, store *memory.FileStore, args map[string]any) (any, error) {
	maxLimit := 100
	limit := 20
	if l, ok := args["limit"].(float64); ok && l > 0 {
		limit = int(l)
		if limit > maxLimit {
			limit = maxLimit
		}
	}

	entries, err := store.GetRecent(limit)
	if err != nil {
		return nil, err
	}

	if len(entries) == 0 {
		return "No memories stored yet.", nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Recent memories (%d):\n\n", len(entries)))
	for _, e := range entries {
		sb.WriteString(fmt.Sprintf("- [%s] [%s] %s\n",
			e.Timestamp.Format("2006-01-02"),
			e.Category,
			e.Content))
	}
	return sb.String(), nil
}

// handleMemoryIndex manually triggers re-indexing.
func handleMemoryIndex(ctx context.Context, sqliteStore *memory.SQLiteStore, cfg MemoryConfig) (any, error) {
	if sqliteStore == nil {
		return "Memory indexing not available (SQLite store not configured).", nil
	}

	memDir := filepath.Join(filepath.Dir(cfg.Path), "memory")
	chunkCfg := memory.ChunkConfig{MaxTokens: cfg.Index.ChunkMaxTokens, Overlap: 100}
	if chunkCfg.MaxTokens <= 0 {
		chunkCfg.MaxTokens = 500
	}

	if err := sqliteStore.IndexMemoryDir(ctx, memDir, chunkCfg); err != nil {
		return nil, fmt.Errorf("indexing failed: %w", err)
	}

	return fmt.Sprintf("Memory index updated: %d files, %d chunks.",
		sqliteStore.FileCount(), sqliteStore.ChunkCount()), nil
}
