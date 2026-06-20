package exec

import (
	"context"
	"testing"
	"time"

	"github.com/spaghetti-coder/estro/internal/config"
)

func TestShellEscape(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{name: "plain", input: "hello", expected: "hello"},
		{name: "single_quote", input: "don't", expected: "don'\\''t"},
		{name: "quote_and_space", input: "it's a test", expected: "it'\\''s a test"},
		{name: "empty", input: "", expected: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
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
		{name: "invalid remote propagated", cmd: config.CommandValue{"uptime"}, remote: config.StringList{""}, wantErr: true},
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

func TestRunCommand(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		cmd        string
		timeout    time.Duration
		wantErr    bool
		wantStdout string
		wantStderr string
	}{
		{name: "stdout", cmd: "echo hello", timeout: 5 * time.Second, wantStdout: "hello"},
		{name: "stderr", cmd: "echo err >&2", timeout: 5 * time.Second, wantStderr: "err"},
		{name: "non-zero exit", cmd: "exit 42", timeout: 5 * time.Second, wantErr: true},
		{name: "timeout", cmd: "sleep 10", timeout: 50 * time.Millisecond, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			stdout, stderr, err := RunCommand(context.Background(), tt.cmd, tt.timeout)
			if (err != nil) != tt.wantErr {
				t.Fatalf("RunCommand(%q) err = %v, wantErr %v", tt.cmd, err, tt.wantErr)
			}
			if tt.wantStdout != "" && stdout != tt.wantStdout {
				t.Errorf("stdout = %q, want %q", stdout, tt.wantStdout)
			}
			if tt.wantStderr != "" && stderr != tt.wantStderr {
				t.Errorf("stderr = %q, want %q", stderr, tt.wantStderr)
			}
		})
	}
}
