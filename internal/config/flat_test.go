package config

import (
	"slices"
	"testing"

	"go.yaml.in/yaml/v4"
)

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

func TestSerializeRestrictedAndEnabled(t *testing.T) {
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

func TestCommandValueUnmarshalYAML(t *testing.T) {
	tests := []struct {
		name string
		yaml string
		want CommandValue
	}{
		{"scalar", "val: echo hi\n", CommandValue{"echo hi"}},
		{"sequence", "val: [a, b]\n", CommandValue{"a", "b"}},
		{"block sequence", "val:\n  - a\n  - b\n", CommandValue{"a", "b"}},
		{"empty scalar", "val: ''\n", CommandValue{""}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			type wrapper struct {
				Val CommandValue `yaml:"val,omitempty"`
			}
			var w wrapper
			if err := yaml.Load([]byte(tt.yaml), &w); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !slices.Equal(w.Val, tt.want) {
				t.Errorf("got %v, want %v", w.Val, tt.want)
			}
		})
	}
}

func TestCommandValueUnmarshalInvalid(t *testing.T) {
	type wrapper struct {
		Val CommandValue `yaml:"val,omitempty"`
	}
	var w wrapper
	if err := yaml.Load([]byte("val:\n  key: value\n"), &w); err == nil {
		t.Error("expected error for mapping node, got nil")
	}
}

