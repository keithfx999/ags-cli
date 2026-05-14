package client

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("NewControlPlaneClient", func() {

	It("rejects invalid backend", func() {
		c, err := NewControlPlaneClient("bad")
		Expect(err).To(HaveOccurred())
		Expect(c).To(BeNil())
	})

	It("accepts cloud backend", func() {
		c, err := NewControlPlaneClient("cloud")
		Expect(err).NotTo(HaveOccurred())
		Expect(c).NotTo(BeNil())
	})

	It("accepts e2b backend", func() {
		c, err := NewControlPlaneClient("e2b")
		Expect(err).NotTo(HaveOccurred())
		Expect(c).NotTo(BeNil())
	})
})
