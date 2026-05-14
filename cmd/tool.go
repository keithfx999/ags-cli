package cmd

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/TencentCloudAgentRuntime/ags-cli/internal/client"
	"github.com/TencentCloudAgentRuntime/ags-cli/internal/config"
	"github.com/TencentCloudAgentRuntime/ags-cli/internal/output"
	"github.com/spf13/cobra"
)

var (
	toolCreateName              string
	toolCreateType              string
	toolCreateDescription       string
	toolCreateTimeout           string
	toolCreateNetworkMode       string
	toolCreateTags              []string
	toolCreateRoleArn           string
	toolCreateMounts            []string
	toolCreateVPCSubnets        []string
	toolCreateVPCSecurityGroups []string
	toolCreateClientToken       string
	toolCreateRequest           string

	toolUpdateDescription string
	toolUpdateNetworkMode string
	toolUpdateTags        []string
	toolUpdateClearTags   bool
	toolUpdateRequest     string

	toolListIDs              []string
	toolListStatus           string
	toolListType             string
	toolListCreatedSince     string
	toolListCreatedSinceTime string
	toolListTags             []string
	toolListOffset           int
	toolListLimit            int
)

func requireCloudBackend() error {
	if config.GetBackend() != "cloud" {
		return output.NewBackendUnsupportedError(
			"this command is only supported with cloud backend",
			"Use --backend cloud with TENCENTCLOUD_SECRET_ID and TENCENTCLOUD_SECRET_KEY.")
	}
	return nil
}

func toolListFn(cmd *cobra.Command, args []string) (*CmdResult, error) {
	if err := requireCloudBackend(); err != nil {
		return nil, err
	}
	ctx := context.Background()

	if toolListCreatedSince != "" && toolListCreatedSinceTime != "" {
		return nil, fmt.Errorf("--created-since and --created-since-time cannot be used together")
	}

	tags := make(map[string]string)
	for _, tag := range toolListTags {
		parts := strings.SplitN(tag, "=", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid tag format: %s (expected key=value)", tag)
		}
		tags[parts[0]] = parts[1]
	}

	apiClient, err := newControlPlaneClient(config.GetBackend())
	if err != nil {
		return nil, err
	}

	result, err := apiClient.ListTools(ctx, &client.ListToolsOptions{
		ToolIDs:          toolListIDs,
		Status:           toolListStatus,
		ToolType:         toolListType,
		CreatedSince:     toolListCreatedSince,
		CreatedSinceTime: toolListCreatedSinceTime,
		Tags:             tags,
		Offset:           toolListOffset,
		Limit:            toolListLimit,
	})
	if err != nil {
		return nil, err
	}

	items := make([]map[string]any, len(result.Tools))
	for i := range result.Tools {
		items[i] = toCanonicalToolData(&result.Tools[i])
	}
	data := map[string]any{
		"Items": items,
		"Pagination": map[string]any{
			"Offset": toolListOffset, "Limit": toolListLimit,
			"Total": result.TotalCount, "NextCursor": nil,
		},
	}

	return OK(data, func(w io.Writer) {
		if len(result.Tools) == 0 {
			fmt.Fprintln(ios.ErrOut, "No tools found")
			return
		}
		headers := []string{"ID", "NAME", "TYPE", "STATUS", "NETWORK", "DESCRIPTION", "TAGS", "CREATED"}
		rows := make([][]string, len(result.Tools))
		for i, t := range result.Tools {
			var tagStrs []string
			for k, v := range t.Tags {
				tagStrs = append(tagStrs, fmt.Sprintf("%s=%s", k, v))
			}
			nm := t.NetworkMode
			if nm == "" {
				nm = "-"
			}
			status := t.Status
			if status == "" {
				status = "-"
			}
			rows[i] = []string{t.ID, t.Name, t.Type, status, nm, output.TruncateString(t.Description, 40), strings.Join(tagStrs, ","), formatShortTime(t.CreatedAt)}
		}
		printTableWithPagination(w, headers, rows, len(rows), result.TotalCount)
	}), nil
}

