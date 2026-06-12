package config

import (
	"slices"
	"testing"

	"go.yaml.in/yaml/v4"
)

func TestStringListUnmarshalYAML(t *testing.T) {
	tests := []struct {
		name  string
		yaml  string
		field StringList
		isNil bool
	}{
		{"null", "val: ~\n", nil, true},
		{"empty string", "val: ''\n", StringList{}, false},
		{"single element no commas", "val: server1\n", StringList{"server1"}, false},
		{"comma-separated", "val: 'server1, server2,server3'\n", StringList{"server1", "server2", "server3"}, false},
		{"yaml array", "val: [server1, server2]\n", StringList{"server1", "server2"}, false},
		{"comma with spaces", "val: 'admins, guest'\n", StringList{"admins", "guest"}, false},
		{"trailing comma kept as empty", "val: 'a,b,'\n", StringList{"a", "b", ""}, false},
		{"empty middle element kept", "val: 'a,,b'\n", StringList{"a", "", "b"}, false},
		{"leading comma kept as empty", "val: ',a'\n", StringList{"", "a"}, false},
		{"whitespace-only element kept as empty", "val: 'a, ,b'\n", StringList{"a", "", "b"}, false},
		{"sequence empty element kept", "val: [a, '', b]\n", StringList{"a", "", "b"}, false},
		{"sequence whitespace-only kept as empty", "val: [a, ' ', b]\n", StringList{"a", "", "b"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			type wrapper struct {
				Val StringList `yaml:"val,omitempty"`
			}
			var w wrapper
			if err := yaml.Load([]byte(tt.yaml), &w); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.isNil {
				if w.Val != nil {
					t.Errorf("expected nil, got %v", w.Val)
				}
			} else {
				if w.Val == nil {
					t.Fatalf("expected non-nil, got nil")
				}
				if !slices.Equal(w.Val, tt.field) {
					t.Errorf("expected %v, got %v", tt.field, w.Val)
				}
			}
		})
	}
}

func TestStringListUnmarshalInvalid(t *testing.T) {
	type wrapper struct {
		Val StringList `yaml:"val,omitempty"`
	}
	var w wrapper
	err := yaml.Load([]byte("val:\n  key: value\n"), &w)
	if err == nil {
		t.Error("expected error for mapping node, got nil")
	}
}

func TestCascadeStringList(t *testing.T) {
	tests := []struct {
		name             string
		svc, sec, global StringList
		want             StringList
	}{
		{"all nil", nil, nil, nil, nil},
		{"service overrides section", StringList{"svc"}, StringList{"sec"}, StringList{"global"}, StringList{"svc"}},
		{"section overrides global", nil, StringList{"sec"}, StringList{"global"}, StringList{"sec"}},
		{"falls back to global", nil, nil, StringList{"global"}, StringList{"global"}},
		{"empty slice is explicit override", StringList{}, StringList{"sec"}, StringList{"global"}, StringList{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cascadeStringList(tt.svc, tt.sec, tt.global)
			if (tt.want == nil) != (got == nil) {
				t.Fatalf("nil mismatch: want %v, got %v", tt.want, got)
			}
			if !slices.Equal(got, tt.want) {
				t.Fatalf("expected %v, got %v", tt.want, got)
			}
		})
	}
}

// ptrOf is declared in validate_test.go

