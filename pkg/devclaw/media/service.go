package media

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jholhewres/devclaw/pkg/devclaw/channels"
)

// UploadRequest contains data for uploading media.
type UploadRequest struct {
	Data      []byte
	Filename  string
	MimeType  string
	Channel   string
	SessionID string
	Temporary bool
}

// EnrichmentResult contains extracted content from media.
type EnrichmentResult struct {
	MediaID     string
	Type        MediaType
	Description string // Vision description
	Transcript  string // Audio transcript
	TextContent string // Document text
	Metadata    map[string]any
}

// LLMMediaPrep contains prepared media for LLM consumption.
type LLMMediaPrep struct {
	MediaID   string
	Type      MediaType
	Base64Data string
	MimeType  string
	Caption   string
	FileSize  int64
}

// EnrichmentConfig configures automatic media enrichment.
type EnrichmentConfig struct {
	AutoEnrichImages    bool `yaml:"auto_enrich_images" json:"auto_enrich_images"`
	AutoEnrichAudio     bool `yaml:"auto_enrich_audio" json:"auto_enrich_audio"`
	AutoEnrichDocuments bool `yaml:"auto_enrich_documents" json:"auto_enrich_documents"`
}

// DefaultEnrichmentConfig returns default enrichment configuration.
func DefaultEnrichmentConfig() EnrichmentConfig {
	return EnrichmentConfig{
		AutoEnrichImages:    true,
		AutoEnrichAudio:     true,
		AutoEnrichDocuments: true,
	}
}

// MediaService provides channel-agnostic media handling.
type MediaService struct {
	store      MediaStore
	validator  *Validator
	channelMgr *channels.Manager
	config     ServiceConfig
	enrichCfg  EnrichmentConfig
	logger     *slog.Logger

	// Enrichment callbacks (injected from assistant)
	visionFn     VisionFunc
	transcribeFn TranscribeFunc
	extractDocFn ExtractDocFunc

	// URL validator for SSRF protection
	urlValidator URLValidator
}

// VisionFunc describes an image using vision API.
type VisionFunc func(ctx context.Context, imageData []byte, mimeType string) (string, error)

// TranscribeFunc transcribes audio using Whisper API.
type TranscribeFunc func(ctx context.Context, audioData []byte, filename string) (string, error)

// ExtractDocFunc extracts text from documents.
type ExtractDocFunc func(ctx context.Context, data []byte, mimeType string) (string, error)

// MediaServiceOption configures the media service.
type MediaServiceOption func(*MediaService)

// WithVision sets the vision function.
func WithVision(fn VisionFunc) MediaServiceOption {
	return func(s *MediaService) { s.visionFn = fn }
}

// WithTranscription sets the transcription function.
func WithTranscription(fn TranscribeFunc) MediaServiceOption {
	return func(s *MediaService) { s.transcribeFn = fn }
}

// WithDocumentExtraction sets the document extraction function.
func WithDocumentExtraction(fn ExtractDocFunc) MediaServiceOption {
	return func(s *MediaService) { s.extractDocFn = fn }
}

// WithEnrichmentConfig sets the enrichment configuration.
func WithEnrichmentConfig(cfg EnrichmentConfig) MediaServiceOption {
	return func(s *MediaService) { s.enrichCfg = cfg }
}

// URLValidator validates URLs for security (SSRF protection).
type URLValidator interface {
	IsAllowed(url string) error
}

// WithURLValidator sets the URL validator for SSRF protection.
func WithURLValidator(v URLValidator) MediaServiceOption {
	return func(s *MediaService) { s.urlValidator = v }
}

// NewMediaService creates a new media service.
func NewMediaService(store MediaStore, channelMgr *channels.Manager, cfg ServiceConfig, logger *slog.Logger, opts ...MediaServiceOption) *MediaService {
	if logger == nil {
		logger = slog.Default()
	}

	s := &MediaService{
		store:      store,
		validator:  NewValidator(cfg),
		channelMgr: channelMgr,
		config:     cfg,
		enrichCfg:  DefaultEnrichmentConfig(),
		logger:     logger.With("component", "media-service"),
	}

	for _, opt := range opts {
		opt(s)
	}

	return s
}

// Config returns the service configuration.
func (s *MediaService) Config() ServiceConfig {
	return s.config
}

