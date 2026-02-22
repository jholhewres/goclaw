package media

import (
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
)

// AllowedMimeTypes defines permitted MIME types for each media category.
var AllowedMimeTypes = map[MediaType][]string{
	MediaTypeImage: {
		"image/jpeg",
		"image/png",
		"image/gif",
		"image/webp",
	},
	MediaTypeAudio: {
		"audio/mpeg",
		"audio/mp3",
		"audio/ogg",
		"audio/wav",
		"audio/x-wav",
		"audio/webm",
		"audio/mp4",
		"audio/x-m4a",
		"video/ogg", // OGG can be audio or video
	},
	MediaTypeDocument: {
		"application/pdf",
		"application/vnd.openxmlformats-officedocument.wordprocessingml.document",
		"text/plain",
		"text/csv",
		"application/json",
		"text/markdown",
	},
}

// ServiceConfig contains validation limits.
type ServiceConfig struct {
	Enabled         bool   `yaml:"enabled" json:"enabled"`
	MaxImageSize    int64  `yaml:"max_image_size" json:"max_image_size"`
	MaxAudioSize    int64  `yaml:"max_audio_size" json:"max_audio_size"`
	MaxDocSize      int64  `yaml:"max_doc_size" json:"max_doc_size"`
	MaxUploadSize   int64  `yaml:"max_upload_size" json:"max_upload_size"`
	TempTTL         string `yaml:"temp_ttl" json:"temp_ttl"`
	CleanupEnabled  bool   `yaml:"cleanup_enabled" json:"cleanup_enabled"`
	CleanupInterval string `yaml:"cleanup_interval" json:"cleanup_interval"`
}

// DefaultServiceConfig returns default configuration.
func DefaultServiceConfig() ServiceConfig {
	return ServiceConfig{
		Enabled:         true,
		MaxImageSize:    20 * 1024 * 1024, // 20MB
		MaxAudioSize:    25 * 1024 * 1024, // 25MB (Whisper limit)
		MaxDocSize:      50 * 1024 * 1024, // 50MB
		MaxUploadSize:   50 * 1024 * 1024, // 50MB
		TempTTL:         "24h",
		CleanupEnabled:  true,
		CleanupInterval: "1h",
	}
}

// MaxSizeForType returns the maximum size for a media type.
func (c ServiceConfig) MaxSizeForType(t MediaType) int64 {
	switch t {
	case MediaTypeImage:
		return c.MaxImageSize
	case MediaTypeAudio:
		return c.MaxAudioSize
	case MediaTypeDocument:
		return c.MaxDocSize
	default:
		return c.MaxUploadSize
	}
}

// ValidationResult contains validation output.
type ValidationResult struct {
	MimeType string
	Type     MediaType
	Size     int64
	Valid    bool
	Error    string
}

// Validator handles media validation.
type Validator struct {
	config ServiceConfig
}

// NewValidator creates a new validator.
func NewValidator(config ServiceConfig) *Validator {
	return &Validator{config: config}
}

// Validate checks MIME type, size, and content.
func (v *Validator) Validate(data []byte, filename, mimeType string) (*ValidationResult, error) {
	result := &ValidationResult{
		Size: int64(len(data)),
	}

	// Detect MIME type from content if not provided
	if mimeType == "" {
		mimeType = DetectMimeType(data, filename)
	}
	result.MimeType = mimeType

	// Categorize media type
	result.Type = CategorizeType(mimeType)

	// Validate MIME type is allowed
	if !v.isAllowedMime(mimeType) {
		result.Error = fmt.Sprintf("MIME type %s is not allowed", mimeType)
		return result, errors.New(result.Error)
	}

	// Validate size
	maxSize := v.config.MaxSizeForType(result.Type)
	if maxSize > 0 && result.Size > maxSize {
		result.Error = fmt.Sprintf("file size %d exceeds maximum %d for type %s", result.Size, maxSize, result.Type)
		return result, errors.New(result.Error)
	}

	result.Valid = true
	return result, nil
}

