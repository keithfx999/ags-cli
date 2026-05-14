package client

import (
	"errors"
	"strings"

	"github.com/TencentCloudAgentRuntime/ags-cli/internal/output"
	sdkerrors "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common/errors"
)

// classifyHTTPStatus returns a CLIError based on HTTP status code (for E2B).
func classifyHTTPStatus(status int, msg string) *output.CLIError {
	switch {
	case status == 401 || status == 403:
		return output.NewAuthError("AUTH_FAILED", msg, "Check your credentials.")
	case status == 404:
		return output.NewNotFoundError("NOT_FOUND", msg, "")
	case status == 409:
		return output.NewConflictError("CONFLICT", msg, "")
	case status == 429:
		return output.NewCLIError(&output.Failure{
			Code: "RATE_LIMIT", Kind: output.KindRateLimit, Message: msg, Retryable: true,
		})
	case status >= 500:
		return output.NewCLIError(&output.Failure{
			Code: "SERVER_ERROR", Kind: output.KindGenericError, Message: msg, Retryable: true,
		})
	default:
		return output.NewCLIError(&output.Failure{
			Code: "API_ERROR", Kind: output.KindGenericError, Message: msg,
		})
	}
}

// ClassifyCloudError wraps a TencentCloud SDK error into a CLIError with
// the correct kind/exit code based on the SDK error code. This eliminates
// string matching in the generic ClassifyError.
func ClassifyCloudError(err error) error {
	if err == nil {
		return nil
	}
	var sdkErr *sdkerrors.TencentCloudSDKError
	if !errors.As(err, &sdkErr) {
		return err
	}

	code := sdkErr.GetCode()
	msg := sdkErr.GetMessage()

	switch {
	case code == "AuthFailure" || strings.HasPrefix(code, "AuthFailure."):
		return output.NewAuthError(code, msg, "Check your credentials (TENCENTCLOUD_SECRET_ID/TENCENTCLOUD_SECRET_KEY).")
	case code == "ResourceNotFound" || strings.HasPrefix(code, "ResourceNotFound."):
		return output.NewNotFoundError(code, msg, "Run 'ags instance list' to find active instances.")
	case code == "ResourceInsufficient" || strings.HasPrefix(code, "LimitExceeded."):
		return output.NewCLIError(&output.Failure{
			Code: code, Kind: output.KindRateLimit, Message: msg, Retryable: true,
		})
	case strings.Contains(code, "DuplicatedClientToken") || strings.Contains(code, "ClientTokenConflict"):
		return output.NewConflictError("CLIENT_TOKEN_CONFLICT", msg,
			"This token may have been used already. Use a different --client-token or omit it for a new instance.")
	case code == "UnauthorizedOperation" || strings.HasPrefix(code, "UnauthorizedOperation."):
		return output.NewAuthError(code, msg, "Check your permissions.")
	case code == "RequestLimitExceeded":
		return output.NewCLIError(&output.Failure{
			Code: code, Kind: output.KindRateLimit, Message: msg, Retryable: true,
		})
	default:
		return output.NewCLIError(&output.Failure{
			Code: code, Kind: output.KindGenericError, Message: msg,
		})
	}
}
