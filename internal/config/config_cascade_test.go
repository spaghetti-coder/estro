package config

import (
	"os"
	"testing"
)

func TestCascadeTimeout(t *testing.T) {
	path := writeTestConfig(t, testConfigYAML())
	defer func() { _ = os.Remove(path) }()

	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	services := cfg.Flatten()

	for _, svc := range services {
		timeout := svc.GetTimeout()
		switch svc.Title {
		case "CPU":
			if timeout != 10 {
				t.Errorf("CPU: expected timeout 10, got %d", timeout)
			}
		case "Auth log":
			if timeout != 5 {
				t.Errorf("Auth log: expected timeout 5, got %d", timeout)
			}
		case "Uptime":
			if timeout != 30 {
				t.Errorf("Uptime: expected timeout 30 (global), got %d", timeout)
			}
		case "Three-hop date":
			if timeout != 20 {
				t.Errorf("Three-hop date: expected timeout 20, got %d", timeout)
			}
		}
	}
}

func TestCascadeConfirm(t *testing.T) {
	path := writeTestConfig(t, testConfigYAML())
	defer func() { _ = os.Remove(path) }()

	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	services := cfg.Flatten()

	for _, svc := range services {
		confirm := svc.GetConfirm()
		switch svc.Title {
		case "Date":
			if confirm != false {
				t.Errorf("Date: expected confirm false, got %v", confirm)
			}
		case "Uptime":
			if confirm != true {
				t.Errorf("Uptime: expected confirm true (global), got %v", confirm)
			}
		case "Syslog (20)":
			if confirm != false {
				t.Errorf("Syslog (20): expected confirm false (section), got %v", confirm)
			}
		case "Auth log":
			if confirm != true {
				t.Errorf("Auth log: expected confirm true (service override), got %v", confirm)
			}
		}
	}
}

func TestGetEnabled_DefaultTrue(t *testing.T) {
	flat := FlatService{Global: &GlobalConfig{}}
	if !flat.GetEnabled() {
		t.Error("expected default enabled to be true")
	}
}

func TestGetEnabled_ServiceOverridesSection(t *testing.T) {
	svcFalse := false
	secFalse := false
	flat := FlatService{
		Enabled:        &svcFalse,
		SectionEnabled: &secFalse,
		Global:         &GlobalConfig{},
	}
	if flat.GetEnabled() {
		t.Error("expected enabled=false when service explicitly sets false")
	}
}

func TestGetEnabled_SectionOverridesGlobal(t *testing.T) {
	secTrue := true
	glbFalse := false
	flat := FlatService{
		SectionEnabled: &secTrue,
		Global:         &GlobalConfig{Enabled: &glbFalse},
	}
	if !flat.GetEnabled() {
		t.Error("expected section enabled=true to override global enabled=false")
	}
}

func TestGetEnabled_GlobalCascade(t *testing.T) {
	glbFalse := false
	flat := FlatService{
		Global: &GlobalConfig{Enabled: &glbFalse},
	}
	if flat.GetEnabled() {
		t.Error("expected global enabled=false to cascade when no service/section override")
	}
}

func TestGetEnabled_ServiceOverridesGlobal(t *testing.T) {
	svcTrue := true
	glbFalse := false
	flat := FlatService{
		Enabled: &svcTrue,
		Global:  &GlobalConfig{Enabled: &glbFalse},
	}
	if !flat.GetEnabled() {
		t.Error("expected service enabled=true to override global enabled=false")
	}
}

func TestGetEnabled_ServiceTrueInSectionFalse(t *testing.T) {
	svcTrue := true
	secFalse := false
	flat := FlatService{
		Enabled:        &svcTrue,
		SectionEnabled: &secFalse,
		Global:         &GlobalConfig{},
	}
	if !flat.GetEnabled() {
		t.Error("expected service enabled=true to override section enabled=false")
	}
}

