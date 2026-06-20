package config

// FlatService is a service with all cascades resolved.
type FlatService struct {
	Title   string
	Command CommandValue
	// resolved cascading fields
	Timeout       int
	Confirm       bool
	Enabled       bool
	Restricted    bool
	Allowed       []string   // nil = public
	Remote        StringList // nil = inherit/cascade, [] = explicit local
	RemoteSSHOpts StringList // nil = unset
	// resolved layout
	SectionCollapsable bool
	SectionColumns     int
	SectionTitle       string
}

// SerializedService is JSON-ready service for frontend.
type SerializedService struct {
	ID                 int      `json:"id"`
	Title              string   `json:"title"`
	Timeout            int      `json:"timeout"`
	Confirm            bool     `json:"confirm"`
	Section            string   `json:"section"`
	SectionCollapsable bool     `json:"sectionCollapsable"`
	SectionColumns     int      `json:"sectionColumns"`
	Public             bool     `json:"public"`
	Accessible         bool     `json:"accessible"`
	Enabled            bool     `json:"enabled"`
	AllowedUsers       []string `json:"allowedUsers"`
	Restricted         bool     `json:"restricted"`
}

// ConfigResponse is title, subtitle, users for the frontend.
type ConfigResponse struct {
	Title    string   `json:"title"`
	Subtitle string   `json:"subtitle"`
	Users    []string `json:"users"`
	Degraded bool     `json:"degraded"`
	Issues   []string `json:"issues,omitempty"`
}

// clientTimeoutBuffer is 10s added to server timeout for client overhead.
const clientTimeoutBuffer = 10000

// Flatten expands all services with cascades resolved inline.
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
				Enabled:            cascade(svc.Enabled, sec.Enabled, global.Enabled, defaultEnabled),
				Restricted:         cascade(svc.Restricted, sec.Restricted, global.Restricted, defaultRestricted),
				Allowed:            resolveAllowed(cascadeStringList(svc.Allowed, sec.Allowed, global.Allowed), c.Users),
				Remote:             cascadeStringList(svc.Remote, sec.Remote, global.Remote),
				RemoteSSHOpts:      cascadeStringList(svc.RemoteSSHOpts, sec.RemoteSSHOpts, global.RemoteSSHOpts),
				SectionCollapsable: cascadeLayout(lay.Collapsable, globalLayout.Collapsable, defaultCollapsable),
				SectionColumns:     cascadeLayout(lay.Columns, globalLayout.Columns, defaultColumns),
				SectionTitle:       section.Title,
			})
		}
	}
	return services
}

// Serialize builds JSON-ready service view for username.
func (s *FlatService) Serialize(index int, username string) SerializedService {
	isPublic := s.Allowed == nil
	return SerializedService{
		ID:                 index,
		Title:              s.Title,
		Timeout:            s.Timeout*1000 + clientTimeoutBuffer,
		Confirm:            s.Confirm,
		Section:            s.SectionTitle,
		SectionCollapsable: s.SectionCollapsable,
		SectionColumns:     s.SectionColumns,
		Public:             isPublic,
		Accessible:         s.IsAccessible(username),
		Enabled:            s.Enabled,
		AllowedUsers:       s.Allowed,
		Restricted:         s.Restricted,
	}
}
