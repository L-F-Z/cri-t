//go:build windows || darwin || freebsd

package server

import (
	"context"

	"github.com/L-F-Z/cri-t/internal/oci"
)

// createContainerPlatform performs platform dependent intermediate steps before calling the container's oci.Runtime().CreateContainer()
func (s *Server) createContainerPlatform(ctx context.Context, container *oci.Container, cgroupParent string) error {
	return CreateContainer(ctx, container, cgroupParent, false)
}
