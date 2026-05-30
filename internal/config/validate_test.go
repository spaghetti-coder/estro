package config

import (
	"os"
	"strings"
	"testing"

	"go.yaml.in/yaml/v4"
)

func TestExtraCapturesUnknownKeys(t *testing.T) {
	src := `
x-anchor: &a
  timeout: 5
  hstnm: bad
global:
  <<: *a
  hostname: 0.0.0.0
  titl: oops
sektions: 1
`
	var c Config
	if err := yaml.Load([]byte(src), &c); err != nil {
		t.Fatalf("load: %v", err)
	}
	if _, ok := c.Extra["sektions"]; !ok {
		t.Error("expected top-level 'sektions' captured in Config.Extra")
	}
	if _, ok := c.Extra["x-anchor"]; !ok {
		t.Error("expected 'x-anchor' captured in Config.Extra")
	}
	if c.Global == nil {
		t.Fatal("global nil")
	}
	if _, ok := c.Global.Extra["titl"]; !ok {
		t.Error("expected 'titl' captured in Global.Extra")
	}
	if _, ok := c.Global.Extra["hstnm"]; !ok {
		t.Error("expected merged-in 'hstnm' captured in Global.Extra")
	}
}

func TestExtraCapturesUnknownKeysDeep(t *testing.T) {
	src := `
users:
  alice:
    password: x
    pasword: typo
sections:
  - title: S
    sektion_typo: 1
    services:
      - title: T
        command: echo
        comand: typo
`
	var c Config
	if err := yaml.Load([]byte(src), &c); err != nil {
		t.Fatalf("load: %v", err)
	}

	alice, ok := c.Users["alice"]
	if !ok || alice == nil {
		t.Fatal("expected users[alice] to be present")
	}
	if _, ok := alice.Extra["pasword"]; !ok {
		t.Error("expected 'pasword' typo captured in users[alice].Extra")
	}

	if len(c.Sections) == 0 {
		t.Fatal("expected at least one section")
	}
	sec := c.Sections[0]
	if _, ok := sec.Extra["sektion_typo"]; !ok {
		t.Error("expected 'sektion_typo' captured in sections[0].Extra")
	}

	if len(sec.Services) == 0 {
		t.Fatal("expected at least one service in sections[0]")
	}
	svc := sec.Services[0]
	if _, ok := svc.Extra["comand"]; !ok {
		t.Error("expected 'comand' typo captured in sections[0].services[0].Extra")
	}
}

func TestAllowedRef(t *testing.T) {
	users := map[string]*UserConfig{
		"alice": {Password: "x", Groups: StringList{"admins"}},
		"bob":   {Password: "y"},
	}
	tests := []struct {
		name    string
		allowed StringList
		wantErr bool
	}{
		{"existing user", StringList{"alice"}, false},
		{"existing group", StringList{"admins"}, false},
		{"user and group", StringList{"alice", "admins"}, false},
		{"unknown name", StringList{"ghost"}, true},
		{"valid plus unknown", StringList{"alice", "ghost"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Config{
				Users: users,
				Sections: []SectionConfig{{
					Title:         "S",
					CascadeFields: CascadeFields{Allowed: tt.allowed},
					Services:      []ServiceConfig{{Title: "t", Command: CommandValue{"echo"}}},
				}},
			}
			err := validate.Struct(cfg)
			if tt.wantErr && err == nil {
				t.Errorf("expected error for allowed=%v, got nil", tt.allowed)
			}
			if !tt.wantErr && err != nil {
				t.Errorf("expected no error for allowed=%v, got %v", tt.allowed, err)
			}
		})
	}
}

func TestIssueString(t *testing.T) {
	if got := (Issue{Path: "global.port", Msg: "invalid value"}).String(); got != "`global.port` invalid value" {
		t.Errorf("got %q", got)
	}
	if got := (Issue{Msg: "Configuration file can't be read"}).String(); got != "Configuration file can't be read" {
		t.Errorf("got %q", got)
	}
}

