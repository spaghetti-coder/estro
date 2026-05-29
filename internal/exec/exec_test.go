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

func TestBuildCmdNoRemote(t *testing.T) {
	cmd := config.CommandValue{"uptime"}
	result, err := BuildCmd(cmd, nil, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "uptime" {
		t.Errorf("expected %q, got %q", "uptime", result)
	}
}

func TestBuildCmdSingleRemote(t *testing.T) {
	cmd := config.CommandValue{"uptime"}
	remote := config.StringList{"server1.local"}
	result, err := BuildCmd(cmd, remote, "-o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null")
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
	remote := config.StringList{"server1.local", "server2.local"}
	result, err := BuildCmd(cmd, remote, "-o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null")
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
	result, err := BuildCmd(cmd, nil, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := "df -h / && echo --- && df -h --total | tail -1"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestBuildCmdRemoteWithPort(t *testing.T) {
	cmd := config.CommandValue{"uptime"}
	remote := config.StringList{"server1.local:2222"}
	result, err := BuildCmd(cmd, remote, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := "ssh -p 2222 server1.local 'uptime'"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestBuildCmdRemoteUserHostPort(t *testing.T) {
	cmd := config.CommandValue{"uptime"}
	remote := config.StringList{"deploy@10.0.0.5:2222"}
	result, err := BuildCmd(cmd, remote, "-o StrictHostKeyChecking=no")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := "ssh -o StrictHostKeyChecking=no -p 2222 deploy@10.0.0.5 'uptime'"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestBuildCmdRemoteIPv6Port(t *testing.T) {
	cmd := config.CommandValue{"uptime"}
	remote := config.StringList{"[2001:db8::1]:2222"}
	result, err := BuildCmd(cmd, remote, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := "ssh -p 2222 2001:db8::1 'uptime'"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestBuildCmdMultiHopMixedPorts(t *testing.T) {
	cmd := config.CommandValue{"uptime"}
	remote := config.StringList{"hop1.local:2222", "target:2223"}
	result, err := BuildCmd(cmd, remote, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := "ssh -p 2222 hop1.local 'ssh -p 2223 target '\\''uptime'\\'''"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestBuildCmdNoSSHOpts(t *testing.T) {
	cmd := config.CommandValue{"uptime"}
	remote := config.StringList{"server1.local"}
	result, err := BuildCmd(cmd, remote, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := "ssh server1.local 'uptime'"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestBuildCmdCustomSSHOpts(t *testing.T) {
	cmd := config.CommandValue{"uptime"}
	remote := config.StringList{"server1.local"}
	result, err := BuildCmd(cmd, remote, "-o UserKnownHostsFile=/dev/null")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := "ssh -o UserKnownHostsFile=/dev/null server1.local 'uptime'"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
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
