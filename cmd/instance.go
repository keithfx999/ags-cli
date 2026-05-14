package cmd

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/TencentCloudAgentRuntime/ags-cli/internal/client"
	"github.com/TencentCloudAgentRuntime/ags-cli/internal/config"
	"github.com/TencentCloudAgentRuntime/ags-cli/internal/output"
	"github.com/TencentCloudAgentRuntime/ags-cli/internal/pty"
	"github.com/TencentCloudAgentRuntime/ags-cli/internal/token"
	"github.com/spf13/cobra"
)

var (
	instanceToolName     string
	instanceToolID       string
	instanceTimeout      int
	instanceMountOptions []string
	instanceAuthMode     string
	instanceClientToken  string
	instanceRequest      string

	// list command flags
	instanceListTool   string
	instanceListStatus string
	instanceListOffset int
	instanceListLimit  int

	// login command flags
	instanceLoginUser string

	// delete command flags
	instanceDeleteIgnoreNotFound bool
)

// ---------------------------------------------------------------------------
// instance create
// ---------------------------------------------------------------------------

func instanceCreateFn(cmd *cobra.Command, args []string) (*CmdResult, error) {
	ctx := context.Background()

	if err := config.Validate(); err != nil {
		return nil, err
	}

	// --request is mutually exclusive with business flags
	if instanceRequest != "" {
		hasBusinessFlag := cmd.Flags().Changed("tool-name") || cmd.Flags().Changed("tool-id") ||
			cmd.Flags().Changed("timeout") || cmd.Flags().Changed("mount-option") ||
			cmd.Flags().Changed("auth-mode") || cmd.Flags().Changed("client-token")
		if hasBusinessFlag {
			return nil, output.NewUsageError("REQUEST_FLAG_CONFLICT",
				"--request cannot be used with --tool-name, --tool-id, --timeout, --mount-option, --auth-mode, or --client-token",
				"Use --request for the complete request body, or use individual flags.")
		}
		reqData, err := parseRequestFlag(instanceRequest)
		if err != nil {
			return nil, err
		}

		knownFields := map[string]bool{
			"ToolName": true, "ToolId": true, "TimeoutSeconds": true,
			"AuthMode": true, "ClientToken": true, "MountOptions": true,
		}
		for k := range reqData {
			if !knownFields[k] {
				return nil, output.NewUsageError("UNKNOWN_REQUEST_FIELD",
					fmt.Sprintf("unknown field in --request: %s", k),
					fmt.Sprintf("Allowed fields: %s. Use 'ags schema instance.create -o json' for the full schema.", joinKeys(knownFields)))
			}
		}

		opts := &client.CreateInstanceOptions{Timeout: 300}
		if v, ok := reqData["ToolName"].(string); ok {
			opts.ToolName = v
		}
		if v, ok := reqData["ToolId"].(string); ok {
			opts.ToolID = v
		}
		if v, ok := reqData["TimeoutSeconds"].(float64); ok {
			opts.Timeout = int(v)
		} else if reqData["TimeoutSeconds"] != nil {
			return nil, output.NewUsageError("INVALID_REQUEST_FIELD",
				"TimeoutSeconds must be an integer", "")
		}
		if v, ok := reqData["AuthMode"].(string); ok {
			opts.AuthMode = v
		}
		if v, ok := reqData["ClientToken"].(string); ok {
			opts.ClientToken = v
		}
		if rawMounts, ok := reqData["MountOptions"].([]any); ok {
			for _, rm := range rawMounts {
				mo, ok := rm.(map[string]any)
				if !ok {
					return nil, output.NewUsageError("INVALID_REQUEST_FIELD", "MountOptions must be an array of objects", "")
				}
				opt := client.MountOption{}
				if v, ok := mo["Name"].(string); ok {
					opt.Name = v
				}
				if v, ok := mo["Dst"].(string); ok {
					opt.MountPath = v
				}
				if v, ok := mo["Subpath"].(string); ok {
					opt.SubPath = v
				}
				opts.MountOptions = append(opts.MountOptions, opt)
			}
		} else if reqData["MountOptions"] != nil {
			return nil, output.NewUsageError("INVALID_REQUEST_FIELD", "MountOptions must be an array", "")
		}

		if opts.ToolName == "" && opts.ToolID == "" {
			return nil, output.NewUsageError("MISSING_REQUIRED_FIELD",
				"ToolName is required in --request",
				"Provide ToolName or ToolId in the request JSON.")
		}

		apiClient, err := newControlPlaneClient(config.GetBackend())
		if err != nil {
			return nil, err
		}
		return doCreateInstance(ctx, apiClient, opts)
	}

	if instanceToolName != "" && instanceToolID != "" {
		return nil, fmt.Errorf("cannot specify both --tool-name and --tool-id")
	}
	if instanceToolName == "" && instanceToolID == "" {
		return nil, fmt.Errorf("must specify either --tool-name/-t or --tool-id")
	}
	if instanceTimeout <= 0 {
		return nil, fmt.Errorf("--timeout must be greater than 0")
	}

	var mountOptions []client.MountOption
	for _, optStr := range instanceMountOptions {
		opt, err := client.ParseMountOption(optStr)
		if err != nil {
			return nil, fmt.Errorf("invalid --mount-option: %w", err)
		}
		mountOptions = append(mountOptions, *opt)
	}

	authMode, err := client.NormalizeAuthMode(instanceAuthMode)
	if err != nil {
		return nil, fmt.Errorf("invalid --auth-mode: %w", err)
	}

	apiClient, err := newControlPlaneClient(config.GetBackend())
	if err != nil {
		return nil, err
	}

	opts := &client.CreateInstanceOptions{
		ToolID:       instanceToolID,
		ToolName:     instanceToolName,
		Timeout:      instanceTimeout,
		MountOptions: mountOptions,
		AuthMode:     authMode,
		ClientToken:  instanceClientToken,
	}
	return doCreateInstance(ctx, apiClient, opts)
}

