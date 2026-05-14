package output

import (
	"context"
	"net"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Exit Codes", func() {

	Describe("ExitCodeForKind", func() {
		DescribeTable("maps kind to exit code",
			func(kind string, expected int) {
				Expect(ExitCodeForKind(kind)).To(Equal(expected))
			},
			Entry("success", KindSuccess, 0),
			Entry("generic_error", KindGenericError, 1),
			Entry("usage", KindUsage, 2),
			Entry("not_found", KindNotFound, 3),
			Entry("auth_or_permission", KindAuthOrPermission, 4),
			Entry("conflict", KindConflict, 5),
			Entry("rate_limit", KindRateLimit, 6),
			Entry("timeout", KindTimeout, 7),
			Entry("network", KindNetwork, 8),
			Entry("backend_unsupported", KindBackendUnsupported, 9),
			Entry("partial_success", KindPartialSuccess, 10),
			Entry("remote_execution_failed", KindRemoteExecFailed, 11),
			Entry("unknown kind", "unknown", 1),
		)
	})

	Describe("ClassifyError", func() {
		It("returns nil for nil error", func() {
			Expect(ClassifyError(nil)).To(BeNil())
		})

		It("preserves existing CLIError", func() {
			orig := NewUsageError("TEST", "msg", "")
			result := ClassifyError(orig)
			Expect(result.ExitCode).To(Equal(ExitUsage))
			Expect(result.Failure.Code).To(Equal("TEST"))
		})

		It("classifies context.DeadlineExceeded as timeout", func() {
			result := ClassifyError(context.DeadlineExceeded)
			Expect(result.ExitCode).To(Equal(ExitTimeout))
			Expect(result.Failure.Kind).To(Equal(KindTimeout))
			Expect(result.Failure.Retryable).To(BeTrue())
		})

		It("classifies net.OpError as network", func() {
			err := &net.OpError{Op: "dial", Net: "tcp", Err: &net.DNSError{Err: "no such host"}}
			result := ClassifyError(err)
			Expect(result.ExitCode).To(Equal(ExitNetwork))
			Expect(result.Failure.Kind).To(Equal(KindNetwork))
		})

		It("classifies unknown errors as generic", func() {
			result := ClassifyError(context.Canceled)
			Expect(result.ExitCode).To(Equal(ExitGenericError))
		})
	})

	Describe("Error constructors", func() {
		It("NewUsageError returns exit 2", func() {
			e := NewUsageError("CODE", "msg", "hint")
			Expect(e.ExitCode).To(Equal(ExitUsage))
			Expect(e.Failure.Hint).To(Equal("hint"))
		})

		It("NewNotFoundError returns exit 3", func() {
			e := NewNotFoundError("CODE", "msg", "")
			Expect(e.ExitCode).To(Equal(ExitNotFound))
		})

		It("NewAuthError returns exit 4", func() {
			e := NewAuthError("CODE", "msg", "")
			Expect(e.ExitCode).To(Equal(ExitAuthOrPermission))
		})

		It("NewBackendUnsupportedError returns exit 9", func() {
			e := NewBackendUnsupportedError("msg", "hint")
			Expect(e.ExitCode).To(Equal(ExitBackendUnsupported))
			Expect(e.Failure.Code).To(Equal("BACKEND_UNSUPPORTED"))
		})

		It("NewConflictError returns exit 5", func() {
			e := NewConflictError("CODE", "msg", "")
			Expect(e.ExitCode).To(Equal(ExitConflict))
		})
	})
})
