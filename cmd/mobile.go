package cmd

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/TencentCloudAgentRuntime/ags-cli/internal/adbtunnel"
	"github.com/TencentCloudAgentRuntime/ags-cli/internal/config"
	"github.com/TencentCloudAgentRuntime/ags-cli/internal/output"
	"github.com/TencentCloudAgentRuntime/ags-cli/internal/tunnelstore"
)

var (
	daemonFlag bool
	portFlag   int

	disconnectAll bool
)

// readyMessage is the JSON protocol message sent by tunnel --daemon to stdout.
type readyMessage struct {
	Status  string `json:"status"`
	Port    int    `json:"port,omitempty"`
	PID     int    `json:"pid,omitempty"`
	Message string `json:"message,omitempty"`
}

// runMobileTunnel runs a foreground ADB tunnel (hidden, used internally by connect).
func runMobileTunnel(_ *cobra.Command, args []string) error {
	sandboxID := args[0]

	if portFlag < 0 || portFlag > 65535 {
		return exitError(2, fmt.Errorf("--port must be between 0 and 65535"))
	}

	if err := config.Validate(); err != nil {
		return exitError(1, err)
	}

	tokenProvider := func() (string, error) {
		return acquireInstanceToken(context.Background(), sandboxID)
	}

	cfg := config.Get()
	domain := cfg.DataPlaneRegionDomain()

	listenAddr := "127.0.0.1:0"
	if portFlag > 0 {
		listenAddr = fmt.Sprintf("127.0.0.1:%d", portFlag)
	}

	tunnel, err := adbtunnel.New(adbtunnel.TunnelOptions{
		InstanceID:    sandboxID,
		Domain:        domain,
		TokenProvider: tokenProvider,
		ListenAddress: listenAddr,
		Insecure:      false,
	})
	if err != nil {
		if daemonFlag {
			errMsg := readyMessage{Status: "error", Message: fmt.Sprintf("failed to create tunnel: %v", err)}
			_ = json.NewEncoder(os.Stdout).Encode(errMsg)
		}
		return exitError(1, fmt.Errorf("failed to create tunnel: %w", err))
	}

	addr, err := tunnel.Start()
	if err != nil {
		if daemonFlag {
			errMsg := readyMessage{Status: "error", Message: fmt.Sprintf("failed to start tunnel: %v", err)}
			_ = json.NewEncoder(os.Stdout).Encode(errMsg)
		}
		return exitError(3, fmt.Errorf("failed to start tunnel: %w", err))
	}

	if err := tunnel.Probe(); err != nil {
		tunnel.Stop()
		if daemonFlag {
			errMsg := readyMessage{Status: "error", Message: err.Error()}
			_ = json.NewEncoder(os.Stdout).Encode(errMsg)
		}
		return exitError(2, fmt.Errorf("upstream probe failed: %w", err))
	}

	_, portStr, _ := strings.Cut(addr, ":")

	if daemonFlag {
		msg := readyMessage{
			Status: "ready",
			Port:   mustAtoi(portStr),
			PID:    os.Getpid(),
		}
		if err := json.NewEncoder(os.Stdout).Encode(msg); err != nil {
			tunnel.Stop()
			return exitError(1, fmt.Errorf("failed to write ready message: %w", err))
		}
	} else {
		fmt.Fprintf(ios.Out, "[Ready] ADB Tunnel established at %s\n", addr)
		fmt.Fprintln(ios.Out, "[Ready] Press Ctrl+C to disconnect.")
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	<-ctx.Done()

	if !daemonFlag {
		fmt.Fprintln(ios.Out, "\n[INFO] Shutting down ADB tunnel...")
	}

	shutdownDone := make(chan struct{})
	go func() {
		tunnel.Stop()
		close(shutdownDone)
	}()

	select {
	case <-shutdownDone:
	case <-time.After(5 * time.Second):
		if !daemonFlag {
			fmt.Fprintln(ios.Out, "[WARN] Graceful shutdown timed out. Forcing exit.")
		}
	}

	return nil
}

func mobileConnectFn(_ *cobra.Command, args []string) (*CmdResult, error) {
	sandboxID := args[0]

	if err := config.Validate(); err != nil {
		return nil, err
	}

	adbPath, err := requireAdb()
	if err != nil {
		return nil, err
	}

	store, err := tunnelstore.NewStore()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize tunnel store: %w", err)
	}

	if oldEntry, ok, _ := store.Get(sandboxID); ok {
		oldAddr := fmt.Sprintf("127.0.0.1:%d", oldEntry.Port)
		_ = runAdbCommand(adbPath, "disconnect", oldAddr)
	}
	if err := store.Cleanup(sandboxID); err != nil {
		stderr("Warning: failed to cleanup existing tunnel: %v\n", err)
	}

	selfPath, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("failed to get executable path: %w", err)
	}

	tunnelArgs := []string{"instance", "mobile", "tunnel", sandboxID, "--daemon", "--port=0"}
	if backend != "" {
		tunnelArgs = append(tunnelArgs, "--backend", backend)
	}
	if region != "" {
		tunnelArgs = append(tunnelArgs, "--region", region)
	}
	if domain != "" {
		tunnelArgs = append(tunnelArgs, "--domain", domain)
	}
	if internal {
		tunnelArgs = append(tunnelArgs, "--internal")
	}

	cmd := exec.Command(selfPath, tunnelArgs...)
	if homeDir, homeErr := os.UserHomeDir(); homeErr == nil {
		logDir := filepath.Join(homeDir, ".ags")
		_ = os.MkdirAll(logDir, 0700)
		logFile, logErr := os.OpenFile(
			filepath.Join(logDir, fmt.Sprintf("tunnel-%s.log", sandboxID)),
			os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600,
		)
		if logErr == nil {
			cmd.Stderr = logFile
		}
	}

	cmd.Env = os.Environ()
	if apiKey != "" {
		cmd.Env = append(cmd.Env, "AGS_API_KEY="+apiKey)
	}
	if secretID != "" {
		cmd.Env = append(cmd.Env, "TENCENTCLOUD_SECRET_ID="+secretID)
	}
	if secretKey != "" {
		cmd.Env = append(cmd.Env, "TENCENTCLOUD_SECRET_KEY="+secretKey)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start tunnel process: %w", err)
	}

	go func() { _ = cmd.Wait() }()

	readyCh := make(chan readyMessage, 1)
	errCh := make(chan error, 1)

	go func() {
		scanner := bufio.NewScanner(stdout)
		if scanner.Scan() {
			var msg readyMessage
			if err := json.Unmarshal(scanner.Bytes(), &msg); err != nil {
				errCh <- fmt.Errorf("failed to parse tunnel ready message: %w", err)
				return
			}
			readyCh <- msg
		} else {
			if err := scanner.Err(); err != nil {
				errCh <- fmt.Errorf("failed to read tunnel output: %w", err)
			} else {
				errCh <- fmt.Errorf("tunnel process exited without ready message")
			}
		}
	}()

	timer := time.NewTimer(30 * time.Second)
	defer timer.Stop()

	var ready readyMessage
	select {
	case ready = <-readyCh:
		if ready.Status != "ready" || ready.Port == 0 {
			_ = cmd.Process.Kill()
			return nil, fmt.Errorf("tunnel reported error: %s", ready.Message)
		}
	case err := <-errCh:
		_ = cmd.Process.Kill()
		return nil, err
	case <-timer.C:
		_ = cmd.Process.Kill()
		return nil, fmt.Errorf("tunnel did not become ready within 30s")
	}

	if err := store.Save(sandboxID, tunnelstore.TunnelEntry{
		PID:       ready.PID,
		Port:      ready.Port,
		CreatedAt: time.Now(),
		ExePath:   selfPath,
	}); err != nil {
		stderr("Warning: failed to save tunnel mapping: %v\n", err)
	}

	adbAddr := fmt.Sprintf("127.0.0.1:%d", ready.Port)
	adbConnectErr := adbConnectWithRetry(adbPath, adbAddr, 3)

	var logPath string
	if homeDir, homeErr := os.UserHomeDir(); homeErr == nil {
		logPath = filepath.Join(homeDir, ".ags", fmt.Sprintf("tunnel-%s.log", sandboxID))
	}

	data := map[string]any{
		"InstanceId": sandboxID,
		"AdbAddress": adbAddr,
		"Port":       ready.Port,
		"Pid":        ready.PID,
	}
	if logPath != "" {
		data["LogPath"] = logPath
	}

	return OK(data, func(w io.Writer) {
		if adbConnectErr != nil {
			fmt.Fprintf(ios.ErrOut, "tunnel ready for %s at %s (adb connect failed: %v; use 'adb connect %s' manually)\n",
				sandboxID, adbAddr, adbConnectErr, adbAddr)
		} else {
			fmt.Fprintf(ios.ErrOut, "connected to %s (%s)\n", sandboxID, adbAddr)
		}
		if logPath != "" {
			fmt.Fprintf(ios.ErrOut, "tunnel log: %s\n", logPath)
		}
	}), nil
}