func joinKeys(m map[string]bool) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return strings.Join(keys, ", ")
}

func doCreateInstance(ctx context.Context, apiClient client.ControlPlaneClient, opts *client.CreateInstanceOptions) (*CmdResult, error) {

	instance, err := apiClient.CreateInstance(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to create instance: %w", err)
	}

	if err := cacheInstanceToken(ctx, apiClient, instance); err != nil {
		stderr("Warning: Failed to cache access token: %v\n", err)
	}

	data := &output.InstanceData{
		Id:       instance.ID,
		ToolId:   instance.ToolID,
		ToolName: instance.ToolName,
		Status:   instance.Status,
		CreatedAt: instance.CreatedAt,
	}
	if len(instance.MountOptions) > 0 {
		data.MountOptions = convertMountOptions(instance.MountOptions)
	}

	return OKWithEffects(data, func(w io.Writer) {
		fmt.Fprintf(ios.ErrOut, "Instance created: %s\n", instance.ID)
		kvs := []KeyValue{
			{Key: "ID", Value: instance.ID},
			{Key: "Tool", Value: instance.ToolName},
			{Key: "Status", Value: instance.Status},
			{Key: "Created", Value: instance.CreatedAt},
		}
		if len(instance.MountOptions) > 0 {
			kvs = append(kvs, KeyValue{Key: "MountOptions", Value: formatMountOptionsSummary(instance.MountOptions)})
		}
		printKV(w, kvs)
	}, output.Effect{Kind: "create", Resource: "instance", Id: instance.ID}), nil
}

// formatMountOptionsSummary formats mount options for display
func formatMountOptionsSummary(opts []client.MountOption) string {
	if len(opts) == 0 {
		return "-"
	}
	var parts []string
	for _, opt := range opts {
		parts = append(parts, opt.Name)
	}
	return strings.Join(parts, ", ")
}

// formatMountOptionsDetail formats mount options for detailed display
func formatMountOptionsDetail(opts []client.MountOption) string {
	if len(opts) == 0 {
		return ""
	}

	var lines []string
	for i, opt := range opts {
		lines = append(lines, fmt.Sprintf("\n  [%d] %s", i+1, opt.Name))
		lines = append(lines, fmt.Sprintf("      MountPath: %s", valueOrDefault(opt.MountPath, "(default)")))
		if opt.SubPath != "" {
			lines = append(lines, fmt.Sprintf("      SubPath:   %s", opt.SubPath))
		}
		readOnly := "(default)"
		if opt.ReadOnly != nil {
			readOnly = fmt.Sprintf("%t", *opt.ReadOnly)
		}
		lines = append(lines, fmt.Sprintf("      ReadOnly:  %s", readOnly))
	}
	return strings.Join(lines, "\n")
}

