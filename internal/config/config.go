// Package config handles YAML configuration loading, validation, and cascading defaults for Estro.
package config

import (
	"bytes"
	"errors"
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

// Config represents the top-level Estro configuration loaded from YAML.
type Config struct {
	Global   *GlobalConfig          `yaml:"global,omitempty" validate:"omitempty"`
	Users    map[string]*UserConfig `yaml:"users,omitempty" validate:"omitempty,dive"`
	Sections []SectionConfig        `yaml:"sections,omitempty" validate:"required,min=1,dive"`
	// Extra captures unknown YAML keys at this level for foreign-key validation; load-internal only.
	Extra map[string]yaml.Node `yaml:",inline"`
}

// GlobalConfig holds settings that apply across all sections and services.
type GlobalConfig struct {
	Title         *string `yaml:"title,omitempty"`
	Subtitle      *string `yaml:"subtitle,omitempty"`
	Hostname      *string `yaml:"hostname,omitempty" validate:"omitempty,hostname_rfc1123|ip"`
	Port          *int    `yaml:"port,omitempty" validate:"omitempty,gte=1,lte=65535"`
	Secret        *string `yaml:"secret,omitempty"`
	CascadeFields `yaml:",inline"`
	LayoutFields  `yaml:",inline"`
	// Extra captures unknown YAML keys at this level for foreign-key validation; load-internal only.
	Extra map[string]yaml.Node `yaml:",inline"`
}

// UserConfig defines a single user's credentials and group memberships.
type UserConfig struct {
	Password string     `yaml:"password" validate:"required"`
	Groups   StringList `yaml:"groups,omitempty" validate:"omitempty,dive,required"`
	// Extra captures unknown YAML keys at this level for foreign-key validation; load-internal only.
	Extra map[string]yaml.Node `yaml:",inline"`
}

// SectionConfig groups services under a common heading in the UI.
type SectionConfig struct {
	Title         string          `yaml:"title" validate:"required"`
	Services      []ServiceConfig `yaml:"services,omitempty" validate:"required,min=1,dive"`
	CascadeFields `yaml:",inline"`
	LayoutFields  `yaml:",inline"`
	// Extra captures unknown YAML keys at this level for foreign-key validation; load-internal only.
	Extra map[string]yaml.Node `yaml:",inline"`
}

// ServiceConfig defines a single runnable command exposed in the UI.
type ServiceConfig struct {
	Title         string       `yaml:"title" validate:"required"`
	Command       CommandValue `yaml:"command" validate:"required,min=1,dive,required"`
	CascadeFields `yaml:",inline"`
	// Extra captures unknown YAML keys at this level for foreign-key validation; load-internal only.
	Extra map[string]yaml.Node `yaml:",inline"`
}

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

func init() {
	if err := validate.RegisterValidation("remote_host", validateRemoteHost); err != nil {
		panic(err)
	}
	if err := validate.RegisterValidation("allowed_ref", validateAllowedRef); err != nil {
		panic(err)
	}
}

// coalesce returns the value pointed to by ptr, or the provided fallback if ptr
// is nil. It is the single-level case of cascade.
func coalesce[T any](ptr *T, fallback T) T {
	return cascade(ptr, nil, nil, fallback)
}

// fileIssueResult builds a degraded result for a config we couldn't read or parse.
func fileIssueResult(msg string) *LoadResult {
	return &LoadResult{Config: &Config{}, Issues: []Issue{{Msg: msg}}}
}

// Load reads the YAML configuration at path and validates the resolved
// configuration. It never returns a fatal error: the result always carries a
// usable Config (default-backed when degraded) plus any collected issues.
func Load(path string) *LoadResult {
	data, err := os.ReadFile(path)
	if err != nil {
		return fileIssueResult("Configuration file can't be read")
	}

	var cfg Config
	var typePaths []string
	if len(bytes.TrimSpace(data)) > 0 {
		if err := yaml.Load(data, &cfg); err != nil {
			var le *yaml.LoadErrors
			if errors.As(err, &le) {
				typePaths = typeErrorPaths(le, data)
			} else {
				return fileIssueResult("Configuration file can't be read")
			}
		}
	}

	return &LoadResult{Config: &cfg, Issues: collectIssues(&cfg, typePaths)}
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
