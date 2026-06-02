package debug

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/TencentCloudAgentRuntime/ags-cli/internal/command"
	"github.com/TencentCloudAgentRuntime/ags-cli/internal/commands/internal/tooltags"
	"github.com/TencentCloudAgentRuntime/ags-cli/internal/output"
	ags "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/ags/v20250920"
)

const (
	envdMountName     = "envd"
	envdMountPath     = "/envd"
	envdImageRef      = "ccr.ccs.tencentyun.com/ags-image/envd:v0.5.14"
	envdImageSubPath  = "/usr/bin/envd"
	envdRegistryType  = "personal"
	defaultNameMaxLen = 50
)

// ControlPlane supplies the tool lookup and create call used by the debug workflow.
type ControlPlane interface {
	GetTool(ctx context.Context, toolID string) (*ags.SandboxTool, error)
	Call(ctx context.Context, action string, request map[string]any) (any, error)
}

// Module returns this package's command module.
func Module() command.Module {
	spec := command.Spec{
		ID:           "instance.debug",
		Path:         []string{"instance", "debug"},
		Use:          "debug <tool-id>",
		Short:        "Create a debug tool from an existing tool",
		Long:         "Create a debug tool by copying an existing tool and replacing its startup command with /envd.",
		SupportsJSON: true,
		Args: []command.ArgSpec{
			{Name: "tool-id", Required: true, Description: "Source sandbox tool ID."},
		},
		Flags: []command.FlagSpec{
			{Name: "debug-tool-name", Usage: "Name for the created debug tool", Type: command.FlagString, Workflow: true},
			{Name: "description", Usage: "Description for the created debug tool", Type: command.FlagString, Workflow: true},
			{Name: "client-token", Usage: "Client token for duplicate creation protection", Type: command.FlagString, Workflow: true},
		},
		Output: command.OutputSpec{
			DataType:    "DebugToolResult",
			Description: "Created debug tool details.",
			Effects:     []string{"create:tool"},
		},
	}
	return command.Module{
		Descriptor: command.Descriptor{
			Spec: spec,
			Groups: []command.GroupSpec{
				{
					Path:    []string{"instance"},
					Use:     "instance",
					Short:   "Manage sandbox instances",
					Long:    "Manage sandbox instances and related data-plane workflows.",
					Aliases: []string{"i"},
				},
			},
			Source: "workflow",
		},
		Build: func(deps command.Deps) (command.Runtime, error) {
			cp, ok := deps.ControlPlane.(ControlPlane)
			if !ok {
				return command.Runtime{}, fmt.Errorf("instance.debug requires command.Deps.ControlPlane implementing instance/debug.ControlPlane")
			}
			deps = deps.WithDefaults()
			return command.Runtime{
				Handler: command.HandlerFunc(func(ctx context.Context, req command.Request) (*command.Result, error) {
					return runDebug(ctx, req, deps, cp)
				}),
			}, nil
		},
	}
}

func runDebug(ctx context.Context, req command.Request, deps command.Deps, cp ControlPlane) (*command.Result, error) {
	sourceToolID := req.ArgValues["tool-id"]
	if sourceToolID == "" && len(req.Args) > 0 {
		sourceToolID = req.Args[0]
	}
	if strings.TrimSpace(sourceToolID) == "" {
		return nil, output.NewUsageError("MISSING_REQUIRED_ARG", "missing tool id", "Provide <tool-id>.")
	}

	sourceTool, err := cp.GetTool(ctx, sourceToolID)
	if err != nil {
		return nil, err
	}
	if err := validateDebugMountAvailable(sourceTool.StorageMounts); err != nil {
		return nil, err
	}
	if sourceTool.RoleArn == nil || strings.TrimSpace(*sourceTool.RoleArn) == "" {
		return nil, output.NewUsageError(
			"DEBUG_ROLE_ARN_REQUIRED",
			"source tool must have RoleArn to add the envd image mount",
			"Use a source tool with RoleArn configured because image storage mounts require it.",
		)
	}

	debugToolName := stringFlag(req, "debug-tool-name")
	if strings.TrimSpace(debugToolName) == "" {
		debugToolName = defaultDebugToolName(derefString(sourceTool.ToolName), sourceToolID, deps.Now())
	}
	description := stringFlag(req, "description")
	if strings.TrimSpace(description) == "" {
		description = fmt.Sprintf("Debug tool for %s (%s)", displayToolName(sourceTool, sourceToolID), sourceToolID)
	}

	addedMount := envdMount()
	createReq, err := buildCreateRequest(sourceTool, debugToolName, description, stringFlag(req, "client-token"), addedMount)
	if err != nil {
		return nil, err
	}
	resp, err := cp.Call(ctx, "CreateSandboxTool", createReq)
	if err != nil {
		return nil, err
	}

	toolID := responseToolID(resp)
	data := map[string]any{
		"SourceToolId":   sourceToolID,
		"SourceToolName": derefString(sourceTool.ToolName),
		"ToolId":         toolID,
		"ToolName":       debugToolName,
		"Command":        []string{envdMountPath},
		"AddedMount":     addedMount,
	}
	return &command.Result{
		Data:    data,
		Effects: []output.Effect{{Kind: "create", Resource: "tool", Id: toolID}},
		Text: func(w io.Writer) {
			fmt.Fprintf(w, "Debug tool created: %s\n", toolID)
			printKV(w, []kv{
				{key: "ID", value: toolID},
				{key: "Name", value: debugToolName},
				{key: "SourceToolID", value: sourceToolID},
				{key: "Command", value: envdMountPath},
				{key: "AddedMount", value: fmt.Sprintf("%s:%s -> %s", envdImageRef, envdImageSubPath, envdMountPath)},
			})
		},
	}, nil
}

