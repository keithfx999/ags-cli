package cmd

import (
	"fmt"
	"io"
	"os"

	"github.com/TencentCloudAgentRuntime/ags-cli/internal/config"
	"github.com/TencentCloudAgentRuntime/ags-cli/internal/token"
	"github.com/spf13/cobra"
)

func init() {
	doctorCmd.RunE = Wrap("doctor", doctorFn)
	rootCmd.AddCommand(doctorCmd)
}

// DoctorCheck represents a single diagnostic check result.
type DoctorCheck struct {
	Name    string `json:"Name"`
	Status  string `json:"Status"`
	Message string `json:"Message"`
	Hint    string `json:"Hint,omitempty"`
}

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Diagnose CLI configuration issues",
	Long:  `Run diagnostic checks on CLI configuration and report any issues with hints for fixing them.`,
}

func doctorFn(cmd *cobra.Command, args []string) (*CmdResult, error) {
	cfg := config.Get()
	var checks []DoctorCheck

	switch cfg.Backend {
	case "cloud", "e2b":
		checks = append(checks, DoctorCheck{
			Name: "backend", Status: "ok",
			Message: fmt.Sprintf("backend is %s", cfg.Backend),
		})
	default:
		checks = append(checks, DoctorCheck{
			Name: "backend", Status: "error",
			Message: fmt.Sprintf("invalid backend: %s", cfg.Backend),
			Hint:    "Set backend to 'cloud' or 'e2b' in config or via --backend flag.",
		})
	}

	switch cfg.Output {
	case "text", "json":
		checks = append(checks, DoctorCheck{
			Name: "output", Status: "ok",
			Message: fmt.Sprintf("output format is %s", cfg.Output),
		})
	default:
		checks = append(checks, DoctorCheck{
			Name: "output", Status: "error",
			Message: fmt.Sprintf("invalid output format: %s", cfg.Output),
			Hint:    "Set output to 'text' or 'json'.",
		})
	}

	if cfg.Auth.APIKey != "" || os.Getenv("AGS_API_KEY") != "" || os.Getenv("E2B_API_KEY") != "" {
		checks = append(checks, DoctorCheck{
			Name: "ApiKey", Status: "ok",
			Message: "API key is configured",
		})
	} else {
		status := "warning"
		hint := "Set AGS_API_KEY or E2B_API_KEY to use the e2b backend."
		if cfg.Backend == "e2b" {
			status = "error"
			hint = "E2B backend requires an API key. Set AGS_API_KEY or E2B_API_KEY."
		}
		checks = append(checks, DoctorCheck{
			Name: "ApiKey", Status: status,
			Message: "API key is not configured",
			Hint:    hint,
		})
	}

	hasSecretID := cfg.Auth.SecretID != "" || os.Getenv("TENCENTCLOUD_SECRET_ID") != ""
	hasSecretKey := cfg.Auth.SecretKey != "" || os.Getenv("TENCENTCLOUD_SECRET_KEY") != ""
	if hasSecretID && hasSecretKey {
		checks = append(checks, DoctorCheck{
			Name: "CloudCredentials", Status: "ok",
			Message: "Cloud credentials are configured",
		})
	} else if hasSecretID != hasSecretKey {
		checks = append(checks, DoctorCheck{
			Name: "CloudCredentials", Status: "error",
			Message: "Partial cloud credentials: both TENCENTCLOUD_SECRET_ID and TENCENTCLOUD_SECRET_KEY are required",
			Hint:    "Set both TENCENTCLOUD_SECRET_ID and TENCENTCLOUD_SECRET_KEY.",
		})
	} else {
		status := "warning"
		hint := "Set TENCENTCLOUD_SECRET_ID and TENCENTCLOUD_SECRET_KEY to use the cloud backend."
		if cfg.Backend == "cloud" {
			status = "error"
			hint = "Cloud backend requires credentials. Set TENCENTCLOUD_SECRET_ID and TENCENTCLOUD_SECRET_KEY."
		}
		checks = append(checks, DoctorCheck{
			Name: "CloudCredentials", Status: status,
			Message: "Cloud credentials are not configured",
			Hint:    hint,
		})
	}

	_, tokenErr := token.NewCache()
	if tokenErr != nil {
		checks = append(checks, DoctorCheck{
			Name: "TokenCache", Status: "warning",
			Message: fmt.Sprintf("Token cache initialization failed: %v", tokenErr),
			Hint:    "Ensure ~/.ags/ directory exists and is writable.",
		})
	} else {
		checks = append(checks, DoctorCheck{
			Name: "TokenCache", Status: "ok",
			Message: "Token cache is accessible",
		})
	}

	if cfg.Backend == "e2b" {
		checks = append(checks, DoctorCheck{
			Name: "BackendCapabilities", Status: "warning",
			Message: "E2B backend does not support: tool management, apikey management",
			Hint:    "Use --backend cloud with TENCENTCLOUD_SECRET_ID and TENCENTCLOUD_SECRET_KEY for full capabilities.",
		})
	}

	data := map[string]any{"Checks": checks}
	return OK(data, func(w io.Writer) {
		hasErrors := false
		for _, c := range checks {
			icon := "✓"
			switch c.Status {
			case "warning":
				icon = "⚠"
			case "error":
				icon = "✗"
				hasErrors = true
			}
			fmt.Fprintf(w, "%s %s: %s\n", icon, c.Name, c.Message)
			if c.Hint != "" {
				fmt.Fprintf(w, "  Hint: %s\n", c.Hint)
			}
		}
		if hasErrors {
			fmt.Fprintln(ios.ErrOut, "\nSome checks failed. Please fix the issues above.")
		} else {
			fmt.Fprintln(w, "\nAll checks passed.")
		}
	}), nil
}
