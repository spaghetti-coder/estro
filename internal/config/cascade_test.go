package config

import (
	"os"
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
				if len(w.Val) != len(tt.field) {
					t.Errorf("expected len %d, got len %d (%v)", len(tt.field), len(w.Val), w.Val)
				}
				for i := range tt.field {
					if w.Val[i] != tt.field[i] {
						t.Errorf("element %d: expected %q, got %q", i, tt.field[i], w.Val[i])
					}
				}
			}
		})
	}
}

func TestStringListUnmarshalInvalid(t *testing.T) {
	yamlStr := "val:\n  key: value\n"
	type wrapper struct {
		Val StringList `yaml:"val,omitempty"`
	}
	var w wrapper
	err := yaml.Load([]byte(yamlStr), &w)
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
			if len(got) != len(tt.want) {
				t.Fatalf("cascadeStringList(%v, %v, %v) = %v, want %v", tt.svc, tt.sec, tt.global, got, tt.want)
			}
			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Fatalf("cascadeStringList(%v, %v, %v)[%d] = %q, want %q", tt.svc, tt.sec, tt.global, i, got[i], tt.want[i])
				}
			}
			if tt.want == nil && got != nil {
				t.Fatalf("cascadeStringList(%v, %v, %v) = %v, want nil", tt.svc, tt.sec, tt.global, got)
			}
		})
	}
}

func TestCascadeFields(t *testing.T) {
	path := writeTestConfig(t, testConfigYAML())
	defer func() { _ = os.Remove(path) }()

	res := Load(path)
	if !res.Healthy() {
		t.Fatalf("unexpected issues: %v", res.IssueStrings())
	}
	cfg := res.Config
	services := cfg.Flatten()

	tests := []struct {
		field     string
		service   string
		wantInt   int
		wantBool  bool
		checkInt  bool
		checkBool bool
	}{
		{"timeout", "CPU", 10, false, true, false},
		{"timeout", "Auth log", 5, false, true, false},
		{"timeout", "Uptime", 30, false, true, false},
		{"timeout", "Three-hop date", 20, false, true, false},
		{"confirm", "Date", 0, false, false, true},
		{"confirm", "Uptime", 0, true, false, true},
		{"confirm", "Syslog (20)", 0, false, false, true},
		{"confirm", "Auth log", 0, true, false, true},
	}

	for _, tt := range tests {
		t.Run(tt.field+"/"+tt.service, func(t *testing.T) {
			var found bool
			for _, svc := range services {
				if svc.Title != tt.service {
					continue
				}
				found = true
				if tt.checkInt && svc.Timeout != tt.wantInt {
					t.Errorf("%s: expected %s %d, got %d", tt.service, tt.field, tt.wantInt, svc.Timeout)
				}
				if tt.checkBool && svc.Confirm != tt.wantBool {
					t.Errorf("%s: expected %s %v, got %v", tt.service, tt.field, tt.wantBool, svc.Confirm)
				}
				break
			}
			if !found {
				t.Errorf("service %q not found", tt.service)
			}
		})
	}
}

