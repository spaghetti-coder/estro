package config

import (
	"os"
	"reflect"
	"strings"
	"testing"

	"go.yaml.in/yaml/v4"
)

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

// TestInlineSegReDerivedFromEmbeds locks in that the embedded-struct segments
// stripped by formatPath are derived from the schema, so renaming or adding an
// embedded struct can't silently leave stale segments in issue paths.
func TestInlineSegReDerivedFromEmbeds(t *testing.T) {
	names := embeddedStructNames(reflect.TypeOf(Config{}), map[reflect.Type]bool{})
	if len(names) == 0 {
		t.Fatal("expected embedded structs (CascadeFields/LayoutFields) to be discovered")
	}
	for _, n := range names {
		if !inlineSegRe.MatchString("." + n) {
			t.Errorf("inlineSegRe does not strip embedded segment %q", n)
		}
	}
	if got := formatPath("Config.global.CascadeFields.timeout"); got != "global.timeout" {
		t.Errorf("formatPath = %q, want global.timeout", got)
	}
}

// TestSchemaShapeCoverage fails if any config field has a type the shape walk
// (checkFieldShape) does not explicitly handle, forcing a conscious update when
// an exotic field type is added.
func TestSchemaShapeCoverage(t *testing.T) {
	scalarKinds := map[reflect.Kind]bool{
		reflect.String: true, reflect.Bool: true,
		reflect.Int: true, reflect.Int8: true, reflect.Int16: true, reflect.Int32: true, reflect.Int64: true,
		reflect.Uint: true, reflect.Uint8: true, reflect.Uint16: true, reflect.Uint32: true, reflect.Uint64: true,
		reflect.Float32: true, reflect.Float64: true,
	}
	custom := map[reflect.Type]bool{
		reflect.TypeOf(StringList(nil)):   true,
		reflect.TypeOf(CommandValue(nil)): true,
	}
	seen := map[reflect.Type]bool{}
	var walk func(reflect.Type)
	walk = func(typ reflect.Type) {
		if seen[typ] {
			return
		}
		seen[typ] = true
		for _, f := range schemaFields(typ) {
			ft := derefType(f.Type)
			switch {
			case custom[ft], scalarKinds[ft.Kind()]:
				// leaf, handled
			case ft.Kind() == reflect.Slice:
				if et := derefType(ft.Elem()); et.Kind() == reflect.Struct {
					walk(et)
				}
			case ft.Kind() == reflect.Map:
				if vt := derefType(ft.Elem()); vt.Kind() == reflect.Struct {
					walk(vt)
				}
			case ft.Kind() == reflect.Struct:
				walk(ft)
			default:
				t.Errorf("field %q type %s (kind %s) not handled by checkFieldShape — extend it and this test", f.Name, f.Type, ft.Kind())
			}
		}
	}
	walk(reflect.TypeOf(Config{}))
}

// TestWrongTypeMergesToOneIssue pins that a wrong-typed field yields exactly one
// issue (the shape "invalid value" subsumes any consequent validator "required"),
// keeping the two passes' path formats in agreement.
func TestWrongTypeMergesToOneIssue(t *testing.T) {
	withSvc := "\nsections:\n  - title: S\n    services:\n      - title: T\n        command: echo\n"
	tests := []struct{ name, src, path string }{
		{"required slice given a map", "sections: {a: b}\n", "sections"},
		{"global scalar given a map", "global:\n  port: {a: b}" + withSvc, "global.port"},
		{"global stringlist given a map", "global:\n  allowed: {a: b}" + withSvc, "global.allowed"},
		{"section cascade field given a map", "sections:\n  - title: S\n    timeout: {a: b}\n    services:\n      - title: T\n        command: echo\n", "sections[0].timeout"},
		{"service field given a map", "sections:\n  - title: S\n    services:\n      - title: T\n        command: echo\n        timeout: {a: b}\n", "sections[0].services[0].timeout"},
		{"user map entry given a map", "users:\n  bob:\n    password: x\n    groups: {a: b}" + withSvc, "users.bob.groups"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeTestConfig(t, tt.src)
			defer func() { _ = os.Remove(path) }()
			n := 0
			for _, is := range Load(path).Issues {
				if is.Path == tt.path {
					n++
				}
			}
			if n != 1 {
				t.Errorf("expected exactly one issue at %q, got %d (all: %v)", tt.path, n, Load(path).IssueStrings())
			}
		})
	}
}

// TestLoadPopulatesValidFieldsDespiteTypeError guards the intentionally-ignored
// decode error in Load: a wrong-typed field must not stop valid sibling fields
// from populating (so validateStruct still sees real values).
func TestLoadPopulatesValidFieldsDespiteTypeError(t *testing.T) {
	src := "global:\n  title: MyApp\n  port: {a: b}\nsections:\n  - title: S\n    services:\n      - title: T\n        command: echo\n"
	path := writeTestConfig(t, src)
	defer func() { _ = os.Remove(path) }()
	g := Load(path).Config.Global
	if g == nil || g.Title == nil || *g.Title != "MyApp" {
		t.Fatalf("expected global.title=MyApp to populate despite a sibling type error, got %+v", g)
	}
}

