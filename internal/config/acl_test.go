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

func TestResolveAllowed(t *testing.T) {
	defaultUsers := map[string]*UserConfig{
		"alice": {Password: "hash", Groups: []string{"admins"}},
		"bob":   {Password: "hash", Groups: []string{"admins", "family"}},
	}

	tests := []struct {
		name  string
		input []string
		users map[string]*UserConfig
		want  []string // nil means nil result
	}{
		{"nil input", nil, defaultUsers, nil},
		{"empty slice", []string{}, defaultUsers, nil},
		{"group expansion", []string{"admins"}, defaultUsers, []string{"alice", "bob"}},
		{"direct username", []string{"guest"}, map[string]*UserConfig{"guest": {Password: "hash"}}, []string{"guest"}},
		{"nonexistent group", []string{"nonexistent-group"}, map[string]*UserConfig{"alice": {Password: "hash", Groups: []string{"admins"}}}, nil},
		{"username-group collision", []string{"admins"}, map[string]*UserConfig{
			"admins": {Password: "hash", Groups: []string{}},
			"alice":  {Password: "hash", Groups: []string{"admins"}},
		}, []string{"admins", "alice"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := resolveAllowed(tt.input, tt.users)
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
