//go:build !linux

package server

import (
	"context"
	"fmt"

	"github.com/L-F-Z/cri-t/internal/factory/container"
	"github.com/L-F-Z/cri-t/internal/lib/sandbox"
	"github.com/L-F-Z/cri-t/internal/oci"
)

func (s *Server) createSandboxContainer(ctx context.Context, ctr container.Container, sb *sandbox.Sandbox) (*oci.Container, error) {
	return nil, fmt.Errorf("not implemented yet")
}
