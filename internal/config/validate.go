// Configuration validation: custom validators, the Issue/LoadResult types, and
// the value/shape/unknown-key checks that build the issue list.
package config

import (
	"errors"
	"fmt"
	"maps"
	"reflect"
	"regexp"
	"slices"
	"sort"
	"strings"

	"github.com/go-playground/validator/v10"
)

// validateAllowedRef checks allowed entry names a known user or group; reaches Config via FieldLevel.Top().
func validateAllowedRef(fl validator.FieldLevel) bool {
	name := fl.Field().String()
	top, ok := fl.Top().Interface().(Config)
	if !ok {
		// allowed_ref is only registered on Config-rooted structs; if the root
		// is ever something else, fail closed rather than silently pass.
		return false
	}
	if _, isUser := top.Users[name]; isUser {
		return true
	}
	for _, u := range top.Users {
		if u != nil && slices.Contains(u.Groups, name) {
			return true
		}
	}
	return false
}

// Issue is a human-readable config problem.
type Issue struct {
	Path string // dotted path (e.g. "global.hostname"); "" for file-level problems
	Msg  string // "required" | "invalid value" | "invalid field"
}

// String renders the issue; back-ticks path when present.
func (i Issue) String() string {
	if i.Path == "" {
		return i.Msg
	}
	return fmt.Sprintf("`%s` %s", i.Path, i.Msg)
}

// LoadResult carries a usable Config plus any issues.
type LoadResult struct {
	Config *Config
	Issues []Issue
}

// Healthy reports no issues.
func (r *LoadResult) Healthy() bool { return len(r.Issues) == 0 }

// IssueStrings returns rendered issue strings for JSON/UI.
func (r *LoadResult) IssueStrings() []string {
	out := make([]string, 0, len(r.Issues))
	for _, is := range r.Issues {
		out = append(out, is.String())
	}
	return out
}

func (r *LoadResult) hasIssue(path string) bool {
	for _, is := range r.Issues {
		if is.Path == path {
			return true
		}
	}
	return false
}

var (
	rootPrefixRe = regexp.MustCompile(`^[^.]+\.`) // strip leading "Config."
	inlineSegRe  = embeddedSegRe()                // strip embedded-struct segments (derived by reflection)
	bracketRe    = regexp.MustCompile(`\[([^\]]*)\]`)
	digitsRe     = regexp.MustCompile(`^\d+$`)
)

// embeddedSegRe builds regex stripping inline-embed names from validator namespace; derived by reflection so renames can't silently break formatPath.
func embeddedSegRe() *regexp.Regexp {
	names := embeddedStructNames(reflect.TypeOf(Config{}), map[reflect.Type]bool{})
	slices.Sort(names)
	names = slices.Compact(names)
	if len(names) == 0 {
		return regexp.MustCompile(`$^`) // matches nothing
	}
	for i, n := range names {
		names[i] = regexp.QuoteMeta(n)
	}
	return regexp.MustCompile(`\.(` + strings.Join(names, "|") + `)`)
}

// embeddedStructNames returns anonymous embed names reachable from t.
func embeddedStructNames(t reflect.Type, seen map[reflect.Type]bool) []string {
	t = derefAll(t)
	if t.Kind() != reflect.Struct || seen[t] {
		return nil
	}
	seen[t] = true
	var names []string
	for i := range t.NumField() {
		f := t.Field(i)
		if f.Anonymous && derefAll(f.Type).Kind() == reflect.Struct {
			names = append(names, derefAll(f.Type).Name())
		}
		names = append(names, embeddedStructNames(f.Type, seen)...)
	}
	return names
}

// derefAll unwraps pointer, slice, array, and map types to their element type.
func derefAll(t reflect.Type) reflect.Type {
	for {
		switch t.Kind() {
		case reflect.Pointer, reflect.Slice, reflect.Array, reflect.Map:
			t = t.Elem()
		default:
			return t
		}
	}
}

// formatPath cleans validator namespace: drops root/embed segments, dots map-key brackets; numeric brackets kept as indices.
func formatPath(ns string) string {
	ns = rootPrefixRe.ReplaceAllString(ns, "")
	ns = inlineSegRe.ReplaceAllString(ns, "")
	ns = bracketRe.ReplaceAllStringFunc(ns, func(m string) string {
		inner := m[1 : len(m)-1]
		if digitsRe.MatchString(inner) {
			// All-digit brackets are array indices; an all-digit map key would be
			// misclassified, which is acceptable for operator-chosen names.
			return m
		}
		return "." + inner // map key — dotted
	})
	return ns
}

