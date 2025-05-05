package storage

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/L-F-Z/TaskC/pkg/bundle"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
)

type StorageService struct {
	root                 string
	runRoot              string
	bm                   *bundle.BundleManager
	regexForPinnedImages []*regexp.Regexp
}

func NewStorageService(ctx context.Context, root string, runRoot string) (*StorageService, error) {
	bm, err := bundle.NewBundleManager(root, "")
	if err != nil {
		return &StorageService{}, err
	}
	return &StorageService{
		root:                 root,
		runRoot:              runRoot,
		bm:                   bm,
		regexForPinnedImages: []*regexp.Regexp{},
	}, nil
}

func (ss *StorageService) Root() string {
	return ss.root
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
			Size_:       0,
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
	err = ss.bm.AssembleHandler(bundle.AssembleConfig{
		ClosureName:    imageName.Name,
		ClosureVersion: imageName.Version,
		Overwrite:      true,
		IgnoreGPU:      false,
	})
	if err != nil {
		return
	}
	bundle, err := ss.bm.Get(imageName.Name, imageName.Version)
	if err != nil {
		return
	}
	id = bundle.Id
	return
}

// type imageCache map[bundle.BundleId]*types.Image

// func (svc *imageService) buildImageCacheItem(ref types.ImageReference) (imageCacheItem, error) {
// 	imageFull, err := ref.NewImage(svc.ctx, nil)
// 	if err != nil {
// 		return imageCacheItem{}, err
// 	}
// 	defer imageFull.Close()
// 	imageConfig, err := imageFull.OCIConfig(svc.ctx)
// 	if err != nil {
// 		return imageCacheItem{}, err
// 	}
// 	size := imageSize(imageFull)

// 	info, err := imageFull.Inspect(svc.ctx)
// 	if err != nil {
// 		return imageCacheItem{}, fmt.Errorf("inspecting image: %w", err)
// 	}

// 	rawSource, err := ref.NewImageSource(svc.ctx, nil)
// 	if err != nil {
// 		return imageCacheItem{}, err
// 	}
// 	defer rawSource.Close()

// 	topManifestBlob, manifestType, err := rawSource.GetManifest(svc.ctx, nil)
// 	if err != nil {
// 		return imageCacheItem{}, err
// 	}
// 	var ociManifest specs.Manifest
// 	if manifestType == specs.MediaTypeImageManifest {
// 		if err := json.Unmarshal(topManifestBlob, &ociManifest); err != nil {
// 			return imageCacheItem{}, err
// 		}
// 	}

// 	return imageCacheItem{
// 		config:      imageConfig,
// 		size:        size,
// 		info:        info,
// 		annotations: ociManifest.Annotations,
// 	}, nil
// }

// func (svc *imageService) ListImages() ([]ImageResult, error) {
// 	images, err := svc.store.Images()
// 	if err != nil {
// 		return nil, err
// 	}
// 	results := make([]ImageResult, 0, len(images))
// 	newImageCache := make(imageCache, len(images))
// 	for i := range images {
// 		image := &images[i]
// 		ref, err := istorage.Transport.NewStoreReference(svc.store, nil, image.ID)
// 		if err != nil {
// 			return nil, err
// 		}
// 		svc.imageCacheLock.Lock()
// 		cacheItem, ok := svc.imageCache[image.ID]
// 		svc.imageCacheLock.Unlock()
// 		if !ok {
// 			cacheItem, err = svc.buildImageCacheItem(ref)
// 			if err != nil {
// 				if os.IsNotExist(err) && imageIsBeingPulled(image) { // skip reporting errors if the images haven't finished pulling
// 					continue
// 				}
// 				return nil, err
// 			}
// 		}

// 		newImageCache[image.ID] = cacheItem
// 		res, err := svc.buildImageResult(image, cacheItem)
// 		if err != nil {
// 			return nil, err
// 		}
// 		results = append(results, res)
// 	}
// 	// replace image cache with cache we just built
// 	// this invalidates all stale entries in cache
// 	svc.imageCacheLock.Lock()
// 	svc.imageCache = newImageCache
// 	svc.imageCacheLock.Unlock()
// 	return results, nil
// }

// func imageIsBeingPulled(image *storage.Image) bool {
// 	for _, name := range image.Names {
// 		if _, ok := ImageBeingPulled.Load(name); ok {
// 			return true
// 		}
// 	}
// 	return false
// }

