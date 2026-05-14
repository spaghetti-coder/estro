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

const SSHOpts = "-o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null"

func ShellEscape(cmd string) string {
	return strings.ReplaceAll(cmd, "'", "'\\''")
}

var hostRegex = regexp.MustCompile(`^[a-zA-Z0-9._@:/-]+$`)

func ValidateHost(host string) error {
	if !hostRegex.MatchString(host) {
		return fmt.Errorf("invalid remote host: %s", host)
	}
	return nil
}

func BuildCmd(command config.CommandValue, remote config.RemoteValue) (string, error) {
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
		result = fmt.Sprintf("ssh %s %s '%s'", SSHOpts, hosts[i], ShellEscape(result))
	}
	return result, nil
}

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