package config

import (
	"fmt"
	"net"
	"regexp"
	"strconv"
	"strings"

	"github.com/go-playground/validator/v10"
)

// RemoteHost is a parsed "[user@]host[:port]" SSH target. Host is always the
// bare address (IPv6 brackets stripped); User and Port may be empty.
type RemoteHost struct {
	User string
	Host string
	Port string
}

// Target returns the SSH connection target, "[user@]host" (no brackets, no port).
func (r RemoteHost) Target() string {
	if r.User != "" {
		return r.User + "@" + r.Host
	}
	return r.Host
}

// SplitRemoteHost decomposes a single "[user@]host[:port]" entry. It performs
// structural parsing only — it does not validate host/user/port content. IPv6
// with a port requires bracket notation ("[::1]:22"); a bare IPv6 literal is
// treated as host-only.
func SplitRemoteHost(s string) (RemoteHost, error) {
	if s == "" {
		return RemoteHost{}, fmt.Errorf("empty remote host")
	}
	var rh RemoteHost
	rest := s
	if i := strings.IndexByte(s, '@'); i >= 0 {
		rh.User = s[:i]
		rest = s[i+1:]
		if rh.User == "" {
			return RemoteHost{}, fmt.Errorf("empty user in remote host %q", s)
		}
		if rest == "" {
			return RemoteHost{}, fmt.Errorf("empty host in remote host %q", s)
		}
		if strings.ContainsRune(rest, '@') {
			return RemoteHost{}, fmt.Errorf("invalid remote host %q", s)
		}
	}

	if host, port, err := net.SplitHostPort(rest); err == nil {
		// A colon-delimited port was present (possibly empty, e.g. "host:").
		if port == "" {
			return RemoteHost{}, fmt.Errorf("empty port in remote host %q", s)
		}
		if host == "" {
			return RemoteHost{}, fmt.Errorf("empty host in remote host %q", s)
		}
		rh.Host, rh.Port = host, port
		return rh, nil
	}

	// No port present: rest is host-only. Strip a matched IPv6 bracket pair.
	if strings.HasPrefix(rest, "[") && strings.HasSuffix(rest, "]") {
		rest = rest[1 : len(rest)-1]
	} else if strings.ContainsAny(rest, "[]") {
		return RemoteHost{}, fmt.Errorf("malformed remote host %q", s)
	}
	if rest == "" {
		return RemoteHost{}, fmt.Errorf("empty host in remote host %q", s)
	}
	rh.Host = rest
	return rh, nil
}

// unixUsernameRe matches a conservative Unix username: starts with a lower-case
// letter or underscore, followed by lower-case letters, digits, underscores or
// hyphens. The trailing '$' (Samba machine-account) form is deliberately not
// allowed: the SSH target is built into a shell string run via "sh -c", where a
// "$@" sequence (e.g. "user$@host") would undergo shell expansion and silently
// connect to the wrong host.
var unixUsernameRe = regexp.MustCompile(`^[a-z_][a-z0-9_-]*$`)

func isValidUnixUsername(s string) bool {
	return len(s) <= 32 && unixUsernameRe.MatchString(s)
}

// validateRemoteHost is the "remote_host" validator: structural parse + segment
// content checks. Host reuses the built-in "hostname_rfc1123|ip" rule; user and
// port are checked explicitly (validator has no primitive for either).
func validateRemoteHost(fl validator.FieldLevel) bool {
	rh, err := SplitRemoteHost(fl.Field().String())
	if err != nil {
		return false
	}
	if rh.User != "" && !isValidUnixUsername(rh.User) {
		return false
	}
	if validate.Var(rh.Host, "hostname_rfc1123|ip") != nil {
		return false
	}
	if rh.Port != "" {
		n, err := strconv.Atoi(rh.Port)
		if err != nil || n < 1 || n > 65535 {
			return false
		}
	}
	return true
}