func TestCascadeFields(t *testing.T) {
	cfg := &Config{
		Global: &GlobalConfig{CascadeFields: CascadeFields{Timeout: ptrOf(30), Confirm: ptrOf(true)}},
		Sections: []SectionConfig{
			{
				Title: "Public Info", LayoutFields: LayoutFields{Collapsable: ptrOf(false), Columns: ptrOf(1)},
				Services: []ServiceConfig{
					{Title: "Uptime", Command: CommandValue{"uptime"}},
					{Title: "Date", Command: CommandValue{"date"}, CascadeFields: CascadeFields{Confirm: ptrOf(false)}},
				},
			},
			{
				Title: "System",
				Services: []ServiceConfig{
					{Title: "Disk usage", Command: CommandValue{"df -h /"}},
					{Title: "Memory", Command: CommandValue{"free -h"}, CascadeFields: CascadeFields{Confirm: ptrOf(false)}},
					{Title: "CPU", Command: CommandValue{"ps aux --sort=-%cpu | head -10"}, CascadeFields: CascadeFields{Timeout: ptrOf(10)}},
					{Title: "Load", Command: CommandValue{"cat /proc/loadavg"}, CascadeFields: CascadeFields{Confirm: ptrOf(false)}},
				},
			},
			{
				Title: "Logs", CascadeFields: CascadeFields{Timeout: ptrOf(15), Confirm: ptrOf(false)},
				LayoutFields: LayoutFields{Columns: ptrOf(2)},
				Services: []ServiceConfig{
					{Title: "Syslog (20)", Command: CommandValue{"journalctl -n 20 --no-pager"}},
					{Title: "Kernel log", Command: CommandValue{"dmesg | tail -20"}},
					{Title: "Auth log", Command: CommandValue{"journalctl -n 20 -u ssh --no-pager"}, CascadeFields: CascadeFields{Confirm: ptrOf(true), Timeout: ptrOf(5)}},
				},
			},
		},
	}
	services := cfg.Flatten()

	tests := []struct {
		field    string
		service  string
		wantInt  int
		wantBool bool
		isBool   bool
	}{
		{"timeout", "CPU", 10, false, false},
		{"timeout", "Auth log", 5, false, false},
		{"timeout", "Uptime", 30, false, false},
	}

	for _, tt := range tests {
		t.Run(tt.field+"/"+tt.service, func(t *testing.T) {
			for _, svc := range services {
				if svc.Title != tt.service {
					continue
				}
				if tt.isBool {
					if svc.Confirm != tt.wantBool {
						t.Errorf("%s: expected %s %v, got %v", tt.service, tt.field, tt.wantBool, svc.Confirm)
					}
				} else {
					if svc.Timeout != tt.wantInt {
						t.Errorf("%s: expected %s %d, got %d", tt.service, tt.field, tt.wantInt, svc.Timeout)
					}
				}
				return
			}
			t.Errorf("service %q not found", tt.service)
		})
	}
}

func TestFlatten_EnabledAndRestrictedCascade(t *testing.T) {
	tests := []struct {
		name       string
		restricted bool
		svc        FlatService
		want       bool
	}{
		{"disabled service inherits section disabled", true, FlatService{Enabled: false}, false},
		{"service overrides section enabled", true, FlatService{Enabled: true}, true},
		{"service overrides section restricted=false", false, FlatService{Restricted: false}, false},
		{"service inherits section restricted", false, FlatService{Restricted: true}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.restricted {
				if tt.svc.Enabled != tt.want {
					t.Errorf("Enabled = %v, want %v", tt.svc.Enabled, tt.want)
				}
			} else {
				if tt.svc.Restricted != tt.want {
					t.Errorf("Restricted = %v, want %v", tt.svc.Restricted, tt.want)
				}
			}
		})
	}

	// Enabled cascade through Flatten
	cfg1 := &Config{
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
	services1 := cfg1.Flatten()
	check := func(title string, field string, got, want bool) {
		t.Helper()
		if got != want {
			t.Errorf("%s %s: got %v, want %v", title, field, got, want)
		}
	}
	check("Disabled Service", "enabled", services1[0].Enabled, false)
	check("Override Enabled", "enabled", services1[1].Enabled, true)
	check("Normal Service", "enabled", services1[2].Enabled, true)
	check("Default Service", "enabled", services1[3].Enabled, false)

	// Restricted cascade through Flatten
	cfg2 := &Config{
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
	services2 := cfg2.Flatten()
	check("Override", "restricted", services2[0].Restricted, false)
	check("Inherit", "restricted", services2[1].Restricted, true)

	cfg3 := &Config{
		Global: &GlobalConfig{CascadeFields: CascadeFields{Restricted: ptrOf(false)}},
		Sections: []SectionConfig{
			{Title: "Sec", Services: []ServiceConfig{{Title: "Inherit Global", Command: CommandValue{"date"}}}},
		},
	}
	services3 := cfg3.Flatten()
	check("Inherit Global", "restricted", services3[0].Restricted, false)
}

func TestRemoteCascade(t *testing.T) {
	t.Run("remote_override", func(t *testing.T) {
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
					t.Fatalf("nil mismatch: want nil=%v, got nil=%v", tt.want == nil, got == nil)
				}
				if !slices.Equal(got, tt.want) {
					t.Errorf("expected %v, got %v", tt.want, got)
				}
			})
		}
	})

	t.Run("remote_chain", func(t *testing.T) {
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
	})
}

