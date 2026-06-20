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
	tests := []struct {
		name   string
		setID  string
		delID  string
		wantOK bool
	}{
		{name: "existing job", setID: "id1", delID: "id1", wantOK: false},
		{name: "nonexistent job", setID: "", delID: "ghost", wantOK: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewStore()
			if tt.setID != "" {
				s.Set(tt.setID, &Job{Status: StatusDone, Title: "test"})
			}
			s.Delete(tt.delID)
			if _, ok := s.Get(tt.delID); ok != tt.wantOK {
				t.Errorf("Get after delete: ok = %v, want %v", ok, tt.wantOK)
			}
		})
	}
}

func TestStoreScheduleCleanup(t *testing.T) {
	s := NewStore()
	s.Set("id1", &Job{Status: StatusDone, Title: "test"})

	s.ScheduleCleanup("id1", 1*time.Millisecond)
	deadline := time.Now().Add(50 * time.Millisecond)
	for time.Now().Before(deadline) {
		if _, ok := s.Get("id1"); !ok {
			return // cleaned up
		}
		time.Sleep(time.Millisecond)
	}
	t.Error("expected job to be cleaned up after TTL")
}

func TestStoreMarkAllRunningAsError(t *testing.T) {
	s := NewStore()
	s.Set("r1", &Job{Status: StatusRunning, Title: "running 1"})
	s.Set("r2", &Job{Status: StatusRunning, Title: "running 2", Stderr: "stale"})
	s.Set("d1", &Job{Status: StatusDone, Title: "done", Stderr: "kept"})
	s.Set("e1", &Job{Status: StatusError, Title: "error", Stderr: "kept"})

	s.MarkAllRunningAsError("server shutting down")

	cases := []struct {
		id         string
		wantStatus string
		wantStderr string
	}{
		{"r1", StatusError, "server shutting down"},
		{"r2", StatusError, "server shutting down"},
		{"d1", StatusDone, "kept"},
		{"e1", StatusError, "kept"},
	}
	for _, c := range cases {
		got, ok := s.Get(c.id)
		if !ok {
			t.Fatalf("%s: job missing", c.id)
		}
		if got.Status != c.wantStatus {
			t.Errorf("%s: status = %q, want %q", c.id, got.Status, c.wantStatus)
		}
		if got.Stderr != c.wantStderr {
			t.Errorf("%s: stderr = %q, want %q", c.id, got.Stderr, c.wantStderr)
		}
	}
}

// Store safe for concurrent Set/Get with overlapping keys; correctness checked by -race.
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