func buildCreateRequest(sourceTool *ags.SandboxTool, toolName, description, clientToken string, addedMount map[string]any) (map[string]any, error) {
	customConfig, err := customConfigurationRequest(sourceTool.CustomConfiguration)
	if err != nil {
		return nil, err
	}
	storageMounts, err := storageMountsRequest(sourceTool.StorageMounts, addedMount)
	if err != nil {
		return nil, err
	}

	req := map[string]any{
		"ToolName":             toolName,
		"ToolType":             derefString(sourceTool.ToolType),
		"NetworkConfiguration": sourceTool.NetworkConfiguration,
		"Description":          description,
		"StorageMounts":        storageMounts,
		"CustomConfiguration":  customConfig,
	}
	if tags := tooltags.FilterInheritedTags(sourceTool.Tags); len(tags) > 0 {
		req["Tags"] = tags
	}
	if sourceTool.RoleArn != nil && *sourceTool.RoleArn != "" {
		req["RoleArn"] = *sourceTool.RoleArn
	}
	if sourceTool.LogConfiguration != nil {
		req["LogConfiguration"] = sourceTool.LogConfiguration
	}
	if sourceTool.Persistent != nil {
		req["Persistent"] = *sourceTool.Persistent
	}
	if sourceTool.DefaultTimeoutSeconds != nil {
		req["DefaultTimeout"] = fmt.Sprintf("%ds", *sourceTool.DefaultTimeoutSeconds)
	}
	if strings.TrimSpace(clientToken) != "" {
		req["ClientToken"] = clientToken
	}
	return req, nil
}

func customConfigurationRequest(source *ags.CustomConfigurationDetail) (map[string]any, error) {
	var custom map[string]any
	if source != nil {
		if err := jsonRoundTrip(source, &custom); err != nil {
			return nil, err
		}
	}
	if custom == nil {
		custom = map[string]any{}
	}
	delete(custom, "ImageDigest")
	custom["Command"] = []string{envdMountPath}
	custom["Args"] = []string{}
	return custom, nil
}

func storageMountsRequest(source []*ags.StorageMount, addedMount map[string]any) ([]map[string]any, error) {
	mounts := make([]map[string]any, 0, len(source)+1)
	for _, mount := range source {
		if mount == nil {
			continue
		}
		var item map[string]any
		if err := jsonRoundTrip(mount, &item); err != nil {
			return nil, err
		}
		if storageSource, ok := item["StorageSource"].(map[string]any); ok {
			if image, ok := storageSource["Image"].(map[string]any); ok {
				delete(image, "Digest")
			}
		}
		mounts = append(mounts, item)
	}
	mounts = append(mounts, addedMount)
	return mounts, nil
}

func validateDebugMountAvailable(mounts []*ags.StorageMount) error {
	for _, mount := range mounts {
		if mount == nil {
			continue
		}
		if strings.EqualFold(derefString(mount.Name), envdMountName) || derefString(mount.MountPath) == envdMountPath {
			return output.NewUsageError(
				"DEBUG_MOUNT_CONFLICT",
				"source tool already uses the debug mount name or path",
				"Choose a source tool that does not define mount name envd or mount path /envd.",
			)
		}
	}
	return nil
}

func envdMount() map[string]any {
	return map[string]any{
		"Name":      envdMountName,
		"MountPath": envdMountPath,
		"ReadOnly":  true,
		"StorageSource": map[string]any{
			"Image": map[string]any{
				"Reference":         envdImageRef,
				"ImageRegistryType": envdRegistryType,
				"SubPath":           envdImageSubPath,
			},
		},
	}
}

func defaultDebugToolName(sourceName, sourceToolID string, now time.Time) string {
	base := strings.TrimSpace(sourceName)
	if base == "" {
		base = strings.TrimSpace(sourceToolID)
	}
	if base == "" {
		base = "tool"
	}
	suffix := "-debug-" + now.UTC().Format("20060102150405")
	if len(base)+len(suffix) > defaultNameMaxLen {
		base = base[:max(1, defaultNameMaxLen-len(suffix))]
		base = strings.TrimRight(base, "-_")
		if base == "" {
			base = "tool"
		}
	}
	return base + suffix
}

func displayToolName(tool *ags.SandboxTool, fallbackID string) string {
	if name := derefString(tool.ToolName); name != "" {
		return name
	}
	return fallbackID
}

func responseToolID(resp any) string {
	switch value := resp.(type) {
	case *ags.CreateSandboxToolResponseParams:
		return derefString(value.ToolId)
	case map[string]any:
		return fmt.Sprint(value["ToolId"])
	default:
		return ""
	}
}

func jsonRoundTrip(src any, dst any) error {
	raw, err := json.Marshal(src)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, dst)
}

type kv struct {
	key   string
	value string
}

func printKV(w io.Writer, pairs []kv) {
	maxLen := 0
	for _, pair := range pairs {
		if len(pair.key) > maxLen {
			maxLen = len(pair.key)
		}
	}
	for _, pair := range pairs {
		if pair.value == "" {
			continue
		}
		fmt.Fprintf(w, "%-*s  %s\n", maxLen, pair.key+":", pair.value)
	}
}

func stringFlag(req command.Request, name string) string {
	flag, ok := req.Flags[name]
	if !ok {
		return ""
	}
	return flag.String
}

func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
