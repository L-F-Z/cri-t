//go:build !linux

package lib

import (
	"github.com/L-F-Z/cri-t/internal/lib/sandbox"
)

func (c *ContainerServer) addSandboxPlatform(sb *sandbox.Sandbox) error {
	return nil
}

func (c *ContainerServer) removeSandboxPlatform(sb *sandbox.Sandbox) error {
	return nil
}
