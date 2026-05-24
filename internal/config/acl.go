package config

import (
	"maps"
	"slices"
)

// ResolveAllowed expands the service's allowed list into concrete usernames,
// resolving group names into their member users. Returns nil for public access.
func (s *FlatService) ResolveAllowed(users map[string]*UserConfig) []string {
	allowed := s.Allowed
	if len(allowed) == 0 {
		return nil
	}
	result := make(map[string]struct{})
	for _, name := range allowed {
		if _, ok := users[name]; ok {
			result[name] = struct{}{}
		} else {
			for uname, u := range users {
				for _, g := range u.Groups {
					if g == name {
						result[uname] = struct{}{}
					}
				}
			}
		}
	}
	return slices.Sorted(maps.Keys(result))
}

// IsAccessible reports whether the given username can access the service.
// A nil allowed list means public access; otherwise the user must be in the resolved list.
func (s *FlatService) IsAccessible(username string, users map[string]*UserConfig) bool {
	allowed := s.ResolveAllowed(users)
	return allowed == nil || (username != "" && slices.Contains(allowed, username))
}