// Upload processes and stores uploaded media.
func (s *MediaService) Upload(ctx context.Context, req UploadRequest) (*StoredMedia, error) {
	if !s.config.Enabled {
		return nil, errors.New("media service is disabled")
	}

	if len(req.Data) == 0 {
		return nil, errors.New("no data provided")
	}

	// Validate
	result, err := s.validator.Validate(req.Data, req.Filename, req.MimeType)
	if err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	// Parse TTL
	var ttl time.Duration
	if req.Temporary {
		ttl, _ = time.ParseDuration(s.config.TempTTL)
		if ttl == 0 {
			ttl = 24 * time.Hour
		}
	}

	// Store the media
	media, err := s.store.Save(ctx, SaveRequest{
		Data:      req.Data,
		Filename:  req.Filename,
		MimeType:  result.MimeType,
		Type:      result.Type,
		Channel:   req.Channel,
		SessionID: req.SessionID,
		Temporary: req.Temporary,
		TTL:       ttl,
	})
	if err != nil {
		return nil, fmt.Errorf("saving media: %w", err)
	}

	s.logger.Info("media uploaded",
		"id", media.ID,
		"filename", media.Filename,
		"type", media.Type,
		"size", media.Size,
		"channel", req.Channel,
	)

	return media, nil
}

// Get retrieves media by ID.
func (s *MediaService) Get(ctx context.Context, id string) ([]byte, *StoredMedia, error) {
	return s.store.GetBytes(ctx, id)
}

// Delete removes media by ID.
func (s *MediaService) Delete(ctx context.Context, id string) error {
	return s.store.Delete(ctx, id)
}

// List returns media matching the filter.
func (s *MediaService) List(ctx context.Context, filter ListFilter) ([]*StoredMedia, error) {
	return s.store.List(ctx, filter)
}

// URL returns the URL for a media ID.
func (s *MediaService) URL(id string) string {
	return s.store.URL(id)
}

// Enrich extracts content from media.
func (s *MediaService) Enrich(ctx context.Context, mediaID string) (*EnrichmentResult, error) {
	data, media, err := s.store.GetBytes(ctx, mediaID)
	if err != nil {
		return nil, fmt.Errorf("getting media: %w", err)
	}

	result := &EnrichmentResult{
		MediaID:  mediaID,
		Type:     media.Type,
		Metadata: media.Metadata,
	}

	switch media.Type {
	case MediaTypeImage:
		if s.visionFn != nil && s.enrichCfg.AutoEnrichImages {
			desc, err := s.visionFn(ctx, data, media.MimeType)
			if err != nil {
				s.logger.Warn("vision enrichment failed", "id", mediaID, "error", err)
			} else {
				result.Description = desc
			}
		}

	case MediaTypeAudio:
		if s.transcribeFn != nil && s.enrichCfg.AutoEnrichAudio {
			transcript, err := s.transcribeFn(ctx, data, media.Filename)
			if err != nil {
				s.logger.Warn("transcription enrichment failed", "id", mediaID, "error", err)
			} else {
				result.Transcript = transcript
			}
		}

	case MediaTypeDocument:
		if s.extractDocFn != nil && s.enrichCfg.AutoEnrichDocuments {
			text, err := s.extractDocFn(ctx, data, media.MimeType)
			if err != nil {
				s.logger.Warn("document extraction failed", "id", mediaID, "error", err)
			} else {
				result.TextContent = text
			}
		}
	}

	return result, nil
}

// PrepareForLLM prepares media for LLM consumption.
func (s *MediaService) PrepareForLLM(ctx context.Context, mediaID string) (*LLMMediaPrep, error) {
	data, media, err := s.store.GetBytes(ctx, mediaID)
	if err != nil {
		return nil, fmt.Errorf("getting media: %w", err)
	}

	prep := &LLMMediaPrep{
		MediaID:  mediaID,
		Type:     media.Type,
		MimeType: media.MimeType,
		FileSize: media.Size,
	}

	// Only encode images to base64 for vision
	if media.Type == MediaTypeImage {
		prep.Base64Data = base64.StdEncoding.EncodeToString(data)
	}

	return prep, nil
}

// SendToChannel sends media through the channel manager.
func (s *MediaService) SendToChannel(ctx context.Context, channelName, to string, mediaID string, caption string) error {
	if s.channelMgr == nil {
		return errors.New("no channel manager configured")
	}

	data, media, err := s.store.GetBytes(ctx, mediaID)
	if err != nil {
		return fmt.Errorf("getting media: %w", err)
	}

	msg := &channels.MediaMessage{
		Type:     channels.MessageType(media.Type),
		Data:     data,
		MimeType: media.MimeType,
		Filename: media.Filename,
		Caption:  caption,
	}

	if err := s.channelMgr.SendMedia(ctx, channelName, to, msg); err != nil {
		return fmt.Errorf("sending media via channel: %w", err)
	}

	s.logger.Info("media sent",
		"id", mediaID,
		"channel", channelName,
		"to", to,
		"type", media.Type,
	)

	return nil
}

