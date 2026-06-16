package exec

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/spaghetti-coder/estro/internal/config"
)

func TestShellEscape(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input    string
		expected string
	}{
		{"hello", "hello"},
		{"don't", "don'\\''t"},
		{"it's a test", "it'\\''s a test"},
		{"", ""},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()
			got := ShellEscape(tt.input)
			if got != tt.expected {
				t.Errorf("ShellEscape(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestBuildCmd(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		cmd     config.CommandValue
		remote  config.StringList
		sshOpts string
		want    string
		wantErr bool
	}{
		{name: "no remote", cmd: config.CommandValue{"uptime"}, want: "uptime"},
		{
			name: "array command no remote",
			cmd:  config.CommandValue{"df -h /", "echo ---", "df -h --total | tail -1"},
			want: "df -h / && echo --- && df -h --total | tail -1",
		},
		{
			name:    "single remote",
			cmd:     config.CommandValue{"uptime"},
			remote:  config.StringList{"server1.local"},
			sshOpts: "-o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null",
			want:    "ssh -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null server1.local 'uptime'",
		},
		{
			name:   "single remote no ssh opts",
			cmd:    config.CommandValue{"uptime"},
			remote: config.StringList{"server1.local"},
			want:   "ssh server1.local 'uptime'",
		},
		{
			name:    "single remote custom ssh opts",
			cmd:     config.CommandValue{"uptime"},
			remote:  config.StringList{"server1.local"},
			sshOpts: "-o UserKnownHostsFile=/dev/null",
			want:    "ssh -o UserKnownHostsFile=/dev/null server1.local 'uptime'",
		},
		{
			name:    "multi-hop remote",
			cmd:     config.CommandValue{"uptime"},
			remote:  config.StringList{"server1.local", "server2.local"},
			sshOpts: "-o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null",
			want:    "ssh -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null server1.local 'ssh -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null server2.local '\\''uptime'\\'''",
		},
		{
			name:   "remote with port",
			cmd:    config.CommandValue{"uptime"},
			remote: config.StringList{"server1.local:2222"},
			want:   "ssh -p 2222 server1.local 'uptime'",
		},
		{
			name:    "remote user host port",
			cmd:     config.CommandValue{"uptime"},
			remote:  config.StringList{"deploy@10.0.0.5:2222"},
			sshOpts: "-o StrictHostKeyChecking=no",
			want:    "ssh -o StrictHostKeyChecking=no -p 2222 deploy@10.0.0.5 'uptime'",
		},
		{
			name:   "remote IPv6 with port",
			cmd:    config.CommandValue{"uptime"},
			remote: config.StringList{"[2001:db8::1]:2222"},
			want:   "ssh -p 2222 2001:db8::1 'uptime'",
		},
		{
			name:   "multi-hop mixed ports",
			cmd:    config.CommandValue{"uptime"},
			remote: config.StringList{"hop1.local:2222", "target:2223"},
			want:   "ssh -p 2222 hop1.local 'ssh -p 2223 target '\\''uptime'\\'''",
		},
		{name: "error empty remote", cmd: config.CommandValue{"uptime"}, remote: config.StringList{""}, wantErr: true},
		{name: "error empty user", cmd: config.CommandValue{"uptime"}, remote: config.StringList{"@host"}, wantErr: true},
		{name: "error empty port", cmd: config.CommandValue{"uptime"}, remote: config.StringList{"host:"}, wantErr: true},
		{name: "error empty host", cmd: config.CommandValue{"uptime"}, remote: config.StringList{"user@"}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := BuildCmd(tt.cmd, tt.remote, tt.sshOpts)
			if (err != nil) != tt.wantErr {
				t.Fatalf("BuildCmd() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("BuildCmd() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRunCommandSuccess(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	stdout, _, err := RunCommand(ctx, "echo hello", 5*time.Second)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.TrimSpace(stdout) != "hello" {
		t.Errorf("expected stdout %q, got %q", "hello", stdout)
	}
}

func TestRunCommandStderr(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	_, stderr, err := RunCommand(ctx, "echo err >&2", 5*time.Second)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.TrimSpace(stderr) != "err" {
		t.Errorf("expected stderr %q, got %q", "err", stderr)
	}
}

func TestRunCommandNonZeroExit(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	_, _, err := RunCommand(ctx, "exit 42", 5*time.Second)
	if err == nil {
		t.Fatal("expected non-zero exit error, got nil")
	}
}

func TestRunCommandTimeout(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	_, _, err := RunCommand(ctx, "sleep 10", 50*time.Millisecond)
	if err == nil {
		t.Error("expected timeout error, got nil")
	}
}