func TestFlatten_EnabledCascade(t *testing.T) {
	glbFalse := false
	svcTrue := true
	secFalse := false

	cfg := &Config{
		Global: &GlobalConfig{CascadeFields: CascadeFields{Enabled: &glbFalse}},
		Sections: []SectionConfig{
			{
				Title:         "Disabled Section",
				CascadeFields: CascadeFields{Enabled: &secFalse},
				Services: []ServiceConfig{
					{Title: "Disabled Service", Command: CommandValue{"echo disabled"}},
					{Title: "Override Enabled", Command: CommandValue{"echo override"}, CascadeFields: CascadeFields{Enabled: &svcTrue}},
				},
			},
			{
				Title:         "Enabled Section",
				CascadeFields: CascadeFields{Enabled: &svcTrue},
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

	if services[0].Enabled {
		t.Error("Disabled Service: expected enabled=false")
	}
	if !services[1].Enabled {
		t.Error("Override Enabled: expected enabled=true (service overrides section)")
	}
	if !services[2].Enabled {
		t.Error("Normal Service: expected enabled=true (section)")
	}
	if services[3].Enabled {
		t.Error("Default Service: expected enabled=false (cascaded from global)")
	}
}

func TestFlatten_RestrictedCascade(t *testing.T) {
	svcFalse := false
	secTrue := true

	cfg := &Config{
		Global: &GlobalConfig{},
		Sections: []SectionConfig{
			{
				Title:         "Sec",
				CascadeFields: CascadeFields{Restricted: &secTrue},
				Services: []ServiceConfig{
					{Title: "Override", Command: CommandValue{"echo"}, CascadeFields: CascadeFields{Restricted: &svcFalse}},
					{Title: "Inherit", Command: CommandValue{"date"}},
				},
			},
		},
	}
	services := cfg.Flatten()
	if services[0].Restricted {
		t.Error("Override: expected restricted=false (service overrides section)")
	}
	if !services[1].Restricted {
		t.Error("Inherit: expected restricted=true (inherits from section)")
	}

	glbFalse := false
	cfg2 := &Config{
		Global: &GlobalConfig{CascadeFields: CascadeFields{Restricted: &glbFalse}},
		Sections: []SectionConfig{
			{
				Title: "Sec",
				Services: []ServiceConfig{
					{Title: "Inherit Global", Command: CommandValue{"date"}},
				},
			},
		},
	}
	services2 := cfg2.Flatten()
	if services2[0].Restricted {
		t.Error("Inherit Global: expected restricted=false (inherited from global)")
	}
}

func TestRemoteCascade(t *testing.T) {
	t.Run("remote_override", func(t *testing.T) {
		yaml := `---
global:
  title: Estro
  subtitle: Remote override test
  hostname: 0.0.0.0
  port: 3000
  timeout: 30
  remote: server1.local
users:
  admin:
    password: '$2y$10$hash'
sections:
  - title: Section with global remote
    services:
      - title: Inherits global remote
        command: uptime
      - title: Local override at service
        command: hostname
        remote: []
  - title: Section with local override
    remote: []
    services:
      - title: Inherits section local
        command: date
      - title: Service remote override
        command: uptime
        remote: server2.local
  - title: Section with section remote
    remote: server2.local
    services:
      - title: Inherits section remote
        command: uptime
      - title: Local override in section remote
        command: hostname
        remote: []
`
		path := writeTestConfig(t, yaml)
		defer func() { _ = os.Remove(path) }()

		res := Load(path)
		if !res.Healthy() {
			t.Fatalf("unexpected issues: %v", res.IssueStrings())
		}
		cfg := res.Config
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
				for _, svc := range services {
					if svc.Title != tt.service {
						continue
					}
					if len(svc.Remote) != len(tt.want) {
						t.Errorf("%s: expected len %d, got %d (%v)", tt.service, len(tt.want), len(svc.Remote), svc.Remote)
						return
					}
					for i := range tt.want {
						if svc.Remote[i] != tt.want[i] {
							t.Errorf("%s: expected %v, got %v", tt.service, tt.want, svc.Remote)
							return
						}
					}
					return
				}
				t.Errorf("service %q not found", tt.service)
			})
		}
	})

	t.Run("remote_chain", func(t *testing.T) {
		path := writeTestConfig(t, testConfigYAML())
		defer func() { _ = os.Remove(path) }()

		res := Load(path)
		if !res.Healthy() {
			t.Fatalf("unexpected issues: %v", res.IssueStrings())
		}
		cfg := res.Config
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
				for _, svc := range services {
					if svc.Title != tt.service {
						continue
					}
					if len(svc.Remote) != tt.wantLen {
						t.Errorf("%s: expected %d remote hosts, got %d", tt.service, tt.wantLen, len(svc.Remote))
						return
					}
					if tt.wantHost != "" && svc.Remote[0] != tt.wantHost {
						t.Errorf("%s: expected first host %q, got %q", tt.service, tt.wantHost, svc.Remote[0])
					}
					return
				}
				t.Errorf("service %q not found", tt.service)
			})
		}
	})
}

