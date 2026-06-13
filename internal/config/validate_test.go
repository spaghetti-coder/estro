package config

import (
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
					Title: "S", CascadeFields: CascadeFields{Allowed: tt.allowed},
					Services: []ServiceConfig{{Title: "t", Command: CommandValue{"echo"}}},
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

func TestWrongTypeMergesToOneIssue(t *testing.T) {
	svc := "\nsections:\n  - title: S\n    services:\n      - title: T\n        command: echo\n"
	tests := []struct{ name, src, path string }{
		{"required slice given a map", "sections: {a: b}\n", "sections"},
		{"global scalar given a map", "global:\n  port: {a: b}" + svc, "global.port"},
		{"global stringlist given a map", "global:\n  allowed: {a: b}" + svc, "global.allowed"},
		{"section cascade field given a map", "sections:\n  - title: S\n    timeout: {a: b}\n    services:\n      - title: T\n        command: echo\n", "sections[0].timeout"},
		{"service field given a map", "sections:\n  - title: S\n    services:\n      - title: T\n        command: echo\n        timeout: {a: b}\n", "sections[0].services[0].timeout"},
		{"user map entry given a map", "users:\n  bob:\n    password: x\n    groups: {a: b}" + svc, "users.bob.groups"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			issues := loadIssues(t, tt.src)
			n := 0
			for _, is := range issues {
				if is.Path == tt.path {
					n++
				}
			}
			if n != 1 {
				t.Errorf("expected exactly one issue at %q, got %d", tt.path, n)
			}
		})
	}
}