func toolGetFn(cmd *cobra.Command, args []string) (*CmdResult, error) {
	if err := requireCloudBackend(); err != nil {
		return nil, err
	}
	ctx := context.Background()
	toolID := args[0]

	apiClient, err := newControlPlaneClient(config.GetBackend())
	if err != nil {
		return nil, err
	}

	tool, err := apiClient.GetTool(ctx, toolID)
	if err != nil {
		return nil, err
	}

	data := toCanonicalToolData(tool)

	return OK(data, func(w io.Writer) {
		var tagStrs []string
		for k, v := range tool.Tags {
			tagStrs = append(tagStrs, fmt.Sprintf("%s=%s", k, v))
		}
		tagsStr := strings.Join(tagStrs, ", ")
		if tagsStr == "" {
			tagsStr = "-"
		}
		nm := tool.NetworkMode
		if nm == "" {
			nm = "-"
		}
		kvs := []KeyValue{
			{Key: "ID", Value: tool.ID}, {Key: "Name", Value: tool.Name},
			{Key: "Type", Value: tool.Type}, {Key: "NetworkMode", Value: nm},
		}
		if tool.NetworkMode == "VPC" && tool.VPCConfig != nil {
			s, sg := client.FormatVPCConfigSummary(tool.VPCConfig)
			kvs = append(kvs, KeyValue{Key: "VPCSubnets", Value: s}, KeyValue{Key: "VPCSecGroups", Value: sg})
		}
		kvs = append(kvs, KeyValue{Key: "Description", Value: tool.Description}, KeyValue{Key: "Tags", Value: tagsStr}, KeyValue{Key: "Created", Value: formatShortTime(tool.CreatedAt)})
		if tool.RoleArn != "" {
			kvs = append(kvs, KeyValue{Key: "RoleArn", Value: tool.RoleArn})
		}
		if m := formatStorageMountsDetail(tool.StorageMounts); m != "" {
			kvs = append(kvs, KeyValue{Key: "StorageMounts", Value: m})
		}
		printKV(w, kvs)
	}), nil
}

func toolCreateFn(cmd *cobra.Command, args []string) (*CmdResult, error) {
	if err := requireCloudBackend(); err != nil {
		return nil, err
	}
	ctx := context.Background()

	if toolCreateRequest != "" {
		hasBusinessFlag := cmd.Flags().Changed("name") || cmd.Flags().Changed("type") ||
			cmd.Flags().Changed("description") || cmd.Flags().Changed("timeout") ||
			cmd.Flags().Changed("network") || cmd.Flags().Changed("tag") ||
			cmd.Flags().Changed("role-arn") || cmd.Flags().Changed("mount") ||
			cmd.Flags().Changed("client-token") || cmd.Flags().Changed("vpc-subnet") ||
			cmd.Flags().Changed("vpc-sg")
		if hasBusinessFlag {
			return nil, output.NewUsageError("REQUEST_FLAG_CONFLICT",
				"--request cannot be used with other tool create flags",
				"Use --request for the complete request body, or use individual flags.")
		}
		return toolCreateFromRequest(ctx)
	}

	if toolCreateName == "" {
		return nil, fmt.Errorf("tool name is required (-n/--name)")
	}
	if toolCreateType == "" {
		return nil, fmt.Errorf("tool type is required (-t/--type)")
	}

	if toolCreateNetworkMode != "" {
		valid := map[string]bool{"PUBLIC": true, "VPC": true, "SANDBOX": true, "INTERNAL_SERVICE": true}
		if !valid[toolCreateNetworkMode] {
			return nil, fmt.Errorf("invalid network mode: %s", toolCreateNetworkMode)
		}
	}
	if toolCreateNetworkMode == "VPC" {
		if len(toolCreateVPCSubnets) == 0 {
			return nil, fmt.Errorf("--vpc-subnet is required when --network=VPC")
		}
		if len(toolCreateVPCSecurityGroups) == 0 {
			return nil, fmt.Errorf("--vpc-sg is required when --network=VPC")
		}
	}

	tags := make(map[string]string)
	for _, tag := range toolCreateTags {
		parts := strings.SplitN(tag, "=", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid tag format: %s (expected key=value)", tag)
		}
		tags[parts[0]] = parts[1]
	}

	var storageMounts []client.StorageMount
	for _, ms := range toolCreateMounts {
		m, err := client.ParseStorageMount(ms)
		if err != nil {
			return nil, fmt.Errorf("invalid --mount: %w", err)
		}
		storageMounts = append(storageMounts, *m)
	}
	if len(storageMounts) > 0 && toolCreateRoleArn == "" {
		return nil, fmt.Errorf("--role-arn is required when --mount is specified")
	}

	var vpcConfig *client.VPCConfig
	if toolCreateNetworkMode == "VPC" {
		vpcConfig = &client.VPCConfig{SubnetIds: toolCreateVPCSubnets, SecurityGroupIds: toolCreateVPCSecurityGroups}
	}

	apiClient, err := newControlPlaneClient(config.GetBackend())
	if err != nil {
		return nil, err
	}

	tool, err := apiClient.CreateTool(ctx, &client.CreateToolOptions{
		Name: toolCreateName, Type: toolCreateType, Description: toolCreateDescription,
		DefaultTimeout: toolCreateTimeout, NetworkMode: toolCreateNetworkMode,
		VPCConfig: vpcConfig, Tags: tags, RoleArn: toolCreateRoleArn, StorageMounts: storageMounts,
		ClientToken: toolCreateClientToken,
	})
	if err != nil {
		return nil, err
	}

	data := map[string]any{
		"Id": tool.ID, "Name": tool.Name, "Type": tool.Type,
		"NetworkMode": tool.NetworkMode, "Description": tool.Description,
	}

	return OKWithEffects(data, func(w io.Writer) {
		fmt.Fprintf(ios.ErrOut, "Tool created: %s\n", tool.ID)
		printKV(w, []KeyValue{
			{Key: "ID", Value: tool.ID}, {Key: "Name", Value: tool.Name},
			{Key: "Type", Value: tool.Type}, {Key: "NetworkMode", Value: tool.NetworkMode},
			{Key: "Description", Value: tool.Description},
		})
	}, output.Effect{Kind: "create", Resource: "tool", Id: tool.ID}), nil
}

