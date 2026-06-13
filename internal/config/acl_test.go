package config

import (
	"slices"
	"testing"
)

func TestIsAccessible(t *testing.T) {
	tests := []struct {
		name    string
		allowed []string
		user    string
		want    bool
	}{
		{"nil allowed with no user", nil, "", true},
		{"nil allowed with any user", nil, "alice", true},
		{"restricted to named users — allowed user", []string{"alice", "bob"}, "alice", true},
		{"restricted to named users — unknown user", []string{"alice", "bob"}, "guest", false},
		{"restricted to named users — empty user", []string{"alice", "bob"}, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flat := FlatService{Allowed: tt.allowed}
			if got := flat.IsAccessible(tt.user); got != tt.want {
				t.Errorf("IsAccessible(%q) = %v, want %v", tt.user, got, tt.want)
			}
		})
	}
}

func TestRestrictedTrue_NilAllowedIsPublic(t *testing.T) {
	flat := FlatService{Restricted: true, Allowed: nil}
	if !flat.IsAccessible("guest") {
		t.Error("restricted=true + nil allowed should be public")
	}
}

func TestRestrictedDefaultIsPublic(t *testing.T) {
	flat := FlatService{}
	if !flat.IsAccessible("") {
		t.Error("default (restricted=true, nil allowed) should be public")
	}
}

func TestResolveAllowed(t *testing.T) {
	users := map[string]*UserConfig{
		"alice": {Password: "hash", Groups: []string{"admins"}},
		"bob":   {Password: "hash", Groups: []string{"admins", "family"}},
	}

	tests := []struct {
		name  string
		input []string
		want  []string // nil means nil result
	}{
		{"nil input", nil, nil},
		{"empty slice", []string{}, nil},
		{"group expansion", []string{"admins"}, []string{"alice", "bob"}},
		{"direct username", []string{"guest"}, []string{"guest"}},
		{"nonexistent group", []string{"nonexistent-group"}, nil},
		{"username-group collision", []string{"admins"}, []string{"admins", "alice"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testUsers := users
			if tt.name == "username-group collision" {
				testUsers = map[string]*UserConfig{
					"admins": {Password: "hash", Groups: []string{}},
					"alice":  {Password: "hash", Groups: []string{"admins"}},
				}
			}
			if tt.name == "direct username" {
				testUsers = map[string]*UserConfig{"guest": {Password: "hash"}}
			}
			if tt.name == "nonexistent group" {
				testUsers = map[string]*UserConfig{"alice": {Password: "hash", Groups: []string{"admins"}}}
			}

			result := resolveAllowed(tt.input, testUsers)
			if (tt.want == nil) != (result == nil) {
				t.Errorf("nil mismatch: want nil=%v, got nil=%v", tt.want == nil, result == nil)
				return
			}
			if !slices.Equal(result, tt.want) {
				t.Errorf("got %v, want %v", result, tt.want)
			}
		})
	}
}
