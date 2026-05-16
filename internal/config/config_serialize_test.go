package config

import (
	"os"
	"testing"
)

func TestSerializeService(t *testing.T) {
	path := writeTestConfig(t, testConfigYAML())
	defer func() { _ = os.Remove(path) }()

	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	services := cfg.Flatten()

	for i, svc := range services {
		serialized := svc.Serialize(i, "alice", cfg.Users)
		if serialized.ID != i {
			t.Errorf("expected id %d, got %d", i, serialized.ID)
		}
		if serialized.Title != svc.Title {
			t.Errorf("expected title %s, got %s", svc.Title, serialized.Title)
		}
		if serialized.Timeout != svc.GetTimeoutMs()+10000 {
			t.Errorf("expected timeout %d, got %d", svc.GetTimeoutMs()+10000, serialized.Timeout)
		}
		if serialized.Confirm != svc.GetConfirm() {
			t.Errorf("expected confirm %v, got %v", svc.GetConfirm(), serialized.Confirm)
		}
		if serialized.Section == nil || *serialized.Section != svc.SectionTitle {
			t.Errorf("expected section %s, got %v", svc.SectionTitle, serialized.Section)
		}
	}
}

func TestSerialize_EnabledField(t *testing.T) {
	glbFalse := false
	svc := FlatService{
		Title:   "test",
		Command: CommandValue{"echo"},
		Global:  &GlobalConfig{Enabled: &glbFalse},
	}
	serialized := svc.Serialize(0, "", nil)
	if serialized.Enabled {
		t.Error("expected serialized enabled=false when global is false and no override")
	}
}

func TestSerialize_RestrictedField(t *testing.T) {
	glbTrue := true
	svc := FlatService{
		Title:   "test",
		Command: CommandValue{"echo"},
		Global:  &GlobalConfig{Restricted: &glbTrue},
	}
	serialized := svc.Serialize(0, "", nil)
	if !serialized.Restricted {
		t.Error("expected serialized restricted=true when global is true and no override")
	}
}

func TestSerialize_RestrictedFalse(t *testing.T) {
	svcFalse := false
	svc := FlatService{
		Title:      "test",
		Command:    CommandValue{"echo"},
		Restricted: &svcFalse,
		Global:     &GlobalConfig{},
	}
	serialized := svc.Serialize(0, "", nil)
	if serialized.Restricted {
		t.Error("expected serialized restricted=false when service sets false")
	}
}