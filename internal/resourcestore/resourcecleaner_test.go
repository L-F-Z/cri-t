package resourcestore_test

import (
	"context"
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/L-F-Z/cri-t/internal/resourcestore"
)

// The actual test suite.
var _ = t.Describe("ResourceCleaner", func() {
	It("should call the cleanup functions", func() {
		// Given
		sut := resourcestore.NewResourceCleaner()
		called1 := false
		called2 := false
		sut.Add(context.Background(), "test1", func() error {
			called1 = true
			return nil
		})
		sut.Add(context.Background(), "test2", func() error {
			called2 = true
			return nil
		})

		// When
		err := sut.Cleanup()

		// Then
		Expect(err).ToNot(HaveOccurred())
		Expect(called1).To(BeTrue())
		Expect(called2).To(BeTrue())
	})

	It("should retry the cleanup functions", func() {
		// Given
		sut := resourcestore.NewResourceCleaner()
		called1 := false
		called2 := false
		sut.Add(context.Background(), "test1", func() error {
			called1 = true
			return nil
		})
		failureCnt := 0
		sut.Add(context.Background(), "test2", func() error {
			if failureCnt == 2 {
				called2 = true
				return nil
			}
			failureCnt++
			return errors.New("")
		})

		// When
		err := sut.Cleanup()

		// Then
		Expect(err).ToNot(HaveOccurred())
		Expect(called1).To(BeTrue())
		Expect(called2).To(BeTrue())
		Expect(failureCnt).To(Equal(2))
	})

	It("should retry three times", func() {
		// Given
		sut := resourcestore.NewResourceCleaner()
		failureCnt := 0
		sut.Add(context.Background(), "test", func() error {
			failureCnt++
			return errors.New("")
		})

		// When
		err := sut.Cleanup()

		// Then
		Expect(err).To(HaveOccurred())
		Expect(failureCnt).To(Equal(3))
	})
})
