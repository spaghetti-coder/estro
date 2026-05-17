package config

import "slices"

// FlatService holds a service with its section and global fallbacks resolved
// into a single flat struct for cascade lookups.
type FlatService struct {
	Title          string
	Command        CommandValue
	ServiceCascade CascadeFields
	SectionCascade CascadeFields
	SectionLayout  LayoutFields
	SectionTitle   string
	Global         *GlobalConfig
}

// SerializedService is the JSON-ready representation of a service sent to the frontend.
type SerializedService struct {
	ID                 int      `json:"id"`
	Title              string   `json:"title"`
	Timeout            int      `json:"timeout"`
	Confirm            bool     `json:"confirm"`
	Section            *string  `json:"section"`
	SectionCollapsable bool     `json:"sectionCollapsable"`
	SectionColumns     int      `json:"sectionColumns"`
	Public             bool     `json:"public"`
	Accessible         bool     `json:"accessible"`
	Enabled            bool     `json:"enabled"`
	AllowedUsers       []string `json:"allowedUsers"`
	Restricted         bool     `json:"restricted"`
}

// ConfigResponse provides the application title, subtitle, and user list for the frontend.
type ConfigResponse struct {
	Title    string   `json:"title"`
	Subtitle string   `json:"subtitle"`
	Users    []string `json:"users"`
}

const clientTimeoutBuffer = 10000

// Flatten expands all sections and services into a flat slice with cascade
// fallbacks wired up for each service.
func (c *Config) Flatten() []FlatService {
	total := 0
	for _, section := range c.Sections {
		total += len(section.Services)
	}
	services := make([]FlatService, 0, total)
	globalRef := c.GetGlobal()
	for _, section := range c.Sections {
		for _, svc := range section.Services {
			flat := FlatService{
				Title:          svc.Title,
				Command:        svc.Command,
				ServiceCascade: svc.CascadeFields,
				SectionCascade: section.CascadeFields,
				SectionLayout:  section.LayoutFields,
				SectionTitle:   section.Title,
				Global:         globalRef,
			}
			services = append(services, flat)
		}
	}
	return services
}

// GetTimeout returns the effective timeout in seconds, cascading through service,
// section, and global levels before falling back to DefaultTimeout.
func (s *FlatService) GetTimeout() int {
	gc := (*int)(nil)
	if s.Global != nil {
		gc = s.Global.Timeout
	}
	return cascadeInt(s.ServiceCascade.Timeout, s.SectionCascade.Timeout, gc, DefaultTimeout)
}

// GetConfirm returns whether user confirmation is required, cascading through
// service, section, and global levels before falling back to DefaultConfirm.
func (s *FlatService) GetConfirm() bool {
	gc := (*bool)(nil)
	if s.Global != nil {
		gc = s.Global.Confirm
	}
	return cascadeBool(s.ServiceCascade.Confirm, s.SectionCascade.Confirm, gc, DefaultConfirm)
}

// GetAllowed returns the resolved ACL list for the service, cascading through
// service, section, and global levels. Returns nil if unset (public access).
func (s *FlatService) GetAllowed() []string {
	if s.ServiceCascade.Allowed != nil {
		return s.ServiceCascade.Allowed
	}
	if s.SectionCascade.Allowed != nil {
		return s.SectionCascade.Allowed
	}
	if s.Global != nil && s.Global.Allowed != nil {
		return s.Global.Allowed
	}
	return nil
}

// GetCollapsable returns whether the section should start collapsed in the UI,
// cascading through section and global levels before falling back to DefaultCollapsable.
func (s *FlatService) GetCollapsable() bool {
	gc := (*bool)(nil)
	if s.Global != nil {
		gc = s.Global.Collapsable
	}
	return cascadeBool(nil, s.SectionLayout.Collapsable, gc, DefaultCollapsable)
}

// GetEnabled returns whether the service is enabled, cascading through service,
// section, and global levels before defaulting to true.
func (s *FlatService) GetEnabled() bool {
	gc := (*bool)(nil)
	if s.Global != nil {
		gc = s.Global.Enabled
	}
	return cascadeBool(s.ServiceCascade.Enabled, s.SectionCascade.Enabled, gc, true)
}

// GetRestricted returns whether the service is restricted from public listing,
// cascading through service, section, and global levels before defaulting to true.
func (s *FlatService) GetRestricted() bool {
	gc := (*bool)(nil)
	if s.Global != nil {
		gc = s.Global.Restricted
	}
	return cascadeBool(s.ServiceCascade.Restricted, s.SectionCascade.Restricted, gc, true)
}

// GetColumns returns the number of columns for the section's service grid,
// cascading through section and global levels before falling back to DefaultColumns.
func (s *FlatService) GetColumns() int {
	gc := (*int)(nil)
	if s.Global != nil {
		gc = s.Global.Columns
	}
	return cascadeInt(nil, s.SectionLayout.Columns, gc, DefaultColumns)
}

// GetRemote returns the SSH remote chain for the service, cascading through
// service, section, and global levels. Returns nil for local execution.
func (s *FlatService) GetRemote() StringList {
	if s.ServiceCascade.Remote != nil {
		return s.ServiceCascade.Remote
	}
	if s.SectionCascade.Remote != nil {
		return s.SectionCascade.Remote
	}
	if s.Global != nil && s.Global.Remote != nil {
		return s.Global.Remote
	}
	return nil
}

// GetTimeoutMs returns the effective timeout in milliseconds, including a buffer
// for client-side overhead.
func (s *FlatService) GetTimeoutMs() int {
	return s.GetTimeout() * 1000
}

// Serialize produces the JSON-ready representation of a service for the frontend,
// resolving access control against the given username and user map.
func (s *FlatService) Serialize(index int, username string, users map[string]*UserConfig) SerializedService {
	allowedUsers := s.ResolveAllowed(users)
	isPublic := allowedUsers == nil
	sectionTitle := s.SectionTitle
	return SerializedService{
		ID:                 index,
		Title:              s.Title,
		Timeout:            s.GetTimeoutMs() + clientTimeoutBuffer,
		Confirm:            s.GetConfirm(),
		Section:            &sectionTitle,
		SectionCollapsable: s.GetCollapsable(),
		SectionColumns:     s.GetColumns(),
		Public:             isPublic,
		Accessible:         isPublic || (username != "" && slices.Contains(allowedUsers, username)),
		Enabled:            s.GetEnabled(),
		AllowedUsers:       allowedUsers,
		Restricted:         s.GetRestricted(),
	}
}
