package config

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func configPath() string {
	_, thisFile, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(thisFile), "testdata", "config.yaml")
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

	res := Load(path)
	if !res.Healthy() {
		t.Fatalf("unexpected issues: %v", res.IssueStrings())
	}
	cfg := res.Config
	if cfg.Global == nil || cfg.Global.Title == nil || *cfg.Global.Title != "Estro" {
		t.Errorf("expected global title 'Estro', got %v", cfg.Global)
	}
}

func TestLoadRealConfigFile(t *testing.T) {
	res := Load(configPath())
	if !res.Healthy() {
		t.Fatalf("unexpected issues: %v", res.IssueStrings())
	}
	cfg := res.Config
	if cfg.Global == nil || cfg.Global.Title == nil || *cfg.Global.Title != "Estro" {
		t.Errorf("expected global title 'Estro', got %v", cfg.Global)
	}
}

func TestLoadInvalidConfig(t *testing.T) {
	content := "sections:\n  - title: ''\n    services:\n      - title: test\n        command: echo\n"
	path := writeTestConfig(t, content)
	defer func() { _ = os.Remove(path) }()

	res := Load(path)
	if res.Healthy() {
		t.Fatal("expected issues for empty required field")
	}
}

func TestLoadMissingFile(t *testing.T) {
	res := Load("/nonexistent/path/config.yaml")
	if res.Healthy() {
		t.Fatal("expected degraded")
	}
	if res.IssueStrings()[0] != "Configuration file can't be read" {
		t.Errorf("got %q", res.IssueStrings()[0])
	}
}

func TestLoadBadYAML(t *testing.T) {
	content := "sections: [{invalid yaml\n"
	path := writeTestConfig(t, content)
	defer func() { _ = os.Remove(path) }()

	res := Load(path)
	if res.Healthy() {
		t.Fatal("expected degraded for bad YAML")
	}
}

func TestConfigResponse(t *testing.T) {
	path := writeTestConfig(t, testConfigYAML())
	defer func() { _ = os.Remove(path) }()

	res := Load(path)
	if !res.Healthy() {
		t.Fatalf("unexpected issues: %v", res.IssueStrings())
	}
	resp := res.Config.GetConfigResponse()
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
