// Package config handles YAML configuration loading, validation, and cascading defaults for Estro.
package config

import (
	"errors"
	"fmt"
	"maps"
	"os"
	"reflect"
	"slices"
	"strings"

	"github.com/go-playground/validator/v10"
	"go.yaml.in/yaml/v4"
)

const (
	defaultHostname    = "127.0.0.1"
	defaultPort        = 3000
	defaultTitle       = "Estro"
	defaultSubtitle    = ""
	defaultTimeout     = 60
	defaultConfirm     = true
	defaultCollapsable = true
	defaultColumns     = 3
)

// validate is a package-level singleton validator instance to avoid
// repeated allocation on every Load() call.
var validate = func() *validator.Validate {
	v := validator.New()
	v.RegisterTagNameFunc(func(fld reflect.StructField) string {
		name := strings.SplitN(fld.Tag.Get("yaml"), ",", 2)[0]
		if name == "-" || name == "" {
			return fld.Name
		}
		return name
	})
	return v
}()

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
	Password string     `yaml:"password" validate:"required"`
	Groups   StringList `yaml:"groups,omitempty"`
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

// coalesce returns the value pointed to by ptr, or the provided fallback if ptr is nil.
func coalesce[T any](ptr *T, fallback T) T {
	if ptr != nil {
		return *ptr
	}
	return fallback
}

// Addr returns the host:port address string for the server to listen on,
// falling back to defaults (127.0.0.1:3000) when not explicitly set.
func (g *GlobalConfig) Addr() string {
	return fmt.Sprintf("%s:%d", coalesce(g.Hostname, defaultHostname), coalesce(g.Port, defaultPort))
}

// Load reads and validates the YAML configuration file at path.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config file %s: %w", path, err)
	}

	var cfg Config
	if err := yaml.Load(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config YAML: %w", err)
	}

	if err := validate.Struct(cfg); err != nil {
		return nil, formatValidationError(err)
	}

	return &cfg, nil
}

func formatValidationError(err error) error {
	var ve validator.ValidationErrors
	if errors.As(err, &ve) {
		var msgs []string
		for _, fe := range ve {
			field := fe.Field()
			msgs = append(msgs, fmt.Sprintf("field %q: %s", field, fe.Tag()))
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
	return ConfigResponse{
		Title:    coalesce(g.Title, defaultTitle),
		Subtitle: coalesce(g.Subtitle, defaultSubtitle),
		Users:    slices.Sorted(maps.Keys(c.Users)),
	}
}