func tagMessage(tag string) string {
	if tag == "required" {
		return "required"
	}
	return "invalid value"
}

// Validate runs structural and value validation on the resolved Config,
// returning any issues found. This is the validation half of Load, extracted
// for use in tests that construct Config programmatically.
func Validate(cfg *Config) []Issue {
	v, err := newValidator()
	if err != nil {
		return []Issue{{Msg: err.Error()}}
	}
	err = v.Struct(*cfg)
	if err == nil {
		return nil
	}
	var ve validator.ValidationErrors
	if !errors.As(err, &ve) {
		return []Issue{{Msg: err.Error()}}
	}
	issues := make([]Issue, 0, len(ve))
	for _, fe := range ve {
		issues = append(issues, Issue{Path: formatPath(fe.Namespace()), Msg: tagMessage(fe.Tag())})
	}
	return issues
}

// collectIssues merges value errors and shape/unknown-key issues; dedupes per path.
func collectIssues(cfg *Config, raw map[string]any) []Issue {
	var issues []Issue
	issues = append(issues, Validate(cfg)...)
	issues = append(issues, shapeIssues(raw)...)
	return dedupeSort(issues)
}

// shapeIssues reports "invalid value" for wrong-shape fields; walks resolved tree against struct schema, skips x-*/unknown.
func shapeIssues(raw map[string]any) []Issue {
	var issues []Issue
	walkStructShape(reflect.TypeOf(Config{}), raw, "", &issues)
	return issues
}

// walkStructShape checks node keys against schema; skips x-*, delegates known, flags unknown.
func walkStructShape(t reflect.Type, node map[string]any, path string, issues *[]Issue) {
	schema := schemaFields(t)
	for key, val := range node {
		if strings.HasPrefix(key, "x-") {
			continue
		}
		field, ok := schema[key]
		if !ok {
			*issues = append(*issues, Issue{Path: joinPath(path, key), Msg: "invalid field"})
			continue
		}
		checkFieldShape(field.Type, val, joinPath(path, key), issues)
	}
}

// checkFieldShape validates YAML shape for ft; recurses into structs/slices/maps.
func checkFieldShape(ft reflect.Type, val any, path string, issues *[]Issue) {
	if val == nil {
		return
	}

	ft = derefType(ft)

	// StringList and CommandValue: must NOT be a mapping (scalar or sequence OK).
	stringListType := reflect.TypeOf(StringList(nil))
	commandValueType := reflect.TypeOf(CommandValue(nil))
	if ft == stringListType || ft == commandValueType {
		if _, isMap := asMap(val); isMap {
			*issues = append(*issues, Issue{Path: path, Msg: "invalid value"})
		}
		return
	}

	switch ft.Kind() {
	case reflect.Slice:
		// Other slices: must be a sequence.
		seq, isSeq := asSeq(val)
		if !isSeq {
			*issues = append(*issues, Issue{Path: path, Msg: "invalid value"})
			return
		}
		// Recurse into struct elements.
		elemType := derefType(ft.Elem())
		if elemType.Kind() == reflect.Struct {
			for i, elem := range seq {
				elemPath := fmt.Sprintf("%s[%d]", path, i)
				if m, ok := asMap(elem); ok {
					walkStructShape(elemType, m, elemPath, issues)
				} else if elem != nil {
					*issues = append(*issues, Issue{Path: elemPath, Msg: "invalid value"})
				}
			}
		}

	case reflect.Struct:
		m, isMap := asMap(val)
		if !isMap {
			*issues = append(*issues, Issue{Path: path, Msg: "invalid value"})
			return
		}
		walkStructShape(ft, m, path, issues)

	case reflect.Map:
		// map[string]*Struct or similar.
		m, isMap := asMap(val)
		if !isMap {
			*issues = append(*issues, Issue{Path: path, Msg: "invalid value"})
			return
		}
		valElemType := derefType(ft.Elem())
		if valElemType.Kind() == reflect.Struct {
			for k, v := range m {
				entryPath := joinPath(path, k)
				if mv, ok := asMap(v); ok {
					walkStructShape(valElemType, mv, entryPath, issues)
				} else if v != nil {
					*issues = append(*issues, Issue{Path: entryPath, Msg: "invalid value"})
				}
			}
		}

	default:
		// Scalar kinds (string, int, bool, float, etc.): must NOT be a mapping or sequence.
		if _, isMap := asMap(val); isMap {
			*issues = append(*issues, Issue{Path: path, Msg: "invalid value"})
			return
		}
		if _, isSeq := asSeq(val); isSeq {
			*issues = append(*issues, Issue{Path: path, Msg: "invalid value"})
			return
		}
		// String value for non-string field; YAML decoders silently convert to zero values.
		if _, isStr := val.(string); isStr && ft.Kind() != reflect.String {
			*issues = append(*issues, Issue{Path: path, Msg: "invalid value"})
		}
	}
}

