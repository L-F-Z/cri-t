package cgmgr_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	. "github.com/L-F-Z/cri-t/test/framework"
)

// TestLib runs the created specs.
func TestLibConfig(t *testing.T) {
	RegisterFailHandler(Fail)
	RunFrameworkSpecs(t, "CgroupManager")
}

var t *TestFramework

var _ = BeforeSuite(func() {
	t = NewTestFramework(NilFunc, NilFunc)
	t.Setup()
})

var _ = AfterSuite(func() {
	t.Teardown()
})