func mobileDisconnectFn(_ *cobra.Command, args []string) (*CmdResult, error) {
	if disconnectAll && len(args) > 0 {
		return nil, fmt.Errorf("--all cannot be used with an instance-id")
	}

	store, err := tunnelstore.NewStore()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize tunnel store: %w", err)
	}

	if disconnectAll {
		entries, err := store.List()
		if err != nil {
			return nil, fmt.Errorf("failed to list tunnels: %w", err)
		}

		if len(entries) == 0 {
			data := map[string]any{"Disconnected": []string{}, "Count": 0}
			return OK(data, func(w io.Writer) {
				fmt.Fprintln(ios.ErrOut, "no active connections")
			}), nil
		}

		adbPath, _ := requireAdb()
		var disconnected []string

		for id, entry := range entries {
			if adbPath != "" {
				adbAddr := fmt.Sprintf("127.0.0.1:%d", entry.Port)
				_ = runAdbCommand(adbPath, "disconnect", adbAddr)
			}
			disconnected = append(disconnected, id)
		}

		if err := store.CleanupAll(); err != nil {
			return nil, fmt.Errorf("failed to cleanup tunnels: %w", err)
		}

		data := map[string]any{"Disconnected": disconnected, "Count": len(disconnected)}
		return OK(data, func(w io.Writer) {
			for _, id := range disconnected {
				fmt.Fprintf(ios.ErrOut, "disconnected from %s\n", id)
			}
		}), nil
	}

	if len(args) == 0 {
		return nil, fmt.Errorf("must specify instance-id or use --all")
	}

	sandboxID := args[0]

	entry, ok, err := store.Get(sandboxID)
	if err != nil {
		return nil, fmt.Errorf("failed to read tunnel store: %w", err)
	}
	if !ok {
		return nil, fmt.Errorf("no active tunnel for %s", sandboxID)
	}

	if adbPath, err := requireAdb(); err == nil {
		adbAddr := fmt.Sprintf("127.0.0.1:%d", entry.Port)
		_ = runAdbCommand(adbPath, "disconnect", adbAddr)
	}

	if err := store.Cleanup(sandboxID); err != nil {
		return nil, fmt.Errorf("failed to cleanup tunnel: %w", err)
	}

	data := map[string]any{"InstanceId": sandboxID}
	return OK(data, func(w io.Writer) {
		fmt.Fprintf(ios.ErrOut, "disconnected from %s\n", sandboxID)
	}), nil
}