// valueOrDefault returns the value if non-empty, otherwise the default
func valueOrDefault(value, defaultValue string) string {
	if value == "" {
		return defaultValue
	}
	return value
}

// ---------------------------------------------------------------------------
// instance list
// ---------------------------------------------------------------------------

func instanceListFn(cmd *cobra.Command, args []string) (*CmdResult, error) {
	ctx := context.Background()

	if err := config.Validate(); err != nil {
		return nil, err
	}

	apiClient, err := newControlPlaneClient(config.GetBackend())
	if err != nil {
		return nil, fmt.Errorf("failed to create API client: %w", err)
	}

	opts := &client.ListInstancesOptions{
		ToolID: instanceListTool,
		Status: instanceListStatus,
		Offset: instanceListOffset,
		Limit:  instanceListLimit,
	}

	result, err := apiClient.ListInstances(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to list instances: %w", err)
	}

	items := make([]map[string]any, len(result.Instances))
	for i := range result.Instances {
		items[i] = toCanonicalInstanceData(&result.Instances[i])
	}
	data := map[string]any{
		"Items": items,
		"Pagination": map[string]any{
			"Offset":     instanceListOffset,
			"Limit":      instanceListLimit,
			"Total":      result.TotalCount,
			"NextCursor": nil,
		},
	}

	return OK(data, func(w io.Writer) {
		if len(result.Instances) == 0 {
			fmt.Fprintln(ios.ErrOut, "No instances found")
			return
		}

		headers := []string{"ID", "TOOL", "STATUS", "TIMEOUT", "EXPIRES", "MOUNTS", "CREATED"}
		rows := make([][]string, len(result.Instances))
		for i, inst := range result.Instances {
			timeout := "-"
			if inst.TimeoutSeconds != nil {
				timeout = formatTimeout(*inst.TimeoutSeconds)
			}
			expires := "-"
			if inst.ExpiresAt != "" {
				expires = formatTimeShort(inst.ExpiresAt)
			}
			mounts := formatMountOptionsSummary(inst.MountOptions)
			rows[i] = []string{
				inst.ID,
				inst.ToolName,
				inst.Status,
				timeout,
				expires,
				mounts,
				formatTimeShort(inst.CreatedAt),
			}
		}

		shown := len(result.Instances)
		total := result.TotalCount
		printTableWithPagination(w, headers, rows, shown, total)
	}), nil
}

// formatTimeout formats timeout seconds to human readable format
func formatTimeout(seconds uint64) string {
	if seconds >= 3600 && seconds%3600 == 0 {
		return fmt.Sprintf("%dh", seconds/3600)
	}
	if seconds >= 60 && seconds%60 == 0 {
		return fmt.Sprintf("%dm", seconds/60)
	}
	return fmt.Sprintf("%ds", seconds)
}

// formatTimeShort formats ISO8601 time to short format
func formatTimeShort(isoTime string) string {
	if isoTime == "" {
		return "-"
	}
	t, err := time.Parse(time.RFC3339, isoTime)
	if err != nil {
		return isoTime
	}
	return t.Local().Format("01-02 15:04")
}

// ---------------------------------------------------------------------------
// instance get
// ---------------------------------------------------------------------------

