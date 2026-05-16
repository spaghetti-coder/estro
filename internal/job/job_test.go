package job

import (
	"sync"
	"testing"
	"time"
)

func TestStoreSetGet(t *testing.T) {
	s := NewStore()
	j := &Job{Status: "running", Title: "test"}
	s.Set("id1", j)
	got, ok := s.Get("id1")
	if !ok {
		t.Fatal("expected job to exist")
	}
	if got.Status != "running" {
		t.Errorf("expected status %q, got %q", "running", got.Status)
	}
	if got.Title != "test" {
		t.Errorf("expected title %q, got %q", "test", got.Title)
	}
}

func TestStoreGetNonexistent(t *testing.T) {
	s := NewStore()
	_, ok := s.Get("nonexistent")
	if ok {
		t.Error("expected job to not exist")
	}
}

func TestStoreDelete(t *testing.T) {
	s := NewStore()
	s.Set("id1", &Job{Status: "done", Title: "test"})
	s.Delete("id1")
	_, ok := s.Get("id1")
	if ok {
		t.Error("expected job to be deleted")
	}
}

func TestStoreScheduleCleanup(t *testing.T) {
	s := NewStore()
	s.Set("id1", &Job{Status: "done", Title: "test"})
	s.ScheduleCleanup("id1", 50*time.Millisecond)
	time.Sleep(100 * time.Millisecond)
	_, ok := s.Get("id1")
	if ok {
		t.Error("expected job to be cleaned up after TTL")
	}
}

func TestStoreMarkAllRunningAsError(t *testing.T) {
	s := NewStore()
	s.Set("r1", &Job{Status: "running", Title: "running job"})
	s.Set("r2", &Job{Status: "running", Title: "another running"})
	s.Set("d1", &Job{Status: "done", Title: "done job"})
	s.MarkAllRunningAsError("server shutting down")
	j1, _ := s.Get("r1")
	if j1.Status != "error" {
		t.Errorf("expected status %q, got %q", "error", j1.Status)
	}
	if j1.Stderr != "server shutting down" {
		t.Errorf("expected stderr %q, got %q", "server shutting down", j1.Stderr)
	}
	j2, _ := s.Get("r2")
	if j2.Status != "error" {
		t.Errorf("expected status %q, got %q", "error", j2.Status)
	}
	j3, _ := s.Get("d1")
	if j3.Status != "done" {
		t.Errorf("expected done job to remain %q, got %q", "done", j3.Status)
	}
}

func TestStoreConcurrentAccess(t *testing.T) {
	s := NewStore()
	var wg sync.WaitGroup
	for i := range 100 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			id := string(rune('a' + i%26))
			s.Set(id, &Job{Status: "running", Title: id})
		}(i)
	}
	for i := range 100 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			id := string(rune('a' + i%26))
			s.Get(id)
		}(i)
	}
	wg.Wait()
}