func TestUnknownKeyBehavior(t *testing.T) {
	tests := []struct {
		name      string
		src       string
		wantPaths []string // paths that must carry "invalid field"
		notPaths  []string // paths that must NOT appear
	}{
		{"top-level typo", "sektions: 1\nsections:\n  - title: S\n    services:\n      - title: T\n        command: echo\n", []string{"sektions"}, nil},
		{"global typo", "global:\n  titl: x\nsections:\n  - title: S\n    services:\n      - title: T\n        command: echo\n", []string{"global.titl"}, nil},
		{"service typo", "sections:\n  - title: S\n    services:\n      - title: T\n        command: echo\n        comand: typo\n", []string{"sections[0].services[0].comand"}, nil},
		{"user typo", "users:\n  bob:\n    password: x\n    pasword: y\nsections:\n  - title: S\n    services:\n      - title: T\n        command: echo\n", []string{"users.bob.pasword"}, nil},
		{"bad key merged via anchor", "x-anchor: &a\n  hstnm: bad\nglobal:\n  <<: *a\nsections:\n  - title: S\n    services:\n      - title: T\n        command: echo\n", []string{"global.hstnm"}, nil},
		{"x-* keys NOT reported", "x-top: anything\nglobal:\n  x-note: ok\nsections:\n  - title: S\n    x-section: ok\n    services:\n      - title: T\n        command: echo\n        x-svc: ok\n", nil, []string{"x-top", "global.x-note", "sections[0].x-section", "sections[0].services[0].x-svc"}},
		{"section typo", "sections:\n  - title: S\n    sektion_typo: 1\n    services:\n      - title: T\n        command: echo\n", []string{"sections[0].sektion_typo"}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			issues := loadIssues(t, tt.src)
			for _, wantPath := range tt.wantPaths {
				found := false
				for _, is := range issues {
					if is.Path == wantPath && is.Msg == "invalid field" {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected issue {Path:%q, Msg:\"invalid field\"}, got: %v", wantPath, issues)
				}
			}
			for _, notPath := range tt.notPaths {
				for _, is := range issues {
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
		{Path: "a", Msg: "y"},
	}
	got := dedupeSort(in)
	if len(got) != 2 || got[0].Path != "a" || got[1].Path != "b" {
		t.Fatalf("got %v", got)
	}
}

func ptrOf[T any](v T) *T { return &v }

func TestValueConstraints(t *testing.T) {
	minimalValid := func() Config {
		return Config{
			Sections: []SectionConfig{{
				Title: "S", Services: []ServiceConfig{{Title: "T", Command: CommandValue{"echo"}}},
			}},
		}
	}

	rejected := []struct {
		name string
		cfg  Config
	}{
		{"port zero", func() Config { c := minimalValid(); c.Global = &GlobalConfig{Port: ptrOf(0)}; return c }()},
		{"port too large", func() Config { c := minimalValid(); c.Global = &GlobalConfig{Port: ptrOf(70000)}; return c }()},
		{"columns zero", func() Config {
			c := minimalValid()
			c.Global = &GlobalConfig{LayoutFields: LayoutFields{Columns: ptrOf(0)}}
			return c
		}()},
		{"columns too large", func() Config {
			c := minimalValid()
			c.Global = &GlobalConfig{LayoutFields: LayoutFields{Columns: ptrOf(13)}}
			return c
		}()},
		{"invalid hostname", func() Config { c := minimalValid(); c.Global = &GlobalConfig{Hostname: ptrOf("not a host!")}; return c }()},
		{"command empty slice", func() Config { c := minimalValid(); c.Sections[0].Services[0].Command = CommandValue{}; return c }()},
		{"command empty element", func() Config { c := minimalValid(); c.Sections[0].Services[0].Command = CommandValue{""}; return c }()},
		{"no sections", Config{}},
		{"section with no services", Config{Sections: []SectionConfig{{Title: "S", Services: nil}}}},
		{"user with empty password", func() Config {
			c := minimalValid()
			c.Users = map[string]*UserConfig{"alice": {Password: ""}}
			return c
		}()},
		{"session_ttl negative", func() Config {
			c := minimalValid()
			c.Global = &GlobalConfig{SessionTTL: ptrOf(-1)}
			return c
		}()},
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
			Global:   &GlobalConfig{Hostname: ptrOf("0.0.0.0"), Port: ptrOf(3000), LayoutFields: LayoutFields{Columns: ptrOf(3)}},
			Users:    map[string]*UserConfig{"alice": {Password: "secret"}},
			Sections: []SectionConfig{{Title: "S", Services: []ServiceConfig{{Title: "T", Command: CommandValue{"echo hello"}}}}},
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
	strs := loadIssueStrings(t, src)
	joined := strings.Join(strs, "\n")
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

func TestAllowedRefZeroMemberGroup(t *testing.T) {
	cfg := Config{
		Users: map[string]*UserConfig{"alice": {Password: "x", Groups: StringList{"admins"}}},
		Sections: []SectionConfig{{
			Title: "S", CascadeFields: CascadeFields{Allowed: StringList{"editors"}},
			Services: []ServiceConfig{{Title: "t", Command: CommandValue{"echo"}}},
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
		{"zero port", &Config{Global: &GlobalConfig{Port: ptrOf(0)}}, "127.0.0.1:3000"},
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
	res := Load(writeTestConfig(t, "global:\n  hostname: \"not a host\"\n"))
	if !res.hasIssue("global.hostname") {
		t.Fatalf("expected a global.hostname issue, got %v", res.IssueStrings())
	}
	if got := res.ServerAddr(); got != "127.0.0.1:3000" {
		t.Errorf("ServerAddr() = %q, want 127.0.0.1:3000", got)
	}
}

func TestLoadTypeMismatchPortFallsBackTo3000(t *testing.T) {
	res := Load(writeTestConfig(t, "global:\n  port: \"abc\"\n  columns: \"xyz\"\n"))
	if res.Healthy() {
		t.Fatal("expected degraded result for type mismatches")
	}
	if got := res.ServerAddr(); got != "127.0.0.1:3000" {
		t.Errorf("ServerAddr() = %q, want 127.0.0.1:3000", got)
	}
}

func TestLoadWrongShapeTopLevel(t *testing.T) {
	strs := loadIssueStrings(t, "sections: 123\n")
	if len(strs) != 1 || strs[0] != "`sections` invalid value" {
		t.Errorf("got %v, want [`sections` invalid value`]", strs)
	}
}

func TestLoadWrongTypeScalarIsInvalidValue(t *testing.T) {
	src := "global:\n  port: \"abc\"\nsections:\n  - title: S\n    services:\n      - title: T\n        command: echo\n"
	strs := loadIssueStrings(t, src)
	if len(strs) != 1 || strs[0] != "`global.port` invalid value" {
		t.Errorf("got %v, want [`global.port` invalid value`]", strs)
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
	if got[0].Path != "global.port" || got[0].Msg != "invalid value" {
		t.Errorf("got[0] = %+v, want {global.port invalid value}", got[0])
	}
}

func TestLoadRejectsEmptyStringListElement(t *testing.T) {
	src := "sections:\n  - title: S\n    remote: 'h1,,h2'\n    services:\n      - title: T\n        command: echo\n"
	strs := loadIssueStrings(t, src)
	joined := strings.Join(strs, "\n")
	if !strings.Contains(joined, "remote[1]") {
		t.Errorf("expected an issue mentioning remote[1], got:\n%s", joined)
	}
}

func TestLoadXStarAnchorMergeNoLeak(t *testing.T) {
	src := "x-foo: &foo\n  timeout: fff\nglobal:\n  <<: *foo\nsections:\n  - title: S\n    services:\n      - title: T\n        command: echo\n"
	strs := loadIssueStrings(t, src)
	for _, s := range strs {
		if strings.Contains(s, "x-foo") {
			t.Errorf("issue leaks x-* anchor internals: %q (all: %v)", s, strs)
		}
	}
	if len(strs) != 1 || strs[0] != "`global.timeout` invalid value" {
		t.Errorf("got %v, want [`global.timeout` invalid value`]", strs)
	}
}

func TestLoadHostnameWrongShapeIsInvalidValueOnce(t *testing.T) {
	src := "global:\n  hostname: [foobar]\nsections:\n  - title: S\n    services:\n      - title: T\n        command: echo\n"
	strs := loadIssueStrings(t, src)
	if len(strs) != 1 || strs[0] != "`global.hostname` invalid value" {
		t.Errorf("got %v, want [`global.hostname` invalid value`]", strs)
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
		{"port scalar string flagged", "global:\n  port: notanint\n", []string{"`global.port` invalid value"}},
		{"groups map", "users:\n  bob:\n    groups: {a: b}\n", []string{"`users.bob.groups` invalid value"}},
		{"inline flow wrong title", "sections: [{title: [a], services: [{title: T, command: echo}]}]\n", []string{"`sections[0].title` invalid value"}},
		{"x-* anchor merged, shape flags type mismatch", "x-foo: &foo\n  timeout: fff\nglobal:\n  <<: *foo\n", []string{"`global.timeout` invalid value"}},
		{"allowed map", "global:\n  allowed: {alice: y}\n", []string{"`global.allowed` invalid value"}},
		{"allowed empty list ok", "global:\n  allowed: []\n", nil},
		{"remote list ok", "global:\n  remote: [h1]\n", nil},
		{"global wrong shape", "global: 123\n", []string{"`global` invalid value"}},
		{"null scalar ok", "global:\n  port:\n", nil},
		{"valid config", "global:\n  port: 3000\nusers:\n  a:\n    password: x\nsections:\n  - title: S\n    services:\n      - title: T\n        command: echo\n", nil},
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
