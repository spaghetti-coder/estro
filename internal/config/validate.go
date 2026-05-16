package config

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

type ValidationError struct {
	Path string
	Line int
	Msg  string
}

func (e ValidationError) String() string {
	if e.Line > 0 {
		return fmt.Sprintf("%s (line %d): %s", e.Path, e.Line, e.Msg)
	}
	return fmt.Sprintf("%s: %s", e.Path, e.Msg)
}

var configKnownFields = map[string]bool{
	"global":   true,
	"users":    true,
	"sections": true,
}

var globalKnownFields = map[string]bool{
	"title":       true,
	"subtitle":    true,
	"hostname":    true,
	"port":        true,
	"secret":      true,
	"timeout":     true,
	"confirm":     true,
	"allowed":     true,
	"collapsable": true,
	"columns":     true,
	"remote":      true,
	"enabled":     true,
	"restricted":  true,
}

var sectionKnownFields = map[string]bool{
	"title":       true,
	"services":    true,
	"allowed":     true,
	"timeout":     true,
	"confirm":     true,
	"collapsable": true,
	"columns":     true,
	"remote":      true,
	"enabled":     true,
	"restricted":  true,
}

var serviceKnownFields = map[string]bool{
	"title":      true,
	"command":    true,
	"allowed":    true,
	"timeout":    true,
	"confirm":    true,
	"remote":     true,
	"enabled":    true,
	"restricted": true,
}

var userKnownFields = map[string]bool{
	"password": true,
	"groups":   true,
}

func ValidateSchema(cfg *Config, node *yaml.Node) []ValidationError {
	var errors []ValidationError
	if node != nil {
		walk(node, "", configKnownFields, &errors)
	}
	if cfg != nil {
		validateValues(cfg, &errors)
		errors = append(errors, validateGroupRefs(cfg)...)
	}
	return errors
}

func validateGroupRefs(cfg *Config) []ValidationError {
	knownGroups := make(map[string]bool)
	for name, user := range cfg.Users {
		knownGroups[name] = true
		for _, g := range user.Groups {
			knownGroups[g] = true
		}
	}

	var errors []ValidationError

	if cfg.Global != nil && len(cfg.Global.Allowed) > 0 {
		for _, name := range cfg.Global.Allowed {
			if !knownGroups[name] {
				errors = append(errors, ValidationError{
					Path: "global.allowed",
					Msg:  fmt.Sprintf("unknown user or group %q", name),
				})
			}
		}
	}

	for i, sec := range cfg.Sections {
		if len(sec.Allowed) > 0 {
			for _, name := range sec.Allowed {
				if !knownGroups[name] {
					errors = append(errors, ValidationError{
						Path: fmt.Sprintf("sections[%d].allowed", i),
						Msg:  fmt.Sprintf("unknown user or group %q", name),
					})
				}
			}
		}
		for j, svc := range sec.Services {
			if len(svc.Allowed) > 0 {
				for _, name := range svc.Allowed {
					if !knownGroups[name] {
						errors = append(errors, ValidationError{
							Path: fmt.Sprintf("sections[%d].services[%d].allowed", i, j),
							Msg:  fmt.Sprintf("unknown user or group %q", name),
						})
					}
				}
			}
		}
	}

	return errors
}

func validateValues(cfg *Config, errors *[]ValidationError) {
	if cfg.Global != nil {
		validateGlobalValues(cfg.Global, errors)
	}
	validateSectionsValues(cfg, errors)
	validateUsersValues(cfg, errors)
}

func validateGlobalValues(g *GlobalConfig, errors *[]ValidationError) {
	if g.Hostname != nil && *g.Hostname == "" {
		*errors = append(*errors, ValidationError{
			Path: "global.hostname",
			Msg:  "must be a non-empty string",
		})
	}
	if g.Port != nil && (*g.Port < 1 || *g.Port > 65535) {
		*errors = append(*errors, ValidationError{
			Path: "global.port",
			Msg:  "must be between 1 and 65535",
		})
	}
	if g.Columns != nil && (*g.Columns < 1 || *g.Columns > 12) {
		*errors = append(*errors, ValidationError{
			Path: "global.columns",
			Msg:  "must be between 1 and 12",
		})
	}
	if g.Timeout != nil && *g.Timeout <= 0 {
		*errors = append(*errors, ValidationError{
			Path: "global.timeout",
			Msg:  "must be greater than 0",
		})
	}
	validateRemoteEntries(g.Remote, "global.remote", errors)
}

