package config

import (
	"slices"
	"testing"
)

func TestIsAccessible(t *testing.T) {
	tests := []struct {
		name    string
		allowed []string
		user    string
		want    bool
	}{
		{"nil allowed with no user", nil, "", true},
		{"nil allowed with any user", nil, "alice", true},
		{"restricted to named users — allowed user", []string{"alice", "bob"}, "alice", true},
		{"restricted to named users — unknown user", []string{"alice", "bob"}, "guest", false},
		{"restricted to named users — empty user", []string{"alice", "bob"}, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flat := FlatService{Allowed: tt.allowed}
			if got := flat.IsAccessible(tt.user); got != tt.want {
				t.Errorf("IsAccessible(%q) = %v, want %v", tt.user, got, tt.want)
			}
		})
	}
}

func TestRestrictedTrue_NilAllowedIsPublic(t *testing.T) {
	flat := FlatService{Restricted: true, Allowed: nil}
	if !flat.IsAccessible("guest") {
		t.Error("restricted=true + nil allowed should be public")
	}
}

func TestRestrictedDefaultIsPublic(t *testing.T) {
	flat := FlatService{}
	if !flat.IsAccessible("") {
		t.Error("default (restricted=true, nil allowed) should be public")
	}
}

// buildTestConfig constructs the test Config programmatically.
func buildTestConfig() *Config {
	return &Config{
		Global: &GlobalConfig{
			Title:    ptrOf("Estro"),
			Subtitle: ptrOf("Config test suite"),
			Hostname: ptrOf("0.0.0.0"),
			Port:     ptrOf(3000),
			CascadeFields: CascadeFields{
				Timeout:    ptrOf(30),
				Confirm:    ptrOf(true),
				Restricted: ptrOf(false),
			},
		},
		Users: map[string]*UserConfig{
			"alice": {Password: "$2y$10$hash1", Groups: StringList{"admins"}},
			"bob":   {Password: "$2y$10$hash2", Groups: StringList{"admins", "family"}},
			"guest": {Password: "$2y$10$hash3"},
		},
		Sections: []SectionConfig{
			{
				Title:        "Public Info",
				LayoutFields: LayoutFields{Collapsable: ptrOf(false), Columns: ptrOf(1)},
				Services: []ServiceConfig{
					{Title: "Uptime", Command: CommandValue{"uptime"}},
					{Title: "Date", Command: CommandValue{"date"}, CascadeFields: CascadeFields{Confirm: ptrOf(false)}},
				},
			},
			{
				Title: "System",
				Services: []ServiceConfig{
					{Title: "Disk usage", Command: CommandValue{"df -h /", "echo ---", "df -h --total | tail -1"}},
					{Title: "Memory", Command: CommandValue{"free -h"}, CascadeFields: CascadeFields{Confirm: ptrOf(false)}},
					{Title: "CPU", Command: CommandValue{"ps aux --sort=-%cpu | head -10"}, CascadeFields: CascadeFields{Timeout: ptrOf(10)}},
					{Title: "Load", Command: CommandValue{"cat /proc/loadavg"}, CascadeFields: CascadeFields{Confirm: ptrOf(false)}},
				},
			},
			{
				Title:         "Logs",
				CascadeFields: CascadeFields{Timeout: ptrOf(15), Confirm: ptrOf(false)},
				LayoutFields:  LayoutFields{Columns: ptrOf(2)},
				Services: []ServiceConfig{
					{Title: "Syslog (20)", Command: CommandValue{"journalctl -n 20 --no-pager"}},
					{Title: "Kernel log", Command: CommandValue{"dmesg | tail -20"}},
					{Title: "Auth log", Command: CommandValue{"journalctl -n 20 -u ssh --no-pager"}, CascadeFields: CascadeFields{Confirm: ptrOf(true), Timeout: ptrOf(5)}},
				},
			},
			{
				Title:         "Admin",
				CascadeFields: CascadeFields{Allowed: StringList{"admins"}},
				LayoutFields:  LayoutFields{Columns: ptrOf(4)},
				Services: []ServiceConfig{
					{Title: "List /etc", Command: CommandValue{"ls -lah /etc"}},
					{Title: "Who", Command: CommandValue{"who"}, CascadeFields: CascadeFields{Confirm: ptrOf(false)}},
				},
			},
			{
				Title:         "Mixed Access",
				CascadeFields: CascadeFields{Allowed: StringList{"admins"}},
				Services: []ServiceConfig{
					{Title: "Public status", Command: CommandValue{"uptime"}, CascadeFields: CascadeFields{Allowed: StringList{}, Confirm: ptrOf(false)}},
					{Title: "Admin only", Command: CommandValue{"id"}},
					{Title: "Guest allowed", Command: CommandValue{"date"}, CascadeFields: CascadeFields{Allowed: StringList{"guest"}}},
				},
			},
			{
				Title:         "Local override",
				CascadeFields: CascadeFields{Allowed: StringList{"admins"}},
				Services: []ServiceConfig{
					{Title: "Local override", Command: CommandValue{"hostname"}, CascadeFields: CascadeFields{Allowed: StringList{"admins", "bob", "guest"}}},
				},
			},
			{
				Title:         "Remote (single hop)",
				CascadeFields: CascadeFields{Allowed: StringList{"admins"}, Remote: StringList{"server1.local"}, Confirm: ptrOf(false)},
				Services: []ServiceConfig{
					{Title: "Remote uptime", Command: CommandValue{"uptime"}},
					{Title: "Remote disk", Command: CommandValue{"df -h /"}},
				},
			},
			{
				Title:         "Remote (chain)",
				LayoutFields:  LayoutFields{Collapsable: ptrOf(false), Columns: ptrOf(2)},
				CascadeFields: CascadeFields{Allowed: StringList{"admins"}},
				Services: []ServiceConfig{
					{Title: "Two-hop uptime", Command: CommandValue{"uptime"}, CascadeFields: CascadeFields{Remote: StringList{"server1.local", "server2.local"}, Confirm: ptrOf(false)}},
					{Title: "Three-hop date", Command: CommandValue{"date"}, CascadeFields: CascadeFields{Remote: StringList{"server1.local", "server2.local", "server3.local"}, Confirm: ptrOf(false), Timeout: ptrOf(20)}},
				},
			},
		},
	}
}

