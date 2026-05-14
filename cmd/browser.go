package cmd

import (
	"context"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/TencentCloudAgentRuntime/ags-cli/internal/config"
)

var (
	browserPort      int
	browserNoBrowser bool
)

func browserVNCFn(cmd *cobra.Command, args []string) (*CmdResult, error) {
	ctx := context.Background()
	instanceID := args[0]

	if err := config.Validate(); err != nil {
		return nil, err
	}

	accessToken, err := acquireInstanceToken(ctx, instanceID)
	if err != nil {
		return nil, fmt.Errorf("failed to acquire access token: %w", err)
	}

	cfg := config.Get()

	vncURL := buildVNCURL(instanceID, cfg.Region, cfg.DataPlaneDomain(), accessToken, browserPort)
	cdpURL := buildCDPURL(instanceID, cfg.Region, cfg.DataPlaneDomain(), accessToken, browserPort)

	data := map[string]any{
		"InstanceId": instanceID,
		"VncUrl":     vncURL,
		"CdpUrl":     cdpURL,
	}

	return OK(data, func(w io.Writer) {
		printKV(w, []KeyValue{
			{Key: "Instance ID", Value: instanceID},
			{Key: "VNC URL", Value: vncURL},
			{Key: "CDP URL", Value: cdpURL},
		})
	}), nil
}

// buildVNCURL constructs the noVNC URL for browser sandbox
func buildVNCURL(instanceID, region, domain, accessToken string, port int) string {
	host := fmt.Sprintf("%d-%s.%s.%s", port, instanceID, region, domain)
	return fmt.Sprintf("https://%s/novnc/vnc_lite.html?&path=websockify?access_token=%s", host, accessToken)
}

// buildCDPURL constructs the CDP (Chrome DevTools Protocol) URL for browser sandbox
func buildCDPURL(instanceID, region, domain, accessToken string, port int) string {
	host := fmt.Sprintf("%d-%s.%s.%s", port, instanceID, region, domain)
	return fmt.Sprintf("https://%s/cdp?access_token=%s", host, accessToken)
}

// addInstanceBrowserCommand registers `instance browser` with vnc subcommand under the given parent.
func addInstanceBrowserCommand(parent *cobra.Command) {
	browserCmd := &cobra.Command{
		Use:   "browser",
		Short: "Browser sandbox commands",
	}

	vncCmd := &cobra.Command{
		Use:   "vnc <instance-id>",
		Short: "Show VNC URL for browser instance",
		Long: `Show the VNC URL for accessing a browser sandbox instance.

Examples:
  ags instance browser vnc <id>
  ags instance browser vnc <id> --port 9000`,
		Args: cobra.ExactArgs(1),
		RunE: Wrap("instance.browser.vnc", browserVNCFn),
	}

	vncCmd.Flags().IntVarP(&browserPort, "port", "p", 9000, "VNC service port")
	vncCmd.Flags().BoolVar(&browserNoBrowser, "no-browser", false, "Don't auto-open browser")

	browserCmd.AddCommand(vncCmd)
	parent.AddCommand(browserCmd)
}
