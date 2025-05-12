package storage

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
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
		uid, username := getUser(bundle.Blueprint.User)
		img := &types.Image{
			Id:          fmt.Sprintf("sha256:%s", bundle.Id),
			RepoTags:    []string{},
			RepoDigests: []string{fmt.Sprintf("%s@sha256:%s", bundle.Blueprint.Name, bundle.Id)},
			Size_:       bundle.Size,
			Uid:         &types.Int64Value{Value: *uid},
			Username:    username,
			Pinned:      false,
		}
		result = append(result, img)
	}
	return
}

// getUserFromImage gets uid or user name of the image user.
// If user is numeric, it will be treated as uid; or else, it is treated as user name.
func getUser(user string) (id *int64, username string) {
	// return both empty if user is not specified in the image.
	if user == "" {
		return nil, ""
	}
	// split instances where the id may contain user:group
	user = strings.Split(user, ":")[0]
	// user could be either uid or user name. Try to interpret as numeric uid.
	uid, err := strconv.ParseInt(user, 10, 64)
	if err != nil {
		// If user is non numeric, assume it's user name.
		return nil, user
	}
	// If user is a numeric uid.
	return &uid, ""
}

// ImageStatusByID returns status of a single image
func (ss *StorageService) ImageStatusByID(id bundle.BundleId) (img *types.Image, err error) {
	bundle, err := ss.bm.GetById(id)
	if err != nil {
		return
	}
	uid, username := getUser(bundle.Blueprint.User)
	img = &types.Image{
		Id:          fmt.Sprintf("sha256:%s", bundle.Id),
		RepoTags:    []string{},
		RepoDigests: []string{fmt.Sprintf("%s@sha256:%s", bundle.Blueprint.Name, bundle.Id)},
		Size_:       0,
		Uid:         &types.Int64Value{Value: *uid},
		Username:    username,
		Pinned:      false,
	}
	return
}

// ImageStatusByName returns status of an image tagged with name.
func (ss *StorageService) ImageStatusByName(name bundle.BundleName) (img *types.Image, err error) {
	bundle, err := ss.bm.Get(name.Name, name.Version)
	if err != nil {
		return
	}
	uid, username := getUser(bundle.Blueprint.User)
	img = &types.Image{
		Id:          fmt.Sprintf("sha256:%s", bundle.Id),
		RepoTags:    []string{},
		RepoDigests: []string{fmt.Sprintf("%s@sha256:%s", bundle.Blueprint.Name, bundle.Id)},
		Size_:       0,
		Uid:         &types.Int64Value{Value: *uid},
		Username:    username,
		Pinned:      false,
	}
	return
}

// PullImage imports an image from the specified location.
func (ss *StorageService) PullImage(ctx context.Context, imageName bundle.BundleName) (id bundle.BundleId, err error) {
	key := imageName.String()
	res, err, _ := ss.pullGroup.Do(key, func() (interface{}, error) {
		if err := ss.bm.AssembleHandler(bundle.AssembleConfig{
			ClosureName:    imageName.Name,
			ClosureVersion: imageName.Version,
			Overwrite:      true,
			IgnoreGPU:      false,
		}); err != nil {
			return nil, err
		}
		b, err := ss.bm.Get(imageName.Name, imageName.Version)
		if err != nil {
			return nil, err
		}
		return b.Id, nil
	})
	if err != nil {
		return "", err
	}
	return res.(bundle.BundleId), nil
}

// DeleteImage deletes a storage image (impacting all its tags)
func (ss *StorageService) DeleteImage(id bundle.BundleId) error {
	sid := strings.TrimPrefix(string(id), "sha256:")
	return ss.bm.DeleteById(bundle.BundleId(sid))
}

// UntagImage removes a name from the specified image, and if it was
// the only name the image had, removes the image.
func (ss *StorageService) UntagImage(name bundle.BundleName) error {
	return ss.bm.DeleteBundle(name.Name, name.Version)
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
