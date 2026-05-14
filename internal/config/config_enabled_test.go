package config

import "testing"

func TestGetEnabled_DefaultTrue(t *testing.T) {
	flat := FlatService{Global: &GlobalConfig{}}
	if !flat.GetEnabled() {
		t.Error("expected default enabled to be true")
	}
}

func TestGetEnabled_ServiceOverridesSection(t *testing.T) {
	svcFalse := false
	secFalse := false
	flat := FlatService{
		Enabled:        &svcFalse,
		SectionEnabled: &secFalse,
		Global:         &GlobalConfig{},
	}
	if flat.GetEnabled() {
		t.Error("expected enabled=false when service explicitly sets false")
	}
}

func TestGetEnabled_SectionOverridesGlobal(t *testing.T) {
	secTrue := true
	glbFalse := false
	flat := FlatService{
		SectionEnabled: &secTrue,
		Global:         &GlobalConfig{Enabled: &glbFalse},
	}
	if !flat.GetEnabled() {
		t.Error("expected section enabled=true to override global enabled=false")
	}
}

func TestGetEnabled_GlobalCascade(t *testing.T) {
	glbFalse := false
	flat := FlatService{
		Global: &GlobalConfig{Enabled: &glbFalse},
	}
	if flat.GetEnabled() {
		t.Error("expected global enabled=false to cascade when no service/section override")
	}
}

func TestGetEnabled_ServiceOverridesGlobal(t *testing.T) {
	svcTrue := true
	glbFalse := false
	flat := FlatService{
		Enabled: &svcTrue,
		Global:  &GlobalConfig{Enabled: &glbFalse},
	}
	if !flat.GetEnabled() {
		t.Error("expected service enabled=true to override global enabled=false")
	}
}

func TestGetEnabled_ServiceTrueInSectionFalse(t *testing.T) {
	svcTrue := true
	secFalse := false
	flat := FlatService{
		Enabled:        &svcTrue,
		SectionEnabled: &secFalse,
		Global:         &GlobalConfig{},
	}
	if !flat.GetEnabled() {
		t.Error("expected service enabled=true to override section enabled=false")
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