func TestFormatPath(t *testing.T) {
	cases := map[string]string{
		"Config.global.CascadeFields.allowed[1]":                 "global.allowed[1]",
		"Config.sections[0].services[3].title":                   "sections[0].services[3].title",
		"Config.sections[0].LayoutFields.columns":                "sections[0].columns",
		"Config.users[bob].password":                             "users.bob.password",
		"Config.sections[0].services[1].CascadeFields.remote[0]": "sections[0].services[1].remote[0]",
	}
	for in, want := range cases {
		if got := formatPath(in); got != want {
			t.Errorf("formatPath(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestUnknownKeyIssues(t *testing.T) {
	c := &Config{
		Extra:  map[string]yaml.Node{"sektions": {}, "x-ok": {}},
		Global: &GlobalConfig{Extra: map[string]yaml.Node{"titl": {}}},
		Sections: []SectionConfig{{
			Title:    "s",
			Services: []ServiceConfig{{Title: "t", Extra: map[string]yaml.Node{"cmdd": {}}}},
			Extra:    map[string]yaml.Node{"x-anchor": {}},
		}},
	}
	got := dedupeSort(unknownKeyIssues(c))
	want := []Issue{
		{Path: "global.titl", Msg: "invalid field"},
		{Path: "sections[0].services[0].cmdd", Msg: "invalid field"},
		{Path: "sektions", Msg: "invalid field"},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d issues %v, want %d", len(got), got, len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("issue %d: got %+v want %+v", i, got[i], want[i])
		}
	}
}

func TestDedupeSort(t *testing.T) {
	in := []Issue{
		{Path: "b", Msg: "x"},
		{Path: "a", Msg: "y"},
		{Path: "a", Msg: "y"}, // dup
	}
	got := dedupeSort(in)
	if len(got) != 2 || got[0].Path != "a" || got[1].Path != "b" {
		t.Fatalf("got %v", got)
	}
}

// ptrOf returns a pointer to v; used in table-driven constraint tests.
func ptrOf[T any](v T) *T { return &v }

// TestValueConstraints verifies that individual field constraints are enforced
// by the validator and that a fully-valid minimal Config passes without error.
func TestValueConstraints(t *testing.T) {
	// minimalValid returns a minimal valid Config used as a starting point.
	minimalValid := func() Config {
		return Config{
			Sections: []SectionConfig{{
				Title:    "S",
				Services: []ServiceConfig{{Title: "T", Command: CommandValue{"echo"}}},
			}},
		}
	}

	// rejected holds cases expected to fail validation.
	rejected := []struct {
		name string
		cfg  Config
	}{
		{
			name: "port zero",
			cfg: func() Config {
				c := minimalValid()
				c.Global = &GlobalConfig{Port: ptrOf(0)}
				return c
			}(),
		},
		{
			name: "port too large",
			cfg: func() Config {
				c := minimalValid()
				c.Global = &GlobalConfig{Port: ptrOf(70000)}
				return c
			}(),
		},
		{
			name: "columns zero",
			cfg: func() Config {
				c := minimalValid()
				c.Global = &GlobalConfig{LayoutFields: LayoutFields{Columns: ptrOf(0)}}
				return c
			}(),
		},
		{
			name: "columns too large",
			cfg: func() Config {
				c := minimalValid()
				c.Global = &GlobalConfig{LayoutFields: LayoutFields{Columns: ptrOf(13)}}
				return c
			}(),
		},
		{
			name: "invalid hostname",
			cfg: func() Config {
				c := minimalValid()
				c.Global = &GlobalConfig{Hostname: ptrOf("not a host!")}
				return c
			}(),
		},
		{
			name: "command empty slice",
			cfg: func() Config {
				c := minimalValid()
				c.Sections[0].Services[0].Command = CommandValue{}
				return c
			}(),
		},
		{
			name: "command empty element",
			cfg: func() Config {
				c := minimalValid()
				c.Sections[0].Services[0].Command = CommandValue{""}
				return c
			}(),
		},
		{
			name: "no sections",
			cfg: Config{
				Sections: nil,
			},
		},
		{
			name: "section with no services",
			cfg: Config{
				Sections: []SectionConfig{{
					Title:    "S",
					Services: nil,
				}},
			},
		},
		{
			name: "user with empty password",
			cfg: func() Config {
				c := minimalValid()
				c.Users = map[string]*UserConfig{
					"alice": {Password: ""},
				}
				return c
			}(),
		},
	}

	for _, tt := range rejected {
		t.Run(tt.name, func(t *testing.T) {
			if err := validate.Struct(tt.cfg); err == nil {
				t.Errorf("expected validation error for %q, got nil", tt.name)
			}
		})
	}

	t.Run("valid minimal config", func(t *testing.T) {
		cfg := Config{
			Global: &GlobalConfig{
				Hostname:     ptrOf("0.0.0.0"),
				Port:         ptrOf(3000),
				LayoutFields: LayoutFields{Columns: ptrOf(3)},
			},
			Users: map[string]*UserConfig{
				"alice": {Password: "secret"},
			},
			Sections: []SectionConfig{{
				Title: "S",
				Services: []ServiceConfig{{
					Title:   "T",
					Command: CommandValue{"echo hello"},
				}},
			}},
		}
		if err := validate.Struct(cfg); err != nil {
			t.Errorf("expected valid config to pass, got: %v", err)
		}
	})
}

func TestLoadCollectsAllIssues(t *testing.T) {
	src := `
global:
  columns: 99
  hostname: "not a host"
sections:
  - title: ""
    services:
      - title: ok
        command: echo
        titl: typo
`
	path := writeTestConfig(t, src)
	defer func() { _ = os.Remove(path) }()

	res := Load(path)
	if res.Healthy() {
		t.Fatal("expected issues")
	}
	if res.Config == nil {
		t.Fatal("Config must always be non-nil")
	}
	joined := ""
	for _, s := range res.IssueStrings() {
		joined += s + "\n"
	}
	for _, want := range []string{
		"`global.columns` invalid value",
		"`global.hostname` invalid value",
		"`sections[0].title` required",
		"`sections[0].services[0].titl` invalid field",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing issue %q in:\n%s", want, joined)
		}
	}
}

func TestLoadUnreadableFile(t *testing.T) {
	res := Load("/nonexistent/path/config.yaml")
	if res.Healthy() {
		t.Fatal("expected degraded result")
	}
	if res.IssueStrings()[0] != "Configuration file can't be read" {
		t.Errorf("got %q", res.IssueStrings()[0])
	}
}

func TestLoadAcceptsXStarEverywhere(t *testing.T) {
	src := `
x-top: &a
  timeout: 5
global:
  <<: *a
  x-note: anything
sections:
  - title: S
    x-section-note: ok
    services:
      - title: T
        command: echo
        x-svc-note: ok
`
	path := writeTestConfig(t, src)
	defer func() { _ = os.Remove(path) }()
	res := Load(path)
	if !res.Healthy() {
		t.Fatalf("expected healthy (x-* valid everywhere), got: %v", res.IssueStrings())
	}
}

func TestLoadEmptyFile(t *testing.T) {
	path := writeTestConfig(t, "   \n")
	defer func() { _ = os.Remove(path) }()
	res := Load(path)
	if res.Healthy() {
		t.Fatal("expected issues for empty file")
	}
	got := res.IssueStrings()
	if len(got) != 1 || got[0] != "`sections` required" {
		t.Errorf("expected exactly [`sections` required], got %v", got)
	}
}

func TestAllowedRefZeroMemberGroup(t *testing.T) {
	cfg := Config{
		Users: map[string]*UserConfig{"alice": {Password: "x", Groups: StringList{"admins"}}},
		Sections: []SectionConfig{{
			Title:         "S",
			CascadeFields: CascadeFields{Allowed: StringList{"editors"}}, // no user has group "editors"
			Services:      []ServiceConfig{{Title: "t", Command: CommandValue{"echo"}}},
		}},
	}
	if err := validate.Struct(cfg); err == nil {
		t.Fatal("expected error: 'editors' is neither a user nor a referenced group")
	}
}

func TestServerAddrPortFallback(t *testing.T) {
	tests := []struct {
		name string
		cfg  *Config
		want string
	}{
		{"valid port", &Config{Global: &GlobalConfig{Port: ptrOf(8080)}}, "127.0.0.1:8080"},
		{"valid host and port", &Config{Global: &GlobalConfig{Hostname: ptrOf("0.0.0.0"), Port: ptrOf(8080)}}, "0.0.0.0:8080"},
		{"out-of-range port", &Config{Global: &GlobalConfig{Port: ptrOf(99999)}}, "127.0.0.1:3000"},
		{"zero port (type-mismatch resolved)", &Config{Global: &GlobalConfig{Port: ptrOf(0)}}, "127.0.0.1:3000"},
		{"no global", &Config{}, "127.0.0.1:3000"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &LoadResult{Config: tt.cfg}
			if got := r.ServerAddr(); got != tt.want {
				t.Errorf("ServerAddr() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestServerAddrInvalidHostnameFallback(t *testing.T) {
	// An invalid-format hostname is flagged by validation; ServerAddr must default it.
	path := writeTestConfig(t, "global:\n  hostname: \"not a host\"\n")
	defer func() { _ = os.Remove(path) }()
	res := Load(path)
	if !res.hasIssue("global.hostname") {
		t.Fatalf("expected a global.hostname issue, got %v", res.IssueStrings())
	}
	if got := res.ServerAddr(); got != "127.0.0.1:3000" {
		t.Errorf("ServerAddr() = %q, want 127.0.0.1:3000", got)
	}
}

func TestLoadTypeMismatchPortFallsBackTo3000(t *testing.T) {
	path := writeTestConfig(t, "global:\n  port: \"abc\"\n  columns: \"xyz\"\n")
	defer func() { _ = os.Remove(path) }()
	res := Load(path)
	if res.Healthy() {
		t.Fatal("expected degraded result for type mismatches")
	}
	// Both type errors surface in one pass; pin that the config is unhealthy
	// and the bind address still falls back to the default port.
	if got := res.ServerAddr(); got != "127.0.0.1:3000" {
		t.Errorf("ServerAddr() = %q, want 127.0.0.1:3000 for a wrong-type port", got)
	}
}

func TestLoadTypeMismatchTopLevel(t *testing.T) {
	path := writeTestConfig(t, "sections: 123\n") // sections present but wrong type
	defer func() { _ = os.Remove(path) }()
	res := Load(path)
	got := res.IssueStrings()
	if len(got) != 1 || got[0] != "`sections` invalid type" {
		t.Errorf("got %v, want [`sections` invalid type]", got)
	}
}

func TestLoadTypeMismatchNestedNoDuplicate(t *testing.T) {
	// port wrong type; must be exactly one "invalid type" (not also "invalid value"/"required")
	src := "global:\n  port: \"abc\"\nsections:\n  - title: S\n    services:\n      - title: T\n        command: echo\n"
	path := writeTestConfig(t, src)
	defer func() { _ = os.Remove(path) }()
	res := Load(path)
	got := res.IssueStrings()
	if len(got) != 1 || got[0] != "`global.port` invalid type" {
		t.Errorf("got %v, want [`global.port` invalid type]", got)
	}
}

func TestDedupeSortPerPathPriority(t *testing.T) {
	in := []Issue{
		{Path: "global.port", Msg: "invalid value"},
		{Path: "global.port", Msg: "invalid type"},
		{Path: "sections", Msg: "required"},
	}
	got := dedupeSort(in)
	if len(got) != 2 {
		t.Fatalf("expected 2 issues, got %v", got)
	}
	if got[0].Path != "global.port" || got[0].Msg != "invalid type" {
		t.Errorf("got[0] = %+v, want {global.port invalid type}", got[0])
	}
}

func TestLoadRejectsEmptyStringListElement(t *testing.T) {
	src := `
sections:
  - title: S
    remote: 'h1,,h2'
    services:
      - title: T
        command: echo
`
	path := writeTestConfig(t, src)
	defer func() { _ = os.Remove(path) }()

	res := Load(path)
	if res.Healthy() {
		t.Fatal("expected an issue for the empty remote element")
	}
	joined := strings.Join(res.IssueStrings(), "\n")
	if !strings.Contains(joined, "remote[1]") {
		t.Errorf("expected an issue mentioning remote[1], got:\n%s", joined)
	}
}

func TestLoadXStarAnchorMergeNoLeak(t *testing.T) {
	src := "x-foo: &foo\n  timeout: fff\nglobal:\n  <<: *foo\nsections:\n  - title: S\n    services:\n      - title: T\n        command: echo\n"
	path := writeTestConfig(t, src)
	defer func() { _ = os.Remove(path) }()
	res := Load(path)
	got := res.IssueStrings()
	for _, s := range got {
		if strings.Contains(s, "x-foo") {
			t.Errorf("issue leaks x-* anchor internals: %q (all: %v)", s, got)
		}
	}
	if len(got) != 1 || got[0] != "`global.timeout` invalid value" {
		t.Errorf("got %v, want [`global.timeout` invalid value]", got)
	}
}

func TestLoadHostnameWrongTypeIsInvalidTypeOnce(t *testing.T) {
	src := "global:\n  hostname: [foobar]\nsections:\n  - title: S\n    services:\n      - title: T\n        command: echo\n"
	path := writeTestConfig(t, src)
	defer func() { _ = os.Remove(path) }()
	res := Load(path)
	got := res.IssueStrings()
	if len(got) != 1 || got[0] != "`global.hostname` invalid type" {
		t.Errorf("got %v, want [`global.hostname` invalid type]", got)
	}
}