// // pullImageImplementation is called in PullImage, both directly and inside pullImageChild.
// // NOTE: That means this code can run in a separate process, and it should not access any CRI-O global state.
// //
// // It returns a name@digest value referring to exactly the pulled image.
// func pullImageImplementation(ctx context.Context, lookup *imageLookupService, store storage.Store, imageName RegistryImageReference, options *ImageCopyOptions) (RegistryImageReference, error) {
// 	srcRef, err := lookup.remoteImageReference(imageName)
// 	if err != nil {
// 		return RegistryImageReference{}, err
// 	}

// 	destRef, err := istorage.Transport.NewStoreReference(store, imageName.Raw(), "")
// 	if err != nil {
// 		return RegistryImageReference{}, err
// 	}

// 	manifestBytes, err := copy.Image(ctx, nil, destRef, srcRef, &copy.Options{
// 		ProgressInterval: options.ProgressInterval,
// 		Progress:         options.Progress,
// 	})
// 	if err != nil {
// 		return RegistryImageReference{}, err
// 	}

// 	canonicalRef, err := reference.WithDigest(reference.TrimNamed(imageName.Raw()), digest.FromBytes(manifestBytes))
// 	if err != nil {
// 		return RegistryImageReference{}, fmt.Errorf("create canonical reference: %w", err)
// 	}

// 	return references.RegistryImageReferenceFromRaw(canonicalRef), nil
// }

// // FilterPinnedImage checks if the given image needs to be pinned
// // and excluded from kubelet's image GC.
// func FilterPinnedImage(image string, pinnedImages []*regexp.Regexp) bool {
// 	if len(pinnedImages) == 0 {
// 		return false
// 	}

// 	for _, pinnedImage := range pinnedImages {
// 		if pinnedImage.MatchString(image) {
// 			return true
// 		}
// 	}
// 	return false
// }

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

// Container returns a specific container.
func (ss *StorageService) Container(id string) (*Container, error) {
	return nil, nil
}

// Containers returns a list of the currently known containers.
func (ss *StorageService) Containers() ([]Container, error) {
	return []Container{}, nil
}

// ContainerDirectory returns a path of a directory which the caller
// can use to store data, specific to the container, which the library
// does not directly manage.  The directory will be deleted when the
// container is deleted.
func (ss *StorageService) ContainerDirectory(id string) (string, error) {
	return "", nil
}

// ContainerRunDirectory returns a path of a directory which the
// caller can use to store data, specific to the container, which the
// library does not directly manage.  The directory will be deleted
// when the host system is restarted.
func (ss *StorageService) ContainerRunDirectory(id string) (string, error) {
	return "", nil
}

// Metadata retrieves the metadata which is associated with a layer,
// image, or container (whichever the passed-in ID refers to).
func (ss *StorageService) Metadata(id string) (string, error) {
	return "", nil
}

// SetMetadata updates the metadata which is associated with a layer,
// image, or container (whichever the passed-in ID refers to) to match
// the specified value.  The metadata value can be retrieved at any
// time using Metadata, or using Layer, Image, or Container and reading
// the object directly.
func (ss *StorageService) SetMetadata(id, metadata string) error {
	return nil
}

func (ss *StorageService) GetUsage(id string) (bytesUsed uint64, inodeUsed uint64) {
	return 0, 0
}

// Mount attempts to mount a layer, image, or container for access, and
// returns the pathname if it succeeds.
// Note if the mountLabel == "", the default label for the container
// will be used.
//
// Note that we do some of this work in a child process.  The calling
// process's main() function needs to import our pkg/reexec package and
// should begin with something like this in order to allow us to
// properly start that child process:
//
//	if reexec.Init() {
//	    return
//	}
func (ss *StorageService) Mount(id, mountLabel string) (string, error) {
	return "", nil
}

// Unmount attempts to unmount a layer, image, or container, given an ID, a
// name, or a mount path. Returns whether or not the layer is still mounted.
// WARNING: The return value may already be obsolete by the time it is available
// to the caller, so it can be used for heuristic sanity checks at best. It should almost always be ignored.
func (ss *StorageService) Unmount(id string, force bool) (bool, error) {
	return true, nil
}

// FromContainerDirectory is a convenience function which reads
// the contents of the specified file relative to the container's
// directory.
func (ss *StorageService) FromContainerDirectory(id, file string) ([]byte, error) {
	return []byte{}, nil
}

// Tries to clean up remainders of previous containers or layers that are not
// references in the json files. These can happen in the case of unclean
// shutdowns or regular restarts in transient store mode.
func (ss *StorageService) GarbageCollect() error {
	return nil
}