func instanceGetFn(cmd *cobra.Command, args []string) (*CmdResult, error) {
	ctx := context.Background()
	instanceID := args[0]

	if err := config.Validate(); err != nil {
		return nil, err
	}

	apiClient, err := newControlPlaneClient(config.GetBackend())
	if err != nil {
		return nil, fmt.Errorf("failed to create API client: %w", err)
	}

	instance, err := apiClient.GetInstance(ctx, instanceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get instance: %w", err)
	}

	data := toCanonicalInstanceData(instance)

	return OK(data, func(w io.Writer) {
		kvs := []KeyValue{
			{Key: "ID", Value: instance.ID},
			{Key: "ToolID", Value: instance.ToolID},
			{Key: "ToolName", Value: instance.ToolName},
			{Key: "Status", Value: instance.Status},
			{Key: "Created", Value: instance.CreatedAt},
		}
		if instance.UpdatedAt != "" {
			kvs = append(kvs, KeyValue{Key: "Updated", Value: instance.UpdatedAt})
		}
		if instance.TimeoutSeconds != nil {
			kvs = append(kvs, KeyValue{Key: "Timeout", Value: formatTimeout(*instance.TimeoutSeconds)})
		}
		if instance.ExpiresAt != "" {
			kvs = append(kvs, KeyValue{Key: "Expires", Value: instance.ExpiresAt})
		}
		if instance.StopReason != "" {
			kvs = append(kvs, KeyValue{Key: "StopReason", Value: instance.StopReason})
		}
		if len(instance.Endpoints) > 0 {
			kvs = append(kvs, KeyValue{Key: "Endpoints", Value: formatEndpoints(instance.Endpoints)})
		}
		mountOptsStr := formatMountOptionsDetail(instance.MountOptions)
		if mountOptsStr != "" {
			kvs = append(kvs, KeyValue{Key: "MountOptions", Value: mountOptsStr})
		}
		printKV(w, kvs)
	}), nil
}

// formatEndpoints formats endpoints for display
func formatEndpoints(endpoints []client.Endpoint) string {
	if len(endpoints) == 0 {
		return "-"
	}
	var parts []string
	for _, ep := range endpoints {
		parts = append(parts, fmt.Sprintf("%s (%s)", ep.URL, ep.Scope))
	}
	return strings.Join(parts, "\n")
}

// ---------------------------------------------------------------------------
// instance delete
// ---------------------------------------------------------------------------

func instanceDeleteFn(cmd *cobra.Command, args []string) (*CmdResult, error) {
	ctx := context.Background()

	if err := config.Validate(); err != nil {
		return nil, err
	}

	apiClient, err := newControlPlaneClient(config.GetBackend())
	if err != nil {
		return nil, fmt.Errorf("failed to create API client: %w", err)
	}

	tokenCache, cacheErr := token.NewCache()
	if cacheErr != nil {
		stderr("Warning: Failed to initialize token cache: %v\n", cacheErr)
	}

	var deleted []string
	var failed []string
	var warnings []string
	var alreadyAbsent []string

	for _, instanceID := range args {
		if err := apiClient.DeleteInstance(ctx, instanceID); err != nil {
			if instanceDeleteIgnoreNotFound && isNotFoundError(err) {
				stderr("Instance %s not found (ignored)\n", instanceID)
				alreadyAbsent = append(alreadyAbsent, instanceID)
				warnings = append(warnings, fmt.Sprintf("Instance %s: AlreadyAbsent", instanceID))
				continue
			}
			stderr("Failed to delete instance %s: %v\n", instanceID, err)
			failed = append(failed, instanceID)
			warnings = append(warnings, fmt.Sprintf("Failed to delete %s: %v", instanceID, err))
		} else {
			if tokenCache != nil {
				_ = tokenCache.Delete(instanceID)
			}
			deleted = append(deleted, instanceID)
		}
	}

	data := map[string]any{
		"Deleted":       len(deleted),
		"Failed":        len(failed),
		"FailedIds":     failed,
		"AlreadyAbsent": alreadyAbsent,
	}

	if len(failed) > 0 {
		return PartialResult(data, warnings, func(w io.Writer) {
			for _, id := range deleted {
				fmt.Fprintf(ios.ErrOut, "Instance deleted: %s\n", id)
			}
			fmt.Fprintf(ios.ErrOut, "failed to delete %d instance(s)\n", len(failed))
		}), nil
	}

	return OK(data, func(w io.Writer) {
		for _, id := range deleted {
			fmt.Fprintf(ios.ErrOut, "Instance deleted: %s\n", id)
		}
	}), nil
}

// isNotFoundError checks whether an error indicates a "not found" condition.
func isNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "not found") || strings.Contains(msg, "404")
}

// ---------------------------------------------------------------------------
// instance login (PTY-only, no JSON support)
// ---------------------------------------------------------------------------