func TestExpandEstroEnv(t *testing.T) {
	t.Setenv("ESTRO_TEST_HOST", "0.0.0.0")
	t.Setenv("ESTRO_TEST_SECRET", "s3cr3t")
	src := "global:\n" +
		"  hostname: \"{estro_env.ESTRO_TEST_HOST}\"\n" +
		"  secret: \"{estro_env.ESTRO_TEST_SECRET}\"\n" +
		"sections:\n  - title: S\n    services:\n      - title: T\n        command: echo {estro_env.ESTRO_TEST_HOST} ${RUNTIME}\n"
	path := writeTestConfig(t, src)
	defer func() { _ = os.Remove(path) }()
	res := Load(path)
	if !res.Healthy() {
		t.Fatalf("expected healthy, got %v", res.IssueStrings())
	}
	g := res.Config.Global
	if g.Hostname == nil || *g.Hostname != "0.0.0.0" {
		t.Errorf("hostname = %v, want 0.0.0.0", g.Hostname)
	}
	if g.Secret == nil || *g.Secret != "s3cr3t" {
		t.Errorf("secret = %v, want s3cr3t", g.Secret)
	}
	// {estro_env.X} is expanded at load; ${RUNTIME} is left for the shell.
	cmd := res.Config.Sections[0].Services[0].Command
	if len(cmd) != 1 || cmd[0] != "echo 0.0.0.0 ${RUNTIME}" {
		t.Errorf("command = %v, want [echo 0.0.0.0 ${RUNTIME}]", cmd)
	}
}

func TestExpandEstroEnvUnsetIsIssue(t *testing.T) {
	_ = os.Unsetenv("ESTRO_UNSET_VAR_XYZ")
	src := "global:\n  secret: \"{estro_env.ESTRO_UNSET_VAR_XYZ}\"\nsections:\n  - title: S\n    services:\n      - title: T\n        command: echo hi\n"
	path := writeTestConfig(t, src)
	defer func() { _ = os.Remove(path) }()
	res := Load(path)
	if res.Healthy() {
		t.Fatal("expected an issue for the unset env var")
	}
	if !strings.Contains(strings.Join(res.IssueStrings(), "\n"), "ESTRO_UNSET_VAR_XYZ is not set") {
		t.Errorf("expected unset-env issue, got %v", res.IssueStrings())
	}
}

