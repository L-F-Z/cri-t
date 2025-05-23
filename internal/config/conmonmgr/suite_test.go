package conmonmgr

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/mock/gomock"

	. "github.com/L-F-Z/cri-t/test/framework"
)

// TestLib runs the created specs.
func TestLibConfig(t *testing.T) {
	RegisterFailHandler(Fail)
	RunFrameworkSpecs(t, "ConmonManager")
}

var (
	t        *TestFramework
	mockCtrl *gomock.Controller
)

var _ = BeforeSuite(func() {
	t = NewTestFramework(NilFunc, NilFunc)
	t.Setup()
	mockCtrl = gomock.NewController(GinkgoT())
})

var _ = AfterSuite(func() {
	t.Teardown()
})
