// Package-internal validation engine: custom validators, the Issue/LoadResult
// types, issue collection from validator/yaml errors, and degraded-mode helpers.
// See docs/kv1/task016-spec.md.
package config

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/go-playground/validator/v10"
	"go.yaml.in/yaml/v4"
)

// validateAllowedRef is the "allowed_ref" element validator: an `allowed`
// entry must name an existing user or a group held by at least one user.
// It reaches the root Config via FieldLevel.Top().
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
		if u == nil {
			continue
		}
		for _, g := range u.Groups {
			if g == name {
				return true
			}
		}
	}
	return false
}

// Issue is one human-readable configuration problem.
type Issue struct {
	Path string // dotted path (e.g. "global.hostname"); "" for file-level problems
	Msg  string // "required" | "invalid value" | "invalid field" | a precondition message
}

// String renders the issue, back-ticking the path when present.
func (i Issue) String() string {
	if i.Path == "" {
		return i.Msg
	}
	return fmt.Sprintf("`%s` %s", i.Path, i.Msg)
}

// LoadResult always yields a usable Config plus any issues found.
type LoadResult struct {
	Config *Config
	Issues []Issue
}

// Healthy reports whether the configuration loaded with no issues.
func (r *LoadResult) Healthy() bool { return len(r.Issues) == 0 }

// IssueStrings returns the rendered issue strings (for JSON responses / UI).
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
	inlineSegRe  = regexp.MustCompile(`\.(CascadeFields|LayoutFields)`)
	bracketRe    = regexp.MustCompile(`\[([^\]]*)\]`)
	digitsRe     = regexp.MustCompile(`^\d+$`)
	xKeyRe       = regexp.MustCompile(`^x-`)
	extPathRe    = regexp.MustCompile(`(^|\.)x-`) // a path segment that is an x-* extension key
)

