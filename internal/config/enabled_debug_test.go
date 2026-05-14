package config

import (
	"testing"
)

func TestDebugEnabledCascade(t *testing.T) {
	glbFalse := false
	svcTrue := true
	secFalse := false
	
	cfg := &Config{
		Global: &GlobalConfig{Enabled: &glbFalse},
		Sections: []SectionConfig{
			{
				Title:   "Disabled Section",
				Enabled: &secFalse,
				Services: []ServiceConfig{
					{Title: "Disabled Service", Command: CommandValue{"echo disabled"}},
					{Title: "Override Enabled", Command: CommandValue{"echo override"}, Enabled: &svcTrue},
				},
			},
			{
				Title:   "Enabled Section",
				Enabled: &svcTrue,
				Services: []ServiceConfig{
					{Title: "Normal Service", Command: CommandValue{"echo normal"}},
				},
			},
			{
				Title: "Default Section",
				Services: []ServiceConfig{
					{Title: "Default Service", Command: CommandValue{"echo default"}},
				},
			},
		},
	}
	
	services := cfg.Flatten()
	for i, svc := range services {
		enabled := svc.GetEnabled()
		t.Logf("Service %d: %q  Enabled=%v  SectionEnabled=%v  Global.Enabled=%v  GetEnabled=%v",
			i, svc.Title, svc.Enabled, svc.SectionEnabled, svc.Global.Enabled, enabled)
	}
	
	// Disabled Service: no service override, section=false -> should be false
	if services[0].GetEnabled() {
		t.Errorf("Service 0 (Disabled Service): expected enabled=false, got true")
	}
	// Override Enabled: service=true, section=false -> service wins, should be true
	if !services[1].GetEnabled() {
		t.Errorf("Service 1 (Override Enabled): expected enabled=true, got false")
	}
	// Normal Service: section=true -> should be true
	if !services[2].GetEnabled() {
		t.Errorf("Service 2 (Normal Service): expected enabled=true, got false")
	}
	// Default Service: no section override, global=false -> inherits global=false
	if services[3].GetEnabled() {
		t.Errorf("Service 3 (Default Service): expected enabled=false (cascaded from global), got true")
	}
}
