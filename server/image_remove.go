package server

import (
	"context"
	"errors"
	"strings"

	types "k8s.io/cri-api/pkg/apis/runtime/v1"

	"github.com/L-F-Z/cri-t/internal/log"
	"github.com/L-F-Z/cri-t/internal/storage"
)

// RemoveImage removes the image.
func (s *Server) RemoveImage(ctx context.Context, req *types.RemoveImageRequest) (*types.RemoveImageResponse, error) {
	_, span := log.StartSpan(ctx)
	defer span.End()
	imageRef := ""
	img := req.Image
	if img != nil {
		imageRef = img.Image
	}
	if imageRef == "" {
		return nil, errors.New("no image specified")
	}

	var err error
	if strings.HasPrefix(imageRef, "sha256:") {
		err = s.StorageService().DeleteImage(imageRef)
	} else {
		err = s.StorageService().UntagImage(storage.PaserNameTag(imageRef))
	}

	if err != nil {
		return nil, err
	}
	return &types.RemoveImageResponse{}, nil
}