func TestFlattenAclResolution(t *testing.T) {
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

func TestFlattenEnabledCascade(t *testing.T) {
	cfg := &Config{
		Global: &GlobalConfig{CascadeFields: CascadeFields{Enabled: ptrOf(false)}},
		Sections: []SectionConfig{
			{
				Title: "Disabled Section", CascadeFields: CascadeFields{Enabled: ptrOf(false)},
				Services: []ServiceConfig{
					{Title: "Disabled Service", Command: CommandValue{"echo disabled"}},
					{Title: "Override Enabled", Command: CommandValue{"echo override"}, CascadeFields: CascadeFields{Enabled: ptrOf(true)}},
				},
			},
			{
				Title: "Enabled Section", CascadeFields: CascadeFields{Enabled: ptrOf(true)},
				Services: []ServiceConfig{
					{Title: "Normal Service", Command: CommandValue{"echo normal"}},
				},
			},
			{
				Title: "Default Section",
				Services: []ServiceConfig{
					{Title: "Default Service", Command: CommandValue{"echo default"}},
				},
			},
		},
	}
	services := cfg.Flatten()

	want := map[string]bool{
		"Disabled Service": false,
		"Override Enabled": true,
		"Normal Service":   true,
		"Default Service":  false,
	}
	for _, svc := range services {
		wantVal, ok := want[svc.Title]
		if !ok {
			t.Fatalf("unexpected service %q", svc.Title)
		}
		if svc.Enabled != wantVal {
			t.Errorf("%s enabled: got %v, want %v", svc.Title, svc.Enabled, wantVal)
		}
	}
}

func TestFlattenRestrictedCascade(t *testing.T) {
	// Section override + service override
	cfg1 := &Config{
		Global: &GlobalConfig{},
		Sections: []SectionConfig{
			{
				Title: "Sec", CascadeFields: CascadeFields{Restricted: ptrOf(true)},
				Services: []ServiceConfig{
					{Title: "Override", Command: CommandValue{"echo"}, CascadeFields: CascadeFields{Restricted: ptrOf(false)}},
					{Title: "Inherit", Command: CommandValue{"date"}},
				},
			},
		},
	}
	services1 := cfg1.Flatten()
	if services1[0].Restricted != false {
		t.Errorf("Override restricted: got %v, want false", services1[0].Restricted)
	}
	if services1[1].Restricted != true {
		t.Errorf("Inherit restricted: got %v, want true", services1[1].Restricted)
	}

	// Global false + section inherit
	cfg2 := &Config{
		Global: &GlobalConfig{CascadeFields: CascadeFields{Restricted: ptrOf(false)}},
		Sections: []SectionConfig{
			{Title: "Sec", Services: []ServiceConfig{{Title: "Inherit Global", Command: CommandValue{"date"}}}},
		},
	}
	services2 := cfg2.Flatten()
	if services2[0].Restricted != false {
		t.Errorf("Inherit Global restricted: got %v, want false", services2[0].Restricted)
	}
}

func TestRemoteOverride(t *testing.T) {
	cfg := &Config{
		Global: &GlobalConfig{CascadeFields: CascadeFields{Timeout: ptrOf(30), Remote: StringList{"server1.local"}}},
		Users:  map[string]*UserConfig{"admin": {Password: "$2y$10$hash"}},
		Sections: []SectionConfig{
			{
				Title: "Section with global remote",
				Services: []ServiceConfig{
					{Title: "Inherits global remote", Command: CommandValue{"uptime"}},
					{Title: "Local override at service", Command: CommandValue{"hostname"}, CascadeFields: CascadeFields{Remote: StringList{}}},
				},
			},
			{
				Title: "Section with local override", CascadeFields: CascadeFields{Remote: StringList{}},
				Services: []ServiceConfig{
					{Title: "Inherits section local", Command: CommandValue{"date"}},
					{Title: "Service remote override", Command: CommandValue{"uptime"}, CascadeFields: CascadeFields{Remote: StringList{"server2.local"}}},
				},
			},
			{
				Title: "Section with section remote", CascadeFields: CascadeFields{Remote: StringList{"server2.local"}},
				Services: []ServiceConfig{
					{Title: "Inherits section remote", Command: CommandValue{"uptime"}},
					{Title: "Local override in section remote", Command: CommandValue{"hostname"}, CascadeFields: CascadeFields{Remote: StringList{}}},
				},
			},
		},
	}
	services := cfg.Flatten()

	tests := []struct {
		service string
		want    []string
	}{
		{"Inherits global remote", []string{"server1.local"}},
		{"Local override at service", []string{}},
		{"Inherits section local", []string{}},
		{"Service remote override", []string{"server2.local"}},
		{"Inherits section remote", []string{"server2.local"}},
		{"Local override in section remote", []string{}},
	}
	for _, tt := range tests {
		t.Run(tt.service, func(t *testing.T) {
			svc := findService(t, services, tt.service)
			got := svc.Remote
			if (tt.want == nil) != (got == nil) {
				t.Errorf("nil mismatch: want nil=%v, got nil=%v", tt.want == nil, got == nil)
				return
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("expected %v, got %v", tt.want, got)
			}
		})
	}
}

func TestRemoteChain(t *testing.T) {
	cfg := &Config{
		Global: &GlobalConfig{},
		Users:  map[string]*UserConfig{"alice": {Password: "$2y$10$hash1", Groups: StringList{"admins"}}},
		Sections: []SectionConfig{
			{
				Title: "Remote (single hop)", CascadeFields: CascadeFields{Allowed: StringList{"admins"}, Remote: StringList{"server1.local"}, Confirm: ptrOf(false)},
				Services: []ServiceConfig{
					{Title: "Remote uptime", Command: CommandValue{"uptime"}},
					{Title: "Remote disk", Command: CommandValue{"df -h /"}},
					{Title: "Local override", Command: CommandValue{"hostname"}, CascadeFields: CascadeFields{Allowed: StringList{"admins", "guest"}}},
				},
			},
			{
				Title: "Remote (chain)", LayoutFields: LayoutFields{Collapsable: ptrOf(false), Columns: ptrOf(2)},
				CascadeFields: CascadeFields{Allowed: StringList{"admins"}},
				Services: []ServiceConfig{
					{Title: "Two-hop uptime", Command: CommandValue{"uptime"}, CascadeFields: CascadeFields{Remote: StringList{"server1.local", "server2.local"}, Confirm: ptrOf(false)}},
					{Title: "Three-hop date", Command: CommandValue{"date"}, CascadeFields: CascadeFields{Remote: StringList{"server1.local", "server2.local", "server3.local"}, Confirm: ptrOf(false), Timeout: ptrOf(20)}},
				},
			},
		},
	}
	services := cfg.Flatten()

	tests := []struct {
		service  string
		wantLen  int
		wantHost string
	}{
		{"Remote uptime", 1, "server1.local"},
		{"Two-hop uptime", 2, ""},
		{"Three-hop date", 3, ""},
	}
	for _, tt := range tests {
		t.Run(tt.service, func(t *testing.T) {
			svc := findService(t, services, tt.service)
			if len(svc.Remote) != tt.wantLen {
				t.Errorf("expected %d remote hosts, got %d", tt.wantLen, len(svc.Remote))
			}
			if tt.wantHost != "" && svc.Remote[0] != tt.wantHost {
				t.Errorf("expected first host %q, got %q", tt.wantHost, svc.Remote[0])
			}
		})
	}
}

func TestFlattenRemoteSSHOpts(t *testing.T) {
	cfg := &Config{
		Global: &GlobalConfig{CascadeFields: CascadeFields{Remote: StringList{"hop1", "hop2", "target"}, RemoteSSHOpts: StringList{"-o", "ForwardAgent=no", "-o", "Compression=yes"}}},
		Users:  map[string]*UserConfig{"admin": {Password: "$2y$10$hash"}},
		Sections: []SectionConfig{{
			Title: "Multi-hop",
			Services: []ServiceConfig{
				{Title: "Three hop chain", Command: CommandValue{"uptime"}},
			},
		}},
	}
	services := cfg.Flatten()
	if len(services) != 1 {
		t.Fatalf("expected 1 service, got %d", len(services))
	}
	svc := services[0]
	if len(svc.Remote) != 3 {
		t.Errorf("expected 3 remote hosts, got %d: %v", len(svc.Remote), svc.Remote)
	}
	wantRemote := []string{"hop1", "hop2", "target"}
	if !slices.Equal(svc.Remote, wantRemote) {
		t.Errorf("remote: expected %v, got %v", wantRemote, svc.Remote)
	}
	wantOpts := []string{"-o", "ForwardAgent=no", "-o", "Compression=yes"}
	if !slices.Equal(svc.RemoteSSHOpts, wantOpts) {
		t.Errorf("ssh opts: expected %v, got %v", wantOpts, svc.RemoteSSHOpts)
	}
}

// findService finds a FlatService by title, failing the test if not found.
func findService(t *testing.T, services []FlatService, title string) FlatService {
	t.Helper()
	for _, svc := range services {
		if svc.Title == title {
			return svc
		}
	}
	t.Fatalf("service %q not found", title)
	return FlatService{}
}
