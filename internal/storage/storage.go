package storage

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/L-F-Z/TaskC/pkg/bundle"
	"golang.org/x/sync/singleflight"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
)

type StorageService struct {
	work                 string
	run                  string
	info                 string
	bm                   *bundle.BundleManager
	regexForPinnedImages []*regexp.Regexp
	pullGroup            singleflight.Group
}

type NameTag struct {
	Name string
	Tag  string
}

func PaserNameTag(uri string) NameTag {
	var name, tag string
	idx := strings.LastIndex(uri, ":")
	if idx == -1 {
		name = uri
		tag = "latest"
	} else {
		name = uri[:idx]
		tag = uri[idx+1:]
	}
	return NameTag{Name: name, Tag: tag}
}

func (n NameTag) String() string {
	return n.Name + ":" + n.Tag
}

func NewStorageService(ctx context.Context, root string, runRoot string) (*StorageService, error) {
	bm, err := bundle.NewBundleManager(root, "https://prefab.cs.ac.cn:10062/")
	if err != nil {
		return &StorageService{}, err
	}
	workDir := filepath.Join(root, "containerWork")
	infoDir := filepath.Join(root, "containerInfo")
	runDir := filepath.Join(runRoot, "containerRun")
	for _, path := range []string{workDir, infoDir, runDir} {
		err := os.MkdirAll(path, 0o755)
		if err != nil {
			return &StorageService{}, err
		}
	}
	return &StorageService{
		work:                 workDir,
		run:                  runDir,
		info:                 infoDir,
		bm:                   bm,
		regexForPinnedImages: []*regexp.Regexp{},
	}, nil
}

func (ss *StorageService) Root() string {
	return ss.work
}

// ListImages returns list of all images.
func (ss *StorageService) ListImages() (result []*types.Image, err error) {
	bundles, err := ss.bm.List()
	if err != nil {
		return
	}
	for _, bundle := range bundles {
		// ignore bundle.Blueprint.User
		img := &types.Image{
			Id:          fmt.Sprintf("sha256:%s", bundle.Id),
			RepoTags:    []string{fmt.Sprintf("%s:%s", bundle.Blueprint.Name, bundle.Blueprint.Version)},
			RepoDigests: []string{fmt.Sprintf("%s@sha256:%s", bundle.Blueprint.Name, bundle.Id)},
			Size_:       bundle.Size,
			Uid:         &types.Int64Value{Value: 0},
			Username:    "root",
			Pinned:      false,
		}
		result = append(result, img)
	}
	return
}

// ImageStatusByID returns status of a single image
func (ss *StorageService) ImageStatusByID(id string) (img *types.Image, err error) {
	bundle, err := ss.bm.GetById(strings.TrimPrefix(id, "sha256:"))
	if err != nil {
		return
	}
	// ignore bundle.Blueprint.User
	img = &types.Image{
		Id:          fmt.Sprintf("sha256:%s", bundle.Id),
		RepoTags:    []string{fmt.Sprintf("%s:%s", bundle.Blueprint.Name, bundle.Blueprint.Version)},
		RepoDigests: []string{fmt.Sprintf("%s@sha256:%s", bundle.Blueprint.Name, bundle.Id)},
		Size_:       bundle.Size,
		Uid:         &types.Int64Value{Value: 0},
		Username:    "root",
		Pinned:      false,
	}
	return
}

func (ss *StorageService) GetBundleByID(id string) (bundle *bundle.Bundle, err error) {
	return ss.bm.GetById(strings.TrimPrefix(id, "sha256:"))
}

// ImageStatusByName returns status of an image tagged with name.
func (ss *StorageService) ImageStatusByName(name NameTag) (img *types.Image, err error) {
	bundle, err := ss.bm.Get(name.Name, name.Tag)
	if err != nil {
		return
	}
	// ignore bundle.Blueprint.User
	img = &types.Image{
		Id:          fmt.Sprintf("sha256:%s", bundle.Id),
		RepoTags:    []string{fmt.Sprintf("%s:%s", bundle.Blueprint.Name, bundle.Blueprint.Version)},
		RepoDigests: []string{fmt.Sprintf("%s@sha256:%s", bundle.Blueprint.Name, bundle.Id)},
		Size_:       bundle.Size,
		Uid:         &types.Int64Value{Value: 0},
		Username:    "root",
		Pinned:      false,
	}
	return
}

// PullImage imports an image from the specified location.
func (ss *StorageService) PullImage(ctx context.Context, imageName NameTag) (id string, err error) {
	key := imageName.String()
	res, err, _ := ss.pullGroup.Do(key, func() (any, error) {
		if err := ss.bm.AssembleHandler(bundle.AssembleConfig{
			ClosureName:    imageName.Name,
			ClosureVersion: imageName.Tag,
			Overwrite:      true,
			IgnoreGPU:      false,
		}); err != nil {
			return nil, err
		}
		b, err := ss.bm.Get(imageName.Name, imageName.Tag)
		if err != nil {
			return nil, err
		}
		return b.Id, nil
	})
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("sha256:%s", res), nil
}

// DeleteImage deletes a storage image (impacting all its tags)
func (ss *StorageService) DeleteImage(id string) error {
	return ss.bm.DeleteById(strings.TrimPrefix(id, "sha256:"))
}

// UntagImage removes a name from the specified image, and if it was
// the only name the image had, removes the image.
func (ss *StorageService) UntagImage(name NameTag) error {
	return ss.bm.DeleteBundle(name.Name, name.Tag)
}

// UpdatePinnedImagesList updates pinned and pause images list in imageService.
func (ss *StorageService) UpdatePinnedImagesList(imageList []string) {
	ss.regexForPinnedImages = CompileRegexpsForPinnedImages(imageList)
}

// MountImage mounts an image to temp directory and returns the mount point.
// MountImage allows caller to mount an image. Images will always
// be mounted read/only
func (ss *StorageService) MountImage(id string, mountOptions []string, mountLabel string) (string, error) {
	return "", nil
}

// Unmount attempts to unmount an image, given an ID.
// Returns whether or not the layer is still mounted.
// WARNING: The return value may already be obsolete by the time it is available
// to the caller, so it can be used for heuristic sanity checks at best. It should almost always be ignored.
func (ss *StorageService) UnmountImage(id string, force bool) (bool, error) {
	return true, nil
}

// CompileRegexpsForPinnedImages compiles regular expressions for the given
// list of pinned images.
func CompileRegexpsForPinnedImages(patterns []string) []*regexp.Regexp {
	regexps := make([]*regexp.Regexp, 0, len(patterns))
	for _, pattern := range patterns {
		var re *regexp.Regexp
		switch {
		case strings.HasPrefix(pattern, "*") && strings.HasSuffix(pattern, "*"):
			// keyword pattern
			keyword := regexp.QuoteMeta(pattern[1 : len(pattern)-1])
			re = regexp.MustCompile("(?i)" + keyword)
		case strings.HasSuffix(pattern, "*"):
			// glob pattern
			pattern = regexp.QuoteMeta(pattern[:len(pattern)-1]) + ".*"
			re = regexp.MustCompile("(?i)" + pattern)
		default:
			// exact pattern
			re = regexp.MustCompile("(?i)^" + regexp.QuoteMeta(pattern) + "$")
		}
		regexps = append(regexps, re)
	}

	return regexps
}
