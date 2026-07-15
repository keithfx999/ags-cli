package fileerr

import (
	"errors"
	"fmt"
	"strings"

	"github.com/TencentCloudAgentRuntime/ags-cli/internal/output"
)

// TransferFailure converts sandbox file-service errors into command-specific
// failures so file commands do not collapse into generic INTERNAL_ERROR.
func TransferFailure(operation, phase, instanceID, path string, err error) error {
	if err == nil {
		return nil
	}
	var cliErr *output.CLIError
	if errors.As(err, &cliErr) {
		return err
	}

	code := "FILE_TRANSFER_FAILED"
	if operation != "" {
		code = "FILE_" + strings.ToUpper(operation) + "_FAILED"
	}

	kind, retryable, hint := classify(err)
	message := fmt.Sprintf("failed to %s file for instance %s", operation, instanceID)
	if phase == "connect" {
		message = fmt.Sprintf("failed to connect to file service for instance %s", instanceID)
	}

	details := map[string]any{
		"InstanceId": instanceID,
		"Operation":  operation,
		"Phase":      phase,
		"Cause":      err.Error(),
	}
	if path != "" {
		details["Path"] = path
	}
	return output.NewCLIError(&output.Failure{
		Code:      code,
		Kind:      kind,
		Message:   message,
		Hint:      hint,
		Retryable: retryable,
		Details:   details,
	})
}

func classify(err error) (kind string, retryable bool, hint string) {
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "permission denied") || strings.Contains(msg, "unauthorized") || strings.Contains(msg, "forbidden"):
		return output.KindAuthOrPermission, false, "Check sandbox file permissions, the --user value, and whether the instance allows this file operation."
	case strings.Contains(msg, "no such file") || strings.Contains(msg, "not found"):
		return output.KindNotFound, false, "Check that the remote path exists and the instance ID is correct."
	case strings.Contains(msg, "invalid path") || strings.Contains(msg, "is a directory") || strings.Contains(msg, "not a directory"):
		return output.KindUsage, false, "Check the remote path and retry with a valid file path."
	default:
		return output.KindNetwork, true, "Verify the instance is RUNNING and its file service is reachable, then retry or choose another RUNNING instance. Use --debug to include backend diagnostics."
	}
}
