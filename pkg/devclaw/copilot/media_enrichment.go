// Package copilot – media_enrichment.go handles extraction of content from
// documents (PDF, DOCX, TXT) and video frames for enriching agent prompts.
package copilot

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// extractDocumentText extracts readable text from a document based on MIME type.
// Supports PDF (via pdftotext), plain text, and common text formats.
// Returns empty string if extraction fails or format is unsupported.
func extractDocumentText(data []byte, mimeType, filename string, logger *slog.Logger) string {
	mime := strings.ToLower(mimeType)
	ext := strings.ToLower(filepath.Ext(filename))

	// Plain text formats — return directly.
	if isPlainText(mime, ext) {
		return string(data)
	}

	// PDF — use pdftotext if available.
	if mime == "application/pdf" || ext == ".pdf" {
		return extractPDFText(data, logger)
	}

	// DOCX — extract from XML inside the zip.
	if mime == "application/vnd.openxmlformats-officedocument.wordprocessingml.document" || ext == ".docx" {
		return extractDOCXText(data, logger)
	}

	logger.Debug("unsupported document format", "mime", mimeType, "ext", ext)
	return ""
}

// isPlainText checks if the MIME type or extension indicates plain text.
func isPlainText(mime, ext string) bool {
	textMimes := []string{
		"text/plain", "text/csv", "text/html", "text/markdown",
		"application/json", "application/xml", "text/xml",
		"application/javascript", "text/css",
	}
	for _, m := range textMimes {
		if mime == m {
			return true
		}
	}
	textExts := []string{
		".txt", ".csv", ".md", ".json", ".xml", ".html", ".htm",
		".js", ".ts", ".py", ".go", ".rs", ".java", ".c", ".cpp",
		".h", ".css", ".yaml", ".yml", ".toml", ".ini", ".cfg",
		".sh", ".bash", ".sql", ".log",
	}
	for _, e := range textExts {
		if ext == e {
			return true
		}
	}
	return false
}

// extractPDFText uses the pdftotext system command to extract text from a PDF.
// Falls back gracefully if pdftotext is not installed.
func extractPDFText(data []byte, logger *slog.Logger) string {
	// Check if pdftotext is available.
	if _, err := exec.LookPath("pdftotext"); err != nil {
		logger.Warn("pdftotext not found — install poppler-utils for PDF support")
		return "[Unable to read PDF: the 'pdftotext' tool is not installed on this server. " +
			"To enable PDF text extraction, the administrator needs to install poppler-utils " +
			"(Ubuntu/Debian: apt install poppler-utils, macOS: brew install poppler). " +
			"You can still describe what you know about this document or ask the user to share its content as text.]"
	}

	// Write to temp file (pdftotext needs a file).
	tmpFile, err := os.CreateTemp("", "devclaw-pdf-*.pdf")
	if err != nil {
		logger.Warn("failed to create temp file for PDF", "error", err)
		return "[Unable to read PDF: temporary file creation failed.]"
	}
	defer os.Remove(tmpFile.Name())
	// Restrict to owner-only: prevent other users on the host from reading
	// potentially sensitive document content from the temp file.
	if err := os.Chmod(tmpFile.Name(), 0o600); err != nil {
		tmpFile.Close()
		logger.Warn("failed to chmod PDF temp file", "error", err)
		return "[Unable to read PDF: file permission error.]"
	}

	if _, err := tmpFile.Write(data); err != nil {
		tmpFile.Close()
		logger.Warn("failed to write PDF temp file", "error", err)
		return "[Unable to read PDF: file write error.]"
	}
	tmpFile.Close()

	// Run pdftotext: input.pdf - (dash = stdout).
	cmd := exec.Command("pdftotext", "-layout", tmpFile.Name(), "-")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		logger.Warn("pdftotext failed", "error", err, "stderr", stderr.String())
		return "[Unable to read PDF: text extraction failed. The file may be corrupted or password-protected.]"
	}

	text := strings.TrimSpace(stdout.String())
	if text == "" {
		logger.Debug("pdftotext returned empty (possibly scanned PDF)")
		return "[This PDF appears to contain no extractable text. It may be a scanned document or image-based PDF. " +
			"If you have vision capabilities enabled, you could try describing individual pages as images.]"
	}
	return text
}

