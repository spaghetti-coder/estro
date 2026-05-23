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

func TestCascadeStringList(t *testing.T) {
	tests := []struct {
		name           string
		svc, sec, global StringList
		want           StringList
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
			if len(got) != len(tt.want) {
				t.Fatalf("cascadeStringList(%v, %v, %v) = %v, want %v", tt.svc, tt.sec, tt.global, got, tt.want)
			}
			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Fatalf("cascadeStringList(%v, %v, %v)[%d] = %q, want %q", tt.svc, tt.sec, tt.global, i, got[i], tt.want[i])
				}
			}
			if tt.want == nil && got != nil {
				t.Fatalf("cascadeStringList(%v, %v, %v) = %v, want nil", tt.svc, tt.sec, tt.global, got)
			}
		})
	}
}
