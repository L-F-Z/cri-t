//go:build !linux

package server

import (
	"context"

	"github.com/L-F-Z/cri-t/internal/oci"
)

func (s *Server) removeSeccompNotifier(ctx context.Context, c *oci.Container) {
}
