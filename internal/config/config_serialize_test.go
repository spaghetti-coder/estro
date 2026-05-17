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
			if serialized.Timeout != svc.GetTimeoutMs()+10000 {
				t.Errorf("%s: expected timeout %d, got %d", wantTitle, svc.GetTimeoutMs()+10000, serialized.Timeout)
			}
			if serialized.Confirm != svc.GetConfirm() {
				t.Errorf("%s: expected confirm %v, got %v", wantTitle, svc.GetConfirm(), serialized.Confirm)
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
	glbTrue := true
	svcFalse := false
	tests := []struct {
		name      string
		flat      FlatService
		wantRestr bool
	}{
		{"global restricted true", FlatService{Title: "t", Command: CommandValue{"echo"}, Global: &GlobalConfig{CascadeFields: CascadeFields{Restricted: &glbTrue}}}, true},
		{"service restricted false", FlatService{Title: "t", Command: CommandValue{"echo"}, ServiceCascade: CascadeFields{Restricted: &svcFalse}, Global: &GlobalConfig{}}, false},
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
	glbFalse := false
	tests := []struct {
		name      string
		flat      FlatService
		wantEnabl bool
	}{
		{"global enabled false", FlatService{Title: "t", Command: CommandValue{"echo"}, Global: &GlobalConfig{CascadeFields: CascadeFields{Enabled: &glbFalse}}}, false},
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