func TestCascadeRemoteSSHOpts(t *testing.T) {
	tests := []struct {
		name       string
		globalOpts StringList
		secOpts    StringList
		svcOpts    StringList
		wantOpts   StringList
	}{
		{"nil → nil → nil", nil, nil, nil, nil},
		{"nil → nil → override", nil, nil, StringList{"-o", "svc=1"}, StringList{"-o", "svc=1"}},
		{"nil → override → nil", nil, StringList{"-o", "sec=1"}, nil, StringList{"-o", "sec=1"}},
		{"nil → override → override", nil, StringList{"-o", "sec=1"}, StringList{"-o", "svc=1"}, StringList{"-o", "svc=1"}},
		{"override → nil → nil", StringList{"-o", "glb=1"}, nil, nil, StringList{"-o", "glb=1"}},
		{"override → nil → override", StringList{"-o", "glb=1"}, nil, StringList{"-o", "svc=1"}, StringList{"-o", "svc=1"}},
		{"override → override → nil", StringList{"-o", "glb=1"}, StringList{"-o", "sec=1"}, nil, StringList{"-o", "sec=1"}},
		{"override → override → override", StringList{"-o", "glb=1"}, StringList{"-o", "sec=1"}, StringList{"-o", "svc=1"}, StringList{"-o", "svc=1"}},
		{"empty slice override at service", StringList{"-o", "glb=1"}, StringList{"-o", "sec=1"}, StringList{}, StringList{}},
		{"empty slice override at section", StringList{"-o", "glb=1"}, StringList{}, nil, StringList{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				Global: &GlobalConfig{CascadeFields: CascadeFields{RemoteSSHOpts: tt.globalOpts}},
				Sections: []SectionConfig{{
					Title: "Test Section", CascadeFields: CascadeFields{RemoteSSHOpts: tt.secOpts},
					Services: []ServiceConfig{{
						Title: "Test Service", Command: CommandValue{"echo test"},
						CascadeFields: CascadeFields{RemoteSSHOpts: tt.svcOpts},
					}},
				}},
			}
			services := cfg.Flatten()
			if len(services) != 1 {
				t.Fatalf("expected 1 service, got %d", len(services))
			}
			got := services[0].RemoteSSHOpts
			if (tt.wantOpts == nil) != (got == nil) {
				t.Fatalf("nil mismatch: want nil=%v, got nil=%v", tt.wantOpts == nil, got == nil)
			}
			if !slices.Equal(got, tt.wantOpts) {
				t.Errorf("expected %v, got %v", tt.wantOpts, got)
			}
		})
	}
}

func TestRemoteSSHOptsFromYAML(t *testing.T) {
	// This test specifically tests YAML parsing of remote_ssh_opts, so keep raw YAML.
	yaml := `---
global:
  title: Estro
  subtitle: SSH opts test
  hostname: 0.0.0.0
  port: 3000
  remote_ssh_opts:
    - -o StrictHostKeyChecking=no
    - -o UserKnownHostsFile=/dev/null
users:
  admin:
    password: '$2y$10$hash'
sections:
  - title: Section with ssh opts
    remote_ssh_opts:
      - -o ConnectTimeout=5
    services:
      - title: Inherits section opts
        command: uptime
      - title: Overrides with own opts
        command: date
        remote_ssh_opts: ['-o', 'CustomOpt=yes']
  - title: Inherits global opts
    services:
      - title: Global opts
        command: uptime
`
	res := mustLoadYAML(t, yaml)
	services := res.Config.Flatten()

	want := map[string]StringList{
		"Inherits section opts":   {"-o ConnectTimeout=5"},
		"Overrides with own opts": {"-o", "CustomOpt=yes"},
		"Global opts":             {"-o StrictHostKeyChecking=no", "-o UserKnownHostsFile=/dev/null"},
	}
	for _, svc := range services {
		if exp, ok := want[svc.Title]; ok {
			if !slices.Equal(svc.RemoteSSHOpts, exp) {
				t.Errorf("%s: expected %v, got %v", svc.Title, exp, svc.RemoteSSHOpts)
			}
		}
	}
}