// ResolveMediaSource resolves media from different sources (media_id, file_path, url).
func (s *MediaService) ResolveMediaSource(ctx context.Context, source map[string]any) ([]byte, string, string, error) {
	// Priority: media_id > file_path > url

	if mediaID, ok := source["media_id"].(string); ok && mediaID != "" {
		data, media, err := s.store.GetBytes(ctx, mediaID)
		if err != nil {
			return nil, "", "", fmt.Errorf("media not found: %w", err)
		}
		return data, media.MimeType, media.Filename, nil
	}

	if filePath, ok := source["file_path"].(string); ok && filePath != "" {
		// Security: validate path doesn't escape allowed directories
		if err := validateFilePath(filePath); err != nil {
			return nil, "", "", err
		}

		data, err := os.ReadFile(filePath)
		if err != nil {
			return nil, "", "", fmt.Errorf("reading file: %w", err)
		}

		filename := filepath.Base(filePath)
		mimeType := http.DetectContentType(data)
		return data, mimeType, filename, nil
	}

	if url, ok := source["url"].(string); ok && url != "" {
		// Security: SSRF protection
		if s.urlValidator != nil {
			if err := s.urlValidator.IsAllowed(url); err != nil {
				return nil, "", "", fmt.Errorf("URL validation failed: %w", err)
			}
		}

		data, filename, err := downloadFromURL(ctx, url)
		if err != nil {
			return nil, "", "", fmt.Errorf("downloading from URL: %w", err)
		}

		mimeType := http.DetectContentType(data)
		return data, mimeType, filename, nil
	}

	return nil, "", "", errors.New("no valid media source provided (media_id, file_path, or url required)")
}

// StartCleanup runs periodic cleanup of expired temporary media.
func (s *MediaService) StartCleanup(ctx context.Context) {
	if !s.config.CleanupEnabled {
		return
	}

	interval, err := time.ParseDuration(s.config.CleanupInterval)
	if err != nil || interval == 0 {
		interval = time.Hour
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	s.logger.Info("media cleanup started", "interval", interval.String())

	for {
		select {
		case <-ctx.Done():
			s.logger.Info("media cleanup stopped")
			return
		case <-ticker.C:
			count, err := s.store.DeleteExpired(ctx)
			if err != nil {
				s.logger.Warn("media cleanup error", "error", err)
			} else if count > 0 {
				s.logger.Info("cleaned up expired media", "count", count)
			}
		}
	}
}

// downloadFromURL downloads content from a URL with SSRF protection.
func downloadFromURL(ctx context.Context, url string) ([]byte, string, error) {
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, "", err
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	// Limit response size (50MB max)
	limitedReader := io.LimitReader(resp.Body, 50*1024*1024)
	data, err := io.ReadAll(limitedReader)
	if err != nil {
		return nil, "", err
	}

	// Extract filename from URL or Content-Disposition
	filename := extractFilename(url, resp)

	return data, filename, nil
}

// extractFilename extracts filename from URL or Content-Disposition header.
func extractFilename(url string, resp *http.Response) string {
	// Try Content-Disposition first
	if cd := resp.Header.Get("Content-Disposition"); cd != "" {
		if parts := strings.Split(cd, "filename="); len(parts) > 1 {
			name := strings.Trim(parts[1], `"`)
			if name != "" {
				return name
			}
		}
	}

	// Fall back to URL path
	parts := strings.Split(url, "/")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}

	return "download"
}

// validateFilePath ensures the file path is safe to read.
func validateFilePath(path string) error {
	// Clean the path
	clean := filepath.Clean(path)

	// Check for path traversal
	if strings.Contains(clean, "..") {
		return errors.New("path traversal not allowed")
	}

	// Check for absolute paths that might escape workspace
	// (This is a basic check; production systems should use allowlists)
	if filepath.IsAbs(clean) {
		// Allow specific safe paths
		// For now, just check it's not a sensitive system path
		sensitive := []string{"/etc", "/root", "/home", "/var", "/usr", "/bin", "/sbin"}
		for _, s := range sensitive {
			if strings.HasPrefix(clean, s) {
				return fmt.Errorf("access to %s not allowed", s)
			}
		}
	}

	return nil
}
