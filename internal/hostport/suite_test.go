package hostport

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	. "github.com/L-F-Z/cri-t/test/framework"
)

// TestHostPort runs the created specs.
func TestHostPort(t *testing.T) {
	RegisterFailHandler(Fail)
	RunFrameworkSpecs(t, "HostPort")
}

var t *TestFramework

var _ = BeforeSuite(func() {
	t = NewTestFramework(NilFunc, NilFunc)
	t.Setup()
})

var _ = AfterSuite(func() {
	t.Teardown()
})