func TestCascadeRemoteSSHOpts(t *testing.T) {
	t.Run("nil means nil (unset)", func(t *testing.T) {
		svc := FlatService{
			Title:         "t",
			Command:       CommandValue{"echo"},
			RemoteSSHOpts: nil,
		}
		if opts := svc.RemoteSSHOpts; opts != nil {
			t.Errorf("expected nil, got %v", opts)
		}
	})

	t.Run("service overrides section", func(t *testing.T) {
		svc := FlatService{
			Title:         "t",
			Command:       CommandValue{"echo"},
			RemoteSSHOpts: StringList{"-o", "svc_opt"},
		}
		opts := svc.RemoteSSHOpts
		if len(opts) != 2 || opts[0] != "-o" || opts[1] != "svc_opt" {
			t.Errorf("expected [-o svc_opt], got %v", opts)
		}
	})

	t.Run("section overrides global", func(t *testing.T) {
		svc := FlatService{
			Title:         "t",
			Command:       CommandValue{"echo"},
			RemoteSSHOpts: StringList{"-o", "sec_opt"},
		}
		opts := svc.RemoteSSHOpts
		if len(opts) != 2 || opts[0] != "-o" || opts[1] != "sec_opt" {
			t.Errorf("expected [-o sec_opt], got %v", opts)
		}
	})

	t.Run("falls back to global", func(t *testing.T) {
		svc := FlatService{
			Title:         "t",
			Command:       CommandValue{"echo"},
			RemoteSSHOpts: StringList{"-o", "glb_opt"},
		}
		opts := svc.RemoteSSHOpts
		if len(opts) != 2 || opts[0] != "-o" || opts[1] != "glb_opt" {
			t.Errorf("expected [-o glb_opt], got %v", opts)
		}
	})

	t.Run("empty slice is explicit override (no opts)", func(t *testing.T) {
		svc := FlatService{
			Title:         "t",
			Command:       CommandValue{"echo"},
			RemoteSSHOpts: StringList{},
		}
		opts := svc.RemoteSSHOpts
		if opts == nil {
			t.Errorf("expected empty slice, got nil")
		}
		if len(opts) != 0 {
			t.Errorf("expected empty slice, got %v", opts)
		}
	})
}

func TestRemoteSSHOptsFromYAML(t *testing.T) {
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
	path := writeTestConfig(t, yaml)
	defer func() { _ = os.Remove(path) }()

	res := Load(path)
	if !res.Healthy() {
		t.Fatalf("unexpected issues: %v", res.IssueStrings())
	}
	cfg := res.Config
	services := cfg.Flatten()

	for _, svc := range services {
		switch svc.Title {
		case "Inherits section opts":
			opts := svc.RemoteSSHOpts
			if len(opts) != 1 || opts[0] != "-o ConnectTimeout=5" {
				t.Errorf("expected [-o ConnectTimeout=5], got %v", opts)
			}
		case "Overrides with own opts":
			opts := svc.RemoteSSHOpts
			if len(opts) != 2 || opts[0] != "-o" || opts[1] != "CustomOpt=yes" {
				t.Errorf("expected [-o CustomOpt=yes], got %v", opts)
			}
		case "Global opts":
			opts := svc.RemoteSSHOpts
			if len(opts) != 2 {
				t.Errorf("expected 2 opts, got %v", opts)
			}
		}
	}
}

