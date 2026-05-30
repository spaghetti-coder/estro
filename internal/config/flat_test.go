package config

import (
	"os"
	"slices"
	"testing"
)

func TestIsAccessible(t *testing.T) {
	publicSvc := FlatService{Allowed: nil}
	if !publicSvc.IsAccessible("") {
		t.Error("public service should be accessible with no user")
	}
	if !publicSvc.IsAccessible("alice") {
		t.Error("public service should be accessible with any user")
	}

	restrictedSvc := FlatService{Allowed: []string{"alice", "bob"}}
	if restrictedSvc.IsAccessible("") {
		t.Error("restricted service should not be accessible with empty user")
	}
	if !restrictedSvc.IsAccessible("alice") {
		t.Error("restricted service should be accessible to named user")
	}
	if restrictedSvc.IsAccessible("guest") {
		t.Error("restricted service should not be accessible to non-named user")
	}

	emptyAllowed := FlatService{Allowed: nil}
	if !emptyAllowed.IsAccessible("guest") {
		t.Error("nil allowed (public) should be accessible")
	}
}

func TestRestrictedTrue_EmptyAllowedIsPublic(t *testing.T) {
	flat := FlatService{
		Restricted: true,
		Allowed:    nil,
	}
	if flat.Restricted {
		if !flat.IsAccessible("guest") {
			t.Error("restricted=true + nil allowed should be public")
		}
	}
}

func TestRestrictedTrue_NilAllowedIsPublic(t *testing.T) {
	flat := FlatService{}
	if !flat.IsAccessible("") {
		t.Error("restricted=true (default) + nil allowed at all levels should be public")
	}
}

func TestSerializeService(t *testing.T) {
	path := writeTestConfig(t, testConfigYAML())
	defer func() { _ = os.Remove(path) }()

	res := Load(path)
	if !res.Healthy() {
		t.Fatalf("unexpected issues: %v", res.IssueStrings())
	}
	cfg := res.Config
	services := cfg.Flatten()

	wantTitles := []string{"Uptime", "CPU", "Auth log", "Three-hop date"}
	for _, wantTitle := range wantTitles {
		found := false
		for i, svc := range services {
			if svc.Title != wantTitle {
				continue
			}
			found = true
			serialized := svc.Serialize(i, "alice")
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
			if serialized.Section != svc.SectionTitle {
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
			serialized := tt.flat.Serialize(0, "")
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
			serialized := tt.flat.Serialize(0, "")
			if serialized.Enabled != tt.wantEnabl {
				t.Errorf("Enabled = %v, want %v", serialized.Enabled, tt.wantEnabl)
			}
		})
	}
}

