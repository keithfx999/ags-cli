package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/TencentCloudAgentRuntime/ags-cli/internal/client"
	"github.com/TencentCloudAgentRuntime/ags-cli/internal/config"
	"github.com/TencentCloudAgentRuntime/ags-cli/internal/iostreams"
	"github.com/TencentCloudAgentRuntime/ags-cli/internal/output"
	"github.com/spf13/cobra"
)

// ═══════════════════════════════════════════════════════════════════════════
// Global IOStreams — set once in Execute(), overridable for tests
// ═══════════════════════════════════════════════════════════════════════════

var ios *iostreams.IOStreams

// newControlPlaneClient is an injectable factory for the control-plane client.
// Tests can override this to inject fakes.
var newControlPlaneClient = client.NewControlPlaneClient

func initIOStreams() {
	if ios == nil {
		ios = iostreams.System()
	}
}

// SetIOStreams allows tests to inject a custom IOStreams.
func SetIOStreams(s *iostreams.IOStreams) {
	ios = s
}

// IO returns the current IOStreams.
func IO() *iostreams.IOStreams { return ios }

// ═══════════════════════════════════════════════════════════════════════════
// KeyValue — used by text-mode renderers
// ═══════════════════════════════════════════════════════════════════════════

type KeyValue struct {
	Key   string
	Value string
}

// ═══════════════════════════════════════════════════════════════════════════
// CmdResult
// ═══════════════════════════════════════════════════════════════════════════

type CmdResult struct {
	Data       any
	Warnings   []string
	Effects    []output.Effect
	ExitCode   int
	Failure    *output.Failure
	RenderText func(w io.Writer) // receives ios.Out from Wrap
	Handled    bool
}

func OK(data any, text func(w io.Writer)) *CmdResult {
	return &CmdResult{Data: data, RenderText: text}
}

func OKWithEffects(data any, text func(w io.Writer), effects ...output.Effect) *CmdResult {
	return &CmdResult{Data: data, RenderText: text, Effects: effects}
}

func PartialResult(data any, warnings []string, text func(w io.Writer)) *CmdResult {
	return &CmdResult{Data: data, Warnings: warnings, ExitCode: output.ExitPartialSuccess, RenderText: text}
}

func FailedData(data any, failure *output.Failure, exitCode int, text func(w io.Writer)) *CmdResult {
	return &CmdResult{Data: data, Failure: failure, ExitCode: exitCode, RenderText: text}
}

func StreamDone(exitCode int) *CmdResult {
	return &CmdResult{Handled: true, ExitCode: exitCode}
}

// ═══════════════════════════════════════════════════════════════════════════
// CmdFunc + Wrap
// ═══════════════════════════════════════════════════════════════════════════

type CmdFunc func(cmd *cobra.Command, args []string) (*CmdResult, error)

func Wrap(commandID string, fn CmdFunc) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		start := time.Now()
		result, err := fn(cmd, args)
		dm := time.Since(start).Milliseconds()

		if err != nil {
			cliErr := output.ClassifyError(err)
			if isJSON() {
				if jqErr := writeEnvelope(ios.Out, commandID, "failed", nil, cliErr.Failure, nil, nil, dm); jqErr != nil {
					return output.NewUsageError("INVALID_JQ_EXPRESSION", jqErr.Error(), "Check your --jq expression syntax.")
				}
				return &envelopeAlreadyWritten{code: cliErr.ExitCode}
			}
			return exitError(cliErr.ExitCode, err)
		}

		if result == nil {
			return nil
		}

		if result.Handled {
			if result.ExitCode != 0 {
				return exitError(result.ExitCode, fmt.Errorf("command failed"))
			}
			return nil
		}

		status := "succeeded"
		if result.Failure != nil {
			status = "failed"
		} else if result.ExitCode == output.ExitPartialSuccess {
			status = "partial"
		} else if result.ExitCode != 0 {
			status = "failed"
		}

		if isJSON() {
			if jqErr := writeEnvelope(ios.Out, commandID, status, result.Data, result.Failure,
				result.Warnings, result.Effects, dm); jqErr != nil {
				return output.NewUsageError("INVALID_JQ_EXPRESSION", jqErr.Error(), "Check your --jq expression syntax.")
			}
			if result.ExitCode != 0 {
				return &envelopeAlreadyWritten{code: result.ExitCode}
			}
			return nil
		}

		if result.RenderText != nil {
			result.RenderText(ios.Out)
		}
		if result.ExitCode != 0 {
			msg := "command failed"
			if result.Failure != nil {
				msg = result.Failure.Message
			}
			return exitError(result.ExitCode, fmt.Errorf("%s", msg))
		}
		return nil
	}
}