// isAllowedMime checks if a MIME type is in the allowed list.
func (v *Validator) isAllowedMime(mimeType string) bool {
	for _, allowed := range AllowedMimeTypes {
		for _, m := range allowed {
			if m == mimeType {
				return true
			}
		}
	}
	return false
}

// DetectMimeType uses http.DetectContentType and extension heuristics.
func DetectMimeType(data []byte, filename string) string {
	// Use http.DetectContentType for first 512 bytes
	detected := http.DetectContentType(data)

	// http.DetectContentType returns "application/octet-stream" for unknown
	// Try to use extension heuristics in that case
	if detected == "application/octet-stream" {
		ext := strings.ToLower(filepath.Ext(filename))
		switch ext {
		case ".mp3":
			return "audio/mpeg"
		case ".m4a":
			return "audio/mp4"
		case ".ogg":
			return "audio/ogg"
		case ".wav":
			return "audio/wav"
		case ".weba":
			return "audio/webm"
		case ".pdf":
			return "application/pdf"
		case ".docx":
			return "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
		case ".txt", ".md":
			return "text/plain"
		case ".json":
			return "application/json"
		case ".csv":
			return "text/csv"
		}
	}

	// For text/plain, check if it might be something more specific
	if strings.HasPrefix(detected, "text/plain") {
		ext := strings.ToLower(filepath.Ext(filename))
		switch ext {
		case ".json":
			return "application/json"
		case ".csv":
			return "text/csv"
		case ".md":
			return "text/markdown"
		}
	}

	return detected
}

// CategorizeType maps MIME type to MediaType.
func CategorizeType(mimeType string) MediaType {
	// Remove parameters (e.g., "image/jpeg; charset=utf-8")
	mimeType = strings.Split(mimeType, ";")[0]
	mimeType = strings.TrimSpace(mimeType)

	switch {
	case strings.HasPrefix(mimeType, "image/"):
		return MediaTypeImage
	case strings.HasPrefix(mimeType, "audio/"):
		return MediaTypeAudio
	case mimeType == "video/ogg":
		// OGG can be audio
		return MediaTypeAudio
	case strings.HasPrefix(mimeType, "video/"):
		// Video support for future
		return MediaTypeDocument // Fallback for now
	case mimeType == "application/pdf":
		return MediaTypeDocument
	case strings.HasPrefix(mimeType, "application/vnd.openxmlformats"):
		return MediaTypeDocument
	case strings.HasPrefix(mimeType, "text/"):
		return MediaTypeDocument
	case mimeType == "application/json":
		return MediaTypeDocument
	default:
		return MediaTypeDocument
	}
}

// IsImage returns true if the MIME type is an image.
func IsImage(mimeType string) bool {
	return CategorizeType(mimeType) == MediaTypeImage
}

// IsAudio returns true if the MIME type is audio.
func IsAudio(mimeType string) bool {
	return CategorizeType(mimeType) == MediaTypeAudio
}

// IsDocument returns true if the MIME type is a document.
func IsDocument(mimeType string) bool {
	return CategorizeType(mimeType) == MediaTypeDocument
}

// AllowedExtensions returns file extensions for allowed types.
func AllowedExtensions() []string {
	return []string{
		".jpg", ".jpeg", ".png", ".gif", ".webp",
		".mp3", ".m4a", ".ogg", ".wav", ".weba",
		".pdf", ".docx", ".txt", ".csv", ".json", ".md",
	}
}

// IsAllowedExtension checks if a file extension is allowed.
func IsAllowedExtension(ext string) bool {
	ext = strings.ToLower(ext)
	if !strings.HasPrefix(ext, ".") {
		ext = "." + ext
	}
	for _, allowed := range AllowedExtensions() {
		if ext == allowed {
			return true
		}
	}
	return false
}