func TestCommandValueString(t *testing.T) {
	path := writeTestConfig(t, testConfigYAML())
	defer func() { _ = os.Remove(path) }()

	res := Load(path)
	if !res.Healthy() {
		t.Fatalf("unexpected issues: %v", res.IssueStrings())
	}
	cfg := res.Config
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

func TestFlatten_AclResolution(t *testing.T) {
	path := writeTestConfig(t, testConfigYAML())
	defer func() { _ = os.Remove(path) }()

	res := Load(path)
	if !res.Healthy() {
		t.Fatalf("unexpected issues: %v", res.IssueStrings())
	}
	cfg := res.Config
	services := cfg.Flatten()

	findSvc := func(title string) *FlatService {
		for i := range services {
			if services[i].Title == title {
				return &services[i]
			}
		}
		return nil
	}

	// Case 1: Service in "Admin" section with allowed: [admins] → resolved to concrete usernames
	adminSvc := findSvc("Who")
	if adminSvc == nil {
		t.Fatal("expected to find 'Who' service")
	}
	if adminSvc.Allowed == nil {
		t.Error("expected non-nil Allowed for 'Who' service (group 'admins' should resolve)")
	} else {
		if !slices.Contains(adminSvc.Allowed, "alice") || !slices.Contains(adminSvc.Allowed, "bob") {
			t.Errorf("expected 'admins' group resolved to [alice, bob], got %v", adminSvc.Allowed)
		}
	}

	// Case 2: Service with allowed: [] (empty) → nil (public)
	publicSvc := findSvc("Public status")
	if publicSvc == nil {
		t.Fatal("expected to find 'Public status' service")
	}
	if publicSvc.Allowed != nil {
		t.Errorf("expected nil Allowed for empty allowed (public), got %v", publicSvc.Allowed)
	}

	// Case 3: Service with allowed: [guest] (direct username) → ["guest"]
	guestSvc := findSvc("Guest allowed")
	if guestSvc == nil {
		t.Fatal("expected to find 'Guest allowed' service")
	}
	if len(guestSvc.Allowed) != 1 || guestSvc.Allowed[0] != "guest" {
		t.Errorf("expected Allowed=[guest] for direct username, got %v", guestSvc.Allowed)
	}

	// Case 4: Service with no allowed (cascades to section's allowed: [admins]) → resolved group
	adminOnlySvc := findSvc("Admin only")
	if adminOnlySvc == nil {
		t.Fatal("expected to find 'Admin only' service")
	}
	if adminOnlySvc.Allowed == nil {
		t.Error("expected non-nil Allowed for 'Admin only' (cascades to section's 'admins')")
	} else {
		if !slices.Contains(adminOnlySvc.Allowed, "alice") || !slices.Contains(adminOnlySvc.Allowed, "bob") {
			t.Errorf("expected 'admins' group resolved to [alice, bob], got %v", adminOnlySvc.Allowed)
		}
	}

	// Case 5: Service with allowed: [admins, guest] in "Local override"
	localSvc := findSvc("Local override")
	if localSvc == nil {
		t.Fatal("expected to find 'Local override' service")
	}
	if localSvc.Allowed == nil {
		t.Error("expected non-nil Allowed for 'Local override'")
	} else {
		if !slices.Contains(localSvc.Allowed, "alice") || !slices.Contains(localSvc.Allowed, "guest") {
			t.Errorf("expected 'admins,guest' resolved to include alice and guest, got %v", localSvc.Allowed)
		}
	}
}

func TestResolveAllowed_NilInput(t *testing.T) {
	users := map[string]*UserConfig{
		"alice": {Password: "hash", Groups: []string{"admins"}},
	}
	result := resolveAllowed(nil, users)
	if result != nil {
		t.Errorf("expected nil for nil input, got %v", result)
	}
}

func TestResolveAllowed_EmptySlice(t *testing.T) {
	users := map[string]*UserConfig{
		"alice": {Password: "hash", Groups: []string{"admins"}},
	}
	result := resolveAllowed([]string{}, users)
	if result != nil {
		t.Errorf("expected nil for empty slice input, got %v", result)
	}
}

func TestResolveAllowed_GroupExpansion(t *testing.T) {
	users := map[string]*UserConfig{
		"alice": {Password: "hash", Groups: []string{"admins"}},
		"bob":   {Password: "hash", Groups: []string{"admins", "family"}},
	}
	result := resolveAllowed([]string{"admins"}, users)
	if len(result) != 2 {
		t.Errorf("expected 2 users for 'admins' group, got %d: %v", len(result), result)
	}
	if !slices.Contains(result, "alice") || !slices.Contains(result, "bob") {
		t.Errorf("expected alice and bob, got %v", result)
	}
}

func TestResolveAllowed_DirectUsername(t *testing.T) {
	users := map[string]*UserConfig{
		"guest": {Password: "hash"},
	}
	result := resolveAllowed([]string{"guest"}, users)
	if len(result) != 1 || result[0] != "guest" {
		t.Errorf("expected [guest], got %v", result)
	}
}

func TestResolveAllowed_NonexistentGroup(t *testing.T) {
	users := map[string]*UserConfig{
		"alice": {Password: "hash", Groups: []string{"admins"}},
	}
	result := resolveAllowed([]string{"nonexistent-group"}, users)
	if result != nil {
		t.Errorf("expected nil for group resolving to zero users, got %v", result)
	}
}

func TestResolveAllowed_UsernameGroupCollision(t *testing.T) {
	// When a name exists as both a username and a group name, both should be resolved.
	users := map[string]*UserConfig{
		"admins": {Password: "hash", Groups: []string{}},         // user named "admins"
		"alice":  {Password: "hash", Groups: []string{"admins"}}, // alice is in the "admins" group
	}
	result := resolveAllowed([]string{"admins"}, users)
	if len(result) != 2 {
		t.Errorf("expected 2 users for username/group collision, got %d: %v", len(result), result)
	}
	if !slices.Contains(result, "admins") {
		t.Errorf("expected 'admins' (direct user) in result, got %v", result)
	}
	if !slices.Contains(result, "alice") {
		t.Errorf("expected 'alice' (group member) in result, got %v", result)
	}
}