func toolCreateFromRequest(ctx context.Context) (*CmdResult, error) {
	reqData, err := parseRequestFlag(toolCreateRequest)
	if err != nil {
		return nil, err
	}
	known := map[string]bool{
		"Name": true, "Type": true, "Description": true, "NetworkMode": true,
		"Tags": true, "StorageMounts": true, "VpcConfig": true, "ClientToken": true,
		"DefaultTimeout": true, "RoleArn": true,
	}
	for k := range reqData {
		if !known[k] {
			return nil, output.NewUsageError("UNKNOWN_REQUEST_FIELD",
				fmt.Sprintf("unknown field in --request: %s", k), "")
		}
	}
	toolOpts := &client.CreateToolOptions{}
	if v, ok := reqData["Name"].(string); ok {
		toolOpts.Name = v
	}
	if v, ok := reqData["Type"].(string); ok {
		toolOpts.Type = v
	}
	if v, ok := reqData["Description"].(string); ok {
		toolOpts.Description = v
	}
	if v, ok := reqData["NetworkMode"].(string); ok {
		toolOpts.NetworkMode = v
	}
	if v, ok := reqData["ClientToken"].(string); ok {
		toolOpts.ClientToken = v
	}
	if v, ok := reqData["DefaultTimeout"].(string); ok {
		toolOpts.DefaultTimeout = v
	}
	if v, ok := reqData["RoleArn"].(string); ok {
		toolOpts.RoleArn = v
	}
	if rawTags, ok := reqData["Tags"].(map[string]any); ok {
		tags := make(map[string]string, len(rawTags))
		for k, v := range rawTags {
			if s, ok := v.(string); ok {
				tags[k] = s
			}
		}
		toolOpts.Tags = tags
	}
	if rawVpc, ok := reqData["VpcConfig"].(map[string]any); ok {
		vpc := &client.VPCConfig{}
		if subs, ok := rawVpc["SubnetIds"].([]any); ok {
			for _, s := range subs {
				if str, ok := s.(string); ok {
					vpc.SubnetIds = append(vpc.SubnetIds, str)
				}
			}
		}
		if sgs, ok := rawVpc["SecurityGroupIds"].([]any); ok {
			for _, s := range sgs {
				if str, ok := s.(string); ok {
					vpc.SecurityGroupIds = append(vpc.SecurityGroupIds, str)
				}
			}
		}
		toolOpts.VPCConfig = vpc
	}
	if rawMounts, ok := reqData["StorageMounts"].([]any); ok {
		for _, rm := range rawMounts {
			mo, ok := rm.(map[string]any)
			if !ok {
				continue
			}
			sm := client.StorageMount{}
			if v, ok := mo["Name"].(string); ok {
				sm.Name = v
			}
			if v, ok := mo["MountPath"].(string); ok {
				sm.MountPath = v
			}
			if v, ok := mo["ReadOnly"].(bool); ok {
				sm.ReadOnly = v
			}
			toolOpts.StorageMounts = append(toolOpts.StorageMounts, sm)
		}
	}

	if toolOpts.Name == "" {
		return nil, output.NewUsageError("MISSING_REQUIRED_FIELD", "Name is required in --request", "")
	}
	if toolOpts.Type == "" {
		return nil, output.NewUsageError("MISSING_REQUIRED_FIELD", "Type is required in --request", "")
	}

	apiClient, err := newControlPlaneClient(config.GetBackend())
	if err != nil {
		return nil, err
	}
	tool, err := apiClient.CreateTool(ctx, toolOpts)
	if err != nil {
		return nil, err
	}
	data := map[string]any{
		"Id": tool.ID, "Name": tool.Name, "Type": tool.Type,
		"NetworkMode": tool.NetworkMode, "Description": tool.Description,
	}
	return OKWithEffects(data, func(w io.Writer) {
		stderr("Tool created: %s\n", tool.ID)
		printKV(w, []KeyValue{{Key: "ID", Value: tool.ID}, {Key: "Name", Value: tool.Name}})
	}, output.Effect{Kind: "create", Resource: "tool", Id: tool.ID}), nil
}

