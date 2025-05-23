package container_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/L-F-Z/cri-t/internal/factory/container"
	. "github.com/L-F-Z/cri-t/test/framework"
)

// TestContainer runs the specs.
func TestContainer(t *testing.T) {
	RegisterFailHandler(Fail)
	RunFrameworkSpecs(t, "Container")
}

var (
	t   *TestFramework
	sut container.Container
)

var _ = BeforeSuite(func() {
	t = NewTestFramework(NilFunc, NilFunc)
	t.Setup()
})

var _ = AfterSuite(func() {
	t.Teardown()
})

var _ = BeforeEach(func() {
	var err error
	sut, err = container.New()
	Expect(err).ToNot(HaveOccurred())
})