func TestSerializeService(t *testing.T) {
	cfg := buildTestConfig()
	services := cfg.Flatten()

	wantTitles := map[string]bool{"Uptime": true, "CPU": true, "Auth log": true, "Three-hop date": true}
	for i, svc := range services {
		if !wantTitles[svc.Title] {
			continue
		}
		serialized := svc.Serialize(i, "alice")
		if serialized.ID != i {
			t.Errorf("%s: expected id %d, got %d", svc.Title, i, serialized.ID)
		}
		if serialized.Title != svc.Title {
			t.Errorf("%s: expected title %s, got %s", svc.Title, svc.Title, serialized.Title)
		}
		if serialized.Timeout != svc.Timeout*1000+10000 {
			t.Errorf("%s: expected timeout %d, got %d", svc.Title, svc.Timeout*1000+10000, serialized.Timeout)
		}
		if serialized.Confirm != svc.Confirm {
			t.Errorf("%s: expected confirm %v, got %v", svc.Title, svc.Confirm, serialized.Confirm)
		}
		if serialized.Section != svc.SectionTitle {
			t.Errorf("%s: expected section %s, got %v", svc.Title, svc.SectionTitle, serialized.Section)
		}
	}
}

func TestSerialize_RestrictedAndEnabled(t *testing.T) {
	tests := []struct {
		name      string
		flat      FlatService
		wantRestr bool
		wantEnabl bool
	}{
		{"restricted true", FlatService{Title: "t", Command: CommandValue{"echo"}, Restricted: true}, true, false},
		{"restricted false", FlatService{Title: "t", Command: CommandValue{"echo"}, Restricted: false}, false, false},
		{"enabled true", FlatService{Title: "t", Command: CommandValue{"echo"}, Enabled: true}, false, true},
		{"enabled false", FlatService{Title: "t", Command: CommandValue{"echo"}, Enabled: false}, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			serialized := tt.flat.Serialize(0, "")
			if serialized.Restricted != tt.wantRestr {
				t.Errorf("Restricted = %v, want %v", serialized.Restricted, tt.wantRestr)
			}
			if serialized.Enabled != tt.wantEnabl {
				t.Errorf("Enabled = %v, want %v", serialized.Enabled, tt.wantEnabl)
			}
		})
	}
}

