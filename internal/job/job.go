package job

import (
	"sync"
	"time"
)

type Job struct {
	Status string `json:"status"`
	Title  string `json:"title"`
	Stdout string `json:"stdout,omitempty"`
	Stderr string `json:"stderr,omitempty"`
}

type Store struct {
	mu   sync.RWMutex
	jobs map[string]*Job
}

func NewStore() *Store {
	return &Store{
		jobs: make(map[string]*Job),
	}
}

func (s *Store) Set(id string, job *Job) {
	s.mu.Lock()
	s.jobs[id] = job
	s.mu.Unlock()
}

func (s *Store) Get(id string) (*Job, bool) {
	s.mu.RLock()
	j, ok := s.jobs[id]
	s.mu.RUnlock()
	return j, ok
}

func (s *Store) Delete(id string) {
	s.mu.Lock()
	delete(s.jobs, id)
	s.mu.Unlock()
}

func (s *Store) ScheduleCleanup(id string, ttl time.Duration) {
	time.AfterFunc(ttl, func() {
		s.Delete(id)
	})
}

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