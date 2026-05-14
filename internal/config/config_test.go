package config

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func configPath() string {
	_, thisFile, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(thisFile), "..", "..", "config.yaml")
}

func testConfigYAML() string {
	return `---
global:
  title: Estro
  subtitle: Config test suite
  hostname: 0.0.0.0
  port: 3000
  timeout: 30
  confirm: true
users:
  alice:
    password: '$2y$10$hash1'
    groups: [admins]
  bob:
    password: '$2y$10$hash2'
    groups: [admins, family]
  guest:
    password: '$2y$10$hash3'
sections:
  - title: Public Info
    collapsable: false
    columns: 1
    services:
      - title: Uptime
        command: uptime
      - title: Date
        command: date
        confirm: false
  - title: System
    services:
      - title: Disk usage
        command:
          - df -h /
          - echo "---"
          - df -h --total | tail -1
      - title: Memory
        command: free -h
        confirm: false
      - title: CPU
        command: ps aux --sort=-%cpu | head -10
        timeout: 10
      - title: Load
        command: cat /proc/loadavg
        confirm: false
  - title: Logs
    timeout: 15
    confirm: false
    columns: 2
    services:
      - title: Syslog (20)
        command: journalctl -n 20 --no-pager
      - title: Kernel log
        command: dmesg | tail -20
      - title: Auth log
        command: journalctl -n 20 -u ssh --no-pager
        confirm: true
        timeout: 5
  - title: Admin
    allowed: [admins]
    columns: 4
    services:
      - title: List /etc
        command: ls -lah /etc
      - title: Who
        command: who
        confirm: false
  - title: Mixed Access
    allowed: [admins]
    services:
      - title: Public status
        command: uptime
        allowed: []
        confirm: false
      - title: Admin only
        command: id
      - title: Guest allowed
        command: date
        allowed: [guest]
  - title: Remote (single hop)
    allowed: [admins]
    remote: server1.local
    confirm: false
    services:
      - title: Remote uptime
        command: uptime
      - title: Remote disk
        command: df -h /
      - title: Local override
        command: hostname
        allowed: [admins, guest]
  - title: Remote (chain)
    allowed: [admins]
    collapsable: false
    columns: 2
    services:
      - title: Two-hop uptime
        command: uptime
        remote: [server1.local, server2.local]
        confirm: false
      - title: Three-hop date
        command: date
        remote: [server1.local, server2.local, server3.local]
        confirm: false
        timeout: 20
`
}

func writeTestConfig(t *testing.T, content string) string {
	t.Helper()
	tmp, err := os.CreateTemp("", "config-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tmp.WriteString(content); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
		t.Fatal(err)
	}
	_ = tmp.Close()
	return tmp.Name()
}

func TestLoadValidConfig(t *testing.T) {
	path := writeTestConfig(t, testConfigYAML())
	defer func() { _ = os.Remove(path) }()

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("expected no error loading valid config, got: %v", err)
	}
	if cfg.Global == nil || cfg.Global.Title == nil || *cfg.Global.Title != "Estro" {
		t.Errorf("expected global title 'Estro', got %v", cfg.Global)
	}
}

func TestLoadRealConfigFile(t *testing.T) {
	cfg, err := Load(configPath())
	if err != nil {
		t.Fatalf("expected no error loading real config.yaml, got: %v", err)
	}
	if cfg.Global == nil || cfg.Global.Title == nil || *cfg.Global.Title != "Estro" {
		t.Errorf("expected global title 'Estro', got %v", cfg.Global)
	}
}

func TestLoadInvalidConfig(t *testing.T) {
	content := "sections:\n  - title: ''\n    services:\n      - title: test\n        command: echo\n"
	path := writeTestConfig(t, content)
	defer func() { _ = os.Remove(path) }()

	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for config with empty required field, got nil")
	}
}

func TestLoadMissingFile(t *testing.T) {
	_, err := Load("/nonexistent/path/config.yaml")
	if err == nil {
		t.Fatal("expected error for missing config file, got nil")
	}
	if !strings.Contains(err.Error(), "reading config file") {
		t.Errorf("expected error about reading config file, got: %v", err)
	}
}

func TestLoadBadYAML(t *testing.T) {
	content := "sections: [{invalid yaml\n"
	path := writeTestConfig(t, content)
	defer func() { _ = os.Remove(path) }()

	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for bad YAML, got nil")
	}
	if !strings.Contains(err.Error(), "parsing config YAML") {
		t.Errorf("expected YAML parse error, got: %v", err)
	}
}

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
	if !contains(result, "alice") || !contains(result, "bob") {
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

func TestSerializeService(t *testing.T) {
	path := writeTestConfig(t, testConfigYAML())
	defer func() { _ = os.Remove(path) }()

	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	services := cfg.Flatten()

	for i, svc := range services {
		serialized := svc.Serialize(i, "alice", cfg.Users)
		if serialized.ID != i {
			t.Errorf("expected id %d, got %d", i, serialized.ID)
		}
		if serialized.Title != svc.Title {
			t.Errorf("expected title %s, got %s", svc.Title, serialized.Title)
		}
		if serialized.Timeout != svc.GetTimeoutMs()+10000 {
			t.Errorf("expected timeout %d, got %d", svc.GetTimeoutMs()+10000, serialized.Timeout)
		}
		if serialized.Confirm != svc.GetConfirm() {
			t.Errorf("expected confirm %v, got %v", svc.GetConfirm(), serialized.Confirm)
		}
		if serialized.Section == nil || *serialized.Section != svc.SectionTitle {
			t.Errorf("expected section %s, got %v", svc.SectionTitle, serialized.Section)
		}
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

func TestConfigResponse(t *testing.T) {
	path := writeTestConfig(t, testConfigYAML())
	defer func() { _ = os.Remove(path) }()

	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	resp := cfg.GetConfigResponse()
	if resp.Title != "Estro" {
		t.Errorf("expected title 'Estro', got %s", resp.Title)
	}
	if resp.Subtitle != "Config test suite" {
		t.Errorf("expected subtitle 'Config test suite', got %s", resp.Subtitle)
	}
	if len(resp.Users) != 3 {
		t.Errorf("expected 3 users, got %d", len(resp.Users))
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