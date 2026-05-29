// Package exec builds and runs shell commands locally or over SSH.
package exec

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/spaghetti-coder/estro/internal/config"
)

// ShellEscape escapes embedded single quotes so cmd can be safely placed inside
// a single-quoted shell string. The surrounding quotes are added by the caller
// (see BuildCmd).
func ShellEscape(cmd string) string {
	return strings.ReplaceAll(cmd, "'", "'\\''")
}

// BuildCmd constructs the final shell command string, wrapping it in nested
// SSH sessions when a remote chain is specified. Each hop is a validated
// "[user@]host[:port]" entry; a port is rendered as an "ssh -p <port>" option.
// The sshOpts parameter is the space-separated SSH options string (e.g.
// "-o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null").
func BuildCmd(command config.CommandValue, remote config.StringList, sshOpts string) (string, error) {
	cmd := strings.Join(command, " && ")
	if len(remote) == 0 {
		return cmd, nil
	}
	for i := len(remote) - 1; i >= 0; i-- {
		rh, err := config.SplitRemoteHost(remote[i])
		if err != nil {
			return "", err
		}
		sshPart := "ssh"
		if sshOpts != "" {
			sshPart += " " + sshOpts
		}
		if rh.Port != "" {
			sshPart += " -p " + rh.Port
		}
		cmd = fmt.Sprintf("%s %s '%s'", sshPart, rh.Target(), ShellEscape(cmd))
	}
	return cmd, nil
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
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		return nil
	}

	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return strings.TrimRight(stdout.String(), "\n"), strings.TrimRight(stderr.String(), "\n"), err
}
