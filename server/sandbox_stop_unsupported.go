//go:build !linux && !freebsd

package server

import (
	"context"
	"fmt"

	"github.com/L-F-Z/cri-t/internal/lib/sandbox"
)

func (s *Server) stopPodSandbox(ctx context.Context, sb *sandbox.Sandbox) error {
	return fmt.Errorf("unsupported")
}