func instanceLoginFn(cmd *cobra.Command, args []string) error {
	if err := requireTTY(); err != nil {
		return err
	}
	if nonInteractive {
		return exitError(2, fmt.Errorf("instance login requires interactive mode"))
	}

	ctx := context.Background()
	instanceID := args[0]

	if err := config.Validate(); err != nil {
		return err
	}

	apiClient, err := newControlPlaneClient(config.GetBackend())
	if err != nil {
		return fmt.Errorf("failed to create API client: %w", err)
	}

	stderr("Connecting to instance %s...\n", instanceID)
	instance, err := apiClient.GetInstance(ctx, instanceID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return fmt.Errorf("instance %s not found. Please check the instance ID and try again", instanceID)
		}
		if strings.Contains(err.Error(), "permission") || strings.Contains(err.Error(), "access") {
			return fmt.Errorf("access denied to instance %s. Please check your permissions", instanceID)
		}
		return fmt.Errorf("failed to get instance %s: %w", instanceID, err)
	}

	status := strings.ToUpper(instance.Status)
	if status != "RUNNING" {
		switch status {
		case "CREATING", "STARTING":
			return fmt.Errorf("instance %s is still being created. Please wait for it to finish and try again", instanceID)
		case "STOPPED", "STOPPING":
			return fmt.Errorf("instance %s is stopped. Please start it first using 'ags instance create' or contact support", instanceID)
		case "ERROR", "FAILED":
			return fmt.Errorf("instance %s is in error state. Please contact support or create a new instance", instanceID)
		default:
			return fmt.Errorf("instance %s is not running (status: %s). Please wait for it to be ready", instanceID, instance.Status)
		}
	}

	var accessToken string
	if instance.Secure {
		accessToken, err = GetCachedTokenOrAcquire(ctx, instanceID)
		if err != nil {
			return fmt.Errorf("failed to get access token: %w", err)
		}
	}

	cfg := config.Get()
	domain := cfg.DataPlaneRegionDomain()

	stderr("Starting PTY session in instance %s...\n", instanceID)
	session := pty.NewSession(accessToken, domain)
	return session.Connect(ctx, instanceID, resolveUser(instanceLoginUser))
}

func init() {
	addInstanceCommand(rootCmd)
}

