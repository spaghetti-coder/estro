package config

import (
	"os"
	"testing"
)

func TestSerializeService(t *testing.T) {
	path := writeTestConfig(t, testConfigYAML())
	defer func() { _ = os.Remove(path) }()

	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	services := cfg.Flatten()

	wantTitles := []string{"Uptime", "CPU", "Auth log", "Three-hop date"}
	for _, wantTitle := range wantTitles {
		found := false
		for i, svc := range services {
			if svc.Title != wantTitle {
				continue
			}
			found = true
			serialized := svc.Serialize(i, "alice", cfg.Users)
			if serialized.ID != i {
				t.Errorf("%s: expected id %d, got %d", wantTitle, i, serialized.ID)
			}
			if serialized.Title != svc.Title {
				t.Errorf("%s: expected title %s, got %s", wantTitle, svc.Title, serialized.Title)
			}
			if serialized.Timeout != svc.Timeout*1000+10000 {
				t.Errorf("%s: expected timeout %d, got %d", wantTitle, svc.Timeout*1000+10000, serialized.Timeout)
			}
			if serialized.Confirm != svc.Confirm {
				t.Errorf("%s: expected confirm %v, got %v", wantTitle, svc.Confirm, serialized.Confirm)
			}
			if serialized.Section == nil || *serialized.Section != svc.SectionTitle {
				t.Errorf("%s: expected section %s, got %v", wantTitle, svc.SectionTitle, serialized.Section)
			}
		}
		if !found {
			t.Errorf("service %q not found in flattened config", wantTitle)
		}
	}
}

func TestSerialize_Restricted(t *testing.T) {
	tests := []struct {
		name      string
		flat      FlatService
		wantRestr bool
	}{
		{"global restricted true", FlatService{Title: "t", Command: CommandValue{"echo"}, Restricted: true}, true},
		{"service restricted false", FlatService{Title: "t", Command: CommandValue{"echo"}, Restricted: false}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			serialized := tt.flat.Serialize(0, "", nil)
			if serialized.Restricted != tt.wantRestr {
				t.Errorf("Restricted = %v, want %v", serialized.Restricted, tt.wantRestr)
			}
		})
	}
}

func TestSerialize_Enabled(t *testing.T) {
	tests := []struct {
		name      string
		flat      FlatService
		wantEnabl bool
	}{
		{"global enabled false", FlatService{Title: "t", Command: CommandValue{"echo"}, Enabled: false}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			serialized := tt.flat.Serialize(0, "", nil)
			if serialized.Enabled != tt.wantEnabl {
				t.Errorf("Enabled = %v, want %v", serialized.Enabled, tt.wantEnabl)
			}
		})
	}
}
