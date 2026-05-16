package config

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func mustParseNode(t *testing.T, input string) *yaml.Node {
	t.Helper()
	var node yaml.Node
	if err := yaml.Unmarshal([]byte(input), &node); err != nil {
		t.Fatalf("failed to parse YAML: %v", err)
	}
	return &node
}

func mustParseConfig(t *testing.T, input string) *Config {
	t.Helper()
	var cfg Config
	if err := yaml.Unmarshal([]byte(input), &cfg); err != nil {
		t.Fatalf("failed to parse config: %v", err)
	}
	return &cfg
}

func TestValidateUnknownFieldAtTopLevel(t *testing.T) {
	input := `unknown_key: foo`
	cfg := mustParseConfig(t, input)
	node := mustParseNode(t, input)
	errors := ValidateSchema(cfg, node)
	if len(errors) == 0 {
		t.Fatal("expected at least one validation error, got none")
	}
	found := false
	for _, e := range errors {
		if e.Path == "unknown_key" && strings.Contains(e.Msg, "unknown field") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected error at path 'unknown_key' with 'unknown field', got %+v", errors)
	}
}

func TestValidateUnknownFieldInGlobal(t *testing.T) {
	input := `global:
  hostname: localhost
  unknown_field: bar`
	cfg := mustParseConfig(t, input)
	node := mustParseNode(t, input)
	errors := ValidateSchema(cfg, node)
	if len(errors) == 0 {
		t.Fatal("expected at least one validation error, got none")
	}
	found := false
	for _, e := range errors {
		if e.Path == "global.unknown_field" && strings.Contains(e.Msg, "unknown field") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected error at path 'global.unknown_field', got %+v", errors)
	}
}

func TestValidateUnknownFieldInSection(t *testing.T) {
	input := `sections:
  - title: T
    services: []
    unknown_sec_field: x`
	cfg := mustParseConfig(t, input)
	node := mustParseNode(t, input)
	errors := ValidateSchema(cfg, node)
	if len(errors) == 0 {
		t.Fatal("expected at least one validation error, got none")
	}
	found := false
	for _, e := range errors {
		if e.Path == "sections[0].unknown_sec_field" && strings.Contains(e.Msg, "unknown field") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected error at path 'sections[0].unknown_sec_field', got %+v", errors)
	}
}

func TestValidateUnknownFieldInService(t *testing.T) {
	input := `sections:
  - title: T
    services:
      - title: S
        command: echo
        unknown_svc_field: x`
	cfg := mustParseConfig(t, input)
	node := mustParseNode(t, input)
	errors := ValidateSchema(cfg, node)
	if len(errors) == 0 {
		t.Fatal("expected at least one validation error, got none")
	}
	found := false
	for _, e := range errors {
		if e.Path == "sections[0].services[0].unknown_svc_field" && strings.Contains(e.Msg, "unknown field") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected error at path 'sections[0].services[0].unknown_svc_field', got %+v", errors)
	}
}

func TestValidateUnknownFieldInUser(t *testing.T) {
	input := `users:
  alice:
    password: hash
    unknown_user_field: x`
	cfg := mustParseConfig(t, input)
	node := mustParseNode(t, input)
	errors := ValidateSchema(cfg, node)
	if len(errors) == 0 {
		t.Fatal("expected at least one validation error, got none")
	}
	found := false
	for _, e := range errors {
		if e.Path == "users.alice.unknown_user_field" && strings.Contains(e.Msg, "unknown field") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected error at path 'users.alice.unknown_user_field', got %+v", errors)
	}
}

func TestValidateXFieldsSilentlyIgnored(t *testing.T) {
	input := `x-comment: note
global:
  x-meta: data`
	cfg := mustParseConfig(t, input)
	node := mustParseNode(t, input)
	errors := ValidateSchema(cfg, node)
	if len(errors) != 0 {
		t.Errorf("expected no errors for x-* fields, got %+v", errors)
	}
}

func TestValidateMultipleUnknownFieldsCollected(t *testing.T) {
	input := `unknown_one: foo
unknown_two: bar`
	cfg := mustParseConfig(t, input)
	node := mustParseNode(t, input)
	errors := ValidateSchema(cfg, node)
	if len(errors) < 2 {
		t.Fatalf("expected at least 2 errors, got %d: %+v", len(errors), errors)
	}
	paths := map[string]bool{}
	for _, e := range errors {
		paths[e.Path] = true
	}
	if !paths["unknown_one"] || !paths["unknown_two"] {
		t.Errorf("expected errors for unknown_one and unknown_two, got %+v", errors)
	}
}

func TestValidateKnownFieldsPass(t *testing.T) {
	input := `global:
  title: Estro
  hostname: 0.0.0.0
  port: 3000
  timeout: 30
  confirm: true
  collapsable: true
  columns: 3
sections:
  - title: Public
    services:
      - title: Uptime
        command: uptime`
	cfg := mustParseConfig(t, input)
	node := mustParseNode(t, input)
	errors := ValidateSchema(cfg, node)
	for _, e := range errors {
		if strings.Contains(e.Msg, "unknown field") {
			t.Errorf("unexpected unknown field error: %v", e)
		}
	}
}

func TestFormatAllErrorsEmpty(t *testing.T) {
	err := formatAllErrors(nil)
	if err != nil {
		t.Errorf("expected nil for empty errors, got %v", err)
	}
	err = formatAllErrors([]ValidationError{})
	if err != nil {
		t.Errorf("expected nil for empty slice, got %v", err)
	}
}

func TestFormatAllErrorsNonEmpty(t *testing.T) {
	errors := []ValidationError{
		{Path: "foo", Line: 1, Msg: "bad"},
		{Path: "bar.baz", Line: 5, Msg: "also bad"},
	}
	err := formatAllErrors(errors)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "foo (line 1): bad") {
		t.Errorf("expected error message to contain 'foo (line 1): bad', got %s", msg)
	}
	if !strings.Contains(msg, "bar.baz (line 5): also bad") {
		t.Errorf("expected error message to contain 'bar.baz (line 5): also bad', got %s", msg)
	}
}

func TestValidateXFieldInSection(t *testing.T) {
	input := `sections:
  - title: T
    services: []
    x-custom: value`
	cfg := mustParseConfig(t, input)
	node := mustParseNode(t, input)
	errors := ValidateSchema(cfg, node)
	if len(errors) != 0 {
		t.Errorf("expected no errors for x-* fields in section, got %+v", errors)
	}
}

func TestValidateXFieldInService(t *testing.T) {
	input := `sections:
  - title: T
    services:
      - title: S
        command: echo
        x-note: internal`
	cfg := mustParseConfig(t, input)
	node := mustParseNode(t, input)
	errors := ValidateSchema(cfg, node)
	if len(errors) != 0 {
		t.Errorf("expected no errors for x-* fields in service, got %+v", errors)
	}
}

func TestValidateXFieldInUser(t *testing.T) {
	input := `users:
  alice:
    password: hash
    x-flag: special`
	cfg := mustParseConfig(t, input)
	node := mustParseNode(t, input)
	errors := ValidateSchema(cfg, node)
	if len(errors) != 0 {
		t.Errorf("expected no errors for x-* fields in user, got %+v", errors)
	}
}