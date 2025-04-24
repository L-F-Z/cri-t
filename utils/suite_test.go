package utils_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	. "github.com/L-F-Z/cri-t/test/framework"
)

// TestUtils runs the created specs.
func TestUtils(t *testing.T) {
	RegisterFailHandler(Fail)
	RunFrameworkSpecs(t, "Utils")
}

var t *TestFramework

var _ = BeforeSuite(func() {
	t = NewTestFramework(NilFunc, NilFunc)
	t.Setup()
})

var _ = AfterSuite(func() {
	t.Teardown()
})
