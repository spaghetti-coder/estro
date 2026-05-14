package config

import (
	"fmt"
	"os"
	"reflect"
	"sort"
	"strings"

	"github.com/go-playground/validator/v10"
	"gopkg.in/yaml.v3"
)

const (
	DefaultTimeout     = 60
	DefaultConfirm     = true
	DefaultCollapsable = true
	DefaultColumns     = 3
)

type Config struct {
	Global   *GlobalConfig          `yaml:"global,omitempty" validate:"omitempty"`
	Users    map[string]*UserConfig `yaml:"users,omitempty" validate:"omitempty,dive"`
	Sections []SectionConfig        `yaml:"sections,omitempty" validate:"omitempty,dive"`
}

type GlobalConfig struct {
	Title       *string     `yaml:"title,omitempty"`
	Subtitle    *string     `yaml:"subtitle,omitempty"`
	Hostname    *string     `yaml:"hostname,omitempty"`
	Port        *int        `yaml:"port,omitempty" validate:"omitempty,gt=0"`
	Secret      *string     `yaml:"secret,omitempty"`
	Timeout     *int        `yaml:"timeout,omitempty" validate:"omitempty,gt=0"`
	Confirm     *bool       `yaml:"confirm,omitempty"`
	Allowed     []string    `yaml:"allowed,omitempty"`
	Collapsable *bool       `yaml:"collapsable,omitempty"`
	Columns     *int        `yaml:"columns,omitempty" validate:"omitempty,gt=0"`
	Remote      RemoteValue `yaml:"remote,omitempty"`
}

type UserConfig struct {
	Password string   `yaml:"password" validate:"required"`
	Groups   []string `yaml:"groups,omitempty"`
}

type SectionConfig struct {
	Title       string          `yaml:"title" validate:"required"`
	Services    []ServiceConfig `yaml:"services,omitempty" validate:"omitempty,dive"`
	Allowed     []string        `yaml:"allowed,omitempty"`
	Timeout     *int            `yaml:"timeout,omitempty" validate:"omitempty,gt=0"`
	Confirm     *bool           `yaml:"confirm,omitempty"`
	Collapsable *bool           `yaml:"collapsable,omitempty"`
	Columns     *int            `yaml:"columns,omitempty" validate:"omitempty,gt=0"`
	Remote      RemoteValue     `yaml:"remote,omitempty"`
}

type ServiceConfig struct {
	Title   string       `yaml:"title" validate:"required"`
	Command CommandValue `yaml:"command" validate:"required"`
	Allowed []string     `yaml:"allowed,omitempty"`
	Timeout *int         `yaml:"timeout,omitempty" validate:"omitempty,gt=0"`
	Confirm *bool        `yaml:"confirm,omitempty"`
	Remote  RemoteValue  `yaml:"remote,omitempty"`
}

type FlatService struct {
	Title   string
	Command CommandValue
	Allowed []string
	Timeout *int
	Confirm *bool
	Remote  RemoteValue

	SectionTitle       string
	SectionTimeout     *int
	SectionConfirm     *bool
	SectionAllowed     []string
	SectionCollapsable *bool
	SectionColumns     *int
	SectionRemote      RemoteValue

	Global *GlobalConfig
}

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
	AllowedUsers       []string `json:"allowedUsers"`
}

type ConfigResponse struct {
	Title    string   `json:"title"`
	Subtitle string   `json:"subtitle"`
	Users    []string `json:"users"`
}

type RemoteValue []string

func (r *RemoteValue) UnmarshalYAML(value *yaml.Node) error {
	switch value.Kind {
	case yaml.ScalarNode:
		*r = []string{value.Value}
		return nil
	case yaml.SequenceNode:
		var hosts []string
		if err := value.Decode(&hosts); err != nil {
			return err
		}
		*r = hosts
		return nil
	default:
		return fmt.Errorf("remote must be a string or array of strings")
	}
}

type CommandValue []string

func (c *CommandValue) UnmarshalYAML(value *yaml.Node) error {
	switch value.Kind {
	case yaml.ScalarNode:
		*c = []string{value.Value}
		return nil
	case yaml.SequenceNode:
		var cmds []string
		if err := value.Decode(&cmds); err != nil {
			return err
		}
		*c = cmds
		return nil
	default:
		return fmt.Errorf("command must be a string or array of strings")
	}
}

var validate *validator.Validate

func init() {
	validate = validator.New()
	validate.RegisterTagNameFunc(func(fld reflect.StructField) string {
		name := strings.SplitN(fld.Tag.Get("yaml"), ",", 2)[0]
		if name == "-" || name == "" {
			return fld.Name
		}
		return name
	})
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config file %s: %w", path, err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config YAML: %w", err)
	}

	if err := validate.Struct(cfg); err != nil {
		return nil, formatValidationError(err)
	}

	return &cfg, nil
}

