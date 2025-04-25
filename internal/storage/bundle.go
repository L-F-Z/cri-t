package storage

import (
	"context"

	"github.com/L-F-Z/TaskC/pkg/bundle"
	"github.com/containers/storage"
)

type bundleService struct {
	bm *bundle.BundleManager
}

// ListImages returns list of all images.
func (bs *bundleService) ListImages() ([]ImageResult, error) {
	return []ImageResult{}, nil
}

// ImageStatusByID returns status of a single image
func (bs *bundleService) ImageStatusByID(id StorageImageID) (*ImageResult, error) {
	return nil, nil
}

// ImageStatusByName returns status of an image tagged with name.
func (bs *bundleService) ImageStatusByName(name RegistryImageReference) (*ImageResult, error) {
	return nil, nil
}

// PullImage imports an image from the specified location.
//
// Arguments:
// - ctx: The context for controlling the function's execution
// - imageName: A RegistryImageReference representing the image to be pulled
// - options: Pointer to ImageCopyOptions, which contains various options for the image copy process
//
// Returns:
//   - A name@digest value referring to exactly the pulled image (the reference might become dangling if the image
//     is removed, but it will not ever match a different image). The value is suitable for PullImageResponse.ImageRef
//     and for ContainerConfig.Image.Image.
//   - error: An error object if pulling the image fails, otherwise nil
func (bs *bundleService) PullImage(ctx context.Context, imageName RegistryImageReference, options *ImageCopyOptions) (RegistryImageReference, error) {
	return RegistryImageReference{}, nil
}

// DeleteImage deletes a storage image (impacting all its tags)
func (bs *bundleService) DeleteImage(id StorageImageID) error {
	return nil
}

// UntagImage removes a name from the specified image, and if it was
// the only name the image had, removes the image.
func (bs *bundleService) UntagImage(name RegistryImageReference) error {
	return nil
}

// GetStore returns the reference to the storage library Store which
// the image server uses to hold images, and is the destination used
// when it's asked to pull an image.
func (bs *bundleService) GetStore() storage.Store {
	storeOpts, _ := storage.DefaultStoreOptions()
	store, _ := storage.GetStore(storeOpts)
	return store
}

// HeuristicallyTryResolvingStringAsIDPrefix checks if heuristicInput could be a valid image ID or a prefix, and returns
// a StorageImageID if so, or nil if the input can be something else.
// DO NOT CALL THIS from in-process callers who know what their input is and don't NEED to involve heuristics.
func (bs *bundleService) HeuristicallyTryResolvingStringAsIDPrefix(heuristicInput string) *StorageImageID {
	return nil
}

// CandidatesForPotentiallyShortImageName resolves an image name into a set of fully-qualified image names (domain/repo/image:tag|@digest).
// It will only return an empty slice if err != nil.
func (bs *bundleService) CandidatesForPotentiallyShortImageName(imageName string) ([]RegistryImageReference, error) {
	return []RegistryImageReference{}, nil
}

// UpdatePinnedImagesList updates pinned and pause images list in imageService.
func (bs *bundleService) UpdatePinnedImagesList(imageList []string) {
	return
}
