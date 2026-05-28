package create

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/TencentCloudAgentRuntime/ags-cli/internal/apicli"
	"github.com/TencentCloudAgentRuntime/ags-cli/internal/command"
	"github.com/TencentCloudAgentRuntime/ags-cli/internal/output"
	ags "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/ags/v20250920"
)

// Module returns this package's command module.
func Module() command.Module {
	api := APIDescriptor()
	spec := api.CommandSpec()
	// Add detailed help for complex flags.
	for i := range spec.Flags {
		switch spec.Flags[i].Name {
		case "network-configuration":
			spec.Flags[i].DetailedHelp = networkConfigDetailedHelp
		case "tags":
			spec.Flags[i].DetailedHelp = tagsDetailedHelp
		case "storage-mounts":
			spec.Flags[i].DetailedHelp = storageMountsDetailedHelp
		}
	}
	return command.Module{
		Descriptor: command.Descriptor{
			Spec: spec,
			Generated: &command.Descriptor{
				Spec:   spec,
				Groups: api.Groups,
				API:    api,
				Source: "apicli",
			},
			Groups: api.Groups,
			API:    api,
			Source: "mixed-api",
		},
		Build: func(deps command.Deps) (command.Runtime, error) {
			builder := apicli.NewRequestBuilder(api)
			executor := apicli.NewExecutor(api, deps.ControlPlane)
			return command.Runtime{
				Handler: command.HandlerFunc(func(ctx context.Context, req command.Request) (*command.Result, error) {
					apiReq, err := builder.Build(req)
					if err != nil {
						return nil, err
					}
					if !requestFlag(req) {
						if err := validateConvenienceRequest(apiReq); err != nil {
							return nil, err
						}
					}
					result, err := executor.Execute(ctx, apiReq)
					if err != nil {
						return nil, err
					}
					applyCreateResultText(result, req)
					return result, nil
				}),
			}, nil
		},
	}
}

func applyCreateResultText(result *command.Result, req command.Request) {
	if result == nil {
		return
	}
	response, ok := result.Data.(*ags.CreateSandboxToolResponseParams)
	if !ok {
		return
	}
	toolID := derefString(response.ToolId)
	result.Effects = append(result.Effects, output.Effect{Kind: "create", Resource: "tool", Id: toolID})
	result.Text = func(w io.Writer) {
		fmt.Fprintf(w, "Tool created: %s\n", toolID)
		if requestFlag(req) {
			return
		}
		printKV(w, []kv{
			{key: "ID", value: toolID},
			{key: "Name", value: stringFlag(req, "tool-name")},
			{key: "Type", value: stringFlag(req, "tool-type")},
			{key: "Description", value: stringFlag(req, "description")},
		})
	}
}

type kv struct {
	key   string
	value string
}