// schemaFields maps YAML keys to StructField; flattens inline embeds, skips Extra/yaml:"-".
func schemaFields(t reflect.Type) map[string]reflect.StructField {
	result := make(map[string]reflect.StructField)
	for i := range t.NumField() {
		f := t.Field(i)
		tag := f.Tag.Get("yaml")
		if tag == "-" {
			continue
		}
		// Split tag into name and options.
		name, opts, _ := strings.Cut(tag, ",")
		isInline := strings.Contains(opts, "inline") || (name == "" && strings.Contains(tag, "inline"))

		if isInline {
			// Inline map (Extra catch-all): skip.
			if f.Type.Kind() == reflect.Map {
				continue
			}
			// Anonymous inline struct: flatten recursively.
			embedded := derefType(f.Type)
			if embedded.Kind() == reflect.Struct {
				maps.Copy(result, schemaFields(embedded))
			}
			continue
		}

		if name == "" {
			name = f.Name
		}
		result[name] = f
	}
	return result
}

// derefType returns the element type if t is a pointer, otherwise t.
func derefType(t reflect.Type) reflect.Type {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	return t
}

// asMap returns (m, true) if val is a map[string]any.
func asMap(val any) (map[string]any, bool) {
	m, ok := val.(map[string]any)
	return m, ok
}

// asSeq returns (s, true) if val is a []any.
func asSeq(val any) ([]any, bool) {
	s, ok := val.([]any)
	return s, ok
}

// joinPath concatenates prefix and key with ".", or returns key when prefix is empty.
func joinPath(prefix, key string) string {
	if prefix == "" {
		return key
	}
	return prefix + "." + key
}

// msgRank ranks issues per path: "invalid value" > "required" (bad value can trigger "required" when decoded empty) > "invalid field".
func msgRank(msg string) int {
	switch msg {
	case "invalid value":
		return 0
	case "required":
		return 1
	case "invalid field":
		return 2
	default:
		return 99
	}
}

// dedupeSort keeps highest-ranked issue per path; dedupes pathless by exact value.
func dedupeSort(in []Issue) []Issue {
	byPath := make(map[string]Issue)
	var pathless []Issue
	seen := make(map[Issue]struct{})
	for _, is := range in {
		if is.Path == "" {
			if _, ok := seen[is]; ok {
				continue
			}
			seen[is] = struct{}{}
			pathless = append(pathless, is)
			continue
		}
		if cur, ok := byPath[is.Path]; !ok || msgRank(is.Msg) < msgRank(cur.Msg) {
			byPath[is.Path] = is
		}
	}
	out := make([]Issue, 0, len(byPath)+len(pathless))
	for _, is := range byPath {
		out = append(out, is)
	}
	out = append(out, pathless...)
	sort.Slice(out, func(i, j int) bool {
		if out[i].Path != out[j].Path {
			return out[i].Path < out[j].Path
		}
		return out[i].Msg < out[j].Msg
	})
	return out
}

// ServerAddr returns host:port; falls back to defaults on invalid values.
func (r *LoadResult) ServerAddr() string {
	g := r.Config.GetGlobal()
	host := coalesce(g.Hostname, defaultHostname)
	if r.hasIssue("global.hostname") {
		host = defaultHostname
	}
	port := coalesce(g.Port, defaultPort)
	if port < 1 || port > 65535 {
		port = defaultPort
	}
	return fmt.Sprintf("%s:%d", host, port)
}
