package config

import (
	"strings"
	"testing"
)

func TestSplitRemoteHost(t *testing.T) {
	tests := []struct {
		in               string
		user, host, port string
		wantErr          bool
	}{
		{in: "server1.local", host: "server1.local"},
		{in: "deploy@server1.local", user: "deploy", host: "server1.local"},
		{in: "deploy@10.0.0.5:2222", user: "deploy", host: "10.0.0.5", port: "2222"},
		{in: "10.0.0.5:22", host: "10.0.0.5", port: "22"},
		{in: "[2001:db8::1]:2222", host: "2001:db8::1", port: "2222"},
		{in: "2001:db8::1", host: "2001:db8::1"},
		{in: "[::1]", host: "::1"},
		{in: "host:ssh", host: "host", port: "ssh"}, // structurally fine; range check is the validator's job
		{in: "", wantErr: true},
		{in: "host:", wantErr: true},
		{in: "@host", wantErr: true},
		{in: "user@", wantErr: true},
		{in: "[::1", wantErr: true},
		{in: "[]", wantErr: true},
		{in: "[]:22", wantErr: true},
		{in: "user@host@other", wantErr: true},
	}
	for _, tt := range tests {
		got, err := SplitRemoteHost(tt.in)
		if tt.wantErr {
			if err == nil {
				t.Errorf("SplitRemoteHost(%q) expected error, got %+v", tt.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("SplitRemoteHost(%q) unexpected error: %v", tt.in, err)
			continue
		}
		if got.User != tt.user || got.Host != tt.host || got.Port != tt.port {
			t.Errorf("SplitRemoteHost(%q) = %+v, want {User:%q Host:%q Port:%q}", tt.in, got, tt.user, tt.host, tt.port)
		}
	}
}

func TestRemoteHostTarget(t *testing.T) {
	if got := (RemoteHost{Host: "h"}).Target(); got != "h" {
		t.Errorf("Target() = %q, want %q", got, "h")
	}
	if got := (RemoteHost{User: "u", Host: "h"}).Target(); got != "u@h" {
		t.Errorf("Target() = %q, want %q", got, "u@h")
	}
}

func TestRemoteHostValidationViaStruct(t *testing.T) {
	valid := []StringList{
		nil,
		{},
		{"server1.local"},
		{"deploy@server1.local"},
		{"deploy@10.0.0.5:2222"},
		{"10.0.0.5:22"},
		{"[2001:db8::1]:2222"},
		{"2001:db8::1"},
		{"host1", "host2"}, // comma scalar is unmarshalled to this
	}
	for _, rem := range valid {
		cfg := Config{Sections: []SectionConfig{{
			Title:         "s",
			CascadeFields: CascadeFields{Remote: rem},
			Services:      []ServiceConfig{{Title: "svc", Command: CommandValue{"true"}}},
		}}}
		if issues := Validate(&cfg); len(issues) > 0 {
			t.Errorf("remote %v: unexpected validation error: %v", rem, issues)
		}
	}

	invalid := []StringList{
		{"host:"},
		{"host:0"},
		{"host:99999"},
		{"host:ssh"},
		{"@host"},
		{"user@"},
		{"a/b/c"},
		{"host with space"},
		{"BAD_USER@host"},
		{"machine$@host"},                   // trailing '$' would shell-expand as "$@"
		{strings.Repeat("a", 33) + "@host"}, // username over the 32-char cap
		{"host1", ""},                       // empty element inside a sequence is invalid
	}
	for _, rem := range invalid {
		cfg := Config{Sections: []SectionConfig{{
			Title:         "s",
			CascadeFields: CascadeFields{Remote: rem},
			Services:      []ServiceConfig{{Title: "svc", Command: CommandValue{"true"}}},
		}}}
		if issues := Validate(&cfg); len(issues) == 0 {
			t.Errorf("remote %v: expected validation error, got nil", rem)
		}
	}
}
