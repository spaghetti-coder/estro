package config

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// RemoteValue represents an SSH remote target, which can be a single host string
// or a multi-hop chain specified as an array of hosts in YAML.
type RemoteValue []string

func (r *RemoteValue) UnmarshalYAML(value *yaml.Node) error {
	switch value.Kind {
	case yaml.ScalarNode:
		*r = []string{value.Value}
		return nil
	case yaml.SequenceNode:
		var hosts []string
		if err := value.Decode(&hosts); err != nil {
			return err
		}
		*r = hosts
		return nil
	default:
		return fmt.Errorf("remote must be a string or array of strings")
	}
}

// CommandValue represents a shell command, which can be a single string
// or an array of commands joined with "&&" in YAML.
type CommandValue []string

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

// CascadeFields holds fields that cascade: global → section → service.
type CascadeFields struct {
	Timeout    *int        `yaml:"timeout,omitempty" validate:"omitempty,gt=0"`
	Confirm    *bool       `yaml:"confirm,omitempty"`
	Allowed    []string    `yaml:"allowed,omitempty"`
	Remote     RemoteValue `yaml:"remote,omitempty"`
	Enabled    *bool       `yaml:"enabled,omitempty"`
	Restricted *bool       `yaml:"restricted,omitempty"`
}

// LayoutFields holds fields that cascade: global → section (not service-level).
type LayoutFields struct {
	Collapsable *bool `yaml:"collapsable,omitempty"`
	Columns     *int  `yaml:"columns,omitempty" validate:"omitempty,gt=0"`
}

func cascadeInt(svc, sec, global *int, defaultVal int) int {
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

func cascadeBool(svc, sec, global *bool, defaultVal bool) bool {
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
