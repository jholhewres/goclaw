// Package scheduler – storage.go provides a JSON file-based JobStorage
// implementation that persists scheduler jobs to disk.
package scheduler

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// FileJobStorage persists jobs as a JSON file on disk.
type FileJobStorage struct {
	path string
	mu   sync.Mutex
}

// NewFileJobStorage creates a file-based job storage at the given path.
// Creates the parent directory if it doesn't exist.
func NewFileJobStorage(path string) (*FileJobStorage, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("creating storage directory: %w", err)
	}

	return &FileJobStorage{path: path}, nil
}

// Save persists a job to the storage file.
func (s *FileJobStorage) Save(job *Job) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	jobs, err := s.readAll()
	if err != nil {
		jobs = make(map[string]*Job)
	}

	jobs[job.ID] = job
	return s.writeAll(jobs)
}

// Delete removes a job from the storage file.
func (s *FileJobStorage) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	jobs, err := s.readAll()
	if err != nil {
		return nil // Nothing to delete.
	}

	delete(jobs, id)
	return s.writeAll(jobs)
}

// LoadAll reads all persisted jobs from the storage file.
func (s *FileJobStorage) LoadAll() ([]*Job, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	jobs, err := s.readAll()
	if err != nil {
		return nil, nil // Empty is OK.
	}

	result := make([]*Job, 0, len(jobs))
	for _, j := range jobs {
		result = append(result, j)
	}
	return result, nil
}

// readAll reads all jobs from the file (caller must hold mu).
func (s *FileJobStorage) readAll() (map[string]*Job, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return make(map[string]*Job), nil
		}
		return nil, err
	}

	var jobs map[string]*Job
	if err := json.Unmarshal(data, &jobs); err != nil {
		return nil, fmt.Errorf("parsing jobs file: %w", err)
	}

	return jobs, nil
}

// writeAll writes all jobs to the file (caller must hold mu).
func (s *FileJobStorage) writeAll(jobs map[string]*Job) error {
	data, err := json.MarshalIndent(jobs, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling jobs: %w", err)
	}

	return os.WriteFile(s.path, data, 0o600)
}