func mobileListFn(_ *cobra.Command, _ []string) (*CmdResult, error) {
	store, err := tunnelstore.NewStore()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize tunnel store: %w", err)
	}

	entries, err := store.List()
	if err != nil {
		return nil, fmt.Errorf("failed to list tunnels: %w", err)
	}

	items := make([]map[string]any, 0, len(entries))
	for id, entry := range entries {
		addr := fmt.Sprintf("127.0.0.1:%d", entry.Port)
		items = append(items, map[string]any{
			"InstanceId": id,
			"AdbAddress": addr,
			"Port":       entry.Port,
			"Pid":        entry.PID,
			"CreatedAt":  entry.CreatedAt.Format(time.RFC3339),
			"Status":     "connected",
		})
	}
	data := map[string]any{"Items": items, "Total": len(items)}

	return OK(data, func(w io.Writer) {
		if len(entries) == 0 {
			fmt.Fprintln(ios.ErrOut, "No active connections.")
			fmt.Fprintln(ios.ErrOut, "Use 'ags instance mobile connect <instance-id>' to connect.")
			return
		}
		headers := []string{"INSTANCE", "ADB ADDRESS", "STATUS"}
		rows := make([][]string, 0, len(entries))
		for id, entry := range entries {
			addr := fmt.Sprintf("127.0.0.1:%d", entry.Port)
			rows = append(rows, []string{id, addr, "connected"})
		}
		printTable(w, headers, rows)
	}), nil
}

