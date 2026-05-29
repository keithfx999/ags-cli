package cli

import (
	"github.com/TencentCloudAgentRuntime/ags-cli/internal/output"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("error rendering", func() {
	It("extracts request IDs from failure details for text output", func() {
		failure := &output.Failure{
			Code:    "InternalError.Backend",
			Kind:    output.KindGenericError,
			Message: "backend failed",
			Details: map[string]any{"RequestId": "req-123"},
		}

		Expect(failureRequestID(failure)).To(Equal("req-123"))
	})

	It("ignores missing or non-string request IDs", func() {
		Expect(failureRequestID(nil)).To(BeEmpty())
		Expect(failureRequestID(&output.Failure{})).To(BeEmpty())
		Expect(failureRequestID(&output.Failure{Details: map[string]any{"RequestId": 123}})).To(BeEmpty())
	})
})
