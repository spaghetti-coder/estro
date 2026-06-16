package config

import (
	"math"
	"os"
	"strings"
	"testing"
)

// writeTestConfig writes content to a temp YAML file and schedules cleanup.
func writeTestConfig(t *testing.T, content string) string {
	t.Helper()
	tmp, err := os.CreateTemp("", "config-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tmp.WriteString(content); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
		t.Fatal(err)
	}
	_ = tmp.Close()
	t.Cleanup(func() { _ = os.Remove(tmp.Name()) })
	return tmp.Name()
}

// loadIssues is a test helper: write YAML content, load it, return issues.
func loadIssues(t *testing.T, src string) []Issue {
	t.Helper()
	path := writeTestConfig(t, src)
	return Load(path).Issues
}

// loadIssueStrings writes YAML, loads, returns rendered issue strings.
func loadIssueStrings(t *testing.T, src string) []string {
	t.Helper()
	path := writeTestConfig(t, src)
	return Load(path).IssueStrings()
}

func TestLoadInvalidConfig(t *testing.T) {
	issues := loadIssues(t, "sections:\n  - title: ''\n    services:\n      - title: test\n        command: echo\n")
	if len(issues) == 0 {
		t.Fatal("expected issues for empty required field")
	}
}

func TestLoadSessionTTL(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want int
	}{
		{"omitted no limit", "global:\n  title: E\nsections:\n  - title: S\n    services:\n      - title: T\n        command: echo\n", math.MaxInt32},
		{"zero no limit", "global:\n  session_ttl: 0\nsections:\n  - title: S\n    services:\n      - title: T\n        command: echo\n", math.MaxInt32},
		{"720 hours", "global:\n  session_ttl: 720\nsections:\n  - title: S\n    services:\n      - title: T\n        command: echo\n", 720 * 3600},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := Load(writeTestConfig(t, tt.src))
			if !res.Healthy() {
				t.Fatalf("unexpected issues: %v", res.IssueStrings())
			}
			if got := res.Config.SessionTTLSeconds(); got != tt.want {
				t.Errorf("SessionTTLSeconds() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestLoadInvalidSessionTTL(t *testing.T) {
	strs := loadIssueStrings(t, "global:\n  session_ttl: -5\nsections:\n  - title: S\n    services:\n      - title: T\n        command: echo\n")
	joined := strings.Join(strs, "\n")
	if !strings.Contains(joined, "session_ttl") {
		t.Errorf("expected session_ttl issue, got: %v", strs)
	}
}

func TestLoadMissingFile(t *testing.T) {
	strs := Load("/nonexistent/path/config.yaml").IssueStrings()
	if len(strs) == 0 || strs[0] != "Configuration file can't be read" {
		t.Errorf("got %v", strs)
	}
}

func TestLoadBadYAML(t *testing.T) {
	issues := loadIssues(t, "sections: [{invalid yaml\n")
	if len(issues) == 0 {
		t.Fatal("expected degraded for bad YAML")
	}
}

func TestSessionTTLSeconds(t *testing.T) {
	tests := []struct {
		name string
		cfg  *Config
		want int
	}{
		{"nil global → no limit", &Config{}, math.MaxInt32},
		{"nil SessionTTL → no limit", &Config{Global: &GlobalConfig{}}, math.MaxInt32},
		{"zero → no limit", &Config{Global: &GlobalConfig{SessionTTL: ptrOf(0)}}, math.MaxInt32},
		{"1 hour", &Config{Global: &GlobalConfig{SessionTTL: ptrOf(1)}}, 3600},
		{"720 hours (30 days)", &Config{Global: &GlobalConfig{SessionTTL: ptrOf(720)}}, 720 * 3600},
		{"8760 hours (1 year)", &Config{Global: &GlobalConfig{SessionTTL: ptrOf(8760)}}, 8760 * 3600},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.cfg.SessionTTLSeconds(); got != tt.want {
				t.Errorf("SessionTTLSeconds() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestLoadPopulatesValidFieldsDespiteTypeError(t *testing.T) {
	src := "global:\n  title: MyApp\n  port: {a: b}\nsections:\n  - title: S\n    services:\n      - title: T\n        command: echo\n"
	res := Load(writeTestConfig(t, src))
	g := res.Config.Global
	if g == nil || g.Title == nil || *g.Title != "MyApp" {
		t.Fatalf("expected global.title=MyApp, got %+v", g)
	}
}

func TestExpandEstroEnv(t *testing.T) {
	t.Setenv("ESTRO_TEST_HOST", "0.0.0.0")
	t.Setenv("ESTRO_TEST_SECRET", "s3cr3t")
	src := "global:\n" +
		"  hostname: \"{estro_env.ESTRO_TEST_HOST}\"\n" +
		"  session_secret: \"{estro_env.ESTRO_TEST_SECRET}\"\n" +
		"sections:\n  - title: S\n    services:\n      - title: T\n        command: echo {estro_env.ESTRO_TEST_HOST} ${RUNTIME}\n"
	res := Load(writeTestConfig(t, src))
	if !res.Healthy() {
		t.Fatalf("expected healthy, got %v", res.IssueStrings())
	}
	g := res.Config.Global
	if g.Hostname == nil || *g.Hostname != "0.0.0.0" {
		t.Errorf("hostname = %v, want 0.0.0.0", g.Hostname)
	}
	if g.SessionSecret == nil || *g.SessionSecret != "s3cr3t" {
		t.Errorf("session_secret = %v, want s3cr3t", g.SessionSecret)
	}
	cmd := res.Config.Sections[0].Services[0].Command
	if len(cmd) != 1 || cmd[0] != "echo 0.0.0.0 ${RUNTIME}" {
		t.Errorf("command = %v, want [echo 0.0.0.0 ${RUNTIME}]", cmd)
	}
}

func TestExpandEstroEnvUnsetIsIssue(t *testing.T) {
	_ = os.Unsetenv("ESTRO_UNSET_VAR_XYZ")
	src := "global:\n  secret: \"{estro_env.ESTRO_UNSET_VAR_XYZ}\"\nsections:\n  - title: S\n    services:\n      - title: T\n        command: echo hi\n"
	strs := loadIssueStrings(t, src)
	if !strings.Contains(strings.Join(strs, "\n"), "ESTRO_UNSET_VAR_XYZ is not set") {
		t.Errorf("expected unset-env issue, got %v", strs)
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
	res := Load(writeTestConfig(t, src))
	if !res.Healthy() {
		t.Fatalf("expected healthy, got: %v", res.IssueStrings())
	}
}

func TestLoadEmptyFile(t *testing.T) {
	strs := loadIssueStrings(t, "   \n")
	if len(strs) != 1 || strs[0] != "`sections` required" {
		t.Errorf("expected [`sections` required], got %v", strs)
	}
}

func TestGetConfigDefaultTitle(t *testing.T) {
	cfg := &Config{
		Global: &GlobalConfig{Title: ptrOf("Estro"), Hostname: ptrOf("0.0.0.0"), Port: ptrOf(3000)},
		Users:  map[string]*UserConfig{"testuser": {Password: "$2y$10$hash"}},
		Sections: []SectionConfig{{
			Title: "Test", Services: []ServiceConfig{{Title: "Svc", Command: CommandValue{"echo hi"}}},
		}},
	}
	issues := Validate(cfg)
	if len(issues) > 0 {
		t.Fatalf("invalid test config: %v", issues)
	}
	resp := cfg.GetConfigResponse()
	if resp.Title != "Estro" {
		t.Errorf("expected default title 'Estro', got %s", resp.Title)
	}
}
