package config

import (
	"slices"
	"testing"

	"go.yaml.in/yaml/v4"
)

func TestStringListUnmarshalYAML(t *testing.T) {
	tests := []struct {
		name  string
		yaml  string
		field StringList
		isNil bool
	}{
		{"null", "val: ~\n", nil, true},
		{"empty string", "val: ''\n", StringList{}, false},
		{"single element no commas", "val: server1\n", StringList{"server1"}, false},
		{"comma-separated", "val: 'server1, server2,server3'\n", StringList{"server1", "server2", "server3"}, false},
		{"yaml array", "val: [server1, server2]\n", StringList{"server1", "server2"}, false},
		{"comma with spaces", "val: 'admins, guest'\n", StringList{"admins", "guest"}, false},
		{"trailing comma kept as empty", "val: 'a,b,'\n", StringList{"a", "b", ""}, false},
		{"sequence whitespace-only kept as empty", "val: [a, ' ', b]\n", StringList{"a", "", "b"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			type wrapper struct {
				Val StringList `yaml:"val,omitempty"`
			}
			var w wrapper
			if err := yaml.Load([]byte(tt.yaml), &w); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.isNil {
				if w.Val != nil {
					t.Errorf("expected nil, got %v", w.Val)
				}
			} else {
				if w.Val == nil {
					t.Fatalf("expected non-nil, got nil")
				}
				if !slices.Equal(w.Val, tt.field) {
					t.Errorf("expected %v, got %v", tt.field, w.Val)
				}
			}
		})
	}
}

func TestStringListUnmarshalInvalid(t *testing.T) {
	type wrapper struct {
		Val StringList `yaml:"val,omitempty"`
	}
	var w wrapper
	if err := yaml.Load([]byte("val:\n  key: value\n"), &w); err == nil {
		t.Error("expected error for mapping node, got nil")
	}
}

func TestStringListUnmarshalSequenceDecodeError(t *testing.T) {
	cases := []string{
		"val: [{a: b}]\n",
		"val: [[x]]\n",
	}
	for _, c := range cases {
		type wrapper struct {
			Val StringList `yaml:"val,omitempty"`
		}
		var w wrapper
		if err := yaml.Load([]byte(c), &w); err == nil {
			t.Errorf("expected error for %q, got nil", c)
		}
	}
}

func TestCommandValueUnmarshalSequenceDecodeError(t *testing.T) {
	cases := []string{
		"val: [{a: b}]\n",
		"val: [[x]]\n",
	}
	for _, c := range cases {
		type wrapper struct {
			Val CommandValue `yaml:"val,omitempty"`
		}
		var w wrapper
		if err := yaml.Load([]byte(c), &w); err == nil {
			t.Errorf("expected error for %q, got nil", c)
		}
	}
}

func TestCascadeStringList(t *testing.T) {
	tests := []struct {
		name             string
		svc, sec, global StringList
		want             StringList
	}{
		{"all nil", nil, nil, nil, nil},
		{"service overrides section", StringList{"svc"}, StringList{"sec"}, StringList{"global"}, StringList{"svc"}},
		{"section overrides global", nil, StringList{"sec"}, StringList{"global"}, StringList{"sec"}},
		{"falls back to global", nil, nil, StringList{"global"}, StringList{"global"}},
		{"empty slice is explicit override", StringList{}, StringList{"sec"}, StringList{"global"}, StringList{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cascadeStringList(tt.svc, tt.sec, tt.global)
			if (tt.want == nil) != (got == nil) {
				t.Errorf("nil mismatch: want %v, got %v", tt.want, got)
				return
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("expected %v, got %v", tt.want, got)
			}
		})
	}
}