func WrapNoJSON(fn func(*cobra.Command, []string) error) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		if isJSON() || isNDJSON() {
			return exitError(output.ExitUsage,
				fmt.Errorf("this command does not support -o json or -o ndjson"))
		}
		return fn(cmd, args)
	}
}

type envelopeAlreadyWritten struct{ code int }

func (e *envelopeAlreadyWritten) Error() string { return "envelope already written" }

// ═══════════════════════════════════════════════════════════════════════════
// writeEnvelope — writes to the given writer (ios.Out or test buffer)
// ═══════════════════════════════════════════════════════════════════════════

func writeEnvelope(
	w io.Writer,
	commandID, status string,
	data any, failure *output.Failure,
	warnings []string, effects []output.Effect,
	dm int64,
) error {
	if warnings == nil {
		warnings = []string{}
	}
	env := &output.Envelope{
		SchemaVersion: "ags.v1",
		Command:       commandID,
		Status:        status,
		Data:          data,
		Failure:       failure,
		Warnings:      warnings,
		Meta: &output.Meta{
			Backend:    config.GetBackend(),
			DurationMs: dm,
			Effects:    effects,
		},
	}
	return output.RenderEnvelope(w, env, jqExpr)
}

// ═══════════════════════════════════════════════════════════════════════════
// Format helpers
// ═══════════════════════════════════════════════════════════════════════════

func isJSON() bool  { return config.GetOutput() == "json" }
func isNDJSON() bool { return config.GetOutput() == "ndjson" }

func stderr(format string, args ...any) {
	fmt.Fprintf(ios.ErrOut, format, args...)
}

func requireTTY() error {
	if !ios.IsStdinTTY() {
		return exitError(2, fmt.Errorf("this command requires a TTY"))
	}
	return nil
}

func validateNDJSONOnlyForStream(hasStream bool) error {
	if isNDJSON() && !hasStream {
		return exitError(2, fmt.Errorf("-o ndjson can only be used with --stream"))
	}
	return nil
}

func validateStreamNotJSON() error {
	if isJSON() {
		return exitError(2, fmt.Errorf("--stream cannot be used with -o json.\nHint: Use -o ndjson for machine-readable stream events, or remove --stream to get a JSON envelope."))
	}
	return nil
}

// ═══════════════════════════════════════════════════════════════════════════
// Text rendering helpers — receive io.Writer from Wrap
// ═══════════════════════════════════════════════════════════════════════════

func printKV(w io.Writer, pairs []KeyValue) {
	maxLen := 0
	for _, kv := range pairs {
		if len(kv.Key) > maxLen {
			maxLen = len(kv.Key)
		}
	}
	for _, kv := range pairs {
		fmt.Fprintf(w, "%-*s  %s\n", maxLen, kv.Key+":", kv.Value)
	}
}

func printTable(w io.Writer, headers []string, rows [][]string) {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, strings.Join(headers, "\t"))
	for _, row := range rows {
		fmt.Fprintln(tw, strings.Join(row, "\t"))
	}
	_ = tw.Flush()
}

func printTableWithPagination(w io.Writer, headers []string, rows [][]string, shown, total int) {
	printTable(w, headers, rows)
	if total > shown {
		fmt.Fprintf(w, "\nShowing %d of %d items (use --offset and --limit for pagination)\n", shown, total)
	}
}

// ═══════════════════════════════════════════════════════════════════════════
// PascalCase converters
// ═══════════════════════════════════════════════════════════════════════════

func convertMountOptions(opts []client.MountOption) []map[string]any {
	result := make([]map[string]any, len(opts))
	for i, o := range opts {
		m := map[string]any{"Name": o.Name}
		if o.MountPath != "" {
			m["MountPath"] = o.MountPath
		}
		if o.SubPath != "" {
			m["SubPath"] = o.SubPath
		}
		if o.ReadOnly != nil {
			m["ReadOnly"] = *o.ReadOnly
		}
		result[i] = m
	}
	return result
}

