package config

import (
	"fmt"
	"os"
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
  x-meta: data
  port: 3000
sections:
  - title: T
    services:
      - title: S
        command: echo`
  cfg := mustParseConfig(t, input)
  node := mustParseNode(t, input)
  errors := ValidateSchema(cfg, node)
  for _, e := range errors {
    if strings.Contains(e.Msg, "unknown field") {
      t.Errorf("unexpected unknown field error: %v", e)
    }
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
    services:
      - title: S
        command: echo
    x-custom: value`
  cfg := mustParseConfig(t, input)
  node := mustParseNode(t, input)
  errors := ValidateSchema(cfg, node)
  for _, e := range errors {
    if strings.Contains(e.Msg, "unknown field") {
      t.Errorf("unexpected unknown field error: %v", e)
    }
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
  input := `sections:
  - title: T
    services:
      - title: S
        command: echo
users:
  alice:
    password: hash
    x-flag: special`
  cfg := mustParseConfig(t, input)
  node := mustParseNode(t, input)
  errors := ValidateSchema(cfg, node)
  for _, e := range errors {
    if strings.Contains(e.Msg, "unknown field") {
      t.Errorf("unexpected unknown field error: %v", e)
    }
  }
}

func TestValidateD04HostnameEmpty(t *testing.T) {
  input := `global:
  hostname: ""
  port: 3000`
  cfg := mustParseConfig(t, input)
  errors := ValidateSchema(cfg, nil)
  found := false
  for _, e := range errors {
    if e.Path == "global.hostname" && strings.Contains(e.Msg, "non-empty") {
      found = true
      break
    }
  }
  if !found {
    t.Errorf("expected hostname empty error, got %+v", errors)
  }
}

func TestValidateD04HostnameNonEmpty(t *testing.T) {
  input := `global:
  hostname: localhost
  port: 3000`
  cfg := mustParseConfig(t, input)
  errors := ValidateSchema(cfg, nil)
  for _, e := range errors {
    if e.Path == "global.hostname" {
      t.Errorf("unexpected hostname error: %v", e)
    }
  }
}

func TestValidateD05PortOutOfRange(t *testing.T) {
  for _, port := range []int{0, -1, 70000} {
    t.Run(fmt.Sprintf("port_%d", port), func(t *testing.T) {
      input := fmt.Sprintf("global:\n  port: %d", port)
      cfg := mustParseConfig(t, input)
      errors := ValidateSchema(cfg, nil)
      found := false
      for _, e := range errors {
        if e.Path == "global.port" && strings.Contains(e.Msg, "1 and 65535") {
          found = true
          break
        }
      }
      if !found {
        t.Errorf("expected port range error for port %d, got %+v", port, errors)
      }
    })
  }
}

func TestValidateD05PortValid(t *testing.T) {
  for _, port := range []int{1, 80, 443, 3000, 65535} {
    t.Run(fmt.Sprintf("port_%d", port), func(t *testing.T) {
      input := fmt.Sprintf("global:\n  port: %d", port)
      cfg := mustParseConfig(t, input)
      errors := ValidateSchema(cfg, nil)
      for _, e := range errors {
        if e.Path == "global.port" {
          t.Errorf("unexpected port error for valid port %d: %v", port, e)
        }
      }
    })
  }
}

func TestValidateD06ColumnsOutOfRange(t *testing.T) {
  for _, ctx := range []struct {
    path string
    yaml string
  }{
    {path: "global.columns", yaml: "global:\n  columns: 0\n  port: 3000"},
    {path: "global.columns", yaml: "global:\n  columns: 13\n  port: 3000"},
    {path: "sections[0].columns", yaml: "sections:\n  - title: T\n    columns: 0\n    services:\n      - title: S\n        command: echo"},
    {path: "sections[0].columns", yaml: "sections:\n  - title: T\n    columns: 15\n    services:\n      - title: S\n        command: echo"},
  } {
    t.Run(ctx.path+"_"+ctx.yaml[:20], func(t *testing.T) {
      cfg := mustParseConfig(t, ctx.yaml)
      errors := ValidateSchema(cfg, nil)
      found := false
      for _, e := range errors {
        if e.Path == ctx.path && strings.Contains(e.Msg, "1 and 12") {
          found = true
          break
        }
      }
      if !found {
        t.Errorf("expected columns range error for %s, got %+v", ctx.path, errors)
      }
    })
  }
}

func TestValidateD06ColumnsValid(t *testing.T) {
  for _, cols := range []int{1, 3, 6, 12} {
    t.Run(fmt.Sprintf("columns_%d", cols), func(t *testing.T) {
      input := fmt.Sprintf("global:\n  columns: %d\n  port: 3000", cols)
      cfg := mustParseConfig(t, input)
      errors := ValidateSchema(cfg, nil)
      for _, e := range errors {
        if e.Path == "global.columns" {
          t.Errorf("unexpected columns error for valid value %d: %v", cols, e)
        }
      }
    })
  }
}

func TestValidateD07TimeoutZero(t *testing.T) {
  for _, ctx := range []struct {
    path string
    yaml string
  }{
    {path: "global.timeout", yaml: "global:\n  timeout: 0\n  port: 3000"},
    {path: "sections[0].timeout", yaml: "sections:\n  - title: T\n    timeout: 0\n    services:\n      - title: S\n        command: echo"},
    {path: "sections[0].services[0].timeout", yaml: "sections:\n  - title: T\n    services:\n      - title: S\n        command: echo\n        timeout: 0"},
  } {
    t.Run(ctx.path, func(t *testing.T) {
      cfg := mustParseConfig(t, ctx.yaml)
      errors := ValidateSchema(cfg, nil)
      found := false
      for _, e := range errors {
        if e.Path == ctx.path && strings.Contains(e.Msg, "greater than 0") {
          found = true
          break
        }
      }
      if !found {
        t.Errorf("expected timeout >0 error for %s, got %+v", ctx.path, errors)
      }
    })
  }
}

func TestValidateD08CommandEmpty(t *testing.T) {
  input := `sections:
  - title: T
    services:
      - title: S
        command: ""`
  cfg := mustParseConfig(t, input)
  errors := ValidateSchema(cfg, nil)
  found := false
  for _, e := range errors {
    if strings.HasPrefix(e.Path, "sections[0].services[0].command") && strings.Contains(e.Msg, "non-empty") {
      found = true
      break
    }
  }
  if !found {
    t.Errorf("expected command empty error, got %+v", errors)
  }
}

func TestValidateD08CommandArrayWithEmpty(t *testing.T) {
  input := `sections:
  - title: T
    services:
      - title: S
        command:
          - "df -h"
          - ""
          - "echo done"`
  cfg := mustParseConfig(t, input)
  errors := ValidateSchema(cfg, nil)
  found := false
  for _, e := range errors {
    if e.Path == "sections[0].services[0].command[1]" && strings.Contains(e.Msg, "non-empty") {
      found = true
      break
    }
  }
  if !found {
    t.Errorf("expected command[1] empty error, got %+v", errors)
  }
}

func TestValidateD09RemoteEmptyString(t *testing.T) {
  for _, ctx := range []struct {
    name string
    path string
    yaml string
  }{
    {name: "global", path: "global.remote", yaml: "global:\n  remote: \"\"\n  port: 3000"},
    {name: "section", path: "sections[0].remote", yaml: "sections:\n  - title: T\n    remote: \"\"\n    services:\n      - title: S\n        command: echo"},
    {name: "service", path: "sections[0].services[0].remote", yaml: "sections:\n  - title: T\n    services:\n      - title: S\n        command: echo\n        remote: \"\""},
  } {
    t.Run(ctx.name, func(t *testing.T) {
      cfg := mustParseConfig(t, ctx.yaml)
      errors := ValidateSchema(cfg, nil)
      found := false
      for _, e := range errors {
        if e.Path == ctx.path+"[0]" && strings.Contains(e.Msg, "non-empty") {
          found = true
          break
        }
      }
      if !found {
        t.Errorf("expected remote empty string error for %s, got %+v", ctx.name, errors)
      }
    })
  }
}

func TestValidateD09RemoteEmptyArrayValid(t *testing.T) {
  input := `sections:
  - title: T
    remote: []
    services:
      - title: S
        command: echo`
  cfg := mustParseConfig(t, input)
  errors := ValidateSchema(cfg, nil)
  for _, e := range errors {
    if strings.Contains(e.Path, "remote") {
      t.Errorf("remote: [] should be valid, got error: %v", e)
    }
  }
}

func TestValidateD11SectionsEmpty(t *testing.T) {
  input := `sections: []`
  cfg := mustParseConfig(t, input)
  errors := ValidateSchema(cfg, nil)
  found := false
  for _, e := range errors {
    if e.Path == "sections" && strings.Contains(e.Msg, "non-empty") {
      found = true
      break
    }
  }
  if !found {
    t.Errorf("expected sections empty error, got %+v", errors)
  }
}

func TestValidateD11SectionMissingTitle(t *testing.T) {
  input := `sections:
  - services:
      - title: S
        command: echo`
  cfg := mustParseConfig(t, input)
  errors := ValidateSchema(cfg, nil)
  found := false
  for _, e := range errors {
    if e.Path == "sections[0].title" && strings.Contains(e.Msg, "required") {
      found = true
      break
    }
  }
  if !found {
    t.Errorf("expected section title required error, got %+v", errors)
  }
}

func TestValidateD11SectionEmptyServices(t *testing.T) {
  input := `sections:
  - title: T
    services: []`
  cfg := mustParseConfig(t, input)
  errors := ValidateSchema(cfg, nil)
  found := false
  for _, e := range errors {
    if e.Path == "sections[0].services" && strings.Contains(e.Msg, "non-empty") {
      found = true
      break
    }
  }
  if !found {
    t.Errorf("expected section services empty error, got %+v", errors)
  }
}

func TestValidateD12UserEmptyPassword(t *testing.T) {
  input := `users:
  alice:
    password: ""`
  cfg := mustParseConfig(t, input)
  errors := ValidateSchema(cfg, nil)
  found := false
  for _, e := range errors {
    if e.Path == "users.alice.password" && strings.Contains(e.Msg, "required") {
      found = true
      break
    }
  }
  if !found {
    t.Errorf("expected user password required error, got %+v", errors)
  }
}

func TestValidateD13AllowedEmptyArrayValid(t *testing.T) {
  input := `global:
  port: 3000
  allowed: []`
  cfg := mustParseConfig(t, input)
  errors := ValidateSchema(cfg, nil)
  for _, e := range errors {
    if strings.Contains(e.Path, "allowed") {
      t.Errorf("allowed: [] should be valid, got error: %v", e)
    }
  }
}

func TestValidateSchemaCollectsMultipleErrors(t *testing.T) {
  input := `global:
  hostname: ""
  port: 0
  columns: 20
  timeout: -5
sections:
  - services: []`
  cfg := mustParseConfig(t, input)
  node := mustParseNode(t, input)
  errors := ValidateSchema(cfg, node)
  if len(errors) < 4 {
    t.Errorf("expected at least 4 errors, got %d: %+v", len(errors), errors)
  }
}

func TestValidateGroupRefsValidGroup(t *testing.T) {
	input := `sections:
  - title: T
    services:
      - title: S
        command: echo
        allowed: [admins]
users:
  alice:
    password: hash
    groups: [admins]`
	cfg := mustParseConfig(t, input)
	errors := validateGroupRefs(cfg)
	for _, e := range errors {
		if strings.Contains(e.Path, "allowed") {
			t.Errorf("unexpected allowed error: %v", e)
		}
	}
}

func TestValidateGroupRefsValidUsername(t *testing.T) {
	input := `sections:
  - title: T
    services:
      - title: S
        command: echo
        allowed: [alice]
users:
  alice:
    password: hash`
	cfg := mustParseConfig(t, input)
	errors := validateGroupRefs(cfg)
	for _, e := range errors {
		if strings.Contains(e.Path, "allowed") {
			t.Errorf("unexpected allowed error: %v", e)
		}
	}
}

func TestValidateGroupRefsInvalidGroup(t *testing.T) {
	input := `sections:
  - title: T
    services:
      - title: S
        command: echo
        allowed: [nonexistent]
users:
  alice:
    password: hash`
	cfg := mustParseConfig(t, input)
	errors := validateGroupRefs(cfg)
	found := false
	for _, e := range errors {
		if e.Path == "sections[0].services[0].allowed" && strings.Contains(e.Msg, "unknown user or group") && strings.Contains(e.Msg, "nonexistent") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected unknown group error, got %+v", errors)
	}
}

func TestValidateGroupRefsInvalidInService(t *testing.T) {
	input := `sections:
  - title: T
    services:
      - title: S
        command: echo
        allowed: [nogroup]
users:
  alice:
    password: hash`
	cfg := mustParseConfig(t, input)
	errors := validateGroupRefs(cfg)
	found := false
	for _, e := range errors {
		if e.Path == "sections[0].services[0].allowed" && strings.Contains(e.Msg, "nogroup") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected error at sections[0].services[0].allowed, got %+v", errors)
	}
}

func TestValidateGroupRefsInvalidInGlobal(t *testing.T) {
	input := `global:
  allowed: [noone]
users:
  alice:
    password: hash
sections:
  - title: T
    services:
      - title: S
        command: echo`
	cfg := mustParseConfig(t, input)
	errors := validateGroupRefs(cfg)
	found := false
	for _, e := range errors {
		if e.Path == "global.allowed" && strings.Contains(e.Msg, "noone") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected error at global.allowed, got %+v", errors)
	}
}

func TestValidateGroupRefsInvalidInSection(t *testing.T) {
	input := `sections:
  - title: T
    allowed: [nogroup]
    services:
      - title: S
        command: echo
users:
  alice:
    password: hash`
	cfg := mustParseConfig(t, input)
	errors := validateGroupRefs(cfg)
	found := false
	for _, e := range errors {
		if e.Path == "sections[0].allowed" && strings.Contains(e.Msg, "nogroup") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected error at sections[0].allowed, got %+v", errors)
	}
}

func TestValidateGroupRefsMixedValidInvalid(t *testing.T) {
	input := `sections:
  - title: T
    services:
      - title: S
        command: echo
        allowed: [admins, nonexistent]
users:
  alice:
    password: hash
    groups: [admins]`
	cfg := mustParseConfig(t, input)
	errors := validateGroupRefs(cfg)
	if len(errors) != 1 {
		t.Fatalf("expected 1 error, got %d: %+v", len(errors), errors)
	}
	if errors[0].Path != "sections[0].services[0].allowed" || !strings.Contains(errors[0].Msg, "nonexistent") {
		t.Errorf("expected error for nonexistent only, got %+v", errors[0])
	}
	if strings.Contains(errors[0].Msg, "admins") {
		t.Errorf("admins should be valid, not reported as error: %+v", errors[0])
	}
}

func TestValidateGroupRefsNilAllowed(t *testing.T) {
	input := `sections:
  - title: T
    services:
      - title: S
        command: echo
users:
  alice:
    password: hash`
	cfg := mustParseConfig(t, input)
	errors := validateGroupRefs(cfg)
	if len(errors) != 0 {
		t.Errorf("nil allowed should produce no cross-ref errors, got %+v", errors)
	}
}

func TestValidateGroupRefsEmptyAllowed(t *testing.T) {
	input := `global:
  port: 3000
  allowed: []
sections:
  - title: T
    allowed: []
    services:
      - title: S
        command: echo
        allowed: []
users:
  alice:
    password: hash`
	cfg := mustParseConfig(t, input)
	errors := validateGroupRefs(cfg)
	if len(errors) != 0 {
		t.Errorf("empty allowed arrays should produce no cross-ref errors, got %+v", errors)
	}
}

func TestValidateGroupRefsCombinedWithOtherErrors(t *testing.T) {
	input := `global:
  port: 0
  allowed: [noone]
sections:
  - title: ""
    services:
      - title: S
        command: echo
users:
  alice:
    password: ""`
	cfg := mustParseConfig(t, input)
	errors := ValidateSchema(cfg, nil)
	foundGroupRef := false
	foundPort := false
	foundTitle := false
	foundPassword := false
	for _, e := range errors {
		if e.Path == "global.allowed" && strings.Contains(e.Msg, "noone") {
			foundGroupRef = true
		}
		if e.Path == "global.port" {
			foundPort = true
		}
		if e.Path == "sections[0].title" {
			foundTitle = true
		}
		if e.Path == "users.alice.password" {
			foundPassword = true
		}
	}
	if !foundGroupRef {
		t.Errorf("expected group ref error, got %+v", errors)
	}
	if !foundPort {
		t.Errorf("expected port error, got %+v", errors)
	}
	if !foundTitle {
		t.Errorf("expected title error, got %+v", errors)
	}
	if !foundPassword {
		t.Errorf("expected password error, got %+v", errors)
	}
}

func TestLoadInvalidConfigPreventsStart(t *testing.T) {
	content := `unknown_field: oops
global:
  port: 0
  timeout: -1
  allowed: [nonexistent]
sections:
  - title: ""
    services: []
users:
  bob:
    password: ""`
	path := writeTestConfig(t, content)
	defer func() { _ = os.Remove(path) }()

	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for multiple validation problems, got nil")
	}
	msg := err.Error()
	for _, sub := range []string{"unknown_field", "port", "timeout", "title", "password", "nonexistent", "services"} {
		if !strings.Contains(msg, sub) {
			t.Errorf("expected error to contain %q, got: %s", sub, msg)
		}
	}
}

