package storage

import (
	"context"
	"errors"
	"time"

	json "github.com/json-iterator/go"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/sirupsen/logrus"

	"github.com/L-F-Z/TaskC/pkg/bundle"
	"github.com/L-F-Z/TaskC/pkg/libtc"
	"github.com/L-F-Z/cri-t/internal/log"
)

const DefaultRoot = "/var/lib/taskc"
const DefaultRunRoot = "/run/taskc"

var (
	// ErrInvalidPodName is returned when a pod name specified to a
	// function call is found to be invalid (most often, because it's
	// empty).
	ErrInvalidPodName = errors.New("invalid pod name")
	// ErrInvalidContainerName is returned when a container name specified
	// to a function call is found to be invalid (most often, because it's
	// empty).
	ErrInvalidContainerName = errors.New("invalid container name")
	// ErrInvalidSandboxID is returned when a sandbox ID specified to a
	// function call is found to be invalid (because it's either
	// empty or doesn't match a valid sandbox).
	ErrInvalidSandboxID = errors.New("invalid sandbox ID")
	// ErrInvalidContainerID is returned when a container ID specified to a
	// function call is found to be invalid (because it's either
	// empty or doesn't match a valid container).
	ErrInvalidContainerID = errors.New("invalid container ID")
	// ErrDuplicateName indicates that a name which is to be assigned to a new item is already being used.
	ErrDuplicateName = errors.New("that name is already in use")
	// ErrContainerUnknown indicates that there was no container with the specified name or ID.
	ErrContainerUnknown = errors.New("container not known")
	// ErrLayerUnknown indicates that there was no layer with the specified name or ID.
	ErrLayerUnknown = errors.New("layer not known")
)

// CreatePodSandbox creates a pod infrastructure container, using the
// specified PodID for the infrastructure container's ID.  In the CRI
// view of things, a sandbox is distinct from its containers, including
// its infrastructure container, but at this level the sandbox is
// essentially the same as its infrastructure container, with a
// container's membership in a pod being signified by it listing the
// same pod ID in its metadata that the pod's other members do, and
// with the pod's infrastructure container having the same value for
// both its pod's ID and its container ID.
// Pointer arguments can be nil.  All other arguments are required.
func (ss *StorageService) CreatePodSandbox(podName, podID string, pauseImage bundle.BundleName, containerName, metadataName, uid, namespace string, attempt uint32, labelOptions []string, privileged bool) (ContainerInfo, error) {
	// Check if we have the specified image.
	var imageID bundle.BundleId
	status, err := ss.ImageStatusByName(pauseImage)
	if err != nil {
		var err error
		imageID, err = ss.PullImage(context.Background(), pauseImage)
		if err != nil {
			return ContainerInfo{}, err
		}
	} else {
		imageID, _ = bundle.ParseBundleId(status.Id)
	}

	return ss.createContainerOrPodSandbox(podID, &runtimeContainerMetadataTemplate{
		podName:            podName,
		podID:              podID,
		userRequestedImage: pauseImage.String(), // userRequestedImage is only used to write to container metadata on disk
		imageID:            imageID,
		containerName:      containerName,
		metadataName:       metadataName,
		uid:                uid,
		namespace:          namespace,
		attempt:            attempt,
		privileged:         privileged,
	}, labelOptions)
}

// CreateContainer creates a container with the specified ID.
// Pointer arguments can be nil.
// All other arguments are required.
func (ss *StorageService) CreateContainer(podName, podID, userRequestedImage string, imageID bundle.BundleId, containerName, containerID, metadataName string, attempt uint32, labelOptions []string, privileged bool) (ContainerInfo, error) {
	return ss.createContainerOrPodSandbox(containerID, &runtimeContainerMetadataTemplate{
		podName:            podName,
		podID:              podID,
		userRequestedImage: userRequestedImage,
		imageID:            imageID,
		containerName:      containerName,
		metadataName:       metadataName,
		uid:                "",
		namespace:          "",
		attempt:            attempt,
		privileged:         privileged,
	}, labelOptions)
}