// addInstanceCommand adds the instance command and ALL subcommands to a parent command.
func addInstanceCommand(parent *cobra.Command) {
	cmd := &cobra.Command{
		Use:     "instance",
		Aliases: []string{"i"},
		Short:   "Manage sandbox instances",
		Long:    `Manage sandbox instances. Instances are running sandboxes created from tools.`,
	}

	// --- create ---
	createCmd := &cobra.Command{
		Use:     "create",
		Aliases: []string{"c"},
		Short:   "Create a new instance",
		Long: `Create a new sandbox instance from a tool template.

Use --tool-name/-t for tool name (e2b/cloud backend) or --tool-id for tool ID (cloud backend only).

Mount option format (--mount-option):
  name=<name>[,dst=<target-path>][,subpath=<sub-path>][,readonly]

Auth mode (--auth-mode):
  DEFAULT  Use backend default (currently TOKEN)
  TOKEN    All ports require X-Access-Token
  PUBLIC   envd management port (49983) requires token; other ports open
  NONE     No authentication on any port

Examples:
  ags instance create -t code-interpreter-v1
  ags instance create --tool-name code-interpreter-v1
  ags instance create --tool-id sdt-xxxx
  ags instance create -t my-tool --timeout 600
  ags instance create --tool-id sdt-xxxx --mount-option "name=data,dst=/workspace,subpath=user-123"
  ags instance create -t my-tool --auth-mode NONE`,
	}
	createCmd.Flags().StringVarP(&instanceToolName, "tool-name", "t", "", "Tool name (e2b/cloud backend)")
	createCmd.Flags().StringVar(&instanceToolID, "tool-id", "", "Tool ID (cloud backend only)")
	createCmd.Flags().IntVar(&instanceTimeout, "timeout", 300, "Instance timeout in seconds")
	createCmd.Flags().StringArrayVar(&instanceMountOptions, "mount-option", nil, "Mount option to override tool storage config\n"+client.FormatMountOptionHelp())
	createCmd.Flags().StringVar(&instanceAuthMode, "auth-mode", client.AuthModeDefault, "Auth mode: DEFAULT, TOKEN, NONE, PUBLIC")
	createCmd.Flags().StringVar(&instanceClientToken, "client-token", "", "Client token for duplicate creation protection")
	createCmd.Flags().StringVar(&instanceRequest, "request", "", "Complete request body as JSON, @file, or - for stdin")
	createCmd.RunE = Wrap("instance.create", instanceCreateFn)
	cmd.AddCommand(createCmd)

	// --- list ---
	listCmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List instances",
		Long: `List sandbox instances with optional filters.

Examples:
  ags instance list
  ags instance list --tool-id sdt-xxx
  ags instance list --status RUNNING
  ags instance list --offset 0 --limit 50`,
	}
	listCmd.Flags().StringVar(&instanceListTool, "tool-id", "", "Filter by tool ID")
	listCmd.Flags().StringVarP(&instanceListStatus, "status", "s", "", "Filter by status (STARTING, RUNNING, FAILED, STOPPING, STOPPED)")
	listCmd.Flags().IntVar(&instanceListOffset, "offset", 0, "Pagination offset")
	listCmd.Flags().IntVar(&instanceListLimit, "limit", 20, "Pagination limit (max 100)")
	listCmd.RunE = Wrap("instance.list", instanceListFn)
	cmd.AddCommand(listCmd)

	// --- get ---
	getCmd := &cobra.Command{
		Use:   "get <instance-id>",
		Short: "Get instance details",
		Long:  `Get detailed information about a specific instance.`,
		Args:  cobra.ExactArgs(1),
	}
	getCmd.RunE = Wrap("instance.get", instanceGetFn)
	cmd.AddCommand(getCmd)

	// --- delete ---
	deleteCmd := &cobra.Command{
		Use:     "delete <instance-id> [instance-id...]",
		Aliases: []string{"rm", "del"},
		Short:   "Delete instances",
		Long:    `Delete one or more sandbox instances. This operation executes immediately and does not prompt for confirmation.`,
		Args:    cobra.MinimumNArgs(1),
	}
	deleteCmd.Flags().BoolVar(&instanceDeleteIgnoreNotFound, "ignore-not-found", false, "Treat 'not found' errors as success")
	deleteCmd.RunE = Wrap("instance.delete", instanceDeleteFn)
	cmd.AddCommand(deleteCmd)

	// --- login (PTY-only) ---
	loginCmd := &cobra.Command{
		Use:   "login <instance-id>",
		Short: "Login to instance via terminal",
		Long: `Login to a sandbox instance interactively using a native PTY session.

Connects a terminal session directly in your current console.

Examples:
  ags instance login <id>
  ags instance login <id> --user root`,
		Args: cobra.ExactArgs(1),
	}
	loginCmd.Flags().StringVar(&instanceLoginUser, "user", "", "User to run terminal as (default: \"user\")")
	loginCmd.RunE = WrapNoJSON(instanceLoginFn)
	cmd.AddCommand(loginCmd)

	// --- code (subgroup with run) ---
	addInstanceCodeRunCommand(cmd)

	// --- exec ---
	addInstanceExecCommand(cmd)

	// --- file (upload / download) ---
	addFileCommand(cmd)

	// --- browser (vnc) ---
	addInstanceBrowserCommand(cmd)

	// --- proxy ---
	addInstanceProxyCommand(cmd)

	// --- mobile (connect / disconnect / list / adb / tunnel) ---
	addInstanceMobileCommand(cmd)

	parent.AddCommand(cmd)
}

// cacheInstanceToken caches the access token for an instance.
func cacheInstanceToken(ctx context.Context, apiClient client.ControlPlaneClient, instance *client.Instance) error {
	if !instance.Secure {
		return nil
	}

	tokenCache, err := token.NewCache()
	if err != nil {
		return fmt.Errorf("failed to create token cache: %w", err)
	}

	var accessToken string

	if instance.AccessToken != "" {
		accessToken = instance.AccessToken
	} else {
		accessToken, err = apiClient.AcquireToken(ctx, instance.ID)
		if err != nil {
			return fmt.Errorf("failed to acquire token: %w", err)
		}
	}

	if accessToken == "" {
		return fmt.Errorf("no access token available")
	}

	if err := tokenCache.Set(instance.ID, accessToken); err != nil {
		return fmt.Errorf("failed to save token: %w", err)
	}

	return nil
}
