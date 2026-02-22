package media

import (
	"context"
	"io"
	"path/filepath"
	"testing"
	"time"
)

func TestFileSystemStore_Save(t *testing.T) {
	// Create temp directory
	tmpDir := t.TempDir()
	baseDir := filepath.Join(tmpDir, "media")
	tempDir := filepath.Join(tmpDir, "media", "temp")

	cfg := StoreConfig{
		BaseDir:     baseDir,
		TempDir:     tempDir,
		MaxFileSize: 10 * 1024 * 1024, // 10MB
	}
	store := NewFileSystemStore(cfg, nil)

	ctx := context.Background()

	tests := []struct {
		name    string
		req     SaveRequest
		wantErr bool
	}{
		{
			name: "save image",
			req: SaveRequest{
				Data:     []byte("fake image data"),
				Filename: "test.png",
				MimeType: "image/png",
				Type:     MediaTypeImage,
				Channel:  "ui",
			},
			wantErr: false,
		},
		{
			name: "save audio",
			req: SaveRequest{
				Data:     []byte("fake audio data"),
				Filename: "test.mp3",
				MimeType: "audio/mpeg",
				Type:     MediaTypeAudio,
				Channel:  "whatsapp",
			},
			wantErr: false,
		},
		{
			name: "save document",
			req: SaveRequest{
				Data:     []byte("fake document data"),
				Filename: "test.pdf",
				MimeType: "application/pdf",
				Type:     MediaTypeDocument,
				Channel:  "telegram",
			},
			wantErr: false,
		},
		{
			name: "save temporary file",
			req: SaveRequest{
				Data:      []byte("temporary data"),
				Filename:  "temp.png",
				MimeType:  "image/png",
				Type:      MediaTypeImage,
				Temporary: true,
				TTL:       time.Hour,
			},
			wantErr: false,
		},
		{
			name: "empty data should fail",
			req: SaveRequest{
				Data:     []byte{},
				Filename: "empty.png",
				MimeType: "image/png",
				Type:     MediaTypeImage,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			media, err := store.Save(ctx, tt.req)
			if (err != nil) != tt.wantErr {
				t.Errorf("Save() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if media.ID == "" {
					t.Error("Save() returned empty ID")
				}
				if media.Filename != tt.req.Filename {
					t.Errorf("Save() filename = %v, want %v", media.Filename, tt.req.Filename)
				}
				if media.Type != tt.req.Type {
					t.Errorf("Save() type = %v, want %v", media.Type, tt.req.Type)
				}
			}
		})
	}
}

func TestFileSystemStore_Get(t *testing.T) {
	tmpDir := t.TempDir()
	baseDir := filepath.Join(tmpDir, "media")
	tempDir := filepath.Join(tmpDir, "media", "temp")

	cfg := StoreConfig{
		BaseDir:     baseDir,
		TempDir:     tempDir,
		MaxFileSize: 10 * 1024 * 1024,
	}
	store := NewFileSystemStore(cfg, nil)

	ctx := context.Background()

	// Save a file first
	media, err := store.Save(ctx, SaveRequest{
		Data:     []byte("test data for get"),
		Filename: "get-test.png",
		MimeType: "image/png",
		Type:     MediaTypeImage,
	})
	if err != nil {
		t.Fatalf("Setup failed: %v", err)
	}

	// Test Get
	reader, storedMedia, err := store.Get(ctx, media.ID)
	if err != nil {
		t.Errorf("Get() error = %v", err)
		return
	}
	defer reader.Close()
	data, err := io.ReadAll(reader)
	if err != nil {
		t.Errorf("Failed to read data: %v", err)
		return
	}
	if string(data) != "test data for get" {
		t.Errorf("Get() data mismatch")
	}
	if storedMedia.ID != media.ID {
		t.Errorf("Get() media ID mismatch")
	}

	// Test GetBytes
	bytes, storedMedia2, err := store.GetBytes(ctx, media.ID)
	if err != nil {
		t.Errorf("GetBytes() error = %v", err)
		return
	}
	if string(bytes) != "test data for get" {
		t.Errorf("GetBytes() data mismatch")
	}
	if storedMedia2.ID != media.ID {
		t.Errorf("GetBytes() media ID mismatch")
	}
}

func TestFileSystemStore_Delete(t *testing.T) {
	tmpDir := t.TempDir()
	baseDir := filepath.Join(tmpDir, "media")
	tempDir := filepath.Join(tmpDir, "media", "temp")

	cfg := StoreConfig{
		BaseDir:     baseDir,
		TempDir:     tempDir,
		MaxFileSize: 10 * 1024 * 1024,
	}
	store := NewFileSystemStore(cfg, nil)

	ctx := context.Background()

	// Save a file
	media, err := store.Save(ctx, SaveRequest{
		Data:     []byte("test data for delete"),
		Filename: "delete-test.png",
		MimeType: "image/png",
		Type:     MediaTypeImage,
	})
	if err != nil {
		t.Fatalf("Setup failed: %v", err)
	}

	// Delete it
	err = store.Delete(ctx, media.ID)
	if err != nil {
		t.Errorf("Delete() error = %v", err)
		return
	}

	// Verify it's gone
	_, _, err = store.Get(ctx, media.ID)
	if err == nil {
		t.Error("Get() should fail after Delete()")
	}
}

