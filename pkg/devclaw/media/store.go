// Package media provides native media handling for DevClaw.
package media

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// MediaType categorizes stored media.
type MediaType string

const (
	MediaTypeImage    MediaType = "image"
	MediaTypeAudio    MediaType = "audio"
	MediaTypeDocument MediaType = "document"
	// MediaTypeVideo - Futuro: extrair frame + descrever via Vision
)

// StoredMedia represents persisted media metadata.
type StoredMedia struct {
	ID        string         `json:"id"`
	Filename  string         `json:"filename"`
	MimeType  string         `json:"mime_type"`
	Type      MediaType      `json:"type"`
	Size      int64          `json:"size"`
	Channel   string         `json:"channel"`
	SessionID string         `json:"session_id,omitempty"`
	Temporary bool           `json:"temporary"`
	CreatedAt time.Time      `json:"created_at"`
	ExpiresAt *time.Time     `json:"expires_at,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

// SaveRequest contains data for storing media.
type SaveRequest struct {
	Data      []byte
	Filename  string
	MimeType  string
	Type      MediaType
	Channel   string
	SessionID string
	Temporary bool
	TTL       time.Duration
	Metadata  map[string]any
}

// ListFilter filters media listings.
type ListFilter struct {
	Channel   string
	SessionID string
	Type      MediaType
	Temporary *bool
	Limit     int
	Offset    int
}

// MediaStore defines the storage abstraction for media files.
type MediaStore interface {
	// Save stores media data and returns metadata.
	Save(ctx context.Context, req SaveRequest) (*StoredMedia, error)

	// Get retrieves media data by ID.
	Get(ctx context.Context, id string) (io.ReadCloser, *StoredMedia, error)

	// GetBytes retrieves media data as bytes.
	GetBytes(ctx context.Context, id string) ([]byte, *StoredMedia, error)

	// Delete removes media by ID.
	Delete(ctx context.Context, id string) error

	// List returns media matching the filter.
	List(ctx context.Context, filter ListFilter) ([]*StoredMedia, error)

	// DeleteExpired removes all temporary media past their expiration.
	DeleteExpired(ctx context.Context) (int, error)

	// URL returns a URL for accessing the media.
	URL(id string) string
}

// StoreConfig configures FileSystemStore.
type StoreConfig struct {
	BaseDir     string `yaml:"base_dir" json:"base_dir"`
	TempDir     string `yaml:"temp_dir" json:"temp_dir"`
	BaseURL     string `yaml:"base_url" json:"base_url"`
	MaxFileSize int64  `yaml:"max_file_size" json:"max_file_size"`
}

// DefaultStoreConfig returns default configuration.
func DefaultStoreConfig() StoreConfig {
	return StoreConfig{
		BaseDir:     "./data/media",
		TempDir:     "./data/media/temp",
		BaseURL:     "/api/media",
		MaxFileSize: 50 * 1024 * 1024, // 50MB
	}
}

// FileSystemStore implements MediaStore using local filesystem.
type FileSystemStore struct {
	config     StoreConfig
	logger     *slog.Logger
	mu         sync.RWMutex
	metaCache  map[string]*StoredMedia // In-memory cache of metadata
}

// NewFileSystemStore creates a new filesystem-based media store.
func NewFileSystemStore(cfg StoreConfig, logger *slog.Logger) *FileSystemStore {
	if logger == nil {
		logger = slog.Default()
	}

	// Set defaults
	if cfg.BaseDir == "" {
		cfg.BaseDir = "./data/media"
	}
	if cfg.TempDir == "" {
		cfg.TempDir = "./data/media/temp"
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = "/api/media"
	}
	if cfg.MaxFileSize == 0 {
		cfg.MaxFileSize = 50 * 1024 * 1024 // 50MB
	}

	return &FileSystemStore{
		config:    cfg,
		logger:    logger.With("component", "media-store"),
		metaCache: make(map[string]*StoredMedia),
	}
}

// EnsureDir creates the storage directories if they don't exist.
func (s *FileSystemStore) EnsureDir() error {
	dirs := []string{s.config.BaseDir, s.config.TempDir, filepath.Join(s.config.BaseDir, "meta")}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return fmt.Errorf("creating directory %s: %w", dir, err)
		}
	}
	return nil
}

// Save stores media data and returns metadata.
func (s *FileSystemStore) Save(ctx context.Context, req SaveRequest) (*StoredMedia, error) {
	if len(req.Data) == 0 {
		return nil, errors.New("no data provided")
	}

	if int64(len(req.Data)) > s.config.MaxFileSize {
		return nil, fmt.Errorf("file size %d exceeds maximum %d", len(req.Data), s.config.MaxFileSize)
	}

	// Generate ID
	id := uuid.New().String()

	// Compute hash for integrity
	hash := sha256.Sum256(req.Data)
	hashStr := hex.EncodeToString(hash[:])[:16]

	// Sanitize filename
	filename := sanitizeFilename(req.Filename)
	if filename == "" {
		filename = "file"
	}

	// Determine storage path
	storageDir := s.config.BaseDir
	if req.Temporary {
		storageDir = s.config.TempDir
	}

	// Create extension from MIME type or filename
	ext := filepath.Ext(filename)
	if ext == "" {
		ext = extFromMIME(req.MimeType)
	}

	// Build stored media
	now := time.Now()
	media := &StoredMedia{
		ID:        id,
		Filename:  filename,
		MimeType:  req.MimeType,
		Type:      req.Type,
		Size:      int64(len(req.Data)),
		Channel:   req.Channel,
		SessionID: req.SessionID,
		Temporary: req.Temporary,
		CreatedAt: now,
		Metadata:  req.Metadata,
	}

	if req.Temporary && req.TTL > 0 {
		expires := now.Add(req.TTL)
		media.ExpiresAt = &expires
	}

	if media.Metadata == nil {
		media.Metadata = make(map[string]any)
	}
	media.Metadata["hash"] = hashStr

	// Ensure directories exist
	if err := s.EnsureDir(); err != nil {
		return nil, err
	}

	// Write data file
	dataPath := filepath.Join(storageDir, id+ext)
	if err := os.WriteFile(dataPath, req.Data, 0600); err != nil {
		return nil, fmt.Errorf("writing data file: %w", err)
	}

	// Write metadata file
	metaPath := filepath.Join(s.config.BaseDir, "meta", id+".json")
	metaData, err := json.Marshal(media)
	if err != nil {
		os.Remove(dataPath) // Cleanup on error
		return nil, fmt.Errorf("marshaling metadata: %w", err)
	}
	if err := os.WriteFile(metaPath, metaData, 0600); err != nil {
		os.Remove(dataPath) // Cleanup on error
		return nil, fmt.Errorf("writing metadata file: %w", err)
	}

	// Cache metadata
	s.mu.Lock()
	s.metaCache[id] = media
	s.mu.Unlock()

	s.logger.Debug("media saved",
		"id", id,
		"filename", filename,
		"type", media.Type,
		"size", len(req.Data),
		"temporary", req.Temporary,
	)

	return media, nil
}

// Get retrieves media data by ID.
func (s *FileSystemStore) Get(ctx context.Context, id string) (io.ReadCloser, *StoredMedia, error) {
	if id == "" {
		return nil, nil, errors.New("id is required")
	}

	// Validate ID format (UUID)
	if _, err := uuid.Parse(id); err != nil {
		return nil, nil, fmt.Errorf("invalid id format: %w", err)
	}

	// Get metadata
	media, err := s.getMeta(id)
	if err != nil {
		return nil, nil, err
	}

	// Determine storage path
	storageDir := s.config.BaseDir
	if media.Temporary {
		storageDir = s.config.TempDir
	}

	// Find the data file
	pattern := filepath.Join(storageDir, id+"*")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, media, fmt.Errorf("finding data file: %w", err)
	}
	if len(matches) == 0 {
		return nil, media, errors.New("data file not found")
	}

	// Open the file
	file, err := os.Open(matches[0])
	if err != nil {
		return nil, media, fmt.Errorf("opening data file: %w", err)
	}

	return file, media, nil
}

// GetBytes retrieves media data as bytes.
func (s *FileSystemStore) GetBytes(ctx context.Context, id string) ([]byte, *StoredMedia, error) {
	reader, media, err := s.Get(ctx, id)
	if err != nil {
		return nil, media, err
	}
	defer reader.Close()

	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, media, fmt.Errorf("reading data: %w", err)
	}

	return data, media, nil
}

// Delete removes media by ID.
func (s *FileSystemStore) Delete(ctx context.Context, id string) error {
	if id == "" {
		return errors.New("id is required")
	}

	// Get metadata first
	media, err := s.getMeta(id)
	if err != nil {
		return err
	}

	// Determine storage path
	storageDir := s.config.BaseDir
	if media.Temporary {
		storageDir = s.config.TempDir
	}

	// Delete data file(s)
	pattern := filepath.Join(storageDir, id+"*")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return fmt.Errorf("finding data files: %w", err)
	}

	for _, path := range matches {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			s.logger.Warn("failed to delete data file", "path", path, "error", err)
		}
	}

	// Delete metadata file
	metaPath := filepath.Join(s.config.BaseDir, "meta", id+".json")
	if err := os.Remove(metaPath); err != nil && !os.IsNotExist(err) {
		s.logger.Warn("failed to delete metadata file", "path", metaPath, "error", err)
	}

	// Remove from cache
	s.mu.Lock()
	delete(s.metaCache, id)
	s.mu.Unlock()

	s.logger.Debug("media deleted", "id", id)
	return nil
}

// List returns media matching the filter.
func (s *FileSystemStore) List(ctx context.Context, filter ListFilter) ([]*StoredMedia, error) {
	metaDir := filepath.Join(s.config.BaseDir, "meta")
	entries, err := os.ReadDir(metaDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading meta directory: %w", err)
	}

	var results []*StoredMedia
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}

		id := strings.TrimSuffix(entry.Name(), ".json")
		media, err := s.getMeta(id)
		if err != nil {
			continue
		}

		// Apply filters
		if filter.Channel != "" && media.Channel != filter.Channel {
			continue
		}
		if filter.SessionID != "" && media.SessionID != filter.SessionID {
			continue
		}
		if filter.Type != "" && media.Type != filter.Type {
			continue
		}
		if filter.Temporary != nil && media.Temporary != *filter.Temporary {
			continue
		}

		results = append(results, media)
	}

	// Apply offset
	if filter.Offset > 0 && filter.Offset < len(results) {
		results = results[filter.Offset:]
	} else if filter.Offset >= len(results) {
		results = nil
	}

	// Apply limit
	if filter.Limit > 0 && len(results) > filter.Limit {
		results = results[:filter.Limit]
	}

	return results, nil
}

// DeleteExpired removes all temporary media past their expiration.
func (s *FileSystemStore) DeleteExpired(ctx context.Context) (int, error) {
	metaDir := filepath.Join(s.config.BaseDir, "meta")
	entries, err := os.ReadDir(metaDir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, fmt.Errorf("reading meta directory: %w", err)
	}

	now := time.Now()
	count := 0

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}

		id := strings.TrimSuffix(entry.Name(), ".json")
		media, err := s.getMeta(id)
		if err != nil {
			continue
		}

		if media.Temporary && media.ExpiresAt != nil && now.After(*media.ExpiresAt) {
			if err := s.Delete(ctx, id); err != nil {
				s.logger.Warn("failed to delete expired media", "id", id, "error", err)
			} else {
				count++
			}
		}
	}

	return count, nil
}

// URL returns a URL for accessing the media.
func (s *FileSystemStore) URL(id string) string {
	return fmt.Sprintf("%s/%s", s.config.BaseURL, id)
}

// getMeta retrieves metadata from cache or file.
func (s *FileSystemStore) getMeta(id string) (*StoredMedia, error) {
	// Check cache first
	s.mu.RLock()
	if media, ok := s.metaCache[id]; ok {
		s.mu.RUnlock()
		return media, nil
	}
	s.mu.RUnlock()

	// Load from file
	metaPath := filepath.Join(s.config.BaseDir, "meta", id+".json")
	data, err := os.ReadFile(metaPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("media not found: %s", id)
		}
		return nil, fmt.Errorf("reading metadata: %w", err)
	}

	var media StoredMedia
	if err := json.Unmarshal(data, &media); err != nil {
		return nil, fmt.Errorf("parsing metadata: %w", err)
	}

	// Cache it
	s.mu.Lock()
	s.metaCache[id] = &media
	s.mu.Unlock()

	return &media, nil
}

// sanitizeFilename removes dangerous characters from filename.
func sanitizeFilename(name string) string {
	// Remove path separators
	name = filepath.Base(name)

	// Remove null bytes and other control characters
	var result strings.Builder
	for _, r := range name {
		if r >= 32 && r != 127 {
			result.WriteRune(r)
		}
	}

	// Limit length
	sanitized := result.String()
	if len(sanitized) > 255 {
		ext := filepath.Ext(sanitized)
		base := sanitized[:255-len(ext)]
		sanitized = base + ext
	}

	return sanitized
}

// extFromMIME returns a file extension for common MIME types.
func extFromMIME(mime string) string {
	switch {
	case strings.HasPrefix(mime, "image/jpeg"):
		return ".jpg"
	case strings.HasPrefix(mime, "image/png"):
		return ".png"
	case strings.HasPrefix(mime, "image/gif"):
		return ".gif"
	case strings.HasPrefix(mime, "image/webp"):
		return ".webp"
	case strings.HasPrefix(mime, "audio/mpeg"), strings.HasPrefix(mime, "audio/mp3"):
		return ".mp3"
	case strings.HasPrefix(mime, "audio/ogg"):
		return ".ogg"
	case strings.HasPrefix(mime, "audio/wav"):
		return ".wav"
	case strings.HasPrefix(mime, "audio/webm"):
		return ".weba"
	case strings.HasPrefix(mime, "audio/mp4"):
		return ".m4a"
	case strings.HasPrefix(mime, "application/pdf"):
		return ".pdf"
	case strings.HasPrefix(mime, "application/vnd.openxmlformats"):
		return ".docx"
	case strings.HasPrefix(mime, "text/plain"):
		return ".txt"
	default:
		return ""
	}
}
