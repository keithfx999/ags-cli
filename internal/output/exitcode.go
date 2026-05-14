package output

import (
	"context"
	"errors"
	"net"
)

// Exit codes per spec
const (
	ExitSuccess            = 0
	ExitGenericError       = 1
	ExitUsage              = 2
	ExitNotFound           = 3
	ExitAuthOrPermission   = 4
	ExitConflict           = 5
	ExitRateLimit          = 6
	ExitTimeout            = 7
	ExitNetwork            = 8
	ExitBackendUnsupported = 9
	ExitPartialSuccess     = 10
	ExitRemoteExecFailed   = 11
)

// Failure kinds
const (
	KindSuccess            = "success"
	KindGenericError       = "generic_error"
	KindUsage              = "usage"
	KindNotFound           = "not_found"
	KindAuthOrPermission   = "auth_or_permission"
	KindConflict           = "conflict"
	KindRateLimit          = "rate_limit"
	KindTimeout            = "timeout"
	KindNetwork            = "network"
	KindBackendUnsupported = "backend_unsupported"
	KindPartialSuccess     = "partial_success"
	KindRemoteExecFailed   = "remote_execution_failed"
)

// ExitCodeForKind maps a failure kind to its exit code.
func ExitCodeForKind(kind string) int {
	switch kind {
	case KindSuccess:
		return ExitSuccess
	case KindUsage:
		return ExitUsage
	case KindNotFound:
		return ExitNotFound
	case KindAuthOrPermission:
		return ExitAuthOrPermission
	case KindConflict:
		return ExitConflict
	case KindRateLimit:
		return ExitRateLimit
	case KindTimeout:
		return ExitTimeout
	case KindNetwork:
		return ExitNetwork
	case KindBackendUnsupported:
		return ExitBackendUnsupported
	case KindPartialSuccess:
		return ExitPartialSuccess
	case KindRemoteExecFailed:
		return ExitRemoteExecFailed
	default:
		return ExitGenericError
	}
}

// CLIError is a structured error that carries failure info and exit code.
type CLIError struct {
	Failure  *Failure
	ExitCode int
}

func (e *CLIError) Error() string {
	return e.Failure.Message
}

// NewCLIError creates a CLIError from a Failure.
func NewCLIError(f *Failure) *CLIError {
	return &CLIError{
		Failure:  f,
		ExitCode: ExitCodeForKind(f.Kind),
	}
}

// NewUsageError creates a usage error (exit 2).
func NewUsageError(code, message, hint string) *CLIError {
	return NewCLIError(&Failure{
		Code:    code,
		Kind:    KindUsage,
		Message: message,
		Hint:    hint,
	})
}

// NewNotFoundError creates a not-found error (exit 3).
func NewNotFoundError(code, message, hint string) *CLIError {
	return NewCLIError(&Failure{
		Code:    code,
		Kind:    KindNotFound,
		Message: message,
		Hint:    hint,
	})
}

// NewAuthError creates an auth/permission error (exit 4).
func NewAuthError(code, message, hint string) *CLIError {
	return NewCLIError(&Failure{
		Code:    code,
		Kind:    KindAuthOrPermission,
		Message: message,
		Hint:    hint,
	})
}

// NewBackendUnsupportedError creates a backend-unsupported error (exit 9).
func NewBackendUnsupportedError(message, hint string) *CLIError {
	return NewCLIError(&Failure{
		Code:    "BACKEND_UNSUPPORTED",
		Kind:    KindBackendUnsupported,
		Message: message,
		Hint:    hint,
	})
}

// NewConflictError creates a conflict error (exit 5).
func NewConflictError(code, message, hint string) *CLIError {
	return NewCLIError(&Failure{
		Code:    code,
		Kind:    KindConflict,
		Message: message,
		Hint:    hint,
	})
}

// ClassifyError translates a Go error into a CLIError.
// If the error is already a *CLIError (from client layer or exitError), it
// is returned as-is. Otherwise, well-known Go error types (context, net)
// are mapped. Everything else becomes a generic error.
func ClassifyError(err error) *CLIError {
	if err == nil {
		return nil
	}

	var cliErr *CLIError
	if errors.As(err, &cliErr) {
		return cliErr
	}

	msg := err.Error()

	if errors.Is(err, context.DeadlineExceeded) {
		return NewCLIError(&Failure{
			Code: "TIMEOUT", Kind: KindTimeout, Message: msg, Retryable: true,
		})
	}
	if errors.Is(err, context.Canceled) {
		return NewCLIError(&Failure{
			Code: "CANCELED", Kind: KindGenericError, Message: msg,
		})
	}

	var netErr *net.OpError
	if errors.As(err, &netErr) {
		return NewCLIError(&Failure{
			Code: "NETWORK_ERROR", Kind: KindNetwork, Message: msg, Retryable: true,
		})
	}
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return NewCLIError(&Failure{
			Code: "DNS_ERROR", Kind: KindNetwork, Message: msg, Retryable: true,
		})
	}

	return NewCLIError(&Failure{
		Code: "INTERNAL_ERROR", Kind: KindGenericError, Message: msg,
	})
}
