package cmd

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("parsePortSpec", func() {

	DescribeTable("valid specs",
		func(spec string, wantLocal, wantRemote int) {
			local, remote, err := parsePortSpec(spec)
			Expect(err).NotTo(HaveOccurred())
			Expect(local).To(Equal(wantLocal))
			Expect(remote).To(Equal(wantRemote))
		},
		Entry("single port", "3000", 3000, 3000),
		Entry("local:remote", "3000:8080", 3000, 8080),
		Entry("different ports", "9090:80", 9090, 80),
		Entry("max port", "65535", 65535, 65535),
		Entry("min port", "1", 1, 1),
	)

	DescribeTable("invalid specs",
		func(spec, errSubstr string) {
			_, _, err := parsePortSpec(spec)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(errSubstr))
		},
		Entry("invalid single port", "abc", "invalid port"),
		Entry("port zero", "0", "between 1 and 65535"),
		Entry("port too large", "70000", "between 1 and 65535"),
		Entry("negative port", "-1", "between 1 and 65535"),
		Entry("invalid local port", "abc:3000", "invalid local port"),
		Entry("invalid remote port", "3000:abc", "invalid remote port"),
		Entry("local port zero", "0:3000", "local port must be between"),
		Entry("remote port zero", "3000:0", "remote port must be between"),
		Entry("local port too large", "70000:3000", "local port must be between"),
		Entry("remote port too large", "3000:70000", "remote port must be between"),
		Entry("empty spec", "", "invalid port"),
		Entry("multiple colons", "8080:80:8080", "invalid remote port"),
	)
})
