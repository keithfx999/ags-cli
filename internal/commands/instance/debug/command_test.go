package debug

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/TencentCloudAgentRuntime/ags-cli/internal/command"
	"github.com/TencentCloudAgentRuntime/ags-cli/internal/output"
	ags "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/ags/v20250920"
)

type fakeControlPlane struct {
	tool    *ags.SandboxTool
	getID   string
	action  string
	request map[string]any
	resp    any
	err     error
}

func (f *fakeControlPlane) GetTool(_ context.Context, toolID string) (*ags.SandboxTool, error) {
	f.getID = toolID
	if f.err != nil {
		return nil, f.err
	}
	return f.tool, nil
}

func (f *fakeControlPlane) Call(_ context.Context, action string, request map[string]any) (any, error) {
	f.action = action
	f.request = request
	if f.err != nil {
		return nil, f.err
	}
	if f.resp != nil {
		return f.resp, nil
	}
	id := "sdt-debug"
	return &ags.CreateSandboxToolResponseParams{ToolId: &id}, nil
}

func TestModuleBuildsDebugToolRequest(t *testing.T) {
	cp := &fakeControlPlane{tool: sourceTool()}
	runtime, err := Module().Build(command.Deps{
		ControlPlane: cp,
		Now:          func() time.Time { return time.Date(2026, 6, 1, 10, 11, 12, 0, time.UTC) },
	})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}

	result, err := runtime.Handler.Run(context.Background(), command.Request{
		ArgValues: map[string]string{"tool-id": "sdt-source"},
		Flags: map[string]command.FlagValue{
			"client-token": {Name: "client-token", Type: command.FlagString, String: "token-1", Changed: true},
		},
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if cp.getID != "sdt-source" {
		t.Fatalf("GetTool id = %q, want sdt-source", cp.getID)
	}
	if cp.action != "CreateSandboxTool" {
		t.Fatalf("action = %q, want CreateSandboxTool", cp.action)
	}
	if cp.request["ToolName"] != "source-debug-20260601101112" {
		t.Fatalf("ToolName = %#v", cp.request["ToolName"])
	}
	if cp.request["ToolType"] != "custom" || cp.request["DefaultTimeout"] != "300s" || cp.request["ClientToken"] != "token-1" {
		t.Fatalf("request = %#v", cp.request)
	}
	custom := cp.request["CustomConfiguration"].(map[string]any)
	if _, ok := custom["ImageDigest"]; ok {
		t.Fatalf("CustomConfiguration leaked ImageDigest: %#v", custom)
	}
	if got := stringSlice(custom["Command"]); len(got) != 1 || got[0] != "/envd" {
		t.Fatalf("Command = %#v", custom["Command"])
	}
	if got := stringSlice(custom["Args"]); len(got) != 0 {
		t.Fatalf("Args = %#v, want empty", custom["Args"])
	}

	mounts := cp.request["StorageMounts"].([]map[string]any)
	if len(mounts) != 2 {
		t.Fatalf("StorageMounts len = %d, want 2: %#v", len(mounts), mounts)
	}
	if mounts[0]["Name"] != "workspace" || mounts[0]["MountPath"] != "/workspace" {
		t.Fatalf("copied mount = %#v", mounts[0])
	}
	added := mounts[1]
	if added["Name"] != envdMountName || added["MountPath"] != envdMountPath || added["ReadOnly"] != true {
		t.Fatalf("added mount = %#v", added)
	}
	image := added["StorageSource"].(map[string]any)["Image"].(map[string]any)
	if image["Reference"] != envdImageRef || image["ImageRegistryType"] != envdRegistryType || image["SubPath"] != envdImageSubPath {
		t.Fatalf("image mount = %#v", image)
	}

	data := result.Data.(map[string]any)
	if data["ToolId"] != "sdt-debug" || data["SourceToolId"] != "sdt-source" {
		t.Fatalf("data = %#v", data)
	}
	if len(result.Effects) != 1 || result.Effects[0].Kind != "create" || result.Effects[0].Resource != "tool" {
		t.Fatalf("effects = %#v", result.Effects)
	}
	var text bytes.Buffer
	result.Text(&text)
	if !strings.Contains(text.String(), "Debug tool created: sdt-debug") || !strings.Contains(text.String(), envdImageRef) {
		t.Fatalf("text = %q", text.String())
	}
}

func TestModuleHonorsExplicitNameAndDescription(t *testing.T) {
	cp := &fakeControlPlane{tool: sourceTool()}
	runtime, err := Module().Build(command.Deps{ControlPlane: cp})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	_, err = runtime.Handler.Run(context.Background(), command.Request{
		ArgValues: map[string]string{"tool-id": "sdt-source"},
		Flags: map[string]command.FlagValue{
			"debug-tool-name": {Name: "debug-tool-name", Type: command.FlagString, String: "custom-debug", Changed: true},
			"description":     {Name: "description", Type: command.FlagString, String: "custom description", Changed: true},
		},
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if cp.request["ToolName"] != "custom-debug" || cp.request["Description"] != "custom description" {
		t.Fatalf("request = %#v", cp.request)
	}
}

func TestModuleRejectsMissingToolID(t *testing.T) {
	cp := &fakeControlPlane{tool: sourceTool()}
	runtime, err := Module().Build(command.Deps{ControlPlane: cp})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	_, err = runtime.Handler.Run(context.Background(), command.Request{})
	if cliErr, ok := err.(*output.CLIError); !ok || cliErr.Failure.Code != "MISSING_REQUIRED_ARG" {
		t.Fatalf("error = %#v, want MISSING_REQUIRED_ARG", err)
	}
}

func TestModuleRejectsDebugMountConflict(t *testing.T) {
	for _, tc := range []struct {
		name  string
		mount *ags.StorageMount
	}{
		{name: "name", mount: &ags.StorageMount{Name: strPtr(envdMountName), MountPath: strPtr("/other")}},
		{name: "path", mount: &ags.StorageMount{Name: strPtr("other"), MountPath: strPtr(envdMountPath)}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tool := sourceTool()
			tool.StorageMounts = []*ags.StorageMount{tc.mount}
			cp := &fakeControlPlane{tool: tool}
			runtime, err := Module().Build(command.Deps{ControlPlane: cp})
			if err != nil {
				t.Fatalf("Build returned error: %v", err)
			}
			_, err = runtime.Handler.Run(context.Background(), command.Request{ArgValues: map[string]string{"tool-id": "sdt-source"}})
			if cliErr, ok := err.(*output.CLIError); !ok || cliErr.Failure.Code != "DEBUG_MOUNT_CONFLICT" {
				t.Fatalf("error = %#v, want DEBUG_MOUNT_CONFLICT", err)
			}
			if cp.action != "" {
				t.Fatalf("Call executed despite conflict")
			}
		})
	}
}

func TestModuleRejectsSourceWithoutRoleArn(t *testing.T) {
	tool := sourceTool()
	tool.RoleArn = nil
	cp := &fakeControlPlane{tool: tool}
	runtime, err := Module().Build(command.Deps{ControlPlane: cp})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	_, err = runtime.Handler.Run(context.Background(), command.Request{ArgValues: map[string]string{"tool-id": "sdt-source"}})
	if cliErr, ok := err.(*output.CLIError); !ok || cliErr.Failure.Code != "DEBUG_ROLE_ARN_REQUIRED" {
		t.Fatalf("error = %#v, want DEBUG_ROLE_ARN_REQUIRED", err)
	}
	if cp.action != "" {
		t.Fatalf("Call executed despite missing RoleArn")
	}
}

func TestModuleReturnsControlPlaneErrors(t *testing.T) {
	cp := &fakeControlPlane{tool: sourceTool(), err: errors.New("boom")}
	runtime, err := Module().Build(command.Deps{ControlPlane: cp})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	_, err = runtime.Handler.Run(context.Background(), command.Request{ArgValues: map[string]string{"tool-id": "sdt-source"}})
	if err == nil || err.Error() != "boom" {
		t.Fatalf("error = %v, want boom", err)
	}
}

func TestDefaultDebugToolNameTruncatesToAPILimit(t *testing.T) {
	got := defaultDebugToolName(strings.Repeat("a", 80), "sdt-source", time.Date(2026, 6, 1, 10, 11, 12, 0, time.UTC))
	if len(got) > defaultNameMaxLen {
		t.Fatalf("name len = %d, want <= %d: %s", len(got), defaultNameMaxLen, got)
	}
	if !strings.HasSuffix(got, "-debug-20260601101112") {
		t.Fatalf("name = %q", got)
	}
}

func sourceTool() *ags.SandboxTool {
	timeout := uint64(300)
	network := "PUBLIC"
	persistent := true
	role := "qcs::role"
	return &ags.SandboxTool{
		ToolId:                strPtr("sdt-source"),
		ToolName:              strPtr("source"),
		ToolType:              strPtr("custom"),
		Description:           strPtr("source description"),
		Persistent:            &persistent,
		DefaultTimeoutSeconds: &timeout,
		RoleArn:               &role,
		NetworkConfiguration:  &ags.NetworkConfiguration{NetworkMode: &network},
		Tags:                  []*ags.Tag{{Key: strPtr("env"), Value: strPtr("unit")}},
		StorageMounts: []*ags.StorageMount{{
			Name:      strPtr("workspace"),
			MountPath: strPtr("/workspace"),
			ReadOnly:  boolPtr(true),
			StorageSource: &ags.StorageSource{Image: &ags.ImageStorageSource{
				Reference:         strPtr("example.com/workspace:latest"),
				ImageRegistryType: strPtr("personal"),
				SubPath:           strPtr("/data"),
				Digest:            strPtr("sha256:abc"),
			}},
		}},
		CustomConfiguration: &ags.CustomConfigurationDetail{
			Image:             strPtr("example.com/source:latest"),
			ImageRegistryType: strPtr("personal"),
			ImageDigest:       strPtr("sha256:def"),
			Command:           []*string{strPtr("/bin/app")},
			Args:              []*string{strPtr("--serve")},
		},
		LogConfiguration: &ags.LogConfiguration{},
	}
}

func stringSlice(value any) []string {
	switch v := value.(type) {
	case []string:
		return v
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			out = append(out, item.(string))
		}
		return out
	default:
		return nil
	}
}

func strPtr(value string) *string { return &value }

func boolPtr(value bool) *bool { return &value }

var _ ControlPlane = (*fakeControlPlane)(nil)
