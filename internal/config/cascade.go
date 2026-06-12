package config

import (
	"fmt"
	"strings"

	"go.yaml.in/yaml/v4"
)

// StringList deserializes from comma-separated scalar or YAML sequence; nil/empty = public in ACL.
type StringList []string

// CommandValue is a shell command: single string or array.
type CommandValue []string

// CascadeFields holds global → section → service cascade fields.
type CascadeFields struct {
	Timeout       *int       `yaml:"timeout" validate:"omitempty,gt=0"`
	Confirm       *bool      `yaml:"confirm"`
	Allowed       StringList `yaml:"allowed" validate:"omitempty,dive,required,allowed_ref"`
	Remote        StringList `yaml:"remote" validate:"omitempty,dive,required,remote_host"`
	RemoteSSHOpts StringList `yaml:"remote_ssh_opts" validate:"omitempty,dive,required"`
	Enabled       *bool      `yaml:"enabled"`
	Restricted    *bool      `yaml:"restricted"`
}

// LayoutFields holds global → section layout fields.
type LayoutFields struct {
	Collapsable *bool `yaml:"collapsable"`
	Columns     *int  `yaml:"columns" validate:"omitempty,gte=1,lte=12"`
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
			result = append(result, strings.TrimSpace(p)) // keep empties; validation flags them
		}
		*s = result
		return nil
	case yaml.SequenceNode:
		var items []string
		if err := value.Decode(&items); err != nil {
			return err
		}
		for i, item := range items {
			items[i] = strings.TrimSpace(item) // mirror scalar path; blank-only -> "" so dive,required flags it
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

// cascade returns first non-nil of svc, sec, global; defaultVal if all nil.
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

// cascadeLayout resolves section → global; no service level.
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