func TestFileSystemStore_List(t *testing.T) {
	tmpDir := t.TempDir()
	baseDir := filepath.Join(tmpDir, "media")
	tempDir := filepath.Join(tmpDir, "media", "temp")

	cfg := StoreConfig{
		BaseDir:     baseDir,
		TempDir:     tempDir,
		MaxFileSize: 10 * 1024 * 1024,
	}
	store := NewFileSystemStore(cfg, nil)

	ctx := context.Background()

	// Save multiple files
	for i := 0; i < 5; i++ {
		_, err := store.Save(ctx, SaveRequest{
			Data:      []byte("test data"),
			Filename:  "test.png",
			MimeType:  "image/png",
			Type:      MediaTypeImage,
			SessionID: "session1",
		})
		if err != nil {
			t.Fatalf("Setup failed: %v", err)
		}
	}

	// Test List
	medias, err := store.List(ctx, ListFilter{
		SessionID: "session1",
		Type:      MediaTypeImage,
		Limit:     10,
	})
	if err != nil {
		t.Errorf("List() error = %v", err)
		return
	}
	if len(medias) != 5 {
		t.Errorf("List() returned %d items, want 5", len(medias))
	}
}

func TestFileSystemStore_DeleteExpired(t *testing.T) {
	tmpDir := t.TempDir()
	baseDir := filepath.Join(tmpDir, "media")
	tempDir := filepath.Join(tmpDir, "media", "temp")

	cfg := StoreConfig{
		BaseDir:     baseDir,
		TempDir:     tempDir,
		MaxFileSize: 10 * 1024 * 1024,
	}
	store := NewFileSystemStore(cfg, nil)

	// Ensure directories exist
	if err := store.EnsureDir(); err != nil {
		t.Fatalf("Failed to create directories: %v", err)
	}

	ctx := context.Background()

	// Save an expired temporary file
	// Set expiration to the past
	expiredTime := time.Now().Add(-2 * time.Hour)
	expiredMedia, err := store.Save(ctx, SaveRequest{
		Data:      []byte("expired data"),
		Filename:  "expired.png",
		MimeType:  "image/png",
		Type:      MediaTypeImage,
		Temporary: true,
	})
	if err != nil {
		t.Fatalf("Setup failed: %v", err)
	}
	// Manually update the expiration time in metadata
	expiredMedia.ExpiresAt = &expiredTime
	store.mu.Lock()
	store.metaCache[expiredMedia.ID] = expiredMedia
	store.mu.Unlock()

	// Save a non-expired file
	_, err = store.Save(ctx, SaveRequest{
		Data:      []byte("valid data"),
		Filename:  "valid.png",
		MimeType:  "image/png",
		Type:      MediaTypeImage,
		Temporary: true,
		TTL:       time.Hour,
	})
	if err != nil {
		t.Fatalf("Setup failed: %v", err)
	}

	// Delete expired
	count, err := store.DeleteExpired(ctx)
	if err != nil {
		t.Errorf("DeleteExpired() error = %v", err)
		return
	}
	if count != 1 {
		t.Errorf("DeleteExpired() deleted %d items, want 1", count)
	}
}

func TestFileSystemStore_URL(t *testing.T) {
	cfg := StoreConfig{
		BaseDir: "./data/media",
		BaseURL: "/api/media",
	}
	store := NewFileSystemStore(cfg, nil)

	url := store.URL("test-id")
	expected := "/api/media/test-id"
	if url != expected {
		t.Errorf("URL() = %v, want %v", url, expected)
	}
}

func TestFileSystemStore_MaxFileSize(t *testing.T) {
	tmpDir := t.TempDir()
	baseDir := filepath.Join(tmpDir, "media")
	tempDir := filepath.Join(tmpDir, "media", "temp")

	cfg := StoreConfig{
		BaseDir:     baseDir,
		TempDir:     tempDir,
		MaxFileSize: 10, // Very small limit
	}
	store := NewFileSystemStore(cfg, nil)

	ctx := context.Background()

	// Try to save a file that's too large
	_, err := store.Save(ctx, SaveRequest{
		Data:     []byte("this is more than 10 bytes"),
		Filename: "large.png",
		MimeType: "image/png",
		Type:     MediaTypeImage,
	})
	if err == nil {
		t.Error("Save() should fail for oversized files")
	}
}

func TestFileSystemStore_ConcurrentAccess(t *testing.T) {
	tmpDir := t.TempDir()
	baseDir := filepath.Join(tmpDir, "media")
	tempDir := filepath.Join(tmpDir, "media", "temp")

	cfg := StoreConfig{
		BaseDir:     baseDir,
		TempDir:     tempDir,
		MaxFileSize: 10 * 1024 * 1024,
	}
	store := NewFileSystemStore(cfg, nil)

	ctx := context.Background()

	// Concurrent saves
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func(n int) {
			_, err := store.Save(ctx, SaveRequest{
				Data:     []byte("concurrent data"),
				Filename: "concurrent.png",
				MimeType: "image/png",
				Type:     MediaTypeImage,
			})
			if err != nil {
				t.Errorf("Concurrent save %d failed: %v", n, err)
			}
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}
}
