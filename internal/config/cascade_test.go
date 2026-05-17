package config

import (
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
		{"trailing comma dropped", "val: 'a,b,'\n", StringList{"a", "b"}, false},
		{"empty elements dropped", "val: 'a,,b'\n", StringList{"a", "b"}, false},
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
				if len(w.Val) != len(tt.field) {
					t.Errorf("expected len %d, got len %d (%v)", len(tt.field), len(w.Val), w.Val)
				}
				for i := range tt.field {
					if w.Val[i] != tt.field[i] {
						t.Errorf("element %d: expected %q, got %q", i, tt.field[i], w.Val[i])
					}
				}
			}
		})
	}
}

func TestStringListUnmarshalInvalid(t *testing.T) {
	yamlStr := "val:\n  key: value\n"
	type wrapper struct {
		Val StringList `yaml:"val,omitempty"`
	}
	var w wrapper
	err := yaml.Load([]byte(yamlStr), &w)
	if err == nil {
		t.Error("expected error for mapping node, got nil")
	}
}
