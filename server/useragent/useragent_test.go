package useragent_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/L-F-Z/cri-t/server/useragent"
)

// The actual test suite.
var _ = t.Describe("Useragent", func() {
	t.Describe("Get", func() {
		It("should succeed", func() {
			// Given
			// When
			result, err := useragent.Get()

			// Then
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(SatisfyAll(
				ContainSubstring("cri-o"),
				ContainSubstring("os"),
				ContainSubstring("arch"),
			))
		})
	})
})
