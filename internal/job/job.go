// Package job provides an in-memory store for async execution jobs.
package job

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"time"
)

// Job statuses.
const (
	StatusRunning = "running"
	StatusDone    = "done"
	StatusError   = "error"
)

// Job holds status and output of async execution.
type Job struct {
	Status string `json:"status"`
	Title  string `json:"title"`
	Stdout string `json:"stdout,omitempty"`
	Stderr string `json:"stderr,omitempty"`
}

// Store is a thread-safe map of jobs.
type Store struct {
	mu   sync.RWMutex
	jobs map[string]*Job
}

// GenerateID returns a random 32-char hex ID.
func GenerateID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate job ID: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// NewStore returns an empty job store.
func NewStore() *Store {
	return &Store{
		jobs: make(map[string]*Job),
	}
}

// Set stores a job under the given ID.
func (s *Store) Set(id string, job *Job) {
	s.mu.Lock()
	s.jobs[id] = job
	s.mu.Unlock()
}

// Get retrieves job by ID.
func (s *Store) Get(id string) (*Job, bool) {
	s.mu.RLock()
	j, ok := s.jobs[id]
	s.mu.RUnlock()
	return j, ok
}

// Delete removes a job from the store.
func (s *Store) Delete(id string) {
	s.mu.Lock()
	delete(s.jobs, id)
	s.mu.Unlock()
}

// ScheduleCleanup deletes job after TTL.
func (s *Store) ScheduleCleanup(id string, ttl time.Duration) {
	time.AfterFunc(ttl, func() {
		s.Delete(id)
	})
}

// MarkAllRunningAsError marks running jobs as error with msg.
func (s *Store) MarkAllRunningAsError(msg string) {
	s.mu.Lock()
	for _, job := range s.jobs {
		if job.Status == StatusRunning {
			job.Status = StatusError
			job.Stderr = msg
		}
	}
	s.mu.Unlock()
}
