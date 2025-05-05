package server

import (
	"context"
	"errors"

	types "k8s.io/cri-api/pkg/apis/runtime/v1"

	"github.com/L-F-Z/TaskC/pkg/bundle"
	"github.com/L-F-Z/cri-t/internal/log"
)

// RemoveImage removes the image.
func (s *Server) RemoveImage(ctx context.Context, req *types.RemoveImageRequest) (*types.RemoveImageResponse, error) {
	ctx, span := log.StartSpan(ctx)
	defer span.End()
	imageRef := ""
	img := req.Image
	if img != nil {
		imageRef = img.Image
	}
	if imageRef == "" {
		return nil, errors.New("no image specified")
	}
	if err := s.removeImage(ctx, imageRef); err != nil {
		return nil, err
	}
	return &types.RemoveImageResponse{}, nil
}

func (s *Server) removeImage(ctx context.Context, imageRef string) (untagErr error) {
	_, span := log.StartSpan(ctx)
	defer span.End()

	name, err := bundle.ParseBundleName(imageRef)
	if err != nil {
		return err
	}
	// TODO: Add --image-volume support
	return s.StorageImageServer().UntagImage(name)
}
