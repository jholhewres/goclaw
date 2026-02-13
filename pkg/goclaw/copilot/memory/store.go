// Package memory implements persistent memory for GoClaw.
// Provides long-term fact storage and daily conversation logs using
// the filesystem (MEMORY.md + daily markdown files).
//
// Architecture:
//   - MEMORY.md: Long-term facts (append-only, curated by the agent)
//   - memory/YYYY-MM-DD.md: Daily conversation summaries (append-only)
//   - Search uses simple substring matching (future: BM25 / embeddings)
package memory

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Entry represents a single memory fact or event.
type Entry struct {
	Content   string    `json:"content"`
	Source    string    `json:"source"`    // "user", "agent", "system"
	Category string    `json:"category"`  // "fact", "preference", "event", "summary"
	Timestamp time.Time `json:"timestamp"`
}

// Store defines the interface for memory persistence.
type Store interface {
	// Save persists a new memory entry.
	Save(entry Entry) error

	// Search returns entries matching the query, limited to maxResults.
	Search(query string, maxResults int) ([]Entry, error)

	// GetRecent returns the most recent entries up to limit.
	GetRecent(limit int) ([]Entry, error)

	// GetByDate returns entries for a specific date.
	GetByDate(date time.Time) ([]Entry, error)

	// GetAll returns all stored entries.
	GetAll() ([]Entry, error)

	// SaveDailyLog appends a summary to the daily log file.
	SaveDailyLog(date time.Time, content string) error
}

// FileStore implements Store using the filesystem.
// Uses MEMORY.md for long-term facts and daily markdown files for logs.
type FileStore struct {
	baseDir string
	mu      sync.RWMutex
}

// NewFileStore creates a file-based memory store at the given directory.
func NewFileStore(baseDir string) (*FileStore, error) {
	if baseDir == "" {
		baseDir = "./data/memory"
	}

	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		return nil, fmt.Errorf("creating memory directory: %w", err)
	}

	return &FileStore{baseDir: baseDir}, nil
}

// Save appends a memory entry to MEMORY.md.
func (fs *FileStore) Save(entry Entry) error {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	memFile := filepath.Join(fs.baseDir, "MEMORY.md")

	// Format the entry as a markdown list item.
	line := fmt.Sprintf("- [%s] [%s] %s\n",
		entry.Timestamp.Format("2006-01-02 15:04"),
		entry.Category,
		entry.Content,
	)

	f, err := os.OpenFile(memFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("opening memory file: %w", err)
	}
	defer f.Close()

	// Add header if file is new.
	info, _ := f.Stat()
	if info != nil && info.Size() == 0 {
		f.WriteString("# GoClaw Memory\n\nLong-term facts and preferences.\n\n")
	}

	_, err = f.WriteString(line)
	return err
}

// Search returns entries whose content matches the query (case-insensitive substring).
func (fs *FileStore) Search(query string, maxResults int) ([]Entry, error) {
	all, err := fs.GetAll()
	if err != nil {
		return nil, err
	}

	query = strings.ToLower(query)
	var results []Entry

	for _, entry := range all {
		if strings.Contains(strings.ToLower(entry.Content), query) {
			results = append(results, entry)
			if maxResults > 0 && len(results) >= maxResults {
				break
			}
		}
	}

	return results, nil
}

// GetRecent returns the most recent entries up to the limit.
func (fs *FileStore) GetRecent(limit int) ([]Entry, error) {
	all, err := fs.GetAll()
	if err != nil {
		return nil, err
	}

	if limit > 0 && len(all) > limit {
		all = all[len(all)-limit:]
	}

	return all, nil
}

// GetByDate returns entries from the daily log for the given date.
func (fs *FileStore) GetByDate(date time.Time) ([]Entry, error) {
	fs.mu.RLock()
	defer fs.mu.RUnlock()

	dayFile := filepath.Join(fs.baseDir, date.Format("2006-01-02")+".md")
	content, err := os.ReadFile(dayFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	return parseMemoryFile(string(content), "daily"), nil
}

// GetAll reads and parses all entries from MEMORY.md.
func (fs *FileStore) GetAll() ([]Entry, error) {
	fs.mu.RLock()
	defer fs.mu.RUnlock()

	memFile := filepath.Join(fs.baseDir, "MEMORY.md")
	content, err := os.ReadFile(memFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	return parseMemoryFile(string(content), "memory"), nil
}

// SaveDailyLog appends a conversation summary to the daily log file.
func (fs *FileStore) SaveDailyLog(date time.Time, content string) error {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	dayFile := filepath.Join(fs.baseDir, date.Format("2006-01-02")+".md")

	f, err := os.OpenFile(dayFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("opening daily log: %w", err)
	}
	defer f.Close()

	info, _ := f.Stat()
	if info != nil && info.Size() == 0 {
		f.WriteString(fmt.Sprintf("# Daily Log – %s\n\n", date.Format("2006-01-02")))
	}

	timestamp := time.Now().Format("15:04")
	_, err = f.WriteString(fmt.Sprintf("## %s\n\n%s\n\n", timestamp, content))
	return err
}

// RecentFacts returns a formatted string of recent facts suitable for
// injection into the system prompt.
func (fs *FileStore) RecentFacts(maxFacts int, query string) string {
	var entries []Entry
	var err error

	if query != "" {
		entries, err = fs.Search(query, maxFacts)
	} else {
		entries, err = fs.GetRecent(maxFacts)
	}

	if err != nil || len(entries) == 0 {
		return ""
	}

	var b strings.Builder
	for _, e := range entries {
		b.WriteString(fmt.Sprintf("- %s\n", e.Content))
	}
	return b.String()
}

// ListDailyLogs returns the dates of all daily log files, sorted newest first.
func (fs *FileStore) ListDailyLogs() ([]string, error) {
	fs.mu.RLock()
	defer fs.mu.RUnlock()

	entries, err := os.ReadDir(fs.baseDir)
	if err != nil {
		return nil, err
	}

	var dates []string
	for _, e := range entries {
		name := e.Name()
		if strings.HasSuffix(name, ".md") && name != "MEMORY.md" {
			dates = append(dates, strings.TrimSuffix(name, ".md"))
		}
	}

	sort.Sort(sort.Reverse(sort.StringSlice(dates)))
	return dates, nil
}

// ---------- Parsing ----------

// parseMemoryFile parses a memory markdown file into entries.
// Recognizes lines formatted as: - [YYYY-MM-DD HH:MM] [category] content
func parseMemoryFile(content, source string) []Entry {
	var entries []Entry

	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "- ") {
			continue
		}

		line = strings.TrimPrefix(line, "- ")
		entry := Entry{Source: source}

		// Try to parse timestamp: [YYYY-MM-DD HH:MM]
		if strings.HasPrefix(line, "[") {
			closeBracket := strings.Index(line, "]")
			if closeBracket > 0 {
				ts := line[1:closeBracket]
				t, err := time.Parse("2006-01-02 15:04", ts)
				if err == nil {
					entry.Timestamp = t
				}
				line = strings.TrimSpace(line[closeBracket+1:])
			}
		}

		// Try to parse category: [category]
		if strings.HasPrefix(line, "[") {
			closeBracket := strings.Index(line, "]")
			if closeBracket > 0 {
				entry.Category = line[1:closeBracket]
				line = strings.TrimSpace(line[closeBracket+1:])
			}
		}

		entry.Content = line
		if entry.Content != "" {
			entries = append(entries, entry)
		}
	}

	return entries
}
