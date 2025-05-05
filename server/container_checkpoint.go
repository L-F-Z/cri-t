package server

import (
	"context"
	"errors"

	types "k8s.io/cri-api/pkg/apis/runtime/v1"
)

// CheckpointContainer checkpoints a container.
func (s *Server) CheckpointContainer(ctx context.Context, req *types.CheckpointContainerRequest) (*types.CheckpointContainerResponse, error) {
	return nil, errors.New("checkpoint/restore support not available")
}
