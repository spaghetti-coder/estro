// Package config handles YAML configuration loading, validation, and cascading defaults for Estro.
package config

import (
	"fmt"
	"os"
	"reflect"
	"sort"
	"strings"

	"github.com/go-playground/validator/v10"
	"gopkg.in/yaml.v3"
)

const (
	// DefaultTimeout is the default command execution timeout in seconds.
	DefaultTimeout = 60
	// DefaultConfirm is the default value for requiring user confirmation before running a command.
	DefaultConfirm = true
	// DefaultCollapsable is the default value for whether sections start collapsed in the UI.
	DefaultCollapsable = true
	// DefaultColumns is the default number of columns in the service grid.
	DefaultColumns = 3

	defaultHostname = "127.0.0.1"
	defaultPort     = 3000
)

// Addr returns the host:port address string for the server to listen on,
// falling back to defaults (127.0.0.1:3000) when not explicitly set.
func (g *GlobalConfig) Addr() string {
	hostname := defaultHostname
	port := defaultPort
	if g.Hostname != nil {
		hostname = *g.Hostname
	}
	if g.Port != nil {
		port = *g.Port
	}
	return fmt.Sprintf("%s:%d", hostname, port)
}

// Config represents the top-level Estro configuration loaded from YAML.
type Config struct {
	Global   *GlobalConfig          `yaml:"global,omitempty" validate:"omitempty"`
	Users    map[string]*UserConfig `yaml:"users,omitempty" validate:"omitempty,dive"`
	Sections []SectionConfig        `yaml:"sections,omitempty" validate:"omitempty,dive"`
}

// GlobalConfig holds settings that apply across all sections and services.
type GlobalConfig struct {
	Title         *string `yaml:"title,omitempty"`
	Subtitle      *string `yaml:"subtitle,omitempty"`
	Hostname      *string `yaml:"hostname,omitempty"`
	Port          *int    `yaml:"port,omitempty" validate:"omitempty,gt=0"`
	Secret        *string `yaml:"secret,omitempty"`
	CascadeFields `yaml:",inline"`
	LayoutFields  `yaml:",inline"`
}

// UserConfig defines a single user's credentials and group memberships.
type UserConfig struct {
	Password string   `yaml:"password" validate:"required"`
	Groups   []string `yaml:"groups,omitempty"`
}

// SectionConfig groups services under a common heading in the UI.
type SectionConfig struct {
	Title         string          `yaml:"title" validate:"required"`
	Services      []ServiceConfig `yaml:"services,omitempty" validate:"omitempty,dive"`
	CascadeFields `yaml:",inline"`
	LayoutFields  `yaml:",inline"`
}

// ServiceConfig defines a single runnable command exposed in the UI.
type ServiceConfig struct {
	Title         string       `yaml:"title" validate:"required"`
	Command       CommandValue `yaml:"command" validate:"required"`
	CascadeFields `yaml:",inline"`
}

func newValidator() *validator.Validate {
	v := validator.New()
	v.RegisterTagNameFunc(func(fld reflect.StructField) string {
		name := strings.SplitN(fld.Tag.Get("yaml"), ",", 2)[0]
		if name == "-" || name == "" {
			return fld.Name
		}
		return name
	})
	return v
}

// Load reads and validates the YAML configuration file at path.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config file %s: %w", path, err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config YAML: %w", err)
	}

	v := newValidator()
	if err := v.Struct(cfg); err != nil {
		return nil, formatValidationError(err)
	}

	return &cfg, nil
}

func formatValidationError(err error) error {
	if ve, ok := err.(validator.ValidationErrors); ok {
		var msgs []string
		for _, fe := range ve {
			field := fe.Field()
			msgs = append(msgs, fmt.Sprintf("field \"%s\": %s", field, fe.Tag()))
		}
		return fmt.Errorf("config validation failed: %s", strings.Join(msgs, "; "))
	}
	return err
}

// GetGlobal returns the global configuration, or a zero-valued GlobalConfig if none is set.
func (c *Config) GetGlobal() *GlobalConfig {
	if c.Global != nil {
		return c.Global
	}
	return &GlobalConfig{}
}

// GetConfigResponse returns a ConfigResponse with the application title, subtitle,
// and sorted list of usernames for the frontend.
func (c *Config) GetConfigResponse() ConfigResponse {
	g := c.GetGlobal()
	title := "Estro"
	if g.Title != nil {
		title = *g.Title
	}
	subtitle := ""
	if g.Subtitle != nil {
		subtitle = *g.Subtitle
	}
	userNames := make([]string, 0, len(c.Users))
	for name := range c.Users {
		userNames = append(userNames, name)
	}
	sort.Strings(userNames)
	return ConfigResponse{
		Title:    title,
		Subtitle: subtitle,
		Users:    userNames,
	}
}