func TestCascadeRemoteSSHOpts_Programmatic(t *testing.T) {
	tests := []struct {
		name       string
		globalOpts StringList
		secOpts    StringList
		svcOpts    StringList
		wantOpts   StringList
	}{
		{
			name:       "nil → nil → nil",
			globalOpts: nil,
			secOpts:    nil,
			svcOpts:    nil,
			wantOpts:   nil,
		},
		{
			name:       "nil → nil → override",
			globalOpts: nil,
			secOpts:    nil,
			svcOpts:    StringList{"-o", "svc=1"},
			wantOpts:   StringList{"-o", "svc=1"},
		},
		{
			name:       "nil → override → nil",
			globalOpts: nil,
			secOpts:    StringList{"-o", "sec=1"},
			svcOpts:    nil,
			wantOpts:   StringList{"-o", "sec=1"},
		},
		{
			name:       "nil → override → override",
			globalOpts: nil,
			secOpts:    StringList{"-o", "sec=1"},
			svcOpts:    StringList{"-o", "svc=1"},
			wantOpts:   StringList{"-o", "svc=1"},
		},
		{
			name:       "override → nil → nil",
			globalOpts: StringList{"-o", "glb=1"},
			secOpts:    nil,
			svcOpts:    nil,
			wantOpts:   StringList{"-o", "glb=1"},
		},
		{
			name:       "override → nil → override",
			globalOpts: StringList{"-o", "glb=1"},
			secOpts:    nil,
			svcOpts:    StringList{"-o", "svc=1"},
			wantOpts:   StringList{"-o", "svc=1"},
		},
		{
			name:       "override → override → nil",
			globalOpts: StringList{"-o", "glb=1"},
			secOpts:    StringList{"-o", "sec=1"},
			svcOpts:    nil,
			wantOpts:   StringList{"-o", "sec=1"},
		},
		{
			name:       "override → override → override",
			globalOpts: StringList{"-o", "glb=1"},
			secOpts:    StringList{"-o", "sec=1"},
			svcOpts:    StringList{"-o", "svc=1"},
			wantOpts:   StringList{"-o", "svc=1"},
		},
		{
			name:       "empty slice override at service",
			globalOpts: StringList{"-o", "glb=1"},
			secOpts:    StringList{"-o", "sec=1"},
			svcOpts:    StringList{},
			wantOpts:   StringList{},
		},
		{
			name:       "empty slice override at section",
			globalOpts: StringList{"-o", "glb=1"},
			secOpts:    StringList{},
			svcOpts:    nil,
			wantOpts:   StringList{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				Global: &GlobalConfig{
					CascadeFields: CascadeFields{
						RemoteSSHOpts: tt.globalOpts,
					},
				},
				Sections: []SectionConfig{
					{
						Title:         "Test Section",
						CascadeFields: CascadeFields{RemoteSSHOpts: tt.secOpts},
						Services: []ServiceConfig{
							{
								Title:         "Test Service",
								Command:       CommandValue{"echo test"},
								CascadeFields: CascadeFields{RemoteSSHOpts: tt.svcOpts},
							},
						},
					},
				},
			}

			services := cfg.Flatten()
			if len(services) != 1 {
				t.Fatalf("expected 1 service, got %d", len(services))
			}

			got := services[0].RemoteSSHOpts

			if len(got) != len(tt.wantOpts) {
				t.Fatalf("expected %d opts, got %d: want %v, got %v", len(tt.wantOpts), len(got), tt.wantOpts, got)
			}

			for i := range tt.wantOpts {
				if got[i] != tt.wantOpts[i] {
					t.Errorf("opt[%d]: expected %q, got %q", i, tt.wantOpts[i], got[i])
				}
			}
		})
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
		path := writeTestConfig(t, yaml)
		defer func() { _ = os.Remove(path) }()

		res := Load(path)
		if !res.Healthy() {
			t.Fatalf("unexpected issues: %v", res.IssueStrings())
		}
		cfg := res.Config
		services := cfg.Flatten()

		for _, svc := range services {
			switch svc.Title {
			case "Inherits empty from section":
				if svc.RemoteSSHOpts == nil {
					t.Error("expected empty slice (not nil) from section override")
				}
				if len(svc.RemoteSSHOpts) != 0 {
					t.Errorf("expected empty slice, got %v", svc.RemoteSSHOpts)
				}
			case "Empty override at service":
				if svc.RemoteSSHOpts == nil {
					t.Error("expected empty slice (not nil) from service override")
				}
				if len(svc.RemoteSSHOpts) != 0 {
					t.Errorf("expected empty slice, got %v", svc.RemoteSSHOpts)
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
		path := writeTestConfig(t, yaml)
		defer func() { _ = os.Remove(path) }()

		res := Load(path)
		if !res.Healthy() {
			t.Fatalf("unexpected issues: %v", res.IssueStrings())
		}
		cfg := res.Config
		services := cfg.Flatten()

		if len(services) != 1 {
			t.Fatalf("expected 1 service, got %d", len(services))
		}

		opts := services[0].RemoteSSHOpts
		if len(opts) != 2 || opts[0] != "-o" || opts[1] != "GlobalOnly=1" {
			t.Errorf("expected [-o GlobalOnly=1], got %v", opts)
		}
	})

	t.Run("single host remote with ssh opts", func(t *testing.T) {
		yaml := `---
global:
  title: Estro
  remote: server1.local
  remote_ssh_opts: ['-o', 'StrictHostKeyChecking=no']
users:
  admin:
    password: '$2y$10$hash'
sections:
  - title: Single hop
    services:
      - title: Single host with opts
        command: uptime
`
		path := writeTestConfig(t, yaml)
		defer func() { _ = os.Remove(path) }()

		res := Load(path)
		if !res.Healthy() {
			t.Fatalf("unexpected issues: %v", res.IssueStrings())
		}
		cfg := res.Config
		services := cfg.Flatten()

		if len(services) != 1 {
			t.Fatalf("expected 1 service, got %d", len(services))
		}

		svc := services[0]
		if len(svc.Remote) != 1 || svc.Remote[0] != "server1.local" {
			t.Errorf("expected remote [server1.local], got %v", svc.Remote)
		}
		if len(svc.RemoteSSHOpts) != 2 || svc.RemoteSSHOpts[0] != "-o" || svc.RemoteSSHOpts[1] != "StrictHostKeyChecking=no" {
			t.Errorf("expected ssh opts [-o StrictHostKeyChecking=no], got %v", svc.RemoteSSHOpts)
		}
	})

	t.Run("multi-hop chain with ssh opts", func(t *testing.T) {
		yaml := `---
global:
  title: Estro
  remote: [hop1, hop2, target]
  remote_ssh_opts: ['-o', 'ForwardAgent=no', '-o', 'Compression=yes']
users:
  admin:
    password: '$2y$10$hash'
sections:
  - title: Multi-hop
    services:
      - title: Three hop chain
        command: uptime
`
		path := writeTestConfig(t, yaml)
		defer func() { _ = os.Remove(path) }()

		res := Load(path)
		if !res.Healthy() {
			t.Fatalf("unexpected issues: %v", res.IssueStrings())
		}
		cfg := res.Config
		services := cfg.Flatten()

		if len(services) != 1 {
			t.Fatalf("expected 1 service, got %d", len(services))
		}

		svc := services[0]
		if len(svc.Remote) != 3 {
			t.Errorf("expected 3 remote hosts, got %d: %v", len(svc.Remote), svc.Remote)
		}
		expectedRemote := []string{"hop1", "hop2", "target"}
		for i, exp := range expectedRemote {
			if svc.Remote[i] != exp {
				t.Errorf("remote[%d]: expected %q, got %q", i, exp, svc.Remote[i])
			}
		}
		if len(svc.RemoteSSHOpts) != 4 {
			t.Errorf("expected 4 ssh opts, got %d: %v", len(svc.RemoteSSHOpts), svc.RemoteSSHOpts)
		}
	})
}
