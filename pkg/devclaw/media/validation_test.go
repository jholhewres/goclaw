package media

import (
	"testing"
)

func TestValidator_ValidImage(t *testing.T) {
	cfg := DefaultServiceConfig()
	validator := NewValidator(cfg)

	// Create a minimal valid PNG
	pngHeader := []byte{
		0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, // PNG signature
	}

	result, err := validator.Validate(pngHeader, "test.png", "image/png")
	if err != nil {
		t.Errorf("Validate() error = %v", err)
		return
	}
	if result.Type != MediaTypeImage {
		t.Errorf("Validate() type = %v, want %v", result.Type, MediaTypeImage)
	}
	if result.MimeType != "image/png" {
		t.Errorf("Validate() mimeType = %v, want image/png", result.MimeType)
	}
}

func TestValidator_ValidJPEG(t *testing.T) {
	cfg := DefaultServiceConfig()
	validator := NewValidator(cfg)

	// JPEG SOI marker
	jpegHeader := []byte{0xFF, 0xD8, 0xFF, 0xE0}

	result, err := validator.Validate(jpegHeader, "test.jpg", "image/jpeg")
	if err != nil {
		t.Errorf("Validate() error = %v", err)
		return
	}
	if result.Type != MediaTypeImage {
		t.Errorf("Validate() type = %v, want %v", result.Type, MediaTypeImage)
	}
}

func TestValidator_ValidAudio(t *testing.T) {
	cfg := DefaultServiceConfig()
	validator := NewValidator(cfg)

	// Generic audio data (won't match magic bytes but extension will work)
	audioData := []byte("ID3") // MP3 ID3 tag header

	result, err := validator.Validate(audioData, "test.mp3", "audio/mpeg")
	if err != nil {
		t.Errorf("Validate() error = %v", err)
		return
	}
	if result.Type != MediaTypeAudio {
		t.Errorf("Validate() type = %v, want %v", result.Type, MediaTypeAudio)
	}
}

func TestValidator_ValidDocument(t *testing.T) {
	cfg := DefaultServiceConfig()
	validator := NewValidator(cfg)

	// PDF header
	pdfHeader := []byte("%PDF-1.4")

	result, err := validator.Validate(pdfHeader, "test.pdf", "application/pdf")
	if err != nil {
		t.Errorf("Validate() error = %v", err)
		return
	}
	if result.Type != MediaTypeDocument {
		t.Errorf("Validate() type = %v, want %v", result.Type, MediaTypeDocument)
	}
}

func TestValidator_RejectOversized(t *testing.T) {
	cfg := ServiceConfig{
		MaxImageSize: 10, // Very small
	}
	validator := NewValidator(cfg)

	data := make([]byte, 100) // Larger than MaxImageSize
	_, err := validator.Validate(data, "test.png", "image/png")
	if err == nil {
		t.Error("Validate() should reject oversized images")
	}
}

func TestValidator_RejectInvalidMime(t *testing.T) {
	cfg := DefaultServiceConfig()
	validator := NewValidator(cfg)

	_, err := validator.Validate([]byte("test"), "test.exe", "application/x-msdownload")
	if err == nil {
		t.Error("Validate() should reject unsupported MIME types")
	}
}

func TestDetectMimeType_JPEG(t *testing.T) {
	jpegHeader := []byte{0xFF, 0xD8, 0xFF, 0xE0}
	mime := DetectMimeType(jpegHeader, "test.jpg")
	if mime != "image/jpeg" {
		t.Errorf("DetectMimeType() = %v, want image/jpeg", mime)
	}
}

func TestDetectMimeType_PNG(t *testing.T) {
	pngHeader := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}
	mime := DetectMimeType(pngHeader, "test.png")
	if mime != "image/png" {
		t.Errorf("DetectMimeType() = %v, want image/png", mime)
	}
}

func TestDetectMimeType_PDF(t *testing.T) {
	pdfHeader := []byte("%PDF-1.4")
	mime := DetectMimeType(pdfHeader, "test.pdf")
	if mime != "application/pdf" {
		t.Errorf("DetectMimeType() = %v, want application/pdf", mime)
	}
}

func TestDetectMimeType_Unknown(t *testing.T) {
	unknownData := []byte("unknown data format")
	mime := DetectMimeType(unknownData, "test.unknown")
	// http.DetectContentType returns "text/plain; charset=utf-8" for plain text
	// The function should handle this case
	if mime == "" {
		t.Error("DetectMimeType() returned empty string")
	}
}

func TestCategorizeType(t *testing.T) {
	tests := []struct {
		mimeType string
		want     MediaType
	}{
		{"image/jpeg", MediaTypeImage},
		{"image/png", MediaTypeImage},
		{"image/gif", MediaTypeImage},
		{"image/webp", MediaTypeImage},
		{"audio/mpeg", MediaTypeAudio},
		{"audio/ogg", MediaTypeAudio},
		{"audio/wav", MediaTypeAudio},
		{"application/pdf", MediaTypeDocument},
		{"text/plain", MediaTypeDocument},
		{"video/mp4", MediaTypeDocument}, // Video not supported, fallback to document
		{"unknown/type", MediaTypeDocument},
	}

	for _, tt := range tests {
		t.Run(tt.mimeType, func(t *testing.T) {
			got := CategorizeType(tt.mimeType)
			if got != tt.want {
				t.Errorf("CategorizeType(%v) = %v, want %v", tt.mimeType, got, tt.want)
			}
		})
	}
}
