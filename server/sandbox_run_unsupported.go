//go:build !linux && !freebsd

package server

import (
	"context"
	"fmt"

	libsandbox "github.com/L-F-Z/cri-t/internal/lib/sandbox"
	"github.com/containers/storage/pkg/idtools"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
)

func (s *Server) runPodSandbox(ctx context.Context, req *types.RunPodSandboxRequest) (*types.RunPodSandboxResponse, error) {
	return nil, fmt.Errorf("unsupported")
}

func (s *Server) getSandboxIDMappings(ctx context.Context, sb *libsandbox.Sandbox) (*idtools.IDMappings, error) {
	return nil, fmt.Errorf("unsupported")
}
