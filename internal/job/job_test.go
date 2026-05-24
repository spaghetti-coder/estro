package job

import (
	"sync"
	"testing"
	"time"
)

func TestStoreSetGet(t *testing.T) {
	tests := []struct {
		name       string
		setup      func(*Store)
		id         string
		wantOK     bool
		wantStatus string
		wantTitle  string
	}{
		{
			name: "existing job",
			setup: func(s *Store) {
				s.Set("id1", &Job{Status: StatusRunning, Title: "test"})
			},
			id:         "id1",
			wantOK:     true,
			wantStatus: StatusRunning,
			wantTitle:  "test",
		},
		{
			name:       "nonexistent job",
			setup:      func(s *Store) {},
			id:         "nonexistent",
			wantOK:     false,
			wantStatus: "",
			wantTitle:  "",
		},
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
	_, ok := s.Get("id1")
	if ok {
		t.Error("expected job to be deleted")
	}
}

func TestStoreScheduleCleanup(t *testing.T) {
	s := NewStore()
	s.Set("id1", &Job{Status: StatusDone, Title: "test"})
	s.ScheduleCleanup("id1", 50*time.Millisecond)
	time.Sleep(100 * time.Millisecond)
	_, ok := s.Get("id1")
	if ok {
		t.Error("expected job to be cleaned up after TTL")
	}
}

func TestStoreMarkAllRunningAsError(t *testing.T) {
	tests := []struct {
		name       string
		id         string
		wantStatus string
		wantStderr string
	}{
		{
			name:       "running job becomes error",
			id:         "r1",
			wantStatus: StatusError,
			wantStderr: "server shutting down",
		},
		{
			name:       "another running job becomes error",
			id:         "r2",
			wantStatus: StatusError,
			wantStderr: "server shutting down",
		},
		{
			name:       "done job stays done",
			id:         "d1",
			wantStatus: StatusDone,
			wantStderr: "",
		},
	}

	s := NewStore()
	s.Set("r1", &Job{Status: StatusRunning, Title: "running job"})
	s.Set("r2", &Job{Status: StatusRunning, Title: "another running"})
	s.Set("d1", &Job{Status: StatusDone, Title: "done job"})
	s.MarkAllRunningAsError("server shutting down")

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
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

func TestStoreConcurrentAccess(t *testing.T) {
	s := NewStore()
	var wg sync.WaitGroup
	for i := range 100 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			id := string(rune('a' + i%26))
			s.Set(id, &Job{Status: StatusRunning, Title: id})
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

func TestGenerateID(t *testing.T) {
	id1, err := GenerateID()
	if err != nil {
		t.Fatalf("GenerateID() error = %v", err)
	}
	if len(id1) != 32 {
		t.Errorf("expected length 32, got %d", len(id1))
	}
	for i, c := range id1 {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			t.Errorf("invalid hex character %q at position %d", c, i)
		}
	}

	id2, err := GenerateID()
	if err != nil {
		t.Fatalf("GenerateID() second call error = %v", err)
	}
	if id1 == id2 {
		t.Error("expected two sequential IDs to be different")
	}
}
