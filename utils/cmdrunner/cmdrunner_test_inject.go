//go:build test

// All *_inject.go files are meant to be used by tests only. Purpose of this
// files is to provide a way to inject mocked data into the current setup.

package cmdrunner

import (
	runnerMock "github.com/L-F-Z/cri-t/test/mocks/cmdrunner"
)

func SetMocked(runner *runnerMock.MockCommandRunner) {
	commandRunner = runner
}
