package storage

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	json "github.com/json-iterator/go"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/sirupsen/logrus"

	"github.com/L-F-Z/TaskC/pkg/bundle"
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
	// ErrRootFsUnknown indicates that the RootFs does not exist
	ErrRootFsUnknown = errors.New("rootfs not known")
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
	ss.SetContainerMetadata(containerID, &metadata)

	id, rootFs, imgConfig, err := ss.bm.CreateContainerById(template.imageID)
	if err != nil {
		if metadata.Pod {
			logrus.Debugf("Failed to create pod sandbox %s(%s): %v", metadata.PodName, metadata.PodID, err)
		} else {
			logrus.Debugf("Failed to create container %s(%s): %v", metadata.ContainerName, containerID, err)
		}
		return ContainerInfo{}, err
	}
	if metadata.Pod {
		logrus.Debugf("Created pod sandbox %q", id)
	} else {
		logrus.Debugf("Created container %q", id)
	}

	// If anything fails after this point, we need to delete the incomplete
	// container before returning.
	defer func() {
		if retErr != nil {
			if err2 := ss.bm.DeleteContainer(id); err2 != nil {
				if metadata.Pod {
					logrus.Debugf("%v deleting partially-created pod sandbox %q", err2, id)
				} else {
					logrus.Debugf("%v deleting partially-created container %q", err2, id)
				}
				return
			}
			logrus.Debugf("Deleted partially-created container %q", id)
		}
	}()

	containerDir := filepath.Join(ss.work, id)
	err = os.MkdirAll(containerDir, 0o755)
	if err != nil {
		return ContainerInfo{}, err
	}
	if metadata.Pod {
		logrus.Debugf("Pod sandbox %q has work directory %q", id, containerDir)
	} else {
		logrus.Debugf("Container %q has work directory %q", id, containerDir)
	}

	containerRunDir := filepath.Join(ss.run, id)
	err = os.MkdirAll(containerRunDir, 0o755)
	if err != nil {
		return ContainerInfo{}, err
	}
	if metadata.Pod {
		logrus.Debugf("Pod sandbox %q has run directory %q", id, containerRunDir)
	} else {
		logrus.Debugf("Container %q has run directory %q", id, containerRunDir)
	}

	// TODO: generate imageConfig from template.imageID
	now := time.Now()
	imageConfig := &v1.Image{
		Created: &now,
		Config:  imgConfig,
	}
	return ContainerInfo{
		ID:           id,
		Dir:          containerDir,
		RunDir:       containerRunDir,
		RootFs:       rootFs,
		Config:       imageConfig,
		ProcessLabel: "",
		MountLabel:   "",
	}, nil
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
	err := ss.bm.DeleteContainer(idOrName)
	if err != nil {
		log.Debugf(ctx, "Failed to delete container %q: %v", idOrName, err)
		return err
	}
	err = ss.DeleteMetadata(idOrName)
	if err != nil {
		log.Debugf(ctx, "Failed to delete container metadata %q: %v", idOrName, err)
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

	metadataFile := filepath.Join(ss.meta, idOrName)
	err = os.WriteFile(metadataFile, mdata, 0o644)
	if err != nil {
		return fmt.Errorf("failed to write metadata file: %w", err)
	}
	return nil
}

// GetContainerMetadata returns the metadata we've stored for a container.
func (ss *StorageService) GetContainerMetadata(idOrName string) (RuntimeContainerMetadata, error) {
	metadata := RuntimeContainerMetadata{}
	metadataFile := filepath.Join(ss.meta, idOrName)
	_, err := os.Stat(metadataFile)
	if os.IsNotExist(err) {
		return metadata, ErrInvalidContainerID
	} else if err != nil {
		return metadata, fmt.Errorf("failed to stat metadata file: %w", err)
	}
	data, err := os.ReadFile(metadataFile)
	if err != nil {
		return metadata, fmt.Errorf("failed to read rootFs file: %w", err)
	}
	if err := json.Unmarshal(data, &metadata); err != nil {
		return metadata, err
	}
	return metadata, nil
}

func (ss *StorageService) DeleteMetadata(id string) error {
	metadataFile := filepath.Join(ss.meta, id)
	if _, err := os.Stat(metadataFile); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return fmt.Errorf("failed to stat metadata file: %w", err)
	}
	if err := os.Remove(metadataFile); err != nil {
		return fmt.Errorf("failed to delete metadata file: %w", err)
	}
	return nil
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
	path := filepath.Join(ss.work, id)
	_, err := os.Stat(path)
	return path, err
}

// ContainerRunDirectory returns a path of a directory which the
// caller can use to store data, specific to the container, which the
// library does not directly manage.  The directory will be deleted
// when the host system is restarted.
func (ss *StorageService) ContainerRunDirectory(id string) (string, error) {
	path := filepath.Join(ss.run, id)
	_, err := os.Stat(path)
	return path, err
}

func (ss *StorageService) GetUsage(id string) (bytesUsed uint64, inodeUsed uint64) {
	return 0, 0
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