func (ss *StorageService) createContainerOrPodSandbox(containerID string, template *runtimeContainerMetadataTemplate, labelOptions []string) (ci ContainerInfo, retErr error) {
	if template.podName == "" || template.podID == "" {
		return ContainerInfo{}, ErrInvalidPodName
	}
	if template.containerName == "" {
		return ContainerInfo{}, ErrInvalidContainerName
	}

	// Build metadata to store with the container.
	metadata := RuntimeContainerMetadata{
		PodName:       template.podName,
		PodID:         template.podID,
		ImageName:     template.userRequestedImage,
		ImageID:       string(template.imageID),
		ContainerName: template.containerName,
		MetadataName:  template.metadataName,
		UID:           template.uid,
		Namespace:     template.namespace,
		MountLabel:    "",
		// CreatedAt is set later
		Attempt: template.attempt,
		// Pod is set later
		Privileged: template.privileged,
	}
	if metadata.MetadataName == "" {
		metadata.MetadataName = metadata.ContainerName
	}

	metadata.Pod = (containerID == metadata.PodID) // Or should this be hard-coded in callers? The caller should know whether it is creating the infra container.
	metadata.CreatedAt = time.Now().Unix()
	mdata, err := json.Marshal(&metadata)
	if err != nil {
		return ContainerInfo{}, err
	}

	// Build the container.
	names := []string{metadata.ContainerName}
	if metadata.Pod {
		names = append(names, metadata.PodName)
	}

	container, err := createContainer(containerID, names, template.imageID, string(mdata), labelOptions)
	if err != nil {
		if metadata.Pod {
			logrus.Debugf("Failed to create pod sandbox %s(%s): %v", metadata.PodName, metadata.PodID, err)
		} else {
			logrus.Debugf("Failed to create container %s(%s): %v", metadata.ContainerName, containerID, err)
		}
		return ContainerInfo{}, err
	}
	if metadata.Pod {
		logrus.Debugf("Created pod sandbox %q", container.ID)
	} else {
		logrus.Debugf("Created container %q", container.ID)
	}

	// If anything fails after this point, we need to delete the incomplete
	// container before returning.
	defer func() {
		if retErr != nil {
			if err2 := libtc.Remove(container.ID, ss.root); err2 != nil {
				if metadata.Pod {
					logrus.Debugf("%v deleting partially-created pod sandbox %q", err2, container.ID)
				} else {
					logrus.Debugf("%v deleting partially-created container %q", err2, container.ID)
				}
				return
			}
			logrus.Debugf("Deleted partially-created container %q", container.ID)
		}
	}()

	// Find out where the container work directories are, so that we can return them.
	containerDir, err := ss.ContainerDirectory(container.ID)
	if err != nil {
		return ContainerInfo{}, err
	}
	if metadata.Pod {
		logrus.Debugf("Pod sandbox %q has work directory %q", container.ID, containerDir)
	} else {
		logrus.Debugf("Container %q has work directory %q", container.ID, containerDir)
	}

	containerRunDir, err := ss.ContainerRunDirectory(container.ID)
	if err != nil {
		return ContainerInfo{}, err
	}
	if metadata.Pod {
		logrus.Debugf("Pod sandbox %q has run directory %q", container.ID, containerRunDir)
	} else {
		logrus.Debugf("Container %q has run directory %q", container.ID, containerRunDir)
	}

	// TODO: generate imageConfig from template.imageID
	imageConfig := &v1.Image{}
	return ContainerInfo{
		ID:           container.ID,
		Dir:          containerDir,
		RunDir:       containerRunDir,
		Config:       imageConfig,
		ProcessLabel: container.ProcessLabel(),
		MountLabel:   container.MountLabel(),
	}, nil
}

// CreateContainer creates a new container, optionally with the
// specified ID (one will be assigned if none is specified), with
// optional names, using the specified image's top layer as the basis
// for the container's layer, and assigning the specified ID to that
// layer (one will be created if none is specified).  A container is a
// layer which is associated with additional bookkeeping information
// which the library stores for the convenience of its caller.
func createContainer(id string, names []string, bundleId bundle.BundleId, metadata string, labelOptions []string) (*Container, error) {
	return nil, nil
}

// DeleteContainer deletes a container, unmounting it first if need be.
// If there is no matching container, or if the container exists but its
// layer does not, an error will be returned.
func (ss *StorageService) DeleteContainer(ctx context.Context, idOrName string) error {
	ctx, span := log.StartSpan(ctx)
	defer span.End()
	if idOrName == "" {
		return ErrInvalidContainerID
	}
	container, err := ss.Container(idOrName)
	// Already deleted
	if errors.Is(err, ErrContainerUnknown) {
		return nil
	}
	if err != nil {
		return err
	}
	// TODO: Delete Container Here
	err = libtc.Remove(container.ID, ss.root)
	if err != nil {
		log.Debugf(ctx, "Failed to delete container %q: %v", container.ID, err)
		return err
	}
	return nil
}