func formatValidationError(err error) error {
	if ve, ok := err.(validator.ValidationErrors); ok {
		var msgs []string
		for _, fe := range ve {
			field := fe.Field()
			msgs = append(msgs, fmt.Sprintf("field \"%s\": %s", field, fe.Tag()))
		}
		return fmt.Errorf("config validation failed: %s", strings.Join(msgs, "; "))
	}
	return err
}

func (c *Config) Flatten() []FlatService {
	var services []FlatService
	globalRef := c.GetGlobal()
	for _, section := range c.Sections {
		for _, svc := range section.Services {
			flat := FlatService{
				Title:   svc.Title,
				Command: svc.Command,
				Allowed: svc.Allowed,
				Timeout: svc.Timeout,
				Confirm: svc.Confirm,
				Remote:  svc.Remote,

				SectionTitle:       section.Title,
				SectionTimeout:     section.Timeout,
				SectionConfirm:     section.Confirm,
				SectionAllowed:     section.Allowed,
				SectionCollapsable: section.Collapsable,
				SectionColumns:     section.Columns,
				SectionRemote:      section.Remote,

				Global: globalRef,
			}
			services = append(services, flat)
		}
	}
	return services
}

func (s *FlatService) GetTimeout() int {
	return cascadeInt(s.Timeout, s.SectionTimeout, s.Global.Timeout, DefaultTimeout)
}

func (s *FlatService) GetConfirm() bool {
	return cascadeBool(s.Confirm, s.SectionConfirm, s.Global.Confirm, DefaultConfirm)
}

func (s *FlatService) GetAllowed() []string {
	if s.Allowed != nil {
		return s.Allowed
	}
	if s.SectionAllowed != nil {
		return s.SectionAllowed
	}
	if s.Global != nil && s.Global.Allowed != nil {
		return s.Global.Allowed
	}
	return nil
}

func (s *FlatService) GetCollapsable() bool {
	return cascadeBool(nil, s.SectionCollapsable, s.Global.Collapsable, DefaultCollapsable)
}

func (s *FlatService) GetColumns() int {
	return cascadeInt(nil, s.SectionColumns, s.Global.Columns, DefaultColumns)
}

func (s *FlatService) GetRemote() RemoteValue {
	if s.Remote != nil {
		return s.Remote
	}
	if s.SectionRemote != nil {
		return s.SectionRemote
	}
	if s.Global != nil && s.Global.Remote != nil {
		return s.Global.Remote
	}
	return nil
}

func (s *FlatService) GetTimeoutMs() int {
	return s.GetTimeout() * 1000
}

func (s *FlatService) ResolveAllowed(users map[string]*UserConfig) []string {
	allowed := s.GetAllowed()
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
	sorted := make([]string, 0, len(result))
	for k := range result {
		sorted = append(sorted, k)
	}
	sort.Strings(sorted)
	return sorted
}

func (s *FlatService) IsAccessible(username string, users map[string]*UserConfig) bool {
	allowed := s.ResolveAllowed(users)
	return allowed == nil || (username != "" && contains(allowed, username))
}

const clientTimeoutBuffer = 10000

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
		Accessible:         isPublic || (username != "" && contains(allowedUsers, username)),
		AllowedUsers:       allowedUsers,
	}
}

func (c *Config) GetGlobal() *GlobalConfig {
	if c.Global != nil {
		return c.Global
	}
	return &GlobalConfig{}
}

func (c *Config) GetConfigResponse() ConfigResponse {
	g := c.GetGlobal()
	title := "Estro"
	if g.Title != nil {
		title = *g.Title
	}
	subtitle := ""
	if g.Subtitle != nil {
		subtitle = *g.Subtitle
	}
	userNames := make([]string, 0, len(c.Users))
	for name := range c.Users {
		userNames = append(userNames, name)
	}
	sort.Strings(userNames)
	return ConfigResponse{
		Title:    title,
		Subtitle: subtitle,
		Users:    userNames,
	}
}

func cascadeInt(svc, sec, global *int, defaultVal int) int {
	if svc != nil {
		return *svc
	}
	if sec != nil {
		return *sec
	}
	if global != nil {
		return *global
	}
	return defaultVal
}

func cascadeBool(svc, sec, global *bool, defaultVal bool) bool {
	if svc != nil {
		return *svc
	}
	if sec != nil {
		return *sec
	}
	if global != nil {
		return *global
	}
	return defaultVal
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
