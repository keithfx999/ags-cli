package config

import (
	"errors"
	"testing"

	"github.com/TencentCloudAgentRuntime/ags-cli/internal/output"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestConfigBDD(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Config Suite")
}

var _ = Describe("Config", func() {

	BeforeEach(func() {
		cfg = nil
	})

	Describe("Default backend", func() {
		It("defaults to cloud", func() {
			c := Get()
			Expect(c.Backend).To(Equal("cloud"))
		})
	})

	Describe("Default region and domain", func() {
		It("defaults to ap-guangzhou and tencentags.com", func() {
			c := Get()
			Expect(c.Region).To(Equal("ap-guangzhou"))
			Expect(c.Domain).To(Equal("tencentags.com"))
		})
	})

	Describe("Default output", func() {
		It("defaults to text", func() {
			c := Get()
			Expect(c.Output).To(Equal("text"))
		})
	})

	Describe("SetBackend", func() {
		It("changes the backend", func() {
			SetBackend("e2b")
			Expect(GetBackend()).To(Equal("e2b"))
			SetBackend("cloud")
		})
	})

	Describe("SetOutput", func() {
		It("changes the output format", func() {
			SetOutput("json")
			Expect(GetOutput()).To(Equal("json"))
			SetOutput("text")
		})
	})

	Describe("Credential setters and getters", func() {
		It("sets and gets API key", func() {
			SetAPIKey("test-key")
			Expect(GetAPIKey()).To(Equal("test-key"))
			SetAPIKey("")
		})

		It("sets and gets secret ID", func() {
			SetSecretID("test-id")
			Expect(GetSecretID()).To(Equal("test-id"))
			SetSecretID("")
		})

		It("sets and gets secret key", func() {
			SetSecretKey("test-key")
			Expect(GetSecretKey()).To(Equal("test-key"))
			SetSecretKey("")
		})
	})

	Describe("SetInternal", func() {
		It("prepends internal. to domain", func() {
			SetInternal(true)
			Expect(Get().Domain).To(HavePrefix("internal."))
			SetInternal(false)
		})

		It("removes internal. prefix when set to false", func() {
			SetInternal(true)
			SetInternal(false)
			Expect(Get().Domain).NotTo(HavePrefix("internal."))
		})
	})

	Describe("ValidateBasics", func() {
		It("accepts cloud backend", func() {
			SetBackend("cloud")
			SetOutput("text")
			Expect(ValidateBasics()).To(Succeed())
		})

		It("accepts e2b backend", func() {
			SetBackend("e2b")
			Expect(ValidateBasics()).To(Succeed())
			SetBackend("cloud")
		})

		It("rejects invalid backend", func() {
			SetBackend("invalid")
			Expect(ValidateBasics()).To(HaveOccurred())
			SetBackend("cloud")
		})

		It("accepts json output", func() {
			SetOutput("json")
			Expect(ValidateBasics()).To(Succeed())
			SetOutput("text")
		})

		It("accepts ndjson output", func() {
			SetOutput("ndjson")
			Expect(ValidateBasics()).To(Succeed())
			SetOutput("text")
		})

		It("rejects invalid output", func() {
			SetOutput("xml")
			Expect(ValidateBasics()).To(HaveOccurred())
			SetOutput("text")
		})
	})

	Describe("Validate", func() {
		It("requires API key for e2b with exit 4", func() {
			SetBackend("e2b")
			SetAPIKey("")
			err := Validate()
			Expect(err).To(HaveOccurred())
			var cliErr *output.CLIError
			Expect(errors.As(err, &cliErr)).To(BeTrue())
			Expect(cliErr.ExitCode).To(Equal(output.ExitAuthOrPermission))
			SetBackend("cloud")
		})

		It("accepts e2b with API key", func() {
			SetBackend("e2b")
			SetAPIKey("key")
			Expect(Validate()).To(Succeed())
			SetBackend("cloud")
			SetAPIKey("")
		})

		It("requires secret ID and key for cloud with exit 4", func() {
			SetBackend("cloud")
			SetSecretID("")
			SetSecretKey("")
			err := Validate()
			Expect(err).To(HaveOccurred())
			var cliErr *output.CLIError
			Expect(errors.As(err, &cliErr)).To(BeTrue())
			Expect(cliErr.ExitCode).To(Equal(output.ExitAuthOrPermission))
		})

		It("accepts cloud with full credentials", func() {
			SetBackend("cloud")
			SetSecretID("id")
			SetSecretKey("key")
			Expect(Validate()).To(Succeed())
			SetSecretID("")
			SetSecretKey("")
		})
	})

	Describe("Endpoint computation", func() {
		It("returns correct control plane endpoint", func() {
			c := Get()
			c.Internal = false
			Expect(c.ControlPlaneEndpoint()).To(Equal("ags.tencentcloudapi.com"))
		})

		It("returns internal control plane endpoint", func() {
			c := Get()
			c.Internal = true
			Expect(c.ControlPlaneEndpoint()).To(Equal("ags.internal.tencentcloudapi.com"))
			c.Internal = false
		})

		It("returns correct data plane region domain", func() {
			c := Get()
			Expect(c.DataPlaneRegionDomain()).To(Equal("ap-guangzhou.tencentags.com"))
		})
	})
})
