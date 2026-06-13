// Package config handles YAML configuration loading, validation, and cascading defaults for Estro.
package config

import (
	"bytes"
	"maps"
	"math"
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

// Config is the top-level Estro YAML configuration.
type Config struct {
	Global   *GlobalConfig          `yaml:"global" validate:"omitempty"`
	Users    map[string]*UserConfig `yaml:"users" validate:"omitempty,dive"`
	Sections []SectionConfig        `yaml:"sections" validate:"required,min=1,dive"`
}

// GlobalConfig holds cross-section settings.
type GlobalConfig struct {
	Title         *string `yaml:"title"`
	Subtitle      *string `yaml:"subtitle"`
	Hostname      *string `yaml:"hostname" validate:"omitempty,hostname_rfc1123|ip"`
	Port          *int    `yaml:"port" validate:"omitempty,gte=1,lte=65535"`
	SessionSecret *string `yaml:"session_secret"`
	SessionTTL    *int    `yaml:"session_ttl" validate:"omitempty,gte=0"`
	CascadeFields `yaml:",inline"`
	LayoutFields  `yaml:",inline"`
}

// UserConfig holds a user's password and groups.
type UserConfig struct {
	Password string     `yaml:"password" validate:"required"`
	Groups   StringList `yaml:"groups" validate:"omitempty,dive,required"`
}

// SectionConfig groups services under a UI heading.
type SectionConfig struct {
	Title         string          `yaml:"title" validate:"required"`
	Services      []ServiceConfig `yaml:"services" validate:"required,min=1,dive"`
	CascadeFields `yaml:",inline"`
	LayoutFields  `yaml:",inline"`
}

// ServiceConfig is a single runnable command.
type ServiceConfig struct {
	Title         string       `yaml:"title" validate:"required"`
	Command       CommandValue `yaml:"command" validate:"required,min=1,dive,required"`
	CascadeFields `yaml:",inline"`
}

// validate is a singleton validator, avoiding repeated allocation.
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

// coalesce returns *ptr or fallback when nil; single-level cascade.
func coalesce[T any](ptr *T, fallback T) T {
	return cascade(ptr, nil, nil, fallback)
}

// fileIssueResult builds a degraded result for a config we couldn't read or parse.
func fileIssueResult(msg string) *LoadResult {
	return &LoadResult{Config: &Config{}, Issues: []Issue{{Msg: msg}}}
}

// estroEnvRe matches {estro_env.VAR}; ${VAR} is left for runtime shell.
var estroEnvRe = regexp.MustCompile(`\{estro_env\.([A-Za-z_][A-Za-z0-9_]*)\}`)

// expandEnv replaces {estro_env.VAR} in YAML scalars; issues for unset vars.
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

// Load reads YAML at path, expands {estro_env.VAR}, validates; never fatal — usable Config + issues always.
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

// GetGlobal returns global config, or zero-value if unset.
func (c *Config) GetGlobal() *GlobalConfig {
	if c.Global != nil {
		return c.Global
	}
	return &GlobalConfig{}
}

// SessionTTLSeconds returns remember-me max-age in seconds; 0/nil → no limit (MaxInt32), N>0 → N hours.
func (c *Config) SessionTTLSeconds() int {
	g := c.GetGlobal()
	if g.SessionTTL != nil && *g.SessionTTL > 0 {
		return *g.SessionTTL * 3600
	}
	return math.MaxInt32
}

// GetConfigResponse returns title, subtitle, sorted usernames for frontend.
func (c *Config) GetConfigResponse() ConfigResponse {
	g := c.GetGlobal()
	return ConfigResponse{
		Title:    coalesce(g.Title, defaultTitle),
		Subtitle: coalesce(g.Subtitle, defaultSubtitle),
		Users:    slices.Sorted(maps.Keys(c.Users)),
	}
}