func mobileAdbFn(cobraCmd *cobra.Command, args []string) (*CmdResult, error) {
	if len(args) == 1 && (args[0] == "--help" || args[0] == "-h") {
		return nil, cobraCmd.Help()
	}

	if len(args) < 3 || args[1] != "--" {
		return nil, output.NewUsageError("MISSING_SEPARATOR",
			"usage: ags instance mobile adb <instance-id> -- <adb-args...>\nUse '--' immediately after <instance-id> to separate the adb command.",
			"")
	}

	sandboxID := args[0]
	adbArgs := args[2:]

	adbPath, err := requireAdb()
	if err != nil {
		return nil, err
	}

	store, err := tunnelstore.NewStore()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize tunnel store: %w", err)
	}

	entry, ok, err := store.Get(sandboxID)
	if err != nil {
		return nil, fmt.Errorf("failed to read tunnel store: %w", err)
	}
	if !ok {
		return nil, fmt.Errorf("no active tunnel for %s; run 'ags instance mobile connect %s' first", sandboxID, sandboxID)
	}

	adbAddr := fmt.Sprintf("127.0.0.1:%d", entry.Port)
	fullArgs := append([]string{"-s", adbAddr}, adbArgs...)

	if isJSON() {
		adbCmd := exec.Command(adbPath, fullArgs...)
		var stdoutBuf, stderrBuf bytes.Buffer
		adbCmd.Stdout = &stdoutBuf
		adbCmd.Stderr = &stderrBuf
		exitCode := 0
		if err := adbCmd.Run(); err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				exitCode = exitErr.ExitCode()
			} else {
				return nil, err
			}
		}
		data := map[string]any{
			"Stdout": stdoutBuf.String(), "Stderr": stderrBuf.String(), "ExitCode": exitCode,
		}
		if exitCode != 0 {
			// AC12: remote details in Data only; AC14: exit code passthrough
			return &CmdResult{Data: data, ExitCode: exitCode}, nil
		}
		return OK(data, nil), nil
	}

	adbCmd := exec.Command(adbPath, fullArgs...)
	adbCmd.Stdin = os.Stdin
	adbCmd.Stdout = os.Stdout
	adbCmd.Stderr = os.Stderr
	if err := adbCmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return StreamDone(exitErr.ExitCode()), nil
		}
		return nil, err
	}
	return StreamDone(0), nil
}

