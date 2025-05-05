package server

import (
	"context"

	types "k8s.io/cri-api/pkg/apis/runtime/v1"

	"github.com/L-F-Z/cri-t/internal/log"
)

// ListImages lists existing images.
func (s *Server) ListImages(ctx context.Context, req *types.ListImagesRequest) (*types.ListImagesResponse, error) {
	_, span := log.StartSpan(ctx)
	defer span.End()

	results, err := s.StorageService().ListImages()
	if err != nil {
		return nil, err
	}
	return &types.ListImagesResponse{
		Images: results,
	}, nil
}
