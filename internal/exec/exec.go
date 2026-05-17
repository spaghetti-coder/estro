// Package exec builds and runs shell commands locally or over SSH.
package exec

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/spaghetti-coder/estro/internal/config"
)

const sshOpts = "-o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null"

// ShellEscape wraps a shell command in single quotes, escaping any embedded single quotes.
func ShellEscape(cmd string) string {
	return strings.ReplaceAll(cmd, "'", "'\\''")
}

var hostRegex = regexp.MustCompile(`^[a-zA-Z0-9._@:/-]+$`)

// ValidateHost checks that a hostname contains only permitted characters
// for use in SSH connection strings.
func ValidateHost(host string) error {
	if !hostRegex.MatchString(host) {
		return fmt.Errorf("invalid remote host: %s", host)
	}
	return nil
}

// BuildCmd constructs the final shell command string, wrapping it in nested
// SSH sessions when a remote chain is specified.
func BuildCmd(command config.CommandValue, remote config.StringList) (string, error) {
	cmd := strings.Join(command, " && ")
	if len(remote) == 0 {
		return cmd, nil
	}
	hosts := remote
	for _, h := range hosts {
		if err := ValidateHost(h); err != nil {
			return "", err
		}
	}
	result := cmd
	for i := len(hosts) - 1; i >= 0; i-- {
		result = fmt.Sprintf("ssh %s %s '%s'", sshOpts, hosts[i], ShellEscape(result))
	}
	return result, nil
}

// RunCommand executes a shell command via "sh -c" with an optional timeout,
// returning trimmed stdout, stderr, and any execution error.
func RunCommand(ctx context.Context, cmdStr string, timeout time.Duration) (string, string, error) {
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}
	cmd := exec.CommandContext(ctx, "sh", "-c", cmdStr)
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return strings.TrimRight(stdout.String(), "\n"), strings.TrimRight(stderr.String(), "\n"), err
}
