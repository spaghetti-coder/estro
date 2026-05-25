package config

import (
	"maps"
	"slices"
)

// resolveAllowed expands allowed names into concrete usernames,
// resolving group names into their member users. Returns nil for
// nil/empty input and for groups that resolve to zero users (public access per CONF-05).
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

// IsAccessible reports whether the given username can access the service.
// A nil Allowed field means public access; otherwise the user must be in the pre-resolved list.
func (s *FlatService) IsAccessible(username string) bool {
	if s.Allowed == nil {
		return true
	}
	return username != "" && slices.Contains(s.Allowed, username)
}
