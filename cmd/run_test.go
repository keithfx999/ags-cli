package cmd

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("validateRunFlags", func() {
	var origLanguage string

	BeforeEach(func() {
		origLanguage = runLanguage
	})

	AfterEach(func() {
		runLanguage = origLanguage
	})

	DescribeTable("accepts valid languages",
		func(lang string) {
			runLanguage = lang
			Expect(validateRunFlags()).To(Succeed())
		},
		Entry("python", "python"),
		Entry("javascript", "javascript"),
		Entry("typescript", "typescript"),
		Entry("bash", "bash"),
		Entry("r", "r"),
		Entry("java", "java"),
	)

	It("rejects unsupported language", func() {
		runLanguage = "ruby"
		err := validateRunFlags()
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("unsupported language"))
	})
})
