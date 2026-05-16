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
  restricted: false
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

func TestGlobalConfigAddr(t *testing.T) {
	ptrStr := func(s string) *string { return &s }
	ptrInt := func(i int) *int { return &i }

	tests := []struct {
		name   string
		global *GlobalConfig
		want   string
	}{
		{"defaults", &GlobalConfig{}, "127.0.0.1:3000"},
		{"custom hostname", &GlobalConfig{Hostname: ptrStr("0.0.0.0")}, "0.0.0.0:3000"},
		{"custom port", &GlobalConfig{Port: ptrInt(8080)}, "127.0.0.1:8080"},
		{"both custom", &GlobalConfig{Hostname: ptrStr("0.0.0.0"), Port: ptrInt(8080)}, "0.0.0.0:8080"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.global.Addr(); got != tt.want {
				t.Errorf("Addr() = %q, want %q", got, tt.want)
			}
		})
	}
}
