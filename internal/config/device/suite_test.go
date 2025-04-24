package device_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	. "github.com/L-F-Z/cri-t/test/framework"
)

func TestDeviceConfig(t *testing.T) {
	RegisterFailHandler(Fail)
	RunFrameworkSpecs(t, "DeviceConfig")
}

var t *TestFramework

var _ = BeforeSuite(func() {
	t = NewTestFramework(NilFunc, NilFunc)
	t.Setup()
})

var _ = AfterSuite(func() {
	t.Teardown()
})