func TestCommandValueString(t *testing.T) {
	cfg := buildTestConfig()
	services := cfg.Flatten()

	svc := findService(t, services, "Uptime")
	if len(svc.Command) != 1 || svc.Command[0] != "uptime" {
		t.Errorf("Uptime: expected single command 'uptime', got %v", svc.Command)
	}

	svc = findService(t, services, "Disk usage")
	if len(svc.Command) != 3 {
		t.Errorf("Disk usage: expected 3 commands, got %d", len(svc.Command))
	}
}

func TestFlatten_AclResolution(t *testing.T) {
	cfg := buildTestConfig()
	services := cfg.Flatten()

	tests := []struct {
		title       string
		wantAllowed []string // nil means nil (public)
	}{
		{"Who", []string{"alice", "bob"}},
		{"Public status", nil},
		{"Guest allowed", []string{"guest"}},
		{"Admin only", []string{"alice", "bob"}},
		{"Local override", []string{"alice", "bob", "guest"}},
	}
	for _, tt := range tests {
		t.Run(tt.title, func(t *testing.T) {
			svc := findService(t, services, tt.title)
			if (tt.wantAllowed == nil) != (svc.Allowed == nil) {
				t.Errorf("%s: nil mismatch, want nil=%v, got nil=%v", tt.title, tt.wantAllowed == nil, svc.Allowed == nil)
			}
			if !slices.Equal(svc.Allowed, tt.wantAllowed) {
				t.Errorf("%s: allowed=%v, want %v", tt.title, svc.Allowed, tt.wantAllowed)
			}
		})
	}
}

func TestResolveAllowed(t *testing.T) {
	users := map[string]*UserConfig{
		"alice": {Password: "hash", Groups: []string{"admins"}},
		"bob":   {Password: "hash", Groups: []string{"admins", "family"}},
	}

	tests := []struct {
		name  string
		input []string
		want  []string // nil means nil result
	}{
		{"nil input", nil, nil},
		{"empty slice", []string{}, nil},
		{"group expansion", []string{"admins"}, []string{"alice", "bob"}},
		{"direct username", []string{"guest"}, []string{"guest"}},
		{"nonexistent group", []string{"nonexistent-group"}, nil},
		{"username-group collision", []string{"admins"}, []string{"admins", "alice"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testUsers := users
			if tt.name == "username-group collision" {
				testUsers = map[string]*UserConfig{
					"admins": {Password: "hash", Groups: []string{}},
					"alice":  {Password: "hash", Groups: []string{"admins"}},
				}
			}
			if tt.name == "direct username" {
				testUsers = map[string]*UserConfig{"guest": {Password: "hash"}}
			}
			if tt.name == "nonexistent group" {
				testUsers = map[string]*UserConfig{"alice": {Password: "hash", Groups: []string{"admins"}}}
			}

			result := resolveAllowed(tt.input, testUsers)
			if (tt.want == nil) != (result == nil) {
				t.Errorf("nil mismatch: want nil=%v, got nil=%v", tt.want == nil, result == nil)
				return
			}
			if !slices.Equal(result, tt.want) {
				t.Errorf("got %v, want %v", result, tt.want)
			}
		})
	}
}