func printKV(w io.Writer, pairs []kv) {
	for _, pair := range pairs {
		if pair.value == "" {
			continue
		}
		fmt.Fprintf(w, "%-14s %s\n", pair.key+":", pair.value)
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

func validateConvenienceRequest(req map[string]any) error {
	if strings.TrimSpace(stringValue(req["ToolName"])) == "" {
		return output.NewUsageError("MISSING_REQUIRED_FLAG", "tool name (-n/--tool-name) is required", "Provide a non-empty value for --tool-name.")
	}
	if strings.TrimSpace(stringValue(req["ToolType"])) == "" {
		return output.NewUsageError("MISSING_REQUIRED_FLAG", "tool type (-t/--tool-type) is required", "Provide a non-empty value for --tool-type.")
	}
	if mounts, ok := req["StorageMounts"]; ok && collectionLen(mounts) > 0 && strings.TrimSpace(stringValue(req["RoleArn"])) == "" {
		return output.NewUsageError("MISSING_REQUIRED_FLAG", "--role-arn is required when --storage-mounts is specified", "Provide --role-arn when using --storage-mounts.")
	}
	return nil
}

func collectionLen(value any) int {
	switch v := value.(type) {
	case []any:
		return len(v)
	case []map[string]any:
		return len(v)
	default:
		rv := fmt.Sprintf("%v", value)
		if rv == "[]" || rv == "<nil>" {
			return 0
		}
		return 1
	}
}

func stringValue(value any) string {
	s, _ := value.(string)
	return s
}

func requestFlag(req command.Request) bool {
	flag, ok := req.Flags["request"]
	return ok && flag.Changed && strings.TrimSpace(flag.String) != ""
}

// networkConfigDetailedHelp is the extended help text for --network-configuration.
const networkConfigDetailedHelp = `The --network-configuration flag defines the network mode for sandbox
instances created from this tool.

Format:
  {"NetworkMode": "<mode>", "VpcConfig": {...}}

Supported NetworkMode values:
  SANDBOX  - No external network access (isolated sandbox)
  PUBLIC   - Public internet access
  VPC      - VPC network access (requires VpcConfig)

VpcConfig (required when NetworkMode is VPC):
  {"SubnetIds": ["subnet-xxx"], "SecurityGroupIds": ["sg-xxx"]}

Input sources:
  Inline JSON:  --network-configuration '{"NetworkMode":"SANDBOX"}'
  File:         --network-configuration @network.json
  Stdin:        echo '{"NetworkMode":"PUBLIC"}' | agr tool create ... --network-configuration -

Examples:
  # Isolated sandbox (no network)
  agr tool create -n my-tool -t code-interpreter --network-configuration '{"NetworkMode":"SANDBOX"}'

  # Public internet access
  agr tool create -n my-tool -t code-interpreter --network-configuration '{"NetworkMode":"PUBLIC"}'

  # VPC network
  agr tool create -n my-tool -t custom --network-configuration '{"NetworkMode":"VPC","VpcConfig":{"SubnetIds":["subnet-xxx"],"SecurityGroupIds":["sg-xxx"]}}'`

// tagsDetailedHelp is the extended help text for --tags.
const tagsDetailedHelp = `The --tags flag attaches resource tags to the tool for organization
and cost allocation.

Format:
  [{"Key": "<key>", "Value": "<value>"}, ...]

Input sources:
  Inline JSON:  --tags '[{"Key":"env","Value":"prod"}]'
  File:         --tags @tags.json
  Stdin:        echo '[...]' | agr tool create ... --tags -

Examples:
  agr tool create -n my-tool -t code-interpreter \
    --network-configuration '{"NetworkMode":"SANDBOX"}' \
    --tags '[{"Key":"team","Value":"platform"},{"Key":"env","Value":"staging"}]'`

// storageMountsDetailedHelp is the extended help text for --storage-mounts.
const storageMountsDetailedHelp = `The --storage-mounts flag configures persistent storage volumes for
sandbox instances. Requires --role-arn to be set.

Format:
  [{"Name": "<name>", "MountPath": "<path>", "ReadOnly": false,
    "StorageSource": {"Cos": {...} | "Image": {...} | "Cfs": {...}}}]

StorageSource types:
  Cos    - Tencent Cloud Object Storage (COS)
           {"Endpoint": "...", "BucketName": "...", "BucketPath": "/..."}
  Image  - Container image volume
           {"Reference": "...", "SubPath": "/..."}
  Cfs    - Cloud File Storage (CFS)
           {"FileSystemId": "cfs-xxx", "Path": "/..."}

Input sources:
  Inline JSON:  --storage-mounts '[...]'
  File:         --storage-mounts @mounts.json
  Stdin:        echo '[...]' | agr tool create ... --storage-mounts -

Examples:
  # Mount a COS bucket
  agr tool create -n my-tool -t custom \
    --network-configuration '{"NetworkMode":"PUBLIC"}' \
    --role-arn "qcs::cam::uin/100000:roleName/my-role" \
    --storage-mounts '[{"Name":"data","MountPath":"/data","StorageSource":{"Cos":{"Endpoint":"cos.ap-guangzhou.myqcloud.com","BucketName":"my-bucket","BucketPath":"/workspace"}}}]'`
