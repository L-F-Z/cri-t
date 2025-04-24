package resourcestore_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	. "github.com/L-F-Z/cri-t/test/framework"
)

func TestResourceStore(t *testing.T) {
	RegisterFailHandler(Fail)
	RunFrameworkSpecs(t, "ResourceStore")
}

var t *TestFramework

var _ = BeforeSuite(func() {
	t = NewTestFramework(NilFunc, NilFunc)
	t.Setup()
})

var _ = AfterSuite(func() {
	t.Teardown()
})
