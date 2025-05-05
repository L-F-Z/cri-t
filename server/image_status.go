package server

import (
	"context"
	"errors"
	"fmt"

	json "github.com/json-iterator/go"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"

	"github.com/L-F-Z/TaskC/pkg/bundle"
	"github.com/L-F-Z/cri-t/internal/log"
)

// ImageStatus returns the status of the image.
func (s *Server) ImageStatus(ctx context.Context, req *types.ImageStatusRequest) (*types.ImageStatusResponse, error) {
	ctx, span := log.StartSpan(ctx)
	defer span.End()
	img := req.Image
	if img == nil || img.Image == "" {
		return nil, errors.New("no image specified")
	}

	log.Infof(ctx, "Checking image status: %s", img.Image)
	status, err := s.storageImageStatus(ctx, *img)
	if err != nil {
		return nil, err
	}
	if status == nil {
		log.Infof(ctx, "Image %s not found", img.Image)
		return &types.ImageStatusResponse{}, nil
	}

	resp := &types.ImageStatusResponse{
		Image: &types.Image{
			Id:          status.Id,
			RepoTags:    status.RepoTags,
			RepoDigests: status.RepoDigests,
			Size_:       uint64(status.Size()),
			Spec: &types.ImageSpec{
				Annotations: status.Spec.Annotations,
			},
			Pinned: status.Pinned,
		},
	}
	if req.Verbose {
		info, err := createImageInfo(status)
		if err != nil {
			return nil, fmt.Errorf("creating image info: %w", err)
		}
		resp.Info = info
	}
	resp.Image.Uid = status.Uid
	resp.Image.Username = status.Username
	log.Infof(ctx, "Image status: %v", resp)
	return resp, nil
}

// storageImageStatus calls ImageStatus for a k8s ImageSpec.
// Returns (nil, nil) if image was not found.
func (s *Server) storageImageStatus(ctx context.Context, spec types.ImageSpec) (*types.Image, error) {
	bundleName, err := bundle.ParseBundleName(spec.Image)
	if err == nil {
		return s.StorageService().ImageStatusByName(bundleName)
	}
	bundleName, err = bundle.ParseBundleName(spec.UserSpecifiedImage)
	if err == nil {
		return s.StorageService().ImageStatusByName(bundleName)
	}
	return s.StorageService().ImageStatusByID(bundle.BundleId(spec.Image))
}

func createImageInfo(result *types.Image) (map[string]string, error) {
	bytes, err := json.Marshal(result.Spec)
	if err != nil {
		return nil, fmt.Errorf("marshal data: %w", err)
	}
	return map[string]string{"info": string(bytes)}, nil
}
