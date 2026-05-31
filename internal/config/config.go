// Package config handles YAML configuration loading, validation, and cascading defaults for Estro.
package config

import (
	"bytes"
	"maps"
	"os"
	"reflect"
	"regexp"
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
	Global   *GlobalConfig          `yaml:"global" validate:"omitempty"`
	Users    map[string]*UserConfig `yaml:"users" validate:"omitempty,dive"`
	Sections []SectionConfig        `yaml:"sections" validate:"required,min=1,dive"`
}

// GlobalConfig holds settings that apply across all sections and services.
type GlobalConfig struct {
	Title         *string `yaml:"title"`
	Subtitle      *string `yaml:"subtitle"`
	Hostname      *string `yaml:"hostname" validate:"omitempty,hostname_rfc1123|ip"`
	Port          *int    `yaml:"port" validate:"omitempty,gte=1,lte=65535"`
	Secret        *string `yaml:"secret"`
	CascadeFields `yaml:",inline"`
	LayoutFields  `yaml:",inline"`
}

// UserConfig defines a single user's credentials and group memberships.
type UserConfig struct {
	Password string     `yaml:"password" validate:"required"`
	Groups   StringList `yaml:"groups" validate:"omitempty,dive,required"`
}

// SectionConfig groups services under a common heading in the UI.
type SectionConfig struct {
	Title         string          `yaml:"title" validate:"required"`
	Services      []ServiceConfig `yaml:"services" validate:"required,min=1,dive"`
	CascadeFields `yaml:",inline"`
	LayoutFields  `yaml:",inline"`
}

// ServiceConfig defines a single runnable command exposed in the UI.
type ServiceConfig struct {
	Title         string       `yaml:"title" validate:"required"`
	Command       CommandValue `yaml:"command" validate:"required,min=1,dive,required"`
	CascadeFields `yaml:",inline"`
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
	for tag, fn := range map[string]validator.Func{
		"remote_host": validateRemoteHost,
		"allowed_ref": validateAllowedRef,
	} {
		if err := validate.RegisterValidation(tag, fn); err != nil {
			panic(err)
		}
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

// estroEnvRe matches the {estro_env.VAR} load-time substitution marker. The
// shell form ${VAR} is intentionally NOT matched — it is left untouched for the
// command's runtime shell.
var estroEnvRe = regexp.MustCompile(`\{estro_env\.([A-Za-z_][A-Za-z0-9_]*)\}`)

// expandEnv substitutes {estro_env.VAR} in every scalar value of the parsed
// YAML with the environment variable's value, returning an issue for each
// referenced variable that is not set.
func expandEnv(n *yaml.Node) []Issue {
	var issues []Issue
	var walk func(*yaml.Node)
	walk = func(n *yaml.Node) {
		if n.Kind == yaml.ScalarNode {
			n.Value = estroEnvRe.ReplaceAllStringFunc(n.Value, func(m string) string {
				name := estroEnvRe.FindStringSubmatch(m)[1]
				if v, ok := os.LookupEnv(name); ok {
					return v
				}
				issues = append(issues, Issue{Msg: "environment variable " + name + " is not set"})
				return m
			})
		}
		for _, c := range n.Content {
			walk(c)
		}
	}
	walk(n)
	return issues
}

// Load reads the YAML configuration at path and validates the resolved
// configuration. {estro_env.VAR} markers are expanded from the environment
// first. It never returns a fatal error: the result always carries a usable
// Config (default-backed when degraded) plus any collected issues.
func Load(path string) *LoadResult {
	data, err := os.ReadFile(path)
	if err != nil {
		return fileIssueResult("Configuration file can't be read")
	}

	var cfg Config
	var raw map[string]any
	var issues []Issue
	if len(bytes.TrimSpace(data)) > 0 {
		var root yaml.Node
		if err := yaml.Load(data, &root); err != nil {
			// Unparseable YAML: there is no usable config to validate.
			return fileIssueResult("Configuration file can't be read")
		}
		issues = expandEnv(&root)
		// Decode the env-expanded node. Type-mismatch errors are ignored:
		// shapeIssues reports wrong shapes, validateStruct reports bad values.
		_ = root.Decode(&raw)
		_ = root.Decode(&cfg)
	}

	issues = append(issues, collectIssues(&cfg, raw)...)
	return &LoadResult{Config: &cfg, Issues: dedupeSort(issues)}
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
