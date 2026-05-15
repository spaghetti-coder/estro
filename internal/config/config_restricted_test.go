package config

import "testing"

func TestGetRestricted_DefaultTrue(t *testing.T) {
	flat := FlatService{Global: &GlobalConfig{}}
	if !flat.GetRestricted() {
		t.Error("expected default restricted to be true")
	}
}

func TestGetRestricted_ServiceOverridesSection(t *testing.T) {
	svcFalse := false
	secTrue := true
	flat := FlatService{
		Restricted:        &svcFalse,
		SectionRestricted: &secTrue,
		Global:            &GlobalConfig{},
	}
	if flat.GetRestricted() {
		t.Error("expected restricted=false when service explicitly sets false")
	}
}

func TestGetRestricted_SectionOverridesGlobal(t *testing.T) {
	secFalse := false
	glbTrue := true
	flat := FlatService{
		SectionRestricted: &secFalse,
		Global:            &GlobalConfig{Restricted: &glbTrue},
	}
	if flat.GetRestricted() {
		t.Error("expected section restricted=false to override global restricted=true")
	}
}

func TestGetRestricted_GlobalCascade(t *testing.T) {
	glbTrue := true
	flat := FlatService{
		Global: &GlobalConfig{Restricted: &glbTrue},
	}
	if !flat.GetRestricted() {
		t.Error("expected global restricted=true to cascade when no service/section override")
	}
}

func TestGetRestricted_ServiceOverridesGlobal(t *testing.T) {
	svcFalse := false
	glbTrue := true
	flat := FlatService{
		Restricted: &svcFalse,
		Global:     &GlobalConfig{Restricted: &glbTrue},
	}
	if flat.GetRestricted() {
		t.Error("expected service restricted=false to override global restricted=true")
	}
}

func TestGetRestricted_NilGlobal_DefaultTrue(t *testing.T) {
	flat := FlatService{}
	if !flat.GetRestricted() {
		t.Error("expected default restricted=true with nil global")
	}
}

func TestFlatten_ServiceRestricted(t *testing.T) {
	svcFalse := false
	secTrue := true
	cfg := &Config{
		Global: &GlobalConfig{},
		Sections: []SectionConfig{
			{
				Title:      "Sec",
				Restricted: &secTrue,
				Services: []ServiceConfig{
					{Title: "Override", Command: CommandValue{"echo"}, Restricted: &svcFalse},
					{Title: "Inherit", Command: CommandValue{"date"}},
				},
			},
		},
	}
	services := cfg.Flatten()
	if len(services) != 2 {
		t.Fatalf("expected 2 services, got %d", len(services))
	}
	if services[0].GetRestricted() {
		t.Error("Override: expected restricted=false (service overrides section)")
	}
	if !services[1].GetRestricted() {
		t.Error("Inherit: expected restricted=true (inherits from section)")
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

func TestGetRestricted_RestrictedTrue_AllowedEmptySlice(t *testing.T) {
	glbTrue := true
	flat := FlatService{
		Global:  &GlobalConfig{Restricted: &glbTrue},
		Allowed: []string{},
	}
	users := map[string]*UserConfig{
		"alice": {Password: "hash"},
	}
	if flat.GetRestricted() {
		if !flat.IsAccessible("guest", users) {
			t.Error("restricted=true + allowed=[] should be public")
		}
	}
}

func TestGetRestricted_RestrictedTrue_NilAllowed_AllLevels(t *testing.T) {
	flat := FlatService{
		Global: &GlobalConfig{},
	}
	users := map[string]*UserConfig{
		"alice": {Password: "hash"},
	}
	if !flat.IsAccessible("", users) {
		t.Error("restricted=true (default) + nil allowed at all levels should be public (allowed: nil means cascade to public)")
	}
}

func TestFlatten_SectionRestrictedTrue_ServiceRestrictedFalse(t *testing.T) {
	secTrue := true
	svcFalse := false
	cfg := &Config{
		Global: &GlobalConfig{},
		Sections: []SectionConfig{
			{
				Title:      "Sec",
				Restricted: &secTrue,
				Services: []ServiceConfig{
					{Title: "Override", Command: CommandValue{"echo"}, Restricted: &svcFalse},
				},
			},
		},
	}
	services := cfg.Flatten()
	if len(services) != 1 {
		t.Fatalf("expected 1 service, got %d", len(services))
	}
	if services[0].GetRestricted() {
		t.Error("expected restricted=false (service overrides section)")
	}
}

func TestFlatten_GlobalRestrictedFalse_Inherited(t *testing.T) {
	glbFalse := false
	cfg := &Config{
		Global: &GlobalConfig{Restricted: &glbFalse},
		Sections: []SectionConfig{
			{
				Title: "Sec",
				Services: []ServiceConfig{
					{Title: "Inherit", Command: CommandValue{"date"}},
				},
			},
		},
	}
	services := cfg.Flatten()
	if len(services) != 1 {
		t.Fatalf("expected 1 service, got %d", len(services))
	}
	if services[0].GetRestricted() {
		t.Error("expected restricted=false (inherited from global)")
	}
}