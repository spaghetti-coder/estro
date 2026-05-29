package config

import (
	"fmt"
	"strings"

	"go.yaml.in/yaml/v4"
)

// StringList is a YAML-aware string list type that deserializes from either
// a comma-separated scalar (e.g., "alice,bob") or a YAML sequence (e.g., [alice, bob]).
// When used in ACL contexts, nil means public access, while an empty slice also means public.
type StringList []string

// CommandValue represents a shell command, which can be a single string
// or an array of commands joined with "&&" in YAML.
type CommandValue []string

// CascadeFields holds fields that cascade: global → section → service.
type CascadeFields struct {
	Timeout       *int       `yaml:"timeout,omitempty" validate:"omitempty,gt=0"`
	Confirm       *bool      `yaml:"confirm,omitempty"`
	Allowed       StringList `yaml:"allowed,omitempty"`
	Remote        StringList `yaml:"remote,omitempty" validate:"omitempty,dive,remote_host"`
	RemoteSSHOpts StringList `yaml:"remote_ssh_opts,omitempty"`
	Enabled       *bool      `yaml:"enabled,omitempty"`
	Restricted    *bool      `yaml:"restricted,omitempty"`
}

// LayoutFields holds fields that cascade: global → section (not service-level).
type LayoutFields struct {
	Collapsable *bool `yaml:"collapsable,omitempty"`
	Columns     *int  `yaml:"columns,omitempty" validate:"omitempty,gt=0"`
}

func (s *StringList) UnmarshalYAML(value *yaml.Node) error {
	switch value.Kind {
	case yaml.ScalarNode:
		raw := value.Value
		if raw == "" {
			*s = []string{}
			return nil
		}
		parts := strings.Split(raw, ",")
		result := make([]string, 0, len(parts))
		for _, p := range parts {
			trimmed := strings.TrimSpace(p)
			if trimmed != "" {
				result = append(result, trimmed)
			}
		}
		*s = result
		return nil
	case yaml.SequenceNode:
		var items []string
		if err := value.Decode(&items); err != nil {
			return err
		}
		*s = items
		return nil
	default:
		return fmt.Errorf("string list must be a string or array of strings")
	}
}

func (c *CommandValue) UnmarshalYAML(value *yaml.Node) error {
	switch value.Kind {
	case yaml.ScalarNode:
		*c = []string{value.Value}
		return nil
	case yaml.SequenceNode:
		var cmds []string
		if err := value.Decode(&cmds); err != nil {
			return err
		}
		*c = cmds
		return nil
	default:
		return fmt.Errorf("command must be a string or array of strings")
	}
}

// cascade returns the first non-nil pointer value among svc, sec, global, or defaultVal if all are nil.
func cascade[T any](svc, sec, global *T, defaultVal T) T {
	if svc != nil {
		return *svc
	}
	if sec != nil {
		return *sec
	}
	if global != nil {
		return *global
	}
	return defaultVal
}

// cascadeLayout resolves a layout field that cascades section → global only
// (there is no service level), falling back to defaultVal.
func cascadeLayout[T any](sec, global *T, defaultVal T) T {
	return cascade(nil, sec, global, defaultVal)
}

func cascadeStringList(svc, sec, global StringList) StringList {
	if svc != nil {
		return svc
	}
	if sec != nil {
		return sec
	}
	if global != nil {
		return global
	}
	return nil
}