func validateSectionsValues(cfg *Config, errors *[]ValidationError) {
	if len(cfg.Sections) == 0 {
		*errors = append(*errors, ValidationError{
			Path: "sections",
			Msg:  "must be a non-empty list",
		})
		return
	}
	for i, sec := range cfg.Sections {
		secPath := fmt.Sprintf("sections[%d]", i)
		if sec.Title == "" {
			*errors = append(*errors, ValidationError{
				Path: secPath + ".title",
				Msg:  "is required",
			})
		}
		if len(sec.Services) == 0 {
			*errors = append(*errors, ValidationError{
				Path: secPath + ".services",
				Msg:  "must be a non-empty list",
			})
		}
		if sec.Columns != nil && (*sec.Columns < 1 || *sec.Columns > 12) {
			*errors = append(*errors, ValidationError{
				Path: secPath + ".columns",
				Msg:  "must be between 1 and 12",
			})
		}
		if sec.Timeout != nil && *sec.Timeout <= 0 {
			*errors = append(*errors, ValidationError{
				Path: secPath + ".timeout",
				Msg:  "must be greater than 0",
			})
		}
		validateRemoteEntries(sec.Remote, secPath+".remote", errors)
		for j, svc := range sec.Services {
			svcPath := fmt.Sprintf("%s.services[%d]", secPath, j)
			if svc.Title == "" {
				*errors = append(*errors, ValidationError{
					Path: svcPath + ".title",
					Msg:  "is required",
				})
			}
			if len(svc.Command) == 0 {
				*errors = append(*errors, ValidationError{
					Path: svcPath + ".command",
					Msg:  "is required",
				})
			}
			for k, cmd := range svc.Command {
				if cmd == "" {
					*errors = append(*errors, ValidationError{
						Path: fmt.Sprintf("%s.command[%d]", svcPath, k),
						Msg:  "must be a non-empty string",
					})
				}
			}
			if svc.Timeout != nil && *svc.Timeout <= 0 {
				*errors = append(*errors, ValidationError{
					Path: svcPath + ".timeout",
					Msg:  "must be greater than 0",
				})
			}
			validateRemoteEntries(svc.Remote, svcPath+".remote", errors)
		}
	}
}

func validateRemoteEntries(remote RemoteValue, basePath string, errors *[]ValidationError) {
	for i, host := range remote {
		if host == "" {
			*errors = append(*errors, ValidationError{
				Path: fmt.Sprintf("%s[%d]", basePath, i),
				Msg:  "must be a non-empty string",
			})
		}
	}
}

func validateUsersValues(cfg *Config, errors *[]ValidationError) {
	for name, user := range cfg.Users {
		if user.Password == "" {
			*errors = append(*errors, ValidationError{
				Path: "users." + name + ".password",
				Msg:  "is required",
			})
		}
	}
}

func walk(node *yaml.Node, path string, knownFields map[string]bool, errors *[]ValidationError) {
	walkWithMode(node, path, knownFields, errors, false)
}

func walkWithMode(node *yaml.Node, path string, knownFields map[string]bool, errors *[]ValidationError, dynamicKeys bool) {
	switch node.Kind {
	case yaml.DocumentNode:
		for _, child := range node.Content {
			walkWithMode(child, path, knownFields, errors, false)
		}

	case yaml.MappingNode:
		for i := 0; i < len(node.Content); i += 2 {
			keyNode := node.Content[i]
			valNode := node.Content[i+1]
			key := keyNode.Value
			childPath := joinPath(path, key)

			if strings.HasPrefix(key, "x-") {
				continue
			}

			if dynamicKeys {
				walkWithMode(valNode, childPath, knownFields, errors, false)
				continue
			}

			if !knownFields[key] {
				*errors = append(*errors, ValidationError{
					Path: childPath,
					Line: keyNode.Line,
					Msg:  fmt.Sprintf("unknown field %q", key),
				})
				continue
			}

			childKnown := childKnownFields(key, path)
			if childKnown != nil {
				walkWithMode(valNode, childPath, childKnown, errors, key == "users")
			}
		}

	case yaml.SequenceNode:
		for i, child := range node.Content {
			idxPath := fmt.Sprintf("%s[%d]", path, i)
			walkWithMode(child, idxPath, knownFields, errors, false)
		}
	}
}

func childKnownFields(key string, _ string) map[string]bool {
	switch key {
	case "global":
		return globalKnownFields
	case "sections":
		return sectionKnownFields
	case "services":
		return serviceKnownFields
	case "users":
		return userKnownFields
	default:
		return nil
	}
}

func joinPath(parent, key string) string {
	if parent == "" {
		return key
	}
	return parent + "." + key
}

func formatAllErrors(validationErrors []ValidationError) error {
	if len(validationErrors) == 0 {
		return nil
	}
	var msgs []string
	for _, e := range validationErrors {
		msgs = append(msgs, e.String())
	}
	return fmt.Errorf("config validation failed:\n  %s", strings.Join(msgs, "\n  "))
}