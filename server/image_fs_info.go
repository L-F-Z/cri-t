package server

import (
	"context"
	"fmt"
	"time"

	types "k8s.io/cri-api/pkg/apis/runtime/v1"

	crioStorage "github.com/L-F-Z/cri-t/utils"
)

// ImageFsInfo returns information of the filesystem that is used to store images.
func (s *Server) ImageFsInfo(context.Context, *types.ImageFsInfoRequest) (*types.ImageFsInfoResponse, error) {
	// TODO: move this function to TaskC
	bundleRoot := "/var/lib/taskc/Bundle"
	instanceRoot := "/var/lib/taskc/Instance"
	bundleUsage, err := getUsage(bundleRoot)
	if err != nil {
		return nil, fmt.Errorf("unable to get usage for %s: %w", bundleRoot, err)
	}
	instanceUsage, err := getUsage(instanceRoot)
	if err != nil {
		return nil, fmt.Errorf("unable to get usage for %s: %w", instanceRoot, err)
	}

	return &types.ImageFsInfoResponse{
		ImageFilesystems:     []*types.FilesystemUsage{bundleUsage},
		ContainerFilesystems: []*types.FilesystemUsage{instanceUsage},
	}, nil
}

func getUsage(containerPath string) (*types.FilesystemUsage, error) {
	bytes, inodes, err := crioStorage.GetDiskUsageStats(containerPath)
	if err != nil {
		return nil, fmt.Errorf("get disk usage for path %s: %w", containerPath, err)
	}
	return &types.FilesystemUsage{
		Timestamp:  time.Now().UnixNano(),
		FsId:       &types.FilesystemIdentifier{Mountpoint: containerPath},
		UsedBytes:  &types.UInt64Value{Value: bytes},
		InodesUsed: &types.UInt64Value{Value: inodes},
	}, nil
}
