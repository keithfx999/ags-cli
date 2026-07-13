package delete

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/TencentCloudAgentRuntime/ags-cli/internal/command"
	"github.com/TencentCloudAgentRuntime/ags-cli/internal/iostreams"
	"github.com/TencentCloudAgentRuntime/ags-cli/internal/output"
)

type fakeControlPlane struct {
	deleted []string
	fail    map[string]error
}

func (f *fakeControlPlane) DeleteTool(_ context.Context, toolID string) error {
	if err := f.fail[toolID]; err != nil {
		return err
	}
	f.deleted = append(f.deleted, toolID)
	return nil
}

func TestModuleDeletesMultipleTools(t *testing.T) {
	cp := &fakeControlPlane{}
	runtime, err := Module().Build(command.Deps{ControlPlane: cp})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	result, err := runtime.Handler.Run(context.Background(), command.Request{
		Args:      []string{"sdt-a", "sdt-b"},
		ArgValues: map[string]string{"tool-id": "sdt-a"},
		Flags:     map[string]command.FlagValue{},
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if len(cp.deleted) != 2 || cp.deleted[0] != "sdt-a" || cp.deleted[1] != "sdt-b" {
		t.Fatalf("deleted = %#v", cp.deleted)
	}
	summary, ok := result.Data.(map[string]any)
	if !ok || summary["Deleted"] != 2 || summary["Failed"] != 0 {
		t.Fatalf("summary = %#v", result.Data)
	}
}

func TestModuleReturnsPartialSummary(t *testing.T) {
	cp := &fakeControlPlane{fail: map[string]error{"sdt-b": errors.New("boom")}}
	runtime, err := Module().Build(command.Deps{ControlPlane: cp})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	result, err := runtime.Handler.Run(context.Background(), command.Request{
		Args:      []string{"sdt-a", "sdt-b"},
		ArgValues: map[string]string{"tool-id": "sdt-a"},
		Flags:     map[string]command.FlagValue{},
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	summary := result.Data.(map[string]any)
	failed := summary["FailedIds"].([]string)
	if summary["Deleted"] != 1 || summary["Failed"] != 1 || len(failed) != 1 || failed[0] != "sdt-b" {
		t.Fatalf("summary = %#v", summary)
	}
	if result.ExitCode == 0 || len(result.Warnings) != 1 {
		t.Fatalf("partial result = %#v", result)
	}
	if result.ExitCode != output.ExitPartialSuccess || result.Failure == nil {
		t.Fatalf("partial result = %#v", result)
	}
	if result.Failure.Code != "PARTIAL_DELETE_FAILED" || result.Failure.Kind != output.KindPartialSuccess {
		t.Fatalf("failure = %#v", result.Failure)
	}
}

func TestModuleDryRunDoesNotDeleteTools(t *testing.T) {
	cp := &fakeControlPlane{}
	runtime, err := Module().Build(command.Deps{ControlPlane: cp})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	result, err := runtime.Handler.Run(context.Background(), command.Request{
		Args:      []string{"sdt-a", "sdt-b"},
		ArgValues: map[string]string{"tool-id": "sdt-a"},
		Flags: map[string]command.FlagValue{
			"dry-run": {Name: "dry-run", Type: command.FlagBool, Changed: true, Bool: true},
		},
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if len(cp.deleted) != 0 {
		t.Fatalf("dry-run deleted tools: %#v", cp.deleted)
	}
	summary := result.Data.(map[string]any)
	wouldDelete := summary["WouldDeleteIds"].([]string)
	if summary["DryRun"] != true || summary["Deleted"] != 0 || len(wouldDelete) != 2 {
		t.Fatalf("summary = %#v", summary)
	}
}

func TestModulePromptCancellationDoesNotDeleteTools(t *testing.T) {
	ios, stdin, _, _ := iostreams.Test()
	ios.SetStdinTTY(true)
	stdin.WriteString("n\n")
	cp := &fakeControlPlane{}
	runtime, err := Module().Build(command.Deps{ControlPlane: cp, IO: ios})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	result, err := runtime.Handler.Run(context.Background(), command.Request{
		Args:      []string{"sdt-a"},
		ArgValues: map[string]string{"tool-id": "sdt-a"},
		Flags:     map[string]command.FlagValue{},
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if len(cp.deleted) != 0 {
		t.Fatalf("cancelled prompt deleted tools: %#v", cp.deleted)
	}
	summary := result.Data.(map[string]any)
	if summary["Cancelled"] != true || summary["Deleted"] != 0 {
		t.Fatalf("summary = %#v", summary)
	}
}

func TestModuleYesSkipsInteractiveConfirmation(t *testing.T) {
	ios := &iostreams.IOStreams{In: &bytes.Buffer{}, Out: &bytes.Buffer{}, ErrOut: &bytes.Buffer{}}
	ios.SetStdinTTY(true)
	cp := &fakeControlPlane{}
	runtime, err := Module().Build(command.Deps{ControlPlane: cp, IO: ios})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	_, err = runtime.Handler.Run(context.Background(), command.Request{
		Args:      []string{"sdt-a"},
		ArgValues: map[string]string{"tool-id": "sdt-a"},
		Flags: map[string]command.FlagValue{
			"yes": {Name: "yes", Type: command.FlagBool, Changed: true, Bool: true},
		},
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if len(cp.deleted) != 1 || cp.deleted[0] != "sdt-a" {
		t.Fatalf("deleted = %#v", cp.deleted)
	}
}

func TestModuleRequestDeletesSingleTool(t *testing.T) {
	cp := &fakeControlPlane{}
	runtime, err := Module().Build(command.Deps{ControlPlane: cp})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	result, err := runtime.Handler.Run(context.Background(), command.Request{
		Args:      []string{"sdt-a"},
		ArgValues: map[string]string{"tool-id": "sdt-a"},
		Flags: map[string]command.FlagValue{
			"request": {Name: "request", Type: command.FlagString, String: `{}`, Changed: true},
		},
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if len(cp.deleted) != 1 || cp.deleted[0] != "sdt-a" {
		t.Fatalf("deleted = %#v", cp.deleted)
	}
	summary := result.Data.(map[string]any)
	if summary["Deleted"] != 1 || summary["Failed"] != 0 {
		t.Fatalf("summary = %#v", summary)
	}
}

func TestSummaryDataIncludesFailedIdsOnlyWhenPresent(t *testing.T) {
	data := Summary{Deleted: 1}.Data()
	if _, ok := data["FailedIds"]; ok {
		t.Fatalf("unexpected FailedIds in %#v", data)
	}
	summary := Summary{Deleted: 1, Failed: 1, WouldDeleteIDs: []string{"sdt-a"}, FailedIDs: []string{"sdt-b"}}
	data = summary.Data()
	failed, ok := data["FailedIds"].([]string)
	if !ok || len(failed) != 1 || failed[0] != "sdt-b" {
		t.Fatalf("FailedIds = %#v", data["FailedIds"])
	}
	wouldDelete := data["WouldDeleteIds"].([]string)
	wouldDelete[0] = "mutated"
	if summary.WouldDeleteIDs[0] != "sdt-a" {
		t.Fatalf("WouldDeleteIds leaked backing slice: %#v", summary.WouldDeleteIDs)
	}
}

func TestModuleRequiresControlPlane(t *testing.T) {
	_, err := Module().Build(command.Deps{})
	if err == nil {
		t.Fatalf("expected missing control plane error")
	}
}

func TestModuleRequestDeleteReturnsControlPlaneError(t *testing.T) {
	cp := &fakeControlPlane{fail: map[string]error{"sdt-a": errors.New("boom")}}
	runtime, err := Module().Build(command.Deps{ControlPlane: cp})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	_, err = runtime.Handler.Run(context.Background(), command.Request{
		Args:      []string{"sdt-a"},
		ArgValues: map[string]string{"tool-id": "sdt-a"},
		Flags: map[string]command.FlagValue{
			"request": {Name: "request", Type: command.FlagString, String: `{}`, Changed: true},
		},
	})
	if err == nil || err.Error() != "boom" {
		t.Fatalf("error = %v, want boom", err)
	}
}

func TestModuleRejectsRequestWithMultipleTools(t *testing.T) {
	cp := &fakeControlPlane{}
	runtime, err := Module().Build(command.Deps{ControlPlane: cp})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	_, err = runtime.Handler.Run(context.Background(), command.Request{
		Args:      []string{"sdt-a", "sdt-b"},
		ArgValues: map[string]string{"tool-id": "sdt-a"},
		Flags: map[string]command.FlagValue{
			"request": {Name: "request", Type: command.FlagString, String: `{}`, Changed: true},
		},
	})
	if err == nil {
		t.Fatalf("expected request conflict error")
	}
}

func TestResultFromSummaryWritesSuccessToStdoutAndWarningsToStderr(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	result := resultFromSummary(
		Summary{Deleted: 1, DeletedIDs: []string{"sdt-a"}},
		[]string{"failed to delete sdt-b: boom"},
		&stderr,
	)
	if result.Text == nil {
		t.Fatal("expected text renderer")
	}

	result.Text(&stdout)

	if got := stdout.String(); got != "Tool deleted: sdt-a\n" {
		t.Fatalf("stdout = %q", got)
	}
	if got := stderr.String(); got != "Warning: failed to delete sdt-b: boom\n" {
		t.Fatalf("stderr = %q", got)
	}
}
