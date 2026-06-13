package config

import (
	"slices"
	"testing"
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

func TestFlatten_RemoteSSHOptsEdgeCases(t *testing.T) {
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
