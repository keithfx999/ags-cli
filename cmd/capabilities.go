package cmd

import (
	"fmt"
	"io"

	"github.com/TencentCloudAgentRuntime/ags-cli/internal/config"
	"github.com/spf13/cobra"
)

func init() {
	capabilitiesCmd.RunE = Wrap("capabilities", capabilitiesFn)
	rootCmd.AddCommand(capabilitiesCmd)
}

// CommandCapability describes a command's availability.
type CommandCapability struct {
	Name              string   `json:"Name"`
	Available         bool     `json:"Available"`
	Reason            string   `json:"Reason,omitempty"`
	SupportedBackends []string `json:"SupportedBackends,omitempty"`
}

var capabilitiesCmd = &cobra.Command{
	Use:   "capabilities",
	Short: "Show available commands for current environment",
	Long:  `Show which commands are available in the current environment based on backend configuration.`,
}

func capabilitiesFn(cmd *cobra.Command, args []string) (*CmdResult, error) {
	backend := config.GetBackend()
	commands := buildCapabilities(backend)

	data := map[string]any{"Backend": backend, "Commands": commands}
	return OK(data, func(w io.Writer) {
		fmt.Fprintf(w, "Backend: %s\n\n", backend)
		fmt.Fprintf(w, "%-30s %s\n", "COMMAND", "AVAILABLE")
		for _, c := range commands {
			status := "yes"
			if !c.Available {
				status = fmt.Sprintf("no (%s)", c.Reason)
			}
			fmt.Fprintf(w, "%-30s %s\n", c.Name, status)
		}
	}), nil
}

func buildCapabilities(backend string) []CommandCapability {
	allCommands := []struct {
		name     string
		backends []string
	}{
		{"instance.create", []string{"cloud", "e2b"}},
		{"instance.list", []string{"cloud", "e2b"}},
		{"instance.get", []string{"cloud", "e2b"}},
		{"instance.delete", []string{"cloud", "e2b"}},
		{"instance.code.run", []string{"cloud", "e2b"}},
		{"instance.exec", []string{"cloud", "e2b"}},
		{"instance.file.upload", []string{"cloud", "e2b"}},
		{"instance.file.download", []string{"cloud", "e2b"}},
		{"instance.login", []string{"cloud", "e2b"}},
		{"instance.browser.vnc", []string{"cloud", "e2b"}},
		{"instance.proxy", []string{"cloud", "e2b"}},
		{"instance.mobile.connect", []string{"cloud", "e2b"}},
		{"instance.mobile.disconnect", []string{"cloud", "e2b"}},
		{"instance.mobile.list", []string{"cloud", "e2b"}},
		{"instance.mobile.adb", []string{"cloud", "e2b"}},
		{"tool.create", []string{"cloud"}},
		{"tool.list", []string{"cloud"}},
		{"tool.get", []string{"cloud"}},
		{"tool.update", []string{"cloud"}},
		{"tool.delete", []string{"cloud"}},
		{"apikey.create", []string{"cloud"}},
		{"apikey.list", []string{"cloud"}},
		{"apikey.delete", []string{"cloud"}},
		{"status", []string{"cloud", "e2b"}},
		{"capabilities", []string{"cloud", "e2b"}},
		{"schema", []string{"cloud", "e2b"}},
		{"doctor", []string{"cloud", "e2b"}},
		{"version", []string{"cloud", "e2b"}},
	}

	var caps []CommandCapability
	for _, c := range allCommands {
		available := false
		for _, b := range c.backends {
			if b == backend {
				available = true
				break
			}
		}
		cap := CommandCapability{
			Name:      c.name,
			Available: available,
		}
		if !available {
			cap.Reason = "backend_unsupported"
			cap.SupportedBackends = c.backends
		}
		caps = append(caps, cap)
	}
	return caps
}
