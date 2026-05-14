package cmd

import (
	"context"
	"fmt"
	"net"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/TencentCloudAgentRuntime/ags-cli/internal/config"
	"github.com/TencentCloudAgentRuntime/ags-cli/internal/proxy"
)

// parsePortSpec parses a port specification in the format [local_port:]<remote_port>.
func parsePortSpec(spec string) (int, int, error) {
	parts := strings.SplitN(spec, ":", 2)

	if len(parts) == 1 {
		port, err := strconv.Atoi(parts[0])
		if err != nil {
			return 0, 0, fmt.Errorf("invalid port: %q", parts[0])
		}
		if port <= 0 || port > 65535 {
			return 0, 0, fmt.Errorf("port must be between 1 and 65535, got %d", port)
		}
		return port, port, nil
	}

	localPort, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, fmt.Errorf("invalid local port: %q", parts[0])
	}
	if localPort <= 0 || localPort > 65535 {
		return 0, 0, fmt.Errorf("local port must be between 1 and 65535, got %d", localPort)
	}

	remotePort, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, 0, fmt.Errorf("invalid remote port: %q", parts[1])
	}
	if remotePort <= 0 || remotePort > 65535 {
		return 0, 0, fmt.Errorf("remote port must be between 1 and 65535, got %d", remotePort)
	}

	return localPort, remotePort, nil
}

func proxyFn(cmd *cobra.Command, args []string) error {
	sandboxID := args[0]
	portSpec := args[1]

	if err := config.Validate(); err != nil {
		return err
	}

	localPort, remotePort, err := parsePortSpec(portSpec)
	if err != nil {
		return fmt.Errorf("invalid port specification: %w", err)
	}

	address, err := cmd.Flags().GetString("address")
	if err != nil {
		return fmt.Errorf("failed to get address flag: %w", err)
	}

	if address != "localhost" {
		if net.ParseIP(address) == nil {
			return fmt.Errorf("invalid address %q: must be a valid IP address or \"localhost\"", address)
		}
	}

	if address != "127.0.0.1" && address != "localhost" && address != "::1" {
		stderr("Warning: binding to %s exposes the proxy (and the sandbox access token) to the network.\n", address)
	}
	verbose, err := cmd.Flags().GetBool("verbose")
	if err != nil {
		return fmt.Errorf("failed to get verbose flag: %w", err)
	}

	token, err := acquireInstanceToken(context.Background(), sandboxID)
	if err != nil {
		return fmt.Errorf("failed to acquire access token: %w", err)
	}

	cfg := config.Get()
	domain := cfg.DataPlaneRegionDomain()

	listenAddr := net.JoinHostPort(address, strconv.Itoa(localPort))

	p, err := proxy.New(proxy.Options{
		InstanceID:    sandboxID,
		Domain:        domain,
		RemotePort:    remotePort,
		Token:         token,
		ListenAddress: listenAddr,
		Insecure:      false,
		Verbose:       verbose,
	})
	if err != nil {
		return fmt.Errorf("failed to create proxy: %w", err)
	}

	addr, err := p.Start()
	if err != nil {
		return fmt.Errorf("failed to start proxy: %w", err)
	}

	fmt.Fprintf(ios.Out, "Forwarding from %s -> %d\n", addr, remotePort)
	fmt.Fprintf(ios.Out, "  Local:  http://%s\n", addr)
	fmt.Fprintf(ios.Out, "  Remote: https://%d-%s.%s\n", remotePort, sandboxID, domain)
	fmt.Fprintln(ios.Out, "\nPress Ctrl+C to stop.")

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	<-ctx.Done()

	fmt.Fprintln(ios.Out, "\nStopping proxy...")
	p.Stop()

	return nil
}

// addInstanceProxyCommand registers `instance proxy` under the given parent.
func addInstanceProxyCommand(parent *cobra.Command) {
	proxyCmd := &cobra.Command{
		Use:   "proxy <instance-id> [local_port:]<remote_port>",
		Short: "Forward an instance port to localhost",
		Long: `Forward a remote instance port to a local address, similar to kubectl port-forward.

Port Syntax:
  <remote_port>                Forward remote port to the same local port
  <local_port>:<remote_port>   Forward remote port to a specific local port

Examples:
  ags instance proxy <id> 8080
  ags instance proxy <id> 3000:8080
  ags instance proxy <id> 3000:8080 --address 0.0.0.0`,
		Args: cobra.ExactArgs(2),
		RunE: WrapNoJSON(proxyFn),
	}

	proxyCmd.Flags().String("address", "127.0.0.1", "Local address to bind to")
	proxyCmd.Flags().Bool("verbose", false, "Enable verbose request logging")

	parent.AddCommand(proxyCmd)
}