// TestUnknownKeyBehavior verifies that unknown YAML keys are detected by the
// shape walk and reported as "invalid field" at every level, that x-* keys are
// never reported, and that keys introduced via YAML merge (<<: *anchor) are
// caught at the use site.
func TestUnknownKeyBehavior(t *testing.T) {
	tests := []struct {
		name      string
		src       string
		wantPaths []string // paths that must carry "invalid field"
		notPaths  []string // paths that must NOT appear
	}{
		{
			name:      "top-level typo",
			src:       "sektions: 1\nsections:\n  - title: S\n    services:\n      - title: T\n        command: echo\n",
			wantPaths: []string{"sektions"},
		},
		{
			name:      "global typo",
			src:       "global:\n  titl: x\nsections:\n  - title: S\n    services:\n      - title: T\n        command: echo\n",
			wantPaths: []string{"global.titl"},
		},
		{
			name: "service typo",
			src: `sections:
  - title: S
    services:
      - title: T
        command: echo
        comand: typo
`,
			wantPaths: []string{"sections[0].services[0].comand"},
		},
		{
			name: "user typo",
			src: `users:
  bob:
    password: x
    pasword: y
sections:
  - title: S
    services:
      - title: T
        command: echo
`,
			wantPaths: []string{"users.bob.pasword"},
		},
		{
			name: "bad key merged via anchor at global level",
			src: `x-anchor: &a
  hstnm: bad
global:
  <<: *a
sections:
  - title: S
    services:
      - title: T
        command: echo
`,
			wantPaths: []string{"global.hstnm"},
		},
		{
			name: "x-* keys are NOT reported",
			src: `x-top: anything
global:
  x-note: ok
sections:
  - title: S
    x-section: ok
    services:
      - title: T
        command: echo
        x-svc: ok
`,
			wantPaths: nil,
			notPaths:  []string{"x-top", "global.x-note", "sections[0].x-section", "sections[0].services[0].x-svc"},
		},
		{
			name: "section typo",
			src: `sections:
  - title: S
    sektion_typo: 1
    services:
      - title: T
        command: echo
`,
			wantPaths: []string{"sections[0].sektion_typo"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeTestConfig(t, tt.src)
			defer func() { _ = os.Remove(path) }()
			res := Load(path)

			for _, wantPath := range tt.wantPaths {
				found := false
				for _, is := range res.Issues {
					if is.Path == wantPath && is.Msg == "invalid field" {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected issue {Path:%q, Msg:\"invalid field\"}, got: %v", wantPath, res.Issues)
				}
			}

			for _, notPath := range tt.notPaths {
				for _, is := range res.Issues {
					if is.Path == notPath {
						t.Errorf("unexpected issue at %q: %v", notPath, is)
					}
				}
			}
		})
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

func TestLoadWrongShapeTopLevel(t *testing.T) {
	path := writeTestConfig(t, "sections: 123\n") // sections present but wrong shape
	defer func() { _ = os.Remove(path) }()
	res := Load(path)
	got := res.IssueStrings()
	if len(got) != 1 || got[0] != "`sections` invalid value" {
		t.Errorf("got %v, want [`sections` invalid value]", got)
	}
}

func TestLoadWrongTypeScalarIsInvalidValue(t *testing.T) {
	// port given a non-int scalar: exactly one "invalid value" (not also "required")
	src := "global:\n  port: \"abc\"\nsections:\n  - title: S\n    services:\n      - title: T\n        command: echo\n"
	path := writeTestConfig(t, src)
	defer func() { _ = os.Remove(path) }()
	res := Load(path)
	got := res.IssueStrings()
	if len(got) != 1 || got[0] != "`global.port` invalid value" {
		t.Errorf("got %v, want [`global.port` invalid value]", got)
	}
}

func TestDedupeSortPerPathPriority(t *testing.T) {
	in := []Issue{
		{Path: "global.port", Msg: "required"},
		{Path: "global.port", Msg: "invalid value"},
		{Path: "sections", Msg: "required"},
	}
	got := dedupeSort(in)
	if len(got) != 2 {
		t.Fatalf("expected 2 issues, got %v", got)
	}
	// global.port keeps "invalid value" (rank 0 < "required" rank 1).
	if got[0].Path != "global.port" || got[0].Msg != "invalid value" {
		t.Errorf("got[0] = %+v, want {global.port invalid value}", got[0])
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

func TestLoadHostnameWrongShapeIsInvalidValueOnce(t *testing.T) {
	src := "global:\n  hostname: [foobar]\nsections:\n  - title: S\n    services:\n      - title: T\n        command: echo\n"
	path := writeTestConfig(t, src)
	defer func() { _ = os.Remove(path) }()
	res := Load(path)
	got := res.IssueStrings()
	if len(got) != 1 || got[0] != "`global.hostname` invalid value" {
		t.Errorf("got %v, want [`global.hostname` invalid value]", got)
	}
}

func shapeIssuesFromYAML(t *testing.T, src string) []string {
	t.Helper()
	var raw map[string]any
	if err := yaml.Load([]byte(src), &raw); err != nil {
		t.Fatalf("load: %v", err)
	}
	var out []string
	for _, is := range dedupeSort(shapeIssues(raw)) {
		out = append(out, is.String())
	}
	return out
}

func TestShapeIssues(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want []string
	}{
		{"sections wrong shape (scalar)", "sections: 123\n", []string{"`sections` invalid value"}},
		{"subtitle map", "global:\n  subtitle: {a: b}\n", []string{"`global.subtitle` invalid value"}},
		{"hostname list", "global:\n  hostname: [foobar]\n", []string{"`global.hostname` invalid value"}},
		{"port scalar string is shape-OK", "global:\n  port: notanint\n", nil},
		{"groups map", "users:\n  bob:\n    groups: {a: b}\n", []string{"`users.bob.groups` invalid value"}},
		{"inline flow wrong title", "sections: [{title: [a], services: [{title: T, command: echo}]}]\n", []string{"`sections[0].title` invalid value"}},
		{"x-* anchor merged, shape ok", "x-foo: &foo\n  timeout: fff\nglobal:\n  <<: *foo\n", nil},
		{"allowed map", "global:\n  allowed: {alice: y}\n", []string{"`global.allowed` invalid value"}},
		{"allowed empty list ok", "global:\n  allowed: []\n", nil},
		{"remote list ok", "global:\n  remote: [h1]\n", nil},
		{"global wrong shape", "global: 123\n", []string{"`global` invalid value"}},
		{"null scalar ok", "global:\n  port:\n", nil},
		{"valid config", "global:\n  port: 3000\nusers:\n  a:\n    password: x\nsections:\n  - title: S\n    services:\n      - title: T\n        command: echo\n", nil},
		// Unknown-key detection via shape walk.
		{"top-level unknown key", "sektions: 1\n", []string{"`sektions` invalid field"}},
		{"global unknown key", "global:\n  titl: x\n", []string{"`global.titl` invalid field"}},
		{"x-* key not flagged", "x-top: 1\nglobal:\n  x-note: ok\n", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shapeIssuesFromYAML(t, tt.src)
			if len(got) != len(tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Errorf("got[%d]=%q want %q (all: %v)", i, got[i], tt.want[i], got)
				}
			}
		})
	}
}
