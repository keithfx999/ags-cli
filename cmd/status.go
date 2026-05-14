package cmd

import (
	"fmt"
	"io"
	"os"

	"github.com/TencentCloudAgentRuntime/ags-cli/internal/config"
	"github.com/spf13/cobra"
)

func init() {
	statusCmd.RunE = Wrap("status", statusFn)
	rootCmd.AddCommand(statusCmd)
}

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show current CLI configuration status",
	Long:  `Show the current CLI configuration including backend, region, domain, auth sources, and config file.`,
}

func statusFn(cmd *cobra.Command, args []string) (*CmdResult, error) {
	cfg := config.Get()
	apiKeyPresent, apiKeySource := detectAuthSource("api_key")
	secretIDPresent, secretIDSource := detectAuthSource("secret_id")
	secretKeyPresent, secretKeySource := detectAuthSource("secret_key")

	data := map[string]any{
		"Backend":  cfg.Backend,
		"Region":   cfg.Region,
		"Domain":   cfg.Domain,
		"Internal": cfg.Internal,
		"Output":   cfg.Output,
		"Auth": map[string]any{
			"ApiKey":    map[string]any{"Present": apiKeyPresent, "Source": apiKeySource},
			"SecretId":  map[string]any{"Present": secretIDPresent, "Source": secretIDSource},
			"SecretKey": map[string]any{"Present": secretKeyPresent, "Source": secretKeySource},
		},
	}

	return OK(data, func(w io.Writer) {
		fmt.Fprintf(w, "Backend:   %s\n", cfg.Backend)
		fmt.Fprintf(w, "Region:    %s\n", cfg.Region)
		fmt.Fprintf(w, "Domain:    %s\n", cfg.Domain)
		fmt.Fprintf(w, "Internal:  %v\n", cfg.Internal)
		fmt.Fprintf(w, "Output:    %s\n", cfg.Output)
		fmt.Fprintln(w)
		fmt.Fprintln(w, "Auth:")
		fmt.Fprintf(w, "  API Key:    %s\n", formatAuthStatus(apiKeyPresent, apiKeySource))
		fmt.Fprintf(w, "  Secret ID:  %s\n", formatAuthStatus(secretIDPresent, secretIDSource))
		fmt.Fprintf(w, "  Secret Key: %s\n", formatAuthStatus(secretKeyPresent, secretKeySource))
	}), nil
}

func detectAuthSource(field string) (bool, string) {
	cfg := config.Get()
	switch field {
	case "api_key":
		if os.Getenv("AGS_API_KEY") != "" {
			return true, "AGS_API_KEY"
		}
		if os.Getenv("E2B_API_KEY") != "" {
			return true, "E2B_API_KEY"
		}
		if cfg.Auth.APIKey != "" {
			return true, "config"
		}
		return false, ""
	case "secret_id":
		if os.Getenv("TENCENTCLOUD_SECRET_ID") != "" {
			return true, "TENCENTCLOUD_SECRET_ID"
		}
		if cfg.Auth.SecretID != "" {
			return true, "config"
		}
		return false, ""
	case "secret_key":
		if os.Getenv("TENCENTCLOUD_SECRET_KEY") != "" {
			return true, "TENCENTCLOUD_SECRET_KEY"
		}
		if cfg.Auth.SecretKey != "" {
			return true, "config"
		}
		return false, ""
	}
	return false, ""
}

func formatAuthStatus(present bool, source string) string {
	if !present {
		return "not configured"
	}
	return fmt.Sprintf("configured (source: %s)", source)
}