// formatPath turns a validator namespace into the display path of spec §5:
// strip the root type, strip inline-embedded struct segments, and turn
// non-numeric map-key brackets into dotted segments (numeric = array index).
func formatPath(ns string) string {
	ns = rootPrefixRe.ReplaceAllString(ns, "")
	ns = inlineSegRe.ReplaceAllString(ns, "")
	ns = bracketRe.ReplaceAllStringFunc(ns, func(m string) string {
		inner := m[1 : len(m)-1]
		if digitsRe.MatchString(inner) {
			// All-digit bracket content is treated as an array index (kept as [n]).
			// An all-digit username would be misclassified here, which is acceptable
			// (usernames are operator-chosen); see spec §5.
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

// validateStruct runs validator/v10 over the resolved config and maps each
// failure to an Issue with a display path.
func validateStruct(cfg *Config) []Issue {
	err := validate.Struct(*cfg)
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

// unknownKeyIssues walks the resolved config and reports every non-"x-" key
// captured in an Extra catch-all map. It does NOT descend into the value.
func unknownKeyIssues(cfg *Config) []Issue {
	var issues []Issue
	add := func(prefix string, extra map[string]yaml.Node) {
		for k := range extra {
			if xKeyRe.MatchString(k) {
				continue
			}
			path := k
			if prefix != "" {
				path = prefix + "." + k
			}
			issues = append(issues, Issue{Path: path, Msg: "invalid field"})
		}
	}
	add("", cfg.Extra)
	if cfg.Global != nil {
		add("global", cfg.Global.Extra)
	}
	for name, u := range cfg.Users {
		if u != nil {
			add("users."+name, u.Extra)
		}
	}
	for i := range cfg.Sections {
		sec := &cfg.Sections[i]
		add(fmt.Sprintf("sections[%d]", i), sec.Extra)
		for j := range sec.Services {
			add(fmt.Sprintf("sections[%d].services[%d]", i, j), sec.Services[j].Extra)
		}
	}
	return issues
}

// typeErrorPaths resolves each YAML type error to the config path it occurred
// at (via the parsed node tree), dropping any path inside an x-* extension key
// (x-* content is always valid). Returns de-duplicated paths.
func typeErrorPaths(le *yaml.LoadErrors, data []byte) []string {
	index := map[int]string{}
	var root yaml.Node
	if yaml.Load(data, &root) == nil {
		buildLineIndex(&root, "", index)
	}
	var paths []string
	seen := map[string]bool{}
	for _, e := range le.Errors {
		p := index[e.Line]
		if extPathRe.MatchString(p) {
			continue
		}
		if !seen[p] {
			seen[p] = true
			paths = append(paths, p)
		}
	}
	return paths
}

// isPathOrDescendant reports whether t equals v or is nested under it
// (e.g. "global.hostname[0]" is under "global.hostname").
func isPathOrDescendant(t, v string) bool {
	return t == v || strings.HasPrefix(t, v+".") || strings.HasPrefix(t, v+"[")
}

// fieldPath strips a trailing "[i]" index so a type error reported on a list
// element collapses to its field when no validator issue claims it.
func fieldPath(p string) string {
	if i := strings.LastIndex(p, "["); i > 0 && strings.HasSuffix(p, "]") {
		return p[:i]
	}
	return p
}

// collectIssues merges all issue sources into the final, deduped list. A YAML
// type error relabels the validator issue for the same field (or its ancestor)
// to "invalid type" rather than adding a separate, element-pathed entry; type
// errors with no matching validator issue are added as standalone "invalid type".
func collectIssues(cfg *Config, typePaths []string) []Issue {
	vIssues := validateStruct(cfg)
	consumed := make([]bool, len(typePaths))
	for i := range vIssues {
		for j, tp := range typePaths {
			if isPathOrDescendant(tp, vIssues[i].Path) {
				vIssues[i].Msg = "invalid type"
				consumed[j] = true
			}
		}
	}
	issues := vIssues
	for j, tp := range typePaths {
		if !consumed[j] {
			issues = append(issues, Issue{Path: fieldPath(tp), Msg: "invalid type"})
		}
	}
	issues = append(issues, unknownKeyIssues(cfg)...)
	return dedupeSort(issues)
}

// buildLineIndex maps each value node's YAML line to its dotted display path
// (yaml key names, numeric [i] for sequence elements), matching formatPath.
func buildLineIndex(n *yaml.Node, prefix string, index map[int]string) {
	switch n.Kind {
	case yaml.DocumentNode:
		for _, c := range n.Content {
			buildLineIndex(c, prefix, index)
		}
	case yaml.MappingNode:
		for i := 0; i+1 < len(n.Content); i += 2 {
			key, val := n.Content[i], n.Content[i+1]
			path := key.Value
			if prefix != "" {
				path = prefix + "." + key.Value
			}
			index[val.Line] = path
			buildLineIndex(val, path, index)
		}
	case yaml.SequenceNode:
		for i, c := range n.Content {
			path := fmt.Sprintf("%s[%d]", prefix, i)
			index[c.Line] = path
			buildLineIndex(c, path, index)
		}
	}
}

// msgRank orders issue messages so that, when several land on the same field,
// the most root-cause one wins (a wrong type subsumes the "required" or
// "invalid value" it incidentally triggers).
func msgRank(msg string) int {
	switch msg {
	case "invalid type":
		return 0
	case "invalid value":
		return 1
	case "required":
		return 2
	case "invalid field":
		return 3
	default:
		return 99
	}
}

// dedupeSort collapses issues to at most one per field path (keeping the
// highest-ranked message) and returns them in a stable order. Path-less issues
// are kept and de-duplicated by exact value.
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

// ServerAddr returns the host:port to bind, substituting defaults for any
// server field that is invalid (degraded-mode boot, spec §6.2). The port is
// defaulted whenever it is outside 1..65535 — this covers an out-of-range
// value and a wrong-type value that yaml resolved to 0 (which would otherwise
// bind a random free port) — so it does not rely on issue bookkeeping.
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
