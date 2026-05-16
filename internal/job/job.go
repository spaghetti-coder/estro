// Package job provides an in-memory store for asynchronous command execution jobs.
package job

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"time"
)

// GenerateID creates a cryptographically random 32-character hex string
// suitable for use as a job identifier.
func GenerateID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Sprintf("failed to generate job ID: %v", err))
	}
	return hex.EncodeToString(b)
}

// Job represents the state and output of an asynchronous command execution.
type Job struct {
	Status string `json:"status"`
	Title  string `json:"title"`
	Stdout string `json:"stdout,omitempty"`
	Stderr string `json:"stderr,omitempty"`
}

// Store is a thread-safe in-memory map of job IDs to Job values.
type Store struct {
	mu   sync.RWMutex
	jobs map[string]*Job
}

// NewStore creates and returns an empty job store.
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

// Get retrieves a job by ID. Returns the job and true if found, nil and false otherwise.
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

// ScheduleCleanup removes the job with the given ID after the specified TTL.
func (s *Store) ScheduleCleanup(id string, ttl time.Duration) {
	time.AfterFunc(ttl, func() {
		s.Delete(id)
	})
}

// MarkAllRunningAsError transitions all jobs with status "running" to "error"
// and sets their Stderr to the provided message.
func (s *Store) MarkAllRunningAsError(msg string) {
	s.mu.Lock()
	for _, job := range s.jobs {
		if job.Status == "running" {
			job.Status = "error"
			job.Stderr = msg
		}
	}
	s.mu.Unlock()
}
