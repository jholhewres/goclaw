// Package webui – media_handlers.go provides HTTP handlers for media upload/download.
package webui

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

// MediaAPI defines the interface for media operations.
// Implemented by the media service adapter in the copilot package.
type MediaAPI interface {
	// Upload stores media and returns its ID, type, and URL.
	Upload(r *http.Request, sessionID string) (mediaID string, mediaType string, filename string, size int64, err error)

	// Get retrieves media by ID.
	Get(mediaID string) ([]byte, string, string, error) // data, mimeType, filename, error

	// List returns media matching the filter.
	List(sessionID string, mediaType string, limit int) ([]MediaInfo, error)

	// Delete removes media by ID.
	Delete(mediaID string) error
}

// MediaInfo contains media metadata for the UI.
type MediaInfo struct {
	ID        string `json:"id"`
	Filename  string `json:"filename"`
	Type      string `json:"type"`
	Size      int64  `json:"size"`
	URL       string `json:"url"`
	CreatedAt string `json:"created_at"`
}

// handleAPIMedia handles POST /api/media (upload) and GET /api/media (list).
func (s *Server) handleAPIMedia(w http.ResponseWriter, r *http.Request) {
	if s.mediaAPI == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": "media service not available",
		})
		return
	}

	// Get session ID from query parameter
	sessionID := r.URL.Query().Get("session_id")

	switch r.Method {
	case http.MethodPost:
		s.handleMediaUpload(w, r, sessionID)
	case http.MethodGet:
		s.handleMediaList(w, r, sessionID)
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{
			"error": "method not allowed",
		})
	}
}

// handleAPIMediaByID handles GET /api/media/{id} (download) and DELETE /api/media/{id}.
func (s *Server) handleAPIMediaByID(w http.ResponseWriter, r *http.Request) {
	if s.mediaAPI == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": "media service not available",
		})
		return
	}

	// Extract media ID from path
	path := strings.TrimPrefix(r.URL.Path, "/api/media/")
	mediaID := strings.Split(path, "/")[0]
	if mediaID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "media ID required",
		})
		return
	}

	switch r.Method {
	case http.MethodGet:
		s.handleMediaGet(w, r, mediaID)
	case http.MethodDelete:
		s.handleMediaDelete(w, r, mediaID)
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{
			"error": "method not allowed",
		})
	}
}

// handleMediaUpload handles POST /api/media (multipart upload).
func (s *Server) handleMediaUpload(w http.ResponseWriter, r *http.Request, sessionID string) {
	// Limit request body size (50MB default)
	maxSize := int64(50 * 1024 * 1024)
	r.Body = http.MaxBytesReader(w, r.Body, maxSize)

	// Parse multipart form
	if err := r.ParseMultipartForm(maxSize); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": fmt.Sprintf("failed to parse form: %v", err),
		})
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "no file provided (use 'file' field)",
		})
		return
	}
	defer file.Close()

	// Get session ID from form or path
	uploadSessionID := r.FormValue("session_id")
	if uploadSessionID == "" {
		uploadSessionID = sessionID
	}

	// Upload via MediaAPI
	mediaID, mediaType, filename, size, err := s.mediaAPI.Upload(r, uploadSessionID)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": err.Error(),
		})
		return
	}

	// Use original filename if not returned
	if filename == "" {
		filename = header.Filename
	}

	s.logger.Info("media uploaded",
		"media_id", mediaID,
		"filename", filename,
		"type", mediaType,
		"size", size,
		"session", uploadSessionID,
	)

	writeJSON(w, http.StatusCreated, map[string]any{
		"id":       mediaID,
		"filename": filename,
		"type":     mediaType,
		"size":     size,
		"url":      fmt.Sprintf("/api/media/%s", mediaID),
	})
}

// handleMediaGet handles GET /api/media/{id} (download).
func (s *Server) handleMediaGet(w http.ResponseWriter, r *http.Request, mediaID string) {
	data, mimeType, filename, err := s.mediaAPI.Get(mediaID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{
			"error": fmt.Sprintf("media not found: %v", err),
		})
		return
	}

	// Set response headers
	w.Header().Set("Content-Type", mimeType)
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))

	// Write data
	if _, err := w.Write(data); err != nil {
		s.logger.Warn("failed to write media response", "error", err)
	}
}

// handleMediaList handles GET /api/media (list).
func (s *Server) handleMediaList(w http.ResponseWriter, r *http.Request, sessionID string) {
	// Parse query parameters
	querySessionID := r.URL.Query().Get("session_id")
	if querySessionID == "" {
		querySessionID = sessionID
	}
	mediaType := r.URL.Query().Get("type")
	limitStr := r.URL.Query().Get("limit")

	limit := 50
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}

	// List via MediaAPI
	medias, err := s.mediaAPI.List(querySessionID, mediaType, limit)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"items": medias,
		"count": len(medias),
	})
}

// handleMediaDelete handles DELETE /api/media/{id}.
func (s *Server) handleMediaDelete(w http.ResponseWriter, r *http.Request, mediaID string) {
	if err := s.mediaAPI.Delete(mediaID); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{
			"error": fmt.Sprintf("failed to delete: %v", err),
		})
		return
	}

	s.logger.Info("media deleted", "media_id", mediaID)
	writeJSON(w, http.StatusOK, map[string]string{
		"status": "deleted",
	})
}
