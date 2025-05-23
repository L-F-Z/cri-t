package config_test

import (
	"errors"
	"os/exec"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/L-F-Z/cri-t/pkg/config"
	. "github.com/L-F-Z/cri-t/test/framework"
)

// TestLib runs the created specs.
func TestLibConfig(t *testing.T) {
	RegisterFailHandler(Fail)
	RunFrameworkSpecs(t, "LibConfig")
}

var (
	t            *TestFramework
	sut          *config.Config
	validDirPath string
)

const (
	validFilePath = "/bin/sh"
	invalidPath   = "/proc/invalid"
)

func validConmonPath() string {
	conmonPath, err := exec.LookPath("conmon")
	if errors.Is(err, exec.ErrNotFound) {
		Skip("conmon not found in $PATH")
	}
	Expect(err).ToNot(HaveOccurred())
	return conmonPath
}

var _ = BeforeSuite(func() {
	t = NewTestFramework(NilFunc, NilFunc)
	t.Setup()

	validDirPath = t.MustTempDir("crio-empty")
})

var _ = AfterSuite(func() {
	t.Teardown()
})

func beforeEach() {
	sut = defaultConfig()
}

func defaultConfig() *config.Config {
	c, err := config.DefaultConfig()
	Expect(err).ToNot(HaveOccurred())
	Expect(c).NotTo(BeNil())
	t.EnsureRuntimeDeps()
	return c
}
