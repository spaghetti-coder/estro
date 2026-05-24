package config

import "slices"

// FlatService holds a service with its section and global fallbacks resolved
// into a single flat struct. All cascading fields are pre-resolved in Flatten().
type FlatService struct {
	Title   string
	Command CommandValue

	// resolved cascading fields
	Timeout       int
	Confirm       bool
	Enabled       bool
	Restricted    bool
	Allowed       StringList // nil = public
	Remote        StringList // nil = inherit/cascade, [] = explicit local
	RemoteSSHOpts StringList // nil = unset

	// resolved layout
	SectionCollapsable bool
	SectionColumns     int

	SectionTitle string
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

// clientTimeoutBuffer is the buffer (10s) added to the server-side timeout
// to account for client-side overhead (network latency, UI rendering, polling delay).
// This ensures the client doesn't timeout before the server completes.
const clientTimeoutBuffer = 10000 // 10s buffer added to server-side timeout for client waits

// Flatten expands all sections and services into a flat slice with cascade
// fallbacks resolved inline for each service.
func (c *Config) Flatten() []FlatService {
	total := 0
	for _, section := range c.Sections {
		total += len(section.Services)
	}
	services := make([]FlatService, 0, total)
	global := c.GetGlobal().CascadeFields
	globalLayout := c.GetGlobal().LayoutFields
	for _, section := range c.Sections {
		sec := section.CascadeFields
		lay := section.LayoutFields
		for _, svc := range section.Services {
			services = append(services, FlatService{
				Title:              svc.Title,
				Command:            svc.Command,
				Timeout:            cascade(svc.Timeout, sec.Timeout, global.Timeout, defaultTimeout),
				Confirm:            cascade(svc.Confirm, sec.Confirm, global.Confirm, defaultConfirm),
				Enabled:            cascade(svc.Enabled, sec.Enabled, global.Enabled, true),
				Restricted:         cascade(svc.Restricted, sec.Restricted, global.Restricted, true),
				Allowed:            cascadeStringList(svc.Allowed, sec.Allowed, global.Allowed),
				Remote:             cascadeStringList(svc.Remote, sec.Remote, global.Remote),
				RemoteSSHOpts:      cascadeStringList(svc.RemoteSSHOpts, sec.RemoteSSHOpts, global.RemoteSSHOpts),
				SectionCollapsable: cascade(nil, lay.Collapsable, globalLayout.Collapsable, defaultCollapsable),
				SectionColumns:     cascade(nil, lay.Columns, globalLayout.Columns, defaultColumns),
				SectionTitle:       section.Title,
			})
		}
	}
	return services
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
		Timeout:            s.Timeout*1000 + clientTimeoutBuffer,
		Confirm:            s.Confirm,
		Section:            &sectionTitle,
		SectionCollapsable: s.SectionCollapsable,
		SectionColumns:     s.SectionColumns,
		Public:             isPublic,
		Accessible:         isPublic || (username != "" && slices.Contains(allowedUsers, username)),
		Enabled:            s.Enabled,
		AllowedUsers:       allowedUsers,
		Restricted:         s.Restricted,
	}
}