func toolUpdateFn(cmd *cobra.Command, args []string) (*CmdResult, error) {
	if err := requireCloudBackend(); err != nil {
		return nil, err
	}
	ctx := context.Background()
	toolID := args[0]

	if toolUpdateRequest != "" {
		hasBusinessFlag := cmd.Flags().Changed("description") || cmd.Flags().Changed("network") ||
			cmd.Flags().Changed("tag") || cmd.Flags().Changed("clear-tags")
		if hasBusinessFlag {
			return nil, output.NewUsageError("REQUEST_FLAG_CONFLICT",
				"--request cannot be used with --description, --network, --tag, or --clear-tags",
				"Use --request for the complete request body, or use individual flags.")
		}
		return toolUpdateFromRequest(ctx, toolID)
	}

	descriptionSet := cmd.Flags().Changed("description")
	networkSet := cmd.Flags().Changed("network")
	tagsSet := cmd.Flags().Changed("tag")

	if !descriptionSet && !networkSet && !tagsSet && !toolUpdateClearTags {
		return nil, fmt.Errorf("at least one of --description, --network, --tag, --clear-tags, or --request must be specified")
	}

	opts := &client.UpdateToolOptions{ToolID: toolID}
	if descriptionSet {
		opts.Description = &toolUpdateDescription
	}
	if networkSet {
		opts.NetworkMode = &toolUpdateNetworkMode
	}
	if toolUpdateClearTags {
		opts.Tags = make(map[string]string)
	} else if tagsSet {
		tags := make(map[string]string)
		for _, tag := range toolUpdateTags {
			parts := strings.SplitN(tag, "=", 2)
			if len(parts) != 2 {
				return nil, fmt.Errorf("invalid tag format: %s (expected key=value)", tag)
			}
			tags[parts[0]] = parts[1]
		}
		opts.Tags = tags
	}

	apiClient, err := newControlPlaneClient(config.GetBackend())
	if err != nil {
		return nil, err
	}
	if err := apiClient.UpdateTool(ctx, opts); err != nil {
		return nil, err
	}

	return OK(map[string]any{"Id": toolID}, func(w io.Writer) {
		fmt.Fprintf(ios.ErrOut, "Tool updated: %s\n", toolID)
	}), nil
}

