package list

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/TencentCloudAgentRuntime/ags-cli/internal/apicli"
	"github.com/TencentCloudAgentRuntime/ags-cli/internal/command"
	instanceview "github.com/TencentCloudAgentRuntime/ags-cli/internal/commands/instance/internal/instanceview"
	"github.com/TencentCloudAgentRuntime/ags-cli/internal/output"
	ags "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/ags/v20250920"
)

// Module returns this package's command module.
func Module() command.Module {
	api := APIDescriptor()
	spec := api.CommandSpec()
	spec.Output = command.OutputSpec{
		DataType:    "InstanceListData",
		Description: "Instance list with normalized items and pagination metadata.",
	}
	// Add detailed help for complex flags.
	for i := range spec.Flags {
		switch spec.Flags[i].Name {
		case "filters":
			spec.Flags[i].DetailedHelp = filtersDetailedHelp
		case "instance-ids":
			spec.Flags[i].DetailedHelp = instanceIdsDetailedHelp
		}
	}
	return command.Module{
		Descriptor: command.Descriptor{
			Spec: spec,
			Generated: &command.Descriptor{
				Spec:   api.CommandSpec(),
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
					if !requestFlag(req) {
						if err := validateRequest(req); err != nil {
							return nil, err
						}
					}
					apiReq, err := builder.Build(req)
					if err != nil {
						return nil, err
					}
					result, err := executor.Execute(ctx, apiReq)
					if err != nil {
						return nil, err
					}
					response, ok := result.Data.(*ags.DescribeSandboxInstanceListResponseParams)
					if !ok {
						return result, nil
					}
					offset := intFlag(req, "offset")
					limit := effectiveLimit(req)
					return instanceListResult(response, offset, limit, result), nil
				}),
			}, nil
		},
	}
}

func validateRequest(req command.Request) error {
	offset := intFlag(req, "offset")
	if offset < 0 {
		return output.NewUsageError("INVALID_PAGINATION", fmt.Sprintf("--offset must be >= 0 (got %d)", offset), "Use a non-negative pagination offset.")
	}
	if limitChanged(req) {
		limit := intFlag(req, "limit")
		if limit < 0 {
			return output.NewUsageError("INVALID_PAGINATION", fmt.Sprintf("--limit must be >= 0 (got %d)", limit), "Use a non-negative pagination limit.")
		}
	}
	return nil
}

func instanceListResult(response *ags.DescribeSandboxInstanceListResponseParams, offset int, limit *int, base *command.Result) *command.Result {
	items := make([]map[string]any, len(response.InstanceSet))
	for i, instance := range response.InstanceSet {
		items[i] = instanceview.CanonicalData(instance)
	}
	pagination := map[string]any{
		"Offset":     offset,
		"Total":      instanceview.DerefInt64(response.TotalCount),
		"NextCursor": nil,
	}
	if limit != nil {
		pagination["Limit"] = *limit
	}
	data := map[string]any{
		"Items":      items,
		"Pagination": pagination,
	}
	return &command.Result{
		Data:      data,
		Warnings:  base.Warnings,
		Effects:   base.Effects,
		ExitCode:  base.ExitCode,
		Failure:   base.Failure,
		MetaExtra: base.MetaExtra,
		Text: func(w io.Writer) {
			renderInstanceList(w, response)
		},
	}
}

func effectiveLimit(req command.Request) *int {
	if !limitChanged(req) {
		return nil
	}
	limit := intFlag(req, "limit")
	return &limit
}

func limitChanged(req command.Request) bool {
	flag, ok := req.Flags["limit"]
	return ok && flag.Changed
}

func requestFlag(req command.Request) bool {
	flag, ok := req.Flags["request"]
	return ok && flag.Changed && strings.TrimSpace(flag.String) != ""
}

func intFlag(req command.Request, name string) int {
	flag, ok := req.Flags[name]
	if !ok {
		return 0
	}
	return flag.Int
}

func renderInstanceList(w io.Writer, response *ags.DescribeSandboxInstanceListResponseParams) {
	if len(response.InstanceSet) == 0 {
		fmt.Fprintln(w, "No instances found")
		return
	}

	headers := []string{"ID", "TOOL", "STATUS", "TIMEOUT", "EXPIRES", "MOUNTS", "CREATED"}
	rows := make([][]string, len(response.InstanceSet))
	for i, inst := range response.InstanceSet {
		timeout := "-"
		if inst.TimeoutSeconds != nil {
			timeout = instanceview.Timeout(*inst.TimeoutSeconds)
		}
		expires := "-"
		if inst.ExpiresAt != nil && *inst.ExpiresAt != "" {
			expires = instanceview.TimeShort(*inst.ExpiresAt)
		}
		rows[i] = []string{
			instanceview.DerefString(inst.InstanceId),
			instanceview.DerefString(inst.ToolName),
			instanceview.DerefString(inst.Status),
			timeout,
			expires,
			instanceview.MountOptionsSummary(inst.MountOptions),
			instanceview.TimeShort(instanceview.DerefString(inst.CreateTime)),
		}
	}
	instanceview.PrintTableWithPagination(w, headers, rows, len(response.InstanceSet), instanceview.DerefInt64(response.TotalCount))
}

// filtersDetailedHelp is the extended help text for the --filters flag.
const filtersDetailedHelp = `The --filters flag accepts a JSON array of filter objects. Each filter
narrows the result set by matching a resource attribute against one or
more values. When multiple filters are supplied, they are combined with
AND logic; values within a single filter use OR logic.

Format:
  [{"Name": "<field>", "Values": ["<value1>", "<value2>", ...]}]

Supported filter names for instance list:
  Status      - Filter by instance status
  ToolName    - Filter by the tool name used to create the instance
  ToolId      - Filter by the tool ID

Supported values for Status:
  STARTING, RUNNING, STOPPING, STOPPED, STOP_FAILED, FAILED

Input sources:
  Inline JSON:  --filters '[{"Name":"Status","Values":["RUNNING"]}]'
  File:         --filters @filters.json
  Stdin:        echo '[...]' | agr instance list --filters -

Examples:
  # List only running instances
  agr instance list --filters '[{"Name":"Status","Values":["RUNNING"]}]'

  # List instances in multiple states (OR within one filter)
  agr instance list --filters '[{"Name":"Status","Values":["RUNNING","STARTING"]}]'

  # Combine filters (AND logic)
  agr instance list --filters '[{"Name":"Status","Values":["RUNNING"]},{"Name":"ToolName","Values":["my-tool"]}]'`

// instanceIdsDetailedHelp is the extended help text for the --instance-ids flag.
const instanceIdsDetailedHelp = `The --instance-ids flag filters results to specific instance IDs.
Pass one or more instance IDs as a repeatable flag.

Format:
  --instance-ids <id1> --instance-ids <id2>

Examples:
  agr instance list --instance-ids ins-abc123
  agr instance list --instance-ids ins-abc123 --instance-ids ins-def456`