// SetContainerMetadata updates the metadata we've stored for a container.
func (ss *StorageService) SetContainerMetadata(idOrName string, metadata *RuntimeContainerMetadata) error {
	mdata, err := json.Marshal(&metadata)
	if err != nil {
		logrus.Debugf("Failed to encode metadata for %q: %v", idOrName, err)
		return err
	}
	return ss.SetMetadata(idOrName, string(mdata))
}

// GetContainerMetadata returns the metadata we've stored for a container.
func (ss *StorageService) GetContainerMetadata(idOrName string) (RuntimeContainerMetadata, error) {
	metadata := RuntimeContainerMetadata{}
	mdata, err := ss.Metadata(idOrName)
	if err != nil {
		return metadata, err
	}
	if err := json.Unmarshal([]byte(mdata), &metadata); err != nil {
		return metadata, err
	}
	return metadata, nil
}

// StartContainer makes sure a container's filesystem is mounted, and
// returns the location of its root filesystem, which is not guaranteed
// by lower-level drivers to never change.
func (ss *StorageService) StartContainer(idOrName string) (string, error) {
	container, err := ss.Container(idOrName)
	if err != nil {
		if errors.Is(err, ErrContainerUnknown) {
			return "", ErrInvalidContainerID
		}
		return "", err
	}
	metadata := RuntimeContainerMetadata{}
	if err := json.Unmarshal([]byte(container.Metadata), &metadata); err != nil {
		return "", err
	}
	mountPoint, err := ss.Mount(container.ID, metadata.MountLabel)
	if err != nil {
		logrus.Debugf("Failed to mount container %q: %v", container.ID, err)
		return "", err
	}
	logrus.Debugf("Mounted container %q at %q", container.ID, mountPoint)
	return mountPoint, nil
}

// StopContainer attempts to unmount a container's root filesystem,
// freeing up any kernel resources which may be limited.
func (ss *StorageService) StopContainer(ctx context.Context, idOrName string) error {
	ctx, span := log.StartSpan(ctx)
	defer span.End()
	if idOrName == "" {
		return ErrInvalidContainerID
	}
	container, err := ss.Container(idOrName)
	if err != nil {
		if errors.Is(err, ErrContainerUnknown) {
			log.Infof(ctx, "Container %s not known, assuming it got already removed", idOrName)
			return nil
		}

		log.Warnf(ctx, "Failed to get container %s: %v", idOrName, err)
		return err
	}

	if _, err := ss.Unmount(container.ID, true); err != nil {
		if errors.Is(err, ErrLayerUnknown) {
			log.Infof(ctx, "Layer for container %s not known", container.ID)
			return nil
		}

		log.Warnf(ctx, "Failed to unmount container %s: %v", container.ID, err)
		return err
	}

	log.Debugf(ctx, "Unmounted container %s", container.ID)
	return nil
}

// GetWorkDir returns the path of a nonvolatile directory on the
// filesystem (somewhere under the Store's Root directory) which can be
// used to store arbitrary data that is specific to the container.  It
// will be removed automatically when the container is deleted.
func (ss *StorageService) GetWorkDir(id string) (string, error) {
	container, err := ss.Container(id)
	if err != nil {
		if errors.Is(err, ErrContainerUnknown) {
			return "", ErrInvalidContainerID
		}
		return "", err
	}
	return ss.ContainerDirectory(container.ID)
}

// GetRunDir returns the path of a volatile directory (does not survive
// the host rebooting, somewhere under the Store's RunRoot directory)
// on the filesystem which can be used to store arbitrary data that is
// specific to the container.  It will be removed automatically when
// the container is deleted.
func (ss *StorageService) GetRunDir(id string) (string, error) {
	container, err := ss.Container(id)
	if err != nil {
		if errors.Is(err, ErrContainerUnknown) {
			return "", ErrInvalidContainerID
		}
		return "", err
	}
	return ss.ContainerRunDirectory(container.ID)
}