func TestFlatten_EnabledCascade(t *testing.T) {
	glbFalse := false
	svcTrue := true
	secFalse := false

	cfg := &Config{
		Global: &GlobalConfig{Enabled: &glbFalse},
		Sections: []SectionConfig{
			{
				Title:   "Disabled Section",
				Enabled: &secFalse,
				Services: []ServiceConfig{
					{Title: "Disabled Service", Command: CommandValue{"echo disabled"}},
					{Title: "Override Enabled", Command: CommandValue{"echo override"}, Enabled: &svcTrue},
				},
			},
			{
				Title:   "Enabled Section",
				Enabled: &svcTrue,
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

	if services[0].GetEnabled() {
		t.Error("Disabled Service: expected enabled=false")
	}
	if !services[1].GetEnabled() {
		t.Error("Override Enabled: expected enabled=true (service overrides section)")
	}
	if !services[2].GetEnabled() {
		t.Error("Normal Service: expected enabled=true (section)")
	}
	if services[3].GetEnabled() {
		t.Error("Default Service: expected enabled=false (cascaded from global)")
	}
}

func TestGetRestricted_DefaultTrue(t *testing.T) {
	flat := FlatService{Global: &GlobalConfig{}}
	if !flat.GetRestricted() {
		t.Error("expected default restricted to be true")
	}
}

func TestGetRestricted_ServiceOverridesSection(t *testing.T) {
	svcFalse := false
	secTrue := true
	flat := FlatService{
		Restricted:        &svcFalse,
		SectionRestricted: &secTrue,
		Global:            &GlobalConfig{},
	}
	if flat.GetRestricted() {
		t.Error("expected restricted=false when service explicitly sets false")
	}
}

func TestGetRestricted_SectionOverridesGlobal(t *testing.T) {
	secFalse := false
	glbTrue := true
	flat := FlatService{
		SectionRestricted: &secFalse,
		Global:            &GlobalConfig{Restricted: &glbTrue},
	}
	if flat.GetRestricted() {
		t.Error("expected section restricted=false to override global restricted=true")
	}
}

func TestGetRestricted_GlobalCascade(t *testing.T) {
	glbTrue := true
	flat := FlatService{
		Global: &GlobalConfig{Restricted: &glbTrue},
	}
	if !flat.GetRestricted() {
		t.Error("expected global restricted=true to cascade when no service/section override")
	}
}

func TestGetRestricted_ServiceOverridesGlobal(t *testing.T) {
	svcFalse := false
	glbTrue := true
	flat := FlatService{
		Restricted: &svcFalse,
		Global:     &GlobalConfig{Restricted: &glbTrue},
	}
	if flat.GetRestricted() {
		t.Error("expected service restricted=false to override global restricted=true")
	}
}

func TestGetRestricted_NilGlobal_DefaultTrue(t *testing.T) {
	flat := FlatService{}
	if !flat.GetRestricted() {
		t.Error("expected default restricted=true with nil global")
	}
}

func TestFlatten_RestrictedCascade(t *testing.T) {
	svcFalse := false
	secTrue := true
	glbFalse := false

	cfg := &Config{
		Global: &GlobalConfig{},
		Sections: []SectionConfig{
			{
				Title:      "Sec",
				Restricted: &secTrue,
				Services: []ServiceConfig{
					{Title: "Override", Command: CommandValue{"echo"}, Restricted: &svcFalse},
					{Title: "Inherit", Command: CommandValue{"date"}},
				},
			},
		},
	}
	services := cfg.Flatten()
	if services[0].GetRestricted() {
		t.Error("Override: expected restricted=false (service overrides section)")
	}
	if !services[1].GetRestricted() {
		t.Error("Inherit: expected restricted=true (inherits from section)")
	}

	cfg2 := &Config{
		Global: &GlobalConfig{Restricted: &glbFalse},
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
	if services2[0].GetRestricted() {
		t.Error("Inherit Global: expected restricted=false (inherited from global)")
	}
}

func TestGetRemoteCascadeOverride(t *testing.T) {
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

	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	services := cfg.Flatten()

	for _, svc := range services {
		remote := svc.GetRemote()
		switch svc.Title {
		case "Inherits global remote":
			if len(remote) != 1 || remote[0] != "server1.local" {
				t.Errorf("expected [server1.local], got %v", remote)
			}
		case "Local override at service":
			if len(remote) != 0 {
				t.Errorf("expected empty slice (local execution override), got %v", remote)
			}
		case "Inherits section local":
			if len(remote) != 0 {
				t.Errorf("expected empty slice (local execution from section), got %v", remote)
			}
		case "Service remote override":
			if len(remote) != 1 || remote[0] != "server2.local" {
				t.Errorf("expected [server2.local], got %v", remote)
			}
		case "Inherits section remote":
			if len(remote) != 1 || remote[0] != "server2.local" {
				t.Errorf("expected [server2.local], got %v", remote)
			}
		case "Local override in section remote":
			if len(remote) != 0 {
				t.Errorf("expected empty slice (local override), got %v", remote)
			}
		}
	}
}

func TestRemoteValueString(t *testing.T) {
	path := writeTestConfig(t, testConfigYAML())
	defer func() { _ = os.Remove(path) }()

	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	services := cfg.Flatten()

	for _, svc := range services {
		switch svc.Title {
		case "Remote uptime":
			remote := svc.GetRemote()
			if len(remote) != 1 || remote[0] != "server1.local" {
				t.Errorf("Remote uptime: expected [server1.local], got %v", remote)
			}
		case "Two-hop uptime":
			remote := svc.GetRemote()
			if len(remote) != 2 {
				t.Errorf("Two-hop uptime: expected 2 remote hosts, got %d", len(remote))
			}
		case "Three-hop date":
			remote := svc.GetRemote()
			if len(remote) != 3 {
				t.Errorf("Three-hop date: expected 3 remote hosts, got %d", len(remote))
			}
		}
	}
}