func toolUpdateFromRequest(ctx context.Context, toolID string) (*CmdResult, error) {
	reqData, err := parseRequestFlag(toolUpdateRequest)
	if err != nil {
		return nil, err
	}
	known := map[string]bool{
		"Description": true, "NetworkMode": true, "Tags": true,
	}
	for k := range reqData {
		if !known[k] {
			return nil, output.NewUsageError("UNKNOWN_REQUEST_FIELD",
				fmt.Sprintf("unknown field in --request: %s", k),
				fmt.Sprintf("Allowed fields: Description, NetworkMode, Tags. Use 'ags schema tool.update -o json' for the full schema."))
		}
	}

	opts := &client.UpdateToolOptions{ToolID: toolID}
	if v, ok := reqData["Description"].(string); ok {
		opts.Description = &v
	}
	if v, ok := reqData["NetworkMode"].(string); ok {
		opts.NetworkMode = &v
	}
	if rawTags, ok := reqData["Tags"].(map[string]any); ok {
		tags := make(map[string]string, len(rawTags))
		for k, v := range rawTags {
			if s, ok := v.(string); ok {
				tags[k] = s
			}
		}
		opts.Tags = tags
	}

	apiClient, err := newControlPlaneClient(config.GetBackend())
	if err != nil {
		return nil, err
	}
	if err := apiClient.UpdateTool(ctx, opts); err != nil {
		return nil, err
	}

	return OK(map[string]any{"Id": toolID}, func(w io.Writer) {
		stderr("Tool updated: %s\n", toolID)
	}), nil
}

func toolDeleteFn(cmd *cobra.Command, args []string) (*CmdResult, error) {
	if err := requireCloudBackend(); err != nil {
		return nil, err
	}
	ctx := context.Background()

	apiClient, err := newControlPlaneClient(config.GetBackend())
	if err != nil {
		return nil, err
	}

	var failed []string
	var warnings []string
	for _, toolID := range args {
		if err := apiClient.DeleteTool(ctx, toolID); err != nil {
			stderr("Warning: failed to delete tool %s: %v\n", toolID, err)
			failed = append(failed, toolID)
			warnings = append(warnings, fmt.Sprintf("failed to delete %s: %v", toolID, err))
		} else {
			stderr("Tool deleted: %s\n", toolID)
		}
	}

	data := map[string]any{"Deleted": len(args) - len(failed), "Failed": len(failed)}
	if len(failed) > 0 {
		data["FailedIds"] = failed
		return PartialResult(data, warnings, func(w io.Writer) {}), nil
	}
	return OK(data, func(w io.Writer) {}), nil
}

func formatStorageMountsDetail(mounts []client.StorageMount) string {
	if len(mounts) == 0 {
		return ""
	}
	var lines []string
	for i, m := range mounts {
		lines = append(lines, fmt.Sprintf("\n  [%d] %s", i+1, m.Name))
		if m.StorageSource != nil && m.StorageSource.Cos != nil {
			lines = append(lines, fmt.Sprintf("      Bucket: %s", m.StorageSource.Cos.BucketName))
			lines = append(lines, fmt.Sprintf("      Path:   %s", m.StorageSource.Cos.BucketPath))
		}
		lines = append(lines, fmt.Sprintf("      Mount:  %s", m.MountPath))
		lines = append(lines, fmt.Sprintf("      RO:     %t", m.ReadOnly))
	}
	return strings.Join(lines, "\n")
}

func formatShortTime(isoTime string) string {
	if isoTime == "" {
		return "-"
	}
	t, err := fmt.Sscanf(isoTime, "%s", new(string))
	_ = t
	if err != nil || len(isoTime) < 16 {
		return isoTime
	}
	return isoTime[5:16]
}

func init() {
	addToolCommand(rootCmd)
}

