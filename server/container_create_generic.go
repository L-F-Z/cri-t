//go:build windows || darwin || freebsd

package server

import (
	"context"

	"github.com/L-F-Z/cri-t/internal/oci"
	"github.com/containers/storage/pkg/idtools"
)

// createContainerPlatform performs platform dependent intermediate steps before calling the container's oci.Runtime().CreateContainer()
func (s *Server) createContainerPlatform(ctx context.Context, container *oci.Container, cgroupParent string, idMappings *idtools.IDMappings) error {
	return s.Runtime().CreateContainer(ctx, container, cgroupParent, false)
}