func TestLoadValidConfigWithXFields(t *testing.T) {
	content := `x-comment: integration test
global:
  x-meta: data
  port: 3000
sections:
  - title: T
    x-info: section note
    services:
      - title: S
        command: echo
        x-note: service note
users:
  alice:
    password: '$2y$10$hash'
    x-flag: admin user`
	path := writeTestConfig(t, content)
	defer func() { _ = os.Remove(path) }()

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("expected no error loading config with x-* fields, got: %v", err)
	}
	if len(cfg.Sections) != 1 || cfg.Sections[0].Title != "T" {
		t.Errorf("expected section title 'T', got %v", cfg.Sections)
	}
}

func TestLoadConfigWithAllowedEmptyAndNil(t *testing.T) {
	t.Run("empty_array", func(t *testing.T) {
		content := `global:
  port: 3000
  allowed: []
sections:
  - title: T
    allowed: []
    services:
      - title: S
        command: echo
        allowed: []`
		path := writeTestConfig(t, content)
		defer func() { _ = os.Remove(path) }()

		cfg, err := Load(path)
		if err != nil {
			t.Fatalf("expected no error for allowed: [], got: %v", err)
		}
		if cfg.Global == nil || len(cfg.Global.Allowed) != 0 {
			t.Error("expected global allowed to be empty slice")
		}
	})

	t.Run("nil_allowed", func(t *testing.T) {
		content := `global:
  port: 3000
sections:
  - title: T
    services:
      - title: S
        command: echo`
		path := writeTestConfig(t, content)
		defer func() { _ = os.Remove(path) }()

		cfg, err := Load(path)
		if err != nil {
			t.Fatalf("expected no error for omitted allowed, got: %v", err)
		}
		if cfg.Global == nil || cfg.Global.Allowed != nil {
			t.Error("expected global allowed to be nil when omitted")
		}
	})
}

func TestLoadConfigWithRemoteEmpty(t *testing.T) {
	content := `global:
  port: 3000
sections:
  - title: T
    remote: []
    services:
      - title: S
        command: echo
        remote: []`
	path := writeTestConfig(t, content)
	defer func() { _ = os.Remove(path) }()

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("expected no error for remote: [], got: %v", err)
	}
	if len(cfg.Sections[0].Remote) != 0 {
		t.Error("expected section remote to be empty slice")
	}
	if len(cfg.Sections[0].Services[0].Remote) != 0 {
		t.Error("expected service remote to be empty slice")
	}
}