// extractDOCXText extracts text from a DOCX file by reading the XML inside the zip.
// Uses a simple approach: unzip word/document.xml and strip XML tags.
func extractDOCXText(data []byte, logger *slog.Logger) string {
	// Write to temp file.
	tmpFile, err := os.CreateTemp("", "devclaw-docx-*.docx")
	if err != nil {
		logger.Warn("failed to create temp file for DOCX", "error", err)
		return "[Unable to read DOCX: temporary file creation failed.]"
	}
	defer os.Remove(tmpFile.Name())
	// Restrict to owner-only: DOCX may contain confidential content.
	if err := os.Chmod(tmpFile.Name(), 0o600); err != nil {
		tmpFile.Close()
		logger.Warn("failed to chmod DOCX temp file", "error", err)
		return "[Unable to read DOCX: file permission error.]"
	}

	if _, err := tmpFile.Write(data); err != nil {
		tmpFile.Close()
		return "[Unable to read DOCX: file write error.]"
	}
	tmpFile.Close()

	// Extract word/document.xml using unzip.
	cmd := exec.Command("unzip", "-p", tmpFile.Name(), "word/document.xml")
	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	if err := cmd.Run(); err != nil {
		logger.Warn("failed to extract DOCX content", "error", err)
		return "[Unable to read DOCX: the file may be corrupted or not a valid Word document.]"
	}

	// Strip XML tags to get plain text.
	text := stripXMLTags(stdout.String())
	text = strings.TrimSpace(text)
	if text == "" {
		return "[This DOCX appears to contain no extractable text. It may be an image-based document.]"
	}
	return text
}

// stripXMLTags removes all XML/HTML tags from a string, preserving text content.
func stripXMLTags(s string) string {
	var result strings.Builder
	inTag := false
	for _, r := range s {
		if r == '<' {
			inTag = true
			continue
		}
		if r == '>' {
			inTag = false
			result.WriteRune(' ')
			continue
		}
		if !inTag {
			result.WriteRune(r)
		}
	}
	// Collapse multiple spaces.
	text := result.String()
	for strings.Contains(text, "  ") {
		text = strings.ReplaceAll(text, "  ", " ")
	}
	return text
}

// extractVideoFrame extracts the first frame from a video file using ffmpeg,
// then describes it via the Vision API.
func extractVideoFrame(ctx context.Context, data []byte, mimeType string, llm *LLMClient, media MediaConfig, logger *slog.Logger) string {
	// Check if ffmpeg is available.
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		logger.Debug("ffmpeg not found — video enrichment unavailable")
		return ""
	}

	// Write video to temp file.
	ext := ".mp4"
	if strings.Contains(mimeType, "webm") {
		ext = ".webm"
	} else if strings.Contains(mimeType, "3gpp") {
		ext = ".3gp"
	}
	tmpVideo, err := os.CreateTemp("", "devclaw-video-*"+ext)
	if err != nil {
		return ""
	}
	defer os.Remove(tmpVideo.Name())
	// Restrict to owner-only: video data may be sensitive.
	if err := os.Chmod(tmpVideo.Name(), 0o600); err != nil {
		tmpVideo.Close()
		return ""
	}

	if _, err := tmpVideo.Write(data); err != nil {
		tmpVideo.Close()
		return ""
	}
	tmpVideo.Close()

	// Create a temp file for the extracted frame before calling ffmpeg.
	// Using os.CreateTemp avoids a predictable path and lets us do an
	// inode-verification read to guard against TOCTOU replacement.
	tmpFrameFile, err := os.CreateTemp("", "devclaw-frame-*.jpg")
	if err != nil {
		return ""
	}
	tmpFramePath := tmpFrameFile.Name()
	defer os.Remove(tmpFramePath)
	// Restrict before ffmpeg writes into it.
	if err := os.Chmod(tmpFramePath, 0o600); err != nil {
		tmpFrameFile.Close()
		return ""
	}
	// Capture the file info of the pre-created file so we can verify it hasn't
	// been replaced between ffmpeg finishing and our read (TOCTOU guard).
	preStat, err := os.Stat(tmpFramePath)
	if err != nil {
		tmpFrameFile.Close()
		return ""
	}
	tmpFrameFile.Close()

	cmd := exec.Command("ffmpeg",
		"-i", tmpVideo.Name(),
		"-vframes", "1",
		"-q:v", "2",
		"-y",
		tmpFramePath,
	)
	cmd.Stderr = nil
	cmd.Stdout = nil

	if err := cmd.Run(); err != nil {
		logger.Warn("ffmpeg frame extraction failed", "error", err)
		return ""
	}

	// TOCTOU guard: verify the file on disk is still the same inode that
	// we created, not a symlink or replacement injected by another process.
	postStat, err := os.Stat(tmpFramePath)
	if err != nil {
		logger.Warn("frame temp file disappeared after ffmpeg", "error", err)
		return ""
	}
	if !os.SameFile(preStat, postStat) {
		logger.Warn("frame temp file changed — possible TOCTOU attack, aborting")
		return ""
	}

	frameData, err := os.ReadFile(tmpFramePath)
	if err != nil {
		return ""
	}

	// Send to Vision API.
	imgBase64 := base64.StdEncoding.EncodeToString(frameData)
	detail := media.VisionDetail
	if detail == "" {
		detail = "auto"
	}

	desc, err := llm.CompleteWithVision(ctx, "",
		imgBase64, "image/jpeg",
		"This is a frame from a video the user sent. Describe what you see in the video. Include any text visible.",
		detail, media.VisionModel,
	)
	if err != nil {
		logger.Warn("video frame vision failed", "error", err)
		return ""
	}

	return fmt.Sprintf("%s", desc)
}
