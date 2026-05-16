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

func ValidateSchema(_ *Config, node *yaml.Node) []ValidationError {
	if node == nil {
		return nil
	}
	var errors []ValidationError
	walk(node, "", configKnownFields, &errors)
	return errors
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