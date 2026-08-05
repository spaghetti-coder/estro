package config

import (
	"fmt"
	"strings"
	"testing"
)

func TestSplitRemoteHost(t *testing.T) {
	tests := []struct {
		name             string
		in               string
		user, host, port string
		wantErr          bool
	}{
		{name: "bare host", in: "server1.local", host: "server1.local"},
		{name: "user@host", in: "deploy@server1.local", user: "deploy", host: "server1.local"},
		{name: "user@host:port", in: "deploy@10.0.0.5:2222", user: "deploy", host: "10.0.0.5", port: "2222"},
		{name: "host:port", in: "10.0.0.5:22", host: "10.0.0.5", port: "22"},
		{name: "ipv6 bracket port", in: "[2001:db8::1]:2222", host: "2001:db8::1", port: "2222"},
		{name: "bare ipv6", in: "2001:db8::1", host: "2001:db8::1"},
		{name: "loopback bracket", in: "[::1]", host: "::1"},
		{name: "non-numeric port", in: "host:ssh", host: "host", port: "ssh"}, // structurally fine; range check is the validator's job
		{name: "empty", in: "", wantErr: true},
		{name: "trailing colon", in: "host:", wantErr: true},
		{name: "leading at", in: "@host", wantErr: true},
		{name: "trailing at", in: "user@", wantErr: true},
		{name: "unclosed bracket", in: "[::1", wantErr: true},
		{name: "empty bracket", in: "[]", wantErr: true},
		{name: "empty bracket port", in: "[]:22", wantErr: true},
		{name: "double at", in: "user@host@other", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := SplitRemoteHost(tt.in)
			if tt.wantErr {
				if err == nil {
					t.Errorf("SplitRemoteHost(%q) expected error, got %+v", tt.in, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("SplitRemoteHost(%q) unexpected error: %v", tt.in, err)
			}
			if got.User != tt.user || got.Host != tt.host || got.Port != tt.port {
				t.Errorf("got %+v, want {User:%q Host:%q Port:%q}", got, tt.user, tt.host, tt.port)
			}
		})
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
		t.Run(fmt.Sprintf("valid/%q", rem), func(t *testing.T) {
			cfg := Config{Sections: []SectionConfig{{
				Title:         "s",
				CascadeFields: CascadeFields{Remote: rem},
				Services:      []ServiceConfig{{Title: "svc", Command: CommandValue{"true"}}},
			}}}
			if issues := Validate(&cfg); len(issues) > 0 {
				t.Errorf("unexpected validation error: %v", issues)
			}
		})
	}

	invalid := []StringList{
		{"host:0"},
		{"host:99999"},
		{"host:ssh"},
		{"a/b/c"},
		{"host with space"},
		{"BAD_USER@host"},
		{"machine$@host"},                   // trailing '$' would shell-expand as "$@"
		{strings.Repeat("a", 33) + "@host"}, // username over the 32-char cap
		{"host1", ""},                       // empty element inside a sequence is invalid
		{"user@[::1:22"},                    // unclosed IPv6 bracket → parse fail
	}
	for _, rem := range invalid {
		t.Run(fmt.Sprintf("invalid/%q", rem), func(t *testing.T) {
			cfg := Config{Sections: []SectionConfig{{
				Title:         "s",
				CascadeFields: CascadeFields{Remote: rem},
				Services:      []ServiceConfig{{Title: "svc", Command: CommandValue{"true"}}},
			}}}
			if issues := Validate(&cfg); len(issues) == 0 {
				t.Errorf("expected validation error, got nil")
			}
		})
	}
}