func addToolCommand(parent *cobra.Command) {
	cmd := &cobra.Command{
		Use:     "tool",
		Aliases: []string{"t"},
		Short:   "Manage sandbox tools",
		Long:    `Manage sandbox tools (templates). Tools define the type and capabilities of sandbox instances.`,
	}

	createCmd := &cobra.Command{Use: "create", Short: "Create a new sandbox tool", Args: cobra.NoArgs}
	createCmd.RunE = Wrap("tool.create", toolCreateFn)
	createCmd.Flags().StringVarP(&toolCreateName, "name", "n", "", "Tool name (required)")
	createCmd.Flags().StringVarP(&toolCreateType, "type", "t", "", "Tool type (required)")
	createCmd.Flags().StringVarP(&toolCreateDescription, "description", "d", "", "Tool description")
	createCmd.Flags().StringVar(&toolCreateTimeout, "timeout", "", "Default timeout (e.g., 5m, 300s, 1h)")
	createCmd.Flags().StringVar(&toolCreateNetworkMode, "network", "", "Network mode: PUBLIC, VPC, SANDBOX, INTERNAL_SERVICE")
	createCmd.Flags().StringArrayVar(&toolCreateVPCSubnets, "vpc-subnet", nil, "VPC subnet ID")
	createCmd.Flags().StringArrayVar(&toolCreateVPCSecurityGroups, "vpc-sg", nil, "Security group ID")
	createCmd.Flags().StringArrayVar(&toolCreateTags, "tag", nil, "Tags (key=value)")
	createCmd.Flags().StringVar(&toolCreateRoleArn, "role-arn", "", "Role ARN for COS access")
	createCmd.Flags().StringArrayVar(&toolCreateMounts, "mount", nil, "Storage mount config\n"+client.FormatStorageMountHelp())
	createCmd.Flags().StringVar(&toolCreateClientToken, "client-token", "", "Client token for duplicate creation protection")
	createCmd.Flags().StringVar(&toolCreateRequest, "request", "", "Complete request body as JSON, @file, or - for stdin")
	cmd.AddCommand(createCmd)

	listCmd := &cobra.Command{Use: "list", Aliases: []string{"ls"}, Short: "List available tools"}
	listCmd.RunE = Wrap("tool.list", toolListFn)
	listCmd.Flags().StringArrayVar(&toolListIDs, "id", nil, "Specific tool IDs")
	listCmd.Flags().StringVar(&toolListStatus, "status", "", "Filter by status")
	listCmd.Flags().StringVar(&toolListType, "type", "", "Filter by type")
	listCmd.Flags().StringVar(&toolListCreatedSince, "created-since", "", "Relative time filter")
	listCmd.Flags().StringVar(&toolListCreatedSinceTime, "created-since-time", "", "Absolute time filter (RFC3339)")
	listCmd.Flags().StringArrayVar(&toolListTags, "tag", nil, "Filter by tag (key=value)")
	listCmd.Flags().IntVar(&toolListOffset, "offset", 0, "Pagination offset")
	listCmd.Flags().IntVar(&toolListLimit, "limit", 20, "Pagination limit")
	cmd.AddCommand(listCmd)

	getCmd := &cobra.Command{Use: "get <tool-id>", Short: "Get tool details", Args: cobra.ExactArgs(1)}
	getCmd.RunE = Wrap("tool.get", toolGetFn)
	cmd.AddCommand(getCmd)

	updateCmd := &cobra.Command{Use: "update <tool-id>", Short: "Update a sandbox tool", Args: cobra.ExactArgs(1)}
	updateCmd.RunE = Wrap("tool.update", toolUpdateFn)
	updateCmd.Flags().StringVarP(&toolUpdateDescription, "description", "d", "", "Tool description")
	updateCmd.Flags().StringVar(&toolUpdateNetworkMode, "network", "", "Network mode")
	updateCmd.Flags().StringArrayVar(&toolUpdateTags, "tag", nil, "Tags (key=value)")
	updateCmd.Flags().BoolVar(&toolUpdateClearTags, "clear-tags", false, "Clear all tags")
	updateCmd.Flags().StringVar(&toolUpdateRequest, "request", "", "Complete request body as JSON, @file, or - for stdin")
	cmd.AddCommand(updateCmd)

	deleteCmd := &cobra.Command{Use: "delete <tool-id> [tool-id...]", Aliases: []string{"rm", "del"}, Short: "Delete sandbox tools", Args: cobra.MinimumNArgs(1)}
	deleteCmd.RunE = Wrap("tool.delete", toolDeleteFn)
	cmd.AddCommand(deleteCmd)

	parent.AddCommand(cmd)
}
