// Package deleteguard contains shared safety behavior for destructive commands.
package deleteguard

import (
	"bufio"
	"fmt"
	"strings"

	"github.com/TencentCloudAgentRuntime/ags-cli/internal/command"
	"github.com/TencentCloudAgentRuntime/ags-cli/internal/iostreams"
)

// Flags returns the common safety flags for destructive commands.
func Flags() []command.FlagSpec {
	return []command.FlagSpec{
		{
			Name:     "dry-run",
			Usage:    "Show resources that would be deleted without deleting them",
			Type:     command.FlagBool,
			Workflow: true,
		},
		{
			Name:     "yes",
			Usage:    "Skip the confirmation prompt",
			Type:     command.FlagBool,
			Workflow: true,
		},
	}
}

// DryRun reports whether --dry-run was explicitly enabled.
func DryRun(req command.Request) bool {
	return boolFlag(req, "dry-run")
}

// Yes reports whether --yes was explicitly enabled.
func Yes(req command.Request) bool {
	return boolFlag(req, "yes")
}

// Confirm asks for confirmation when stdin is interactive. Non-interactive
// callers keep the historical no-prompt behavior unless --dry-run is used.
func Confirm(ios *iostreams.IOStreams, resource string, ids []string, req command.Request) (bool, error) {
	if DryRun(req) || Yes(req) || ios == nil || !ios.IsStdinTTY() {
		return true, nil
	}
	fmt.Fprintf(ios.ErrOut, "Delete %d %s(s): %s? [y/N] ", len(ids), resource, strings.Join(ids, ", "))
	line, err := bufio.NewReader(ios.In).ReadString('\n')
	if err != nil && len(line) == 0 {
		return false, err
	}
	answer := strings.ToLower(strings.TrimSpace(line))
	return answer == "y" || answer == "yes", nil
}

func boolFlag(req command.Request, name string) bool {
	flag, ok := req.Flags[name]
	return ok && flag.Changed && flag.Bool
}