// requireAdb finds the adb binary, checking ADB_PATH env var first, then PATH.
func requireAdb() (string, error) {
	if p := os.Getenv("ADB_PATH"); p != "" {
		info, err := os.Lstat(p)
		if err != nil {
			return "", fmt.Errorf("ADB_PATH=%q not accessible: %w", p, err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("ADB_PATH=%q is a symlink (not allowed for security)", p)
		}
		if !info.Mode().IsRegular() {
			return "", fmt.Errorf("ADB_PATH=%q is not a regular file", p)
		}
		if runtime.GOOS == "windows" {
			if !strings.HasSuffix(strings.ToLower(p), ".exe") {
				return "", fmt.Errorf("ADB_PATH=%q does not have .exe extension (required on Windows)", p)
			}
		} else {
			if info.Mode()&0111 == 0 {
				return "", fmt.Errorf("ADB_PATH=%q is not executable", p)
			}
		}
		return p, nil
	}

	path, err := exec.LookPath("adb")
	if err != nil {
		return "", fmt.Errorf("adb not found in PATH; install Android SDK Platform-Tools or set ADB_PATH")
	}
	return path, nil
}

// adbConnectWithRetry runs 'adb connect <addr>' with bounded retries.
func adbConnectWithRetry(adbPath, addr string, maxRetries int) error {
	var lastErr error
	for i := 0; i < maxRetries; i++ {
		if i > 0 {
			time.Sleep(500 * time.Millisecond)
		}
		out, err := exec.Command(adbPath, "connect", addr).CombinedOutput()
		if err != nil {
			lastErr = err
			continue
		}
		outStr := strings.TrimSpace(string(out))
		fmt.Fprintln(ios.ErrOut, outStr)
		if strings.Contains(outStr, "connected") {
			time.Sleep(2 * time.Second)
			return nil
		}
		lastErr = fmt.Errorf("adb connect: %s", outStr)
	}
	return fmt.Errorf("adb connect failed after %d attempts: %w", maxRetries, lastErr)
}

// runAdbCommand executes an adb command and returns any error.
func runAdbCommand(adbPath string, args ...string) error {
	cmd := exec.Command(adbPath, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// mustAtoi converts a string to int, returning 0 on error.
func mustAtoi(s string) int {
	var n int
	_, _ = fmt.Sscanf(s, "%d", &n)
	return n
}

// exitError creates a CLIError with the given exit code. This is the only
// way business functions signal non-zero exit to the Wrap framework.
func exitError(code int, err error) error {
	msg := "command failed"
	if err != nil {
		msg = err.Error()
	}
	return &output.CLIError{
		Failure:  &output.Failure{Code: "CLI_ERROR", Kind: kindFromExitCode(code), Message: msg},
		ExitCode: code,
	}
}

func kindFromExitCode(code int) string {
	switch code {
	case output.ExitUsage:
		return output.KindUsage
	case output.ExitNotFound:
		return output.KindNotFound
	case output.ExitAuthOrPermission:
		return output.KindAuthOrPermission
	case output.ExitConflict:
		return output.KindConflict
	case output.ExitRateLimit:
		return output.KindRateLimit
	case output.ExitTimeout:
		return output.KindTimeout
	case output.ExitNetwork:
		return output.KindNetwork
	case output.ExitBackendUnsupported:
		return output.KindBackendUnsupported
	case output.ExitPartialSuccess:
		return output.KindPartialSuccess
	case output.ExitRemoteExecFailed:
		return output.KindRemoteExecFailed
	default:
		return output.KindGenericError
	}
}

// addInstanceMobileCommand registers `instance mobile` with connect/disconnect/list/adb/tunnel under the given parent.
func addInstanceMobileCommand(parent *cobra.Command) {
	mobileCmd := &cobra.Command{
		Use:   "mobile",
		Short: "Mobile sandbox ADB commands",
		Long: `Manage ADB connections to mobile sandbox instances.

Examples:
  ags instance mobile connect <instance-id>
  ags instance mobile list
  ags instance mobile adb <instance-id> -- shell ls /sdcard
  ags instance mobile disconnect <instance-id>`,
	}

	tunnelCmd := &cobra.Command{
		Use:    "tunnel <instance-id>",
		Short:  "Run ADB tunnel in foreground (used internally by connect)",
		Args:   cobra.ExactArgs(1),
		RunE:   WrapNoJSON(runMobileTunnel),
		Hidden: true,
	}
	tunnelCmd.Flags().BoolVar(&daemonFlag, "daemon", false, "Run in daemon mode (used by connect)")
	tunnelCmd.Flags().IntVar(&portFlag, "port", 0, "Local port to listen on (0 = auto-assign)")

	connectCmd := &cobra.Command{
		Use:   "connect <instance-id>",
		Short: "Connect to mobile instance (background tunnel + adb connect)",
		Args:  cobra.ExactArgs(1),
		RunE:  Wrap("instance.mobile.connect", mobileConnectFn),
	}

	disconnectCmd := &cobra.Command{
		Use:   "disconnect [instance-id]",
		Short: "Disconnect from mobile instance",
		Args:  cobra.MaximumNArgs(1),
		RunE:  Wrap("instance.mobile.disconnect", mobileDisconnectFn),
	}
	disconnectCmd.Flags().BoolVar(&disconnectAll, "all", false, "Disconnect all active connections")

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List active mobile instance connections",
		Args:  cobra.NoArgs,
		RunE:  Wrap("instance.mobile.list", mobileListFn),
	}

	adbCmd := &cobra.Command{
		Use:   "adb <instance-id> -- <adb-args...>",
		Short: "Execute adb command on mobile instance by ID",
		Long: `Execute an adb command targeting a specific mobile instance.

Use '--' to separate from adb arguments.

Examples:
  ags instance mobile adb <id> -- shell ls /sdcard
  ags instance mobile adb <id> -- install app.apk
  ags instance mobile adb <id> -- logcat`,
		Args:               cobra.MinimumNArgs(1),
		RunE:               Wrap("instance.mobile.adb", mobileAdbFn),
		DisableFlagParsing: true,
	}

	mobileCmd.AddCommand(tunnelCmd, connectCmd, disconnectCmd, listCmd, adbCmd)
	parent.AddCommand(mobileCmd)
}
