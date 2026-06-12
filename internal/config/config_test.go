package config

import (
	"os"
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
