package exec

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/spaghetti-coder/estro/internal/config"
)

func TestShellEscape(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"hello", "hello"},
		{"don't", "don'\\''t"},
		{"it's a test", "it'\\''s a test"},
		{"simple", "simple"},
		{"", ""},
	}
	for _, tt := range tests {
		got := ShellEscape(tt.input)
		if got != tt.expected {
			t.Errorf("ShellEscape(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestValidateHostAccepts(t *testing.T) {
	hosts := []string{"server1.local", "user@host", "192.168.1.1", "host:22", "my-server.local"}
	for _, h := range hosts {
		if err := ValidateHost(h); err != nil {
			t.Errorf("ValidateHost(%q) returned unexpected error: %v", h, err)
		}
	}
}

func TestValidateHostRejects(t *testing.T) {
	hosts := []string{"host with space", "host;rm -rf", "host|pipe", "host`cmd`", "host$(cmd)"}
	for _, h := range hosts {
		if err := ValidateHost(h); err == nil {
			t.Errorf("ValidateHost(%q) expected error, got nil", h)
		}
	}
}

func TestBuildCmdNoRemote(t *testing.T) {
	cmd := config.CommandValue{"uptime"}
	result, err := BuildCmd(cmd, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "uptime" {
		t.Errorf("expected %q, got %q", "uptime", result)
	}
}

func TestBuildCmdEmptyRemote(t *testing.T) {
	cmd := config.CommandValue{"uptime"}
	result, err := BuildCmd(cmd, config.RemoteValue{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "uptime" {
		t.Errorf("expected %q, got %q", "uptime", result)
	}
}

func TestBuildCmdSingleRemote(t *testing.T) {
	cmd := config.CommandValue{"uptime"}
	remote := config.RemoteValue{"server1.local"}
	result, err := BuildCmd(cmd, remote)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := "ssh -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null server1.local 'uptime'"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestBuildCmdMultiHopRemote(t *testing.T) {
	cmd := config.CommandValue{"uptime"}
	remote := config.RemoteValue{"server1.local", "server2.local"}
	result, err := BuildCmd(cmd, remote)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := "ssh -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null server1.local 'ssh -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null server2.local '\\''uptime'\\'''"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestBuildCmdArrayCommand(t *testing.T) {
	cmd := config.CommandValue{"df -h /", "echo ---", "df -h --total | tail -1"}
	result, err := BuildCmd(cmd, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := "df -h / && echo --- && df -h --total | tail -1"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestBuildCmdInvalidHost(t *testing.T) {
	cmd := config.CommandValue{"uptime"}
	remote := config.RemoteValue{"host with space"}
	_, err := BuildCmd(cmd, remote)
	if err == nil {
		t.Error("expected error for invalid host, got nil")
	}
}

func TestRunCommandSuccess(t *testing.T) {
	ctx := context.Background()
	stdout, _, err := RunCommand(ctx, "echo hello", 5*time.Second)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.TrimSpace(stdout) != "hello" {
		t.Errorf("expected stdout %q, got %q", "hello", stdout)
	}
}

func TestRunCommandTimeout(t *testing.T) {
	ctx := context.Background()
	_, _, err := RunCommand(ctx, "sleep 5", 200*time.Millisecond)
	if err == nil {
		t.Error("expected timeout error, got nil")
	}
}