func TestRemoteSSHOpts_EdgeCases(t *testing.T) {
	t.Run("empty array is explicit override", func(t *testing.T) {
		yaml := `---
global:
  title: Estro
  remote_ssh_opts: ['-o', 'GlobalOpt=1']
users:
  admin:
    password: '$2y$10$hash'
sections:
  - title: Empty override section
    remote_ssh_opts: []
    services:
      - title: Inherits empty from section
        command: uptime
  - title: Section with opts
    remote_ssh_opts: ['-o', 'SecOpt=1']
    services:
      - title: Empty override at service
        command: date
        remote_ssh_opts: []
`
		services := mustLoadYAML(t, yaml).Config.Flatten()
		for _, svc := range services {
			switch svc.Title {
			case "Inherits empty from section", "Empty override at service":
				if svc.RemoteSSHOpts == nil || len(svc.RemoteSSHOpts) != 0 {
					t.Errorf("%s: expected empty slice, got %v", svc.Title, svc.RemoteSSHOpts)
				}
			}
		}
	})

	t.Run("nil means cascade", func(t *testing.T) {
		yaml := `---
global:
  title: Estro
  remote_ssh_opts: ['-o', 'GlobalOnly=1']
users:
  admin:
    password: '$2y$10$hash'
sections:
  - title: No override section
    services:
      - title: Cascades to global
        command: uptime
`
		services := mustLoadYAML(t, yaml).Config.Flatten()
		if len(services) != 1 {
			t.Fatalf("expected 1 service, got %d", len(services))
		}
		want := StringList{"-o", "GlobalOnly=1"}
		if !slices.Equal(services[0].RemoteSSHOpts, want) {
			t.Errorf("expected %v, got %v", want, services[0].RemoteSSHOpts)
		}
	})

	t.Run("single host remote with ssh opts", func(t *testing.T) {
		cfg := &Config{
			Global: &GlobalConfig{CascadeFields: CascadeFields{Remote: StringList{"server1.local"}, RemoteSSHOpts: StringList{"-o", "StrictHostKeyChecking=no"}}},
			Users:  map[string]*UserConfig{"admin": {Password: "$2y$10$hash"}},
			Sections: []SectionConfig{{
				Title: "Single hop",
				Services: []ServiceConfig{
					{Title: "Single host with opts", Command: CommandValue{"uptime"}},
				},
			}},
		}
		services := cfg.Flatten()
		if len(services) != 1 {
			t.Fatalf("expected 1 service, got %d", len(services))
		}
		svc := services[0]
		if !slices.Equal(svc.Remote, StringList{"server1.local"}) {
			t.Errorf("remote: expected [server1.local], got %v", svc.Remote)
		}
		if !slices.Equal(svc.RemoteSSHOpts, StringList{"-o", "StrictHostKeyChecking=no"}) {
			t.Errorf("ssh opts: expected [-o StrictHostKeyChecking=no], got %v", svc.RemoteSSHOpts)
		}
	})

	t.Run("multi-hop chain with ssh opts", func(t *testing.T) {
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
		if len(svc.RemoteSSHOpts) != 4 {
			t.Errorf("expected 4 ssh opts, got %d: %v", len(svc.RemoteSSHOpts), svc.RemoteSSHOpts)
		}
	})
}

// mustLoadYAML writes the YAML content, loads it, and fails the test if unhealthy.
func mustLoadYAML(t *testing.T, yaml string) *LoadResult {
	t.Helper()
	path := writeTestConfig(t, yaml)
	res := Load(path)
	if !res.Healthy() {
		t.Fatalf("unexpected issues: %v", res.IssueStrings())
	}
	return res
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
