package config

import (
	"os"
	"slices"
	"testing"
)

func TestResolveAllowedNil(t *testing.T) {
	users := map[string]*UserConfig{
		"alice": {Password: "hash", Groups: []string{"admins"}},
	}
	svc := FlatService{Allowed: nil}
	result := svc.ResolveAllowed(users)
	if result != nil {
		t.Errorf("expected nil for Allowed=nil, got %v", result)
	}
}

func TestResolveAllowedEmptySlice(t *testing.T) {
	users := map[string]*UserConfig{
		"alice": {Password: "hash", Groups: []string{"admins"}},
	}
	svc := FlatService{Allowed: []string{}}
	result := svc.ResolveAllowed(users)
	if result != nil {
		t.Errorf("expected nil for empty Allowed, got %v", result)
	}
}

func TestResolveAllowedGroup(t *testing.T) {
	users := map[string]*UserConfig{
		"alice": {Password: "hash", Groups: []string{"admins"}},
		"bob":   {Password: "hash", Groups: []string{"admins", "family"}},
	}
	svc := FlatService{Allowed: []string{"admins"}}
	result := svc.ResolveAllowed(users)
	if len(result) != 2 {
		t.Errorf("expected 2 users for 'admins' group, got %d: %v", len(result), result)
	}
	if !slices.Contains(result, "alice") || !slices.Contains(result, "bob") {
		t.Errorf("expected alice and bob, got %v", result)
	}
}

func TestResolveAllowedSingleUser(t *testing.T) {
	users := map[string]*UserConfig{
		"guest": {Password: "hash"},
	}
	svc := FlatService{Allowed: []string{"guest"}}
	result := svc.ResolveAllowed(users)
	if len(result) != 1 || result[0] != "guest" {
		t.Errorf("expected [guest], got %v", result)
	}
}

func TestIsAccessible(t *testing.T) {
	users := map[string]*UserConfig{
		"alice": {Password: "hash", Groups: []string{"admins"}},
		"bob":   {Password: "hash", Groups: []string{"admins"}},
		"guest": {Password: "hash"},
	}

	publicSvc := FlatService{Allowed: nil}
	if !publicSvc.IsAccessible("", users) {
		t.Error("public service should be accessible with no user")
	}
	if !publicSvc.IsAccessible("alice", users) {
		t.Error("public service should be accessible with any user")
	}

	restrictedSvc := FlatService{Allowed: []string{"admins"}}
	if restrictedSvc.IsAccessible("", users) {
		t.Error("restricted service should not be accessible with empty user")
	}
	if !restrictedSvc.IsAccessible("alice", users) {
		t.Error("restricted service should be accessible to admin user")
	}
	if restrictedSvc.IsAccessible("guest", users) {
		t.Error("restricted service should not be accessible to guest")
	}

	emptyAllowed := FlatService{Allowed: []string{}}
	if !emptyAllowed.IsAccessible("", users) {
		t.Error("empty allowed should be accessible (public)")
	}
}

func TestRestrictedTrue_EmptyAllowedIsPublic(t *testing.T) {
	flat := FlatService{
		Restricted: true,
		Allowed:    []string{},
	}
	users := map[string]*UserConfig{
		"alice": {Password: "hash"},
	}
	if flat.Restricted {
		if !flat.IsAccessible("guest", users) {
			t.Error("restricted=true + allowed=[] should be public")
		}
	}
}

func TestRestrictedTrue_NilAllowedIsPublic(t *testing.T) {
	flat := FlatService{}
	users := map[string]*UserConfig{
		"alice": {Password: "hash"},
	}
	if !flat.IsAccessible("", users) {
		t.Error("restricted=true (default) + nil allowed at all levels should be public")
	}
}

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

func TestCommandValueString(t *testing.T) {
	path := writeTestConfig(t, testConfigYAML())
	defer func() { _ = os.Remove(path) }()

	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	services := cfg.Flatten()

	for _, svc := range services {
		switch svc.Title {
		case "Uptime":
			if len(svc.Command) != 1 || svc.Command[0] != "uptime" {
				t.Errorf("Uptime: expected single command 'uptime', got %v", svc.Command)
			}
		case "Disk usage":
			if len(svc.Command) != 3 {
				t.Errorf("Disk usage: expected 3 commands, got %d", len(svc.Command))
			}
		}
	}
}