func convertEndpoints(eps []client.Endpoint) []map[string]any {
	result := make([]map[string]any, len(eps))
	for i, e := range eps {
		result[i] = map[string]any{"Scheme": e.Scheme, "Scope": e.Scope, "Url": e.URL}
	}
	return result
}

func convertVPCConfig(vpc *client.VPCConfig) map[string]any {
	if vpc == nil {
		return nil
	}
	return map[string]any{"SubnetIds": vpc.SubnetIds, "SecurityGroupIds": vpc.SecurityGroupIds}
}

func convertStorageMounts(mounts []client.StorageMount) []map[string]any {
	result := make([]map[string]any, len(mounts))
	for i, m := range mounts {
		sm := map[string]any{"Name": m.Name, "MountPath": m.MountPath, "ReadOnly": m.ReadOnly}
		if m.StorageSource != nil && m.StorageSource.Cos != nil {
			sm["StorageSource"] = map[string]any{
				"Cos": map[string]any{
					"BucketName": m.StorageSource.Cos.BucketName,
					"BucketPath": m.StorageSource.Cos.BucketPath,
					"Endpoint":   m.StorageSource.Cos.Endpoint,
				},
			}
		}
		result[i] = sm
	}
	return result
}

// ═══════════════════════════════════════════════════════════════════════════
// Canonical resource converters — shared by list and get commands
// ═══════════════════════════════════════════════════════════════════════════

func toCanonicalInstanceData(inst *client.Instance) map[string]any {
	data := map[string]any{
		"Id":        inst.ID,
		"ToolId":    inst.ToolID,
		"ToolName":  inst.ToolName,
		"Status":    inst.Status,
		"CreatedAt": inst.CreatedAt,
	}
	if inst.UpdatedAt != "" {
		data["UpdatedAt"] = inst.UpdatedAt
	}
	if inst.TimeoutSeconds != nil {
		data["TimeoutSeconds"] = *inst.TimeoutSeconds
	}
	if inst.ExpiresAt != "" {
		data["ExpiresAt"] = inst.ExpiresAt
	}
	if inst.StopReason != "" {
		data["StopReason"] = inst.StopReason
	}
	if len(inst.Endpoints) > 0 {
		data["Endpoints"] = convertEndpoints(inst.Endpoints)
	}
	if len(inst.MountOptions) > 0 {
		data["MountOptions"] = convertMountOptions(inst.MountOptions)
	}
	return data
}

func toCanonicalToolData(t *client.Tool) map[string]any {
	data := map[string]any{
		"Id":          t.ID,
		"Name":        t.Name,
		"Type":        t.Type,
		"Status":      t.Status,
		"NetworkMode": t.NetworkMode,
		"Description": t.Description,
		"Tags":        t.Tags,
		"CreatedAt":   t.CreatedAt,
	}
	if t.UpdatedAt != "" {
		data["UpdatedAt"] = t.UpdatedAt
	}
	if t.RoleArn != "" {
		data["RoleArn"] = t.RoleArn
	}
	if t.NetworkMode == "VPC" && t.VPCConfig != nil {
		data["VpcConfig"] = convertVPCConfig(t.VPCConfig)
	}
	if len(t.StorageMounts) > 0 {
		data["StorageMounts"] = convertStorageMounts(t.StorageMounts)
	}
	return data
}

// ═══════════════════════════════════════════════════════════════════════════
// --request flag parser
// ═══════════════════════════════════════════════════════════════════════════

func parseRequestFlag(value string) (map[string]any, error) {
	var data []byte
	var err error

	switch {
	case value == "-":
		data, err = io.ReadAll(os.Stdin)
		if err != nil {
			return nil, fmt.Errorf("failed to read request from stdin: %w", err)
		}
	case strings.HasPrefix(value, "@"):
		data, err = os.ReadFile(value[1:])
		if err != nil {
			return nil, fmt.Errorf("failed to read request file %s: %w", value[1:], err)
		}
	default:
		data = []byte(value)
	}

	var raw any
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, output.NewUsageError("INVALID_REQUEST_JSON",
			fmt.Sprintf("invalid JSON in --request: %v", err),
			"Provide valid JSON as a string, @file, or - for stdin.")
	}
	result, ok := raw.(map[string]any)
	if !ok {
		return nil, output.NewUsageError("INVALID_REQUEST_JSON",
			"--request must be a JSON object",
			"The top-level value must be a JSON object, not an array or scalar.")
	}
	return result, nil
}
