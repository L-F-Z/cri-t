package server

import (
	"context"

	"github.com/L-F-Z/cri-t/internal/config/seccomp"
	"github.com/L-F-Z/cri-t/internal/log"
	"github.com/L-F-Z/cri-t/internal/oci"
)

func (s *Server) removeSeccompNotifier(ctx context.Context, c *oci.Container) {
	if notifier, ok := s.seccompNotifiers.Load(c.ID()); ok {
		n, ok := notifier.(*seccomp.Notifier)
		if ok {
			if err := n.Close(); err != nil {
				log.Errorf(ctx, "Unable to close seccomp notifier: %v", err)
			}
		}
	}
}
