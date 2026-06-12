package config

import (
	"maps"
	"slices"
)

// resolveAllowed expands names to usernames (resolves groups); zero matches → nil (public).
func resolveAllowed(names []string, users map[string]*UserConfig) []string {
	result := make(map[string]struct{})
	for _, name := range names {
		// Add as direct username if it exists
		if _, ok := users[name]; ok {
			result[name] = struct{}{}
		}
		// Also expand as group name — a name can be both a user and a group
		for uname, u := range users {
			for _, g := range u.Groups {
				if g == name {
					result[uname] = struct{}{}
				}
			}
		}
	}
	if len(result) == 0 {
		return nil
	}
	return slices.Sorted(maps.Keys(result))
}

// IsAccessible reports whether username can access; nil Allowed = public.
func (s *FlatService) IsAccessible(username string) bool {
	if s.Allowed == nil {
		return true
	}
	return username != "" && slices.Contains(s.Allowed, username)
}

// IsHidden reports restricted && !accessible.
func (s *FlatService) IsHidden(username string) bool {
	return s.Restricted && !s.IsAccessible(username)
}
