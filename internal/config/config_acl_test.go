package config

import (
	"slices"
	"testing"
)

func TestResolveAllowedNil(t *testing.T) {
	users := map[string]*UserConfig{
		"alice": {Password: "hash", Groups: []string{"admins"}},
	}
	svc := FlatService{ServiceCascade: CascadeFields{Allowed: nil}}
	result := svc.ResolveAllowed(users)
	if result != nil {
		t.Errorf("expected nil for Allowed=nil, got %v", result)
	}
}

func TestResolveAllowedEmptySlice(t *testing.T) {
	users := map[string]*UserConfig{
		"alice": {Password: "hash", Groups: []string{"admins"}},
	}
	svc := FlatService{ServiceCascade: CascadeFields{Allowed: []string{}}}
	result := svc.ResolveAllowed(users)
	if result != nil {
		t.Errorf("expected nil for empty Allowed, got %v", result)
	}
}

func TestResolveAllowedGroup(t *testing.T) {
	users := map[string]*UserConfig{
		"alice": {Password: "hash", Groups: []string{"admins"}},
		"bob":   {Password: "hash", Groups: []string{"admins", "family"}},
	}
	svc := FlatService{ServiceCascade: CascadeFields{Allowed: []string{"admins"}}}
	result := svc.ResolveAllowed(users)
	if len(result) != 2 {
		t.Errorf("expected 2 users for 'admins' group, got %d: %v", len(result), result)
	}
	if !slices.Contains(result, "alice") || !slices.Contains(result, "bob") {
		t.Errorf("expected alice and bob, got %v", result)
	}
}

func TestResolveAllowedSingleUser(t *testing.T) {
	users := map[string]*UserConfig{
		"guest": {Password: "hash"},
	}
	svc := FlatService{ServiceCascade: CascadeFields{Allowed: []string{"guest"}}}
	result := svc.ResolveAllowed(users)
	if len(result) != 1 || result[0] != "guest" {
		t.Errorf("expected [guest], got %v", result)
	}
}

func TestIsAccessible(t *testing.T) {
	users := map[string]*UserConfig{
		"alice": {Password: "hash", Groups: []string{"admins"}},
		"bob":   {Password: "hash", Groups: []string{"admins"}},
		"guest": {Password: "hash"},
	}

	publicSvc := FlatService{ServiceCascade: CascadeFields{Allowed: nil}}
	if !publicSvc.IsAccessible("", users) {
		t.Error("public service should be accessible with no user")
	}
	if !publicSvc.IsAccessible("alice", users) {
		t.Error("public service should be accessible with any user")
	}

	restrictedSvc := FlatService{ServiceCascade: CascadeFields{Allowed: []string{"admins"}}}
	if restrictedSvc.IsAccessible("", users) {
		t.Error("restricted service should not be accessible with empty user")
	}
	if !restrictedSvc.IsAccessible("alice", users) {
		t.Error("restricted service should be accessible to admin user")
	}
	if restrictedSvc.IsAccessible("guest", users) {
		t.Error("restricted service should not be accessible to guest")
	}

	emptyAllowed := FlatService{ServiceCascade: CascadeFields{Allowed: []string{}}}
	if !emptyAllowed.IsAccessible("", users) {
		t.Error("empty allowed should be accessible (public)")
	}
}

func TestRestrictedTrue_EmptyAllowedIsPublic(t *testing.T) {
	glbTrue := true
	flat := FlatService{
		Global:         &GlobalConfig{CascadeFields: CascadeFields{Restricted: &glbTrue}},
		ServiceCascade: CascadeFields{Allowed: []string{}},
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

func TestRestrictedTrue_NilAllowedIsPublic(t *testing.T) {
	flat := FlatService{
		Global: &GlobalConfig{},
	}
	users := map[string]*UserConfig{
		"alice": {Password: "hash"},
	}
	if !flat.IsAccessible("", users) {
		t.Error("restricted=true (default) + nil allowed at all levels should be public")
	}
}
