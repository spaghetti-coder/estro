package job

import (
	"encoding/hex"
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestStoreSetGet(t *testing.T) {
	tests := []struct {
		name       string
		id         string
		wantOK     bool
		wantStatus string
		wantTitle  string
		setup      func(*Store)
	}{
		{name: "existing job", id: "id1", wantOK: true, wantStatus: StatusRunning, wantTitle: "test",
			setup: func(s *Store) {
				s.Set("id1", &Job{Status: StatusRunning, Title: "test"})
			},
		},
		{name: "nonexistent job", id: "nonexistent", wantOK: false, wantStatus: "", wantTitle: "", setup: func(s *Store) {}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewStore()
			tt.setup(s)
			got, ok := s.Get(tt.id)
			if ok != tt.wantOK {
				t.Fatalf("expected ok=%v, got ok=%v", tt.wantOK, ok)
			}
			if !ok {
				return
			}
			if got.Status != tt.wantStatus {
				t.Errorf("expected status %q, got %q", tt.wantStatus, got.Status)
			}
			if got.Title != tt.wantTitle {
				t.Errorf("expected title %q, got %q", tt.wantTitle, got.Title)
			}
		})
	}
}

func TestStoreDelete(t *testing.T) {
	s := NewStore()
	s.Set("id1", &Job{Status: StatusDone, Title: "test"})
	s.Delete("id1")
	if _, ok := s.Get("id1"); ok {
		t.Error("expected job to be deleted")
	}
}

func TestStoreScheduleCleanup(t *testing.T) {
	tests := []struct {
		name  string
		id    string
		setup func(*Store)
	}{
		{name: "existing job", id: "id1",
			setup: func(s *Store) {
				s.Set("id1", &Job{Status: StatusDone, Title: "test"})
			},
		},
		{name: "nonexistent job", id: "ghost", setup: func(*Store) {}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewStore()
			tt.setup(s)
			s.ScheduleCleanup(tt.id, 1*time.Millisecond)
			time.Sleep(10 * time.Millisecond)
			if _, ok := s.Get(tt.id); ok {
				t.Error("expected job to be cleaned up after TTL")
			}
		})
	}
}

func TestStoreMarkAllRunningAsError(t *testing.T) {
	tests := []struct {
		name       string
		id         string
		wantStatus string
		wantStderr string
		setup      func(*Store)
	}{
		{name: "running job becomes error", id: "r1", wantStatus: StatusError, wantStderr: "server shutting down",
			setup: func(s *Store) {
				s.Set("r1", &Job{Status: StatusRunning, Title: "running job"})
			},
		},
		{name: "done job stays done with existing stderr", id: "d1", wantStatus: StatusDone, wantStderr: "original error",
			setup: func(s *Store) {
				s.Set("d1", &Job{Status: StatusDone, Title: "done job", Stderr: "original error"})
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewStore()
			tt.setup(s)
			s.MarkAllRunningAsError("server shutting down")

			got, _ := s.Get(tt.id)
			if got.Status != tt.wantStatus {
				t.Errorf("expected status %q, got %q", tt.wantStatus, got.Status)
			}
			if got.Stderr != tt.wantStderr {
				t.Errorf("expected stderr %q, got %q", tt.wantStderr, got.Stderr)
			}
		})
	}
}

// Verifies Store is safe for concurrent Set/Get with overlapping keys.
// Correctness is checked by -race detector, not by assertions.
func TestStoreConcurrentAccess(t *testing.T) {
	s := NewStore()
	var wg sync.WaitGroup

	// 26 keys, 100 goroutines → intentional collisions test mutex correctness
	for i := range 100 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			id := fmt.Sprintf("id%d", i%26)
			s.Set(id, &Job{Status: StatusRunning, Title: id})
		}(i)
	}
	for i := range 100 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			id := fmt.Sprintf("id%d", i%26)
			s.Get(id)
		}(i)
	}
	wg.Wait()
}

func TestGenerateID(t *testing.T) {
	id1, err := GenerateID()
	if err != nil {
		t.Fatalf("GenerateID() error = %v", err)
	}
	if len(id1) != 32 {
		t.Errorf("expected length 32, got %d", len(id1))
	}
	if _, err := hex.DecodeString(id1); err != nil {
		t.Errorf("ID is not valid hex: %v", err)
	}

	id2, err := GenerateID()
	if err != nil {
		t.Fatalf("GenerateID() second call error = %v", err)
	}
	if id1 == id2 {
		t.Error("expected two sequential IDs to be different")
	}
}
