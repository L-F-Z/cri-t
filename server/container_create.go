package server

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/containers/storage/pkg/idtools"
	"github.com/containers/storage/pkg/mount"
	"github.com/containers/storage/pkg/stringid"
	securejoin "github.com/cyphar/filepath-securejoin"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
	rspec "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/opencontainers/runtime-tools/generate"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"

	"github.com/L-F-Z/cri-t/internal/factory/container"
	"github.com/L-F-Z/cri-t/internal/lib/sandbox"
	"github.com/L-F-Z/cri-t/internal/log"
	"github.com/L-F-Z/cri-t/internal/oci"
	"github.com/L-F-Z/cri-t/internal/resourcestore"
	"github.com/L-F-Z/cri-t/internal/storage"
	"github.com/L-F-Z/cri-t/pkg/config"
	"github.com/L-F-Z/cri-t/utils"
)

// sync with https://github.com/containers/storage/blob/7fe03f6c765f2adbc75a5691a1fb4f19e56e7071/pkg/truncindex/truncindex.go#L92
const noSuchID = "no such id"

type orderedMounts []rspec.Mount

// Len returns the number of mounts. Used in sorting.
func (m orderedMounts) Len() int {
	return len(m)
}

// Less returns true if the number of parts (a/b/c would be 3 parts) in the
// mount indexed by parameter 1 is less than that of the mount indexed by
// parameter 2. Used in sorting.
func (m orderedMounts) Less(i, j int) bool {
	return m.parts(i) < m.parts(j)
}

// Swap swaps two items in an array of mounts. Used in sorting.
func (m orderedMounts) Swap(i, j int) {
	m[i], m[j] = m[j], m[i]
}

// parts returns the number of parts in the destination of a mount. Used in sorting.
func (m orderedMounts) parts(i int) int {
	return strings.Count(filepath.Clean(m[i].Destination), string(os.PathSeparator))
}

// mounts defines how to sort runtime.Mount.
// This is the same with the Docker implementation:
//
//	https://github.com/moby/moby/blob/17.05.x/daemon/volumes.go#L26
type criOrderedMounts []*types.Mount

// Len returns the number of mounts. Used in sorting.
func (m criOrderedMounts) Len() int {
	return len(m)
}

// Less returns true if the number of parts (a/b/c would be 3 parts) in the
// mount indexed by parameter 1 is less than that of the mount indexed by
// parameter 2. Used in sorting.
func (m criOrderedMounts) Less(i, j int) bool {
	return m.parts(i) < m.parts(j)
}

// Swap swaps two items in an array of mounts. Used in sorting.
func (m criOrderedMounts) Swap(i, j int) {
	m[i], m[j] = m[j], m[i]
}

// parts returns the number of parts in the destination of a mount. Used in sorting.
func (m criOrderedMounts) parts(i int) int {
	return strings.Count(filepath.Clean(m[i].ContainerPath), string(os.PathSeparator))
}

// Ensure mount point on which path is mounted, is shared.
func ensureShared(path string, mountInfos []*mount.Info) error {
	sourceMount, optionalOpts, err := getSourceMount(path, mountInfos)
	if err != nil {
		return err
	}

	// Make sure source mount point is shared.
	optsSplit := strings.Split(optionalOpts, " ")
	for _, opt := range optsSplit {
		if strings.HasPrefix(opt, "shared:") {
			return nil
		}
	}

	return fmt.Errorf("path %q is mounted on %q but it is not a shared mount", path, sourceMount)
}

// Ensure mount point on which path is mounted, is either shared or slave.
func ensureSharedOrSlave(path string, mountInfos []*mount.Info) error {
	sourceMount, optionalOpts, err := getSourceMount(path, mountInfos)
	if err != nil {
		return err
	}
	// Make sure source mount point is shared.
	optsSplit := strings.Split(optionalOpts, " ")
	for _, opt := range optsSplit {
		if strings.HasPrefix(opt, "shared:") {
			return nil
		} else if strings.HasPrefix(opt, "master:") {
			return nil
		}
	}
	return fmt.Errorf("path %q is mounted on %q but it is not a shared or slave mount", path, sourceMount)
}

func addImageVolumes(ctx context.Context, rootfs string, s *Server, containerInfo *storage.ContainerInfo, mountLabel string, specgen *generate.Generator) ([]rspec.Mount, error) {
	ctx, span := log.StartSpan(ctx)
	defer span.End()

	mounts := []rspec.Mount{}
	for dest := range containerInfo.Config.Config.Volumes {
		fp, err := securejoin.SecureJoin(rootfs, dest)
		if err != nil {
			return nil, err
		}
		switch s.config.ImageVolumes {
		case config.ImageVolumesMkdir:
			IDs := idtools.IDPair{UID: int(specgen.Config.Process.User.UID), GID: int(specgen.Config.Process.User.GID)}
			if err1 := idtools.MkdirAllAndChownNew(fp, 0o755, IDs); err1 != nil {
				return nil, err1
			}
			if mountLabel != "" {
				if err1 := securityLabel(fp, mountLabel, true, false); err1 != nil {
					return nil, err1
				}
			}
		case config.ImageVolumesBind:
			volumeDirName := stringid.GenerateNonCryptoID()
			src := filepath.Join(containerInfo.RunDir, "mounts", volumeDirName)
			if err1 := os.MkdirAll(src, 0o755); err1 != nil {
				return nil, err1
			}
			// Label the source with the sandbox selinux mount label
			if mountLabel != "" {
				if err1 := securityLabel(src, mountLabel, true, false); err1 != nil {
					return nil, err1
				}
			}

			log.Debugf(ctx, "Adding bind mounted volume: %s to %s", src, dest)
			mounts = append(mounts, rspec.Mount{
				Source:      src,
				Destination: dest,
				Type:        "bind",
				Options:     []string{"private", "bind", "rw"},
			})

		case config.ImageVolumesIgnore:
			log.Debugf(ctx, "Ignoring volume %v", dest)
		default:
			log.Errorf(ctx, "Unrecognized image volumes setting")
		}
	}
	return mounts, nil
}

// resolveSymbolicLink resolves a possible symlink path. If the path is a symlink, returns resolved
// path; if not, returns the original path.
// note: strictly SecureJoin is not sufficient, as it does not error when a part of the path doesn't exist
// but simply moves on. If the last part of the path doesn't exist, it may need to be created.
func resolveSymbolicLink(scope, path string) (string, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return "", err
	}
	if info.Mode()&os.ModeSymlink != os.ModeSymlink {
		return path, nil
	}
	if scope == "" {
		scope = "/"
	}
	return securejoin.SecureJoin(scope, path)
}

// setupContainerUser sets the UID, GID and supplemental groups in OCI runtime config.
func setupContainerUser(ctx context.Context, specgen *generate.Generator, rootfs, mountLabel, ctrRunDir string, sc *types.LinuxContainerSecurityContext, imageConfig *v1.Image) error {
	ctx, span := log.StartSpan(ctx)
	defer span.End()

	if sc == nil {
		return nil
	}
	if sc.RunAsGroup != nil && sc.RunAsUser == nil && sc.RunAsUsername == "" {
		return errors.New("user group is specified without user or username")
	}
	imageUser := ""
	homedir := ""
	for _, env := range specgen.Config.Process.Env {
		if strings.HasPrefix(env, "HOME=") {
			homedir = strings.TrimPrefix(env, "HOME=")
			if idx := strings.Index(homedir, `\n`); idx > -1 {
				return errors.New("invalid HOME environment; newline not allowed")
			}
			break
		}
	}
	if homedir == "" {
		homedir = specgen.Config.Process.Cwd
	}

	if imageConfig != nil {
		imageUser = imageConfig.Config.User
	}
	containerUser := generateUserString(
		sc.RunAsUsername,
		imageUser,
		sc.RunAsUser,
	)
	log.Debugf(ctx, "CONTAINER USER: %+v", containerUser)

	// Add uid, gid and groups from user
	uid, gid, addGroups, err := utils.GetUserInfo(rootfs, containerUser)
	if err != nil {
		return err
	}

	genPasswd := true
	genGroup := true
	for _, mount := range specgen.Config.Mounts {
		switch mount.Destination {
		case "/etc", "/etc/":
			genPasswd = false
			genGroup = false
		case "/etc/passwd":
			genPasswd = false
		case "/etc/group":
			genGroup = false
		}

		if !genPasswd && !genGroup {
			break
		}
	}
	if genPasswd {
		// verify uid exists in containers /etc/passwd, else generate a passwd with the user entry
		passwdPath, err := utils.GeneratePasswd(containerUser, uid, gid, homedir, rootfs, ctrRunDir)
		if err != nil {
			return err
		}
		if passwdPath != "" {
			if err := securityLabel(passwdPath, mountLabel, false, false); err != nil {
				return err
			}

			mnt := rspec.Mount{
				Type:        "bind",
				Source:      passwdPath,
				Destination: "/etc/passwd",
				Options:     []string{"rw", "bind", "nodev", "nosuid", "noexec"},
			}
			specgen.AddMount(mnt)
		}
	}
	if genGroup {
		if sc.RunAsGroup != nil {
			gid = uint32(sc.RunAsGroup.Value)
		}

		// verify gid exists in containers /etc/group, else generate a group with the group entry
		groupPath, err := utils.GenerateGroup(gid, rootfs, ctrRunDir)
		if err != nil {
			return err
		}
		if groupPath != "" {
			if err := securityLabel(groupPath, mountLabel, false, false); err != nil {
				return err
			}
			specgen.AddMount(rspec.Mount{
				Type:        "bind",
				Source:      groupPath,
				Destination: "/etc/group",
				Options:     []string{"rw", "bind", "nodev", "nosuid", "noexec"},
			})
		}
	}

	specgen.SetProcessUID(uid)
	if sc.RunAsGroup != nil {
		gid = uint32(sc.RunAsGroup.Value)
	}
	specgen.SetProcessGID(gid)
	specgen.AddProcessAdditionalGid(gid)

	supplementalGroupsPolicy := sc.GetSupplementalGroupsPolicy()

	switch supplementalGroupsPolicy {
	case types.SupplementalGroupsPolicy_Merge:
		// Add groups from /etc/passwd and SupplementalGroups defined
		// in security context.
		for _, group := range addGroups {
			specgen.AddProcessAdditionalGid(group)
		}
		for _, group := range sc.SupplementalGroups {
			specgen.AddProcessAdditionalGid(uint32(group))
		}
	case types.SupplementalGroupsPolicy_Strict:
		// Don't merge group defined in /etc/passwd.
		for _, group := range sc.SupplementalGroups {
			specgen.AddProcessAdditionalGid(uint32(group))
		}

	default:
		return fmt.Errorf("not implemented in this CRI-O release: SupplementalGroupsPolicy=%v", supplementalGroupsPolicy)
	}

	return nil
}

// generateUserString generates valid user string based on OCI Image Spec v1.0.0.
func generateUserString(username, imageUser string, uid *types.Int64Value) string {
	var userstr string
	if uid != nil {
		userstr = strconv.FormatInt(uid.Value, 10)
	}
	if username != "" {
		userstr = username
	}
	// We use the user from the image config if nothing is provided
	if userstr == "" {
		userstr = imageUser
	}
	if userstr == "" {
		return ""
	}
	return userstr
}

// CreateContainer creates a new container in specified PodSandbox.
func (s *Server) CreateContainer(ctx context.Context, req *types.CreateContainerRequest) (res *types.CreateContainerResponse, retErr error) {
	if req.Config == nil {
		return nil, errors.New("config is nil")
	}
	if req.Config.Image == nil {
		return nil, errors.New("config image is nil")
	}
	if req.SandboxConfig == nil {
		return nil, errors.New("sandbox config is nil")
	}
	if req.SandboxConfig.Metadata == nil {
		return nil, errors.New("sandbox config metadata is nil")
	}

	log.Infof(ctx, "Creating container: %s", oci.LabelsToDescription(req.GetConfig().GetLabels()))

	sb, err := s.getPodSandboxFromRequest(ctx, req.PodSandboxId)
	if err != nil {
		if errors.Is(err, sandbox.ErrIDEmpty) {
			return nil, err
		}
		return nil, fmt.Errorf("specified sandbox not found: %s: %w", req.PodSandboxId, err)
	}

	stopMutex := sb.StopMutex()
	stopMutex.RLock()
	defer stopMutex.RUnlock()
	if sb.Stopped() {
		return nil, fmt.Errorf("CreateContainer failed as the sandbox was stopped: %s", sb.ID())
	}

	ctr, err := container.New()
	if err != nil {
		return nil, fmt.Errorf("failed to create container: %w", err)
	}

	if err := ctr.SetConfig(req.Config, req.SandboxConfig); err != nil {
		return nil, fmt.Errorf("setting container config: %w", err)
	}

	if err := ctr.SetNameAndID(""); err != nil {
		return nil, fmt.Errorf("setting container name and ID: %w", err)
	}

	resourceCleaner := resourcestore.NewResourceCleaner()
	// in some cases, it is still necessary to reserve container resources when an error occurs (such as just a request context timeout error)
	storeResource := false
	defer func() {
		// No errors or resource need to be stored, no need to cleanup
		if retErr == nil || storeResource {
			return
		}
		if err := resourceCleaner.Cleanup(); err != nil {
			log.Errorf(ctx, "Unable to cleanup: %v", err)
		}
	}()

	if _, err = s.ReserveContainerName(ctr.ID(), ctr.Name()); err != nil {
		reservedID, getErr := s.ContainerIDForName(ctr.Name())
		if getErr != nil {
			return nil, fmt.Errorf("failed to get ID of container with reserved name (%s), after failing to reserve name with %w: %w", ctr.Name(), getErr, getErr)
		}
		// if we're able to find the container, and it's created, this is actually a duplicate request
		// Just return that container
		if reservedCtr := s.GetContainer(ctx, reservedID); reservedCtr != nil && reservedCtr.Created() {
			return &types.CreateContainerResponse{ContainerId: reservedID}, nil
		}
		cachedID, resourceErr := s.getResourceOrWait(ctx, ctr.Name(), "container")
		if resourceErr == nil {
			return &types.CreateContainerResponse{ContainerId: cachedID}, nil
		}
		return nil, fmt.Errorf("%w: %w", resourceErr, err)
	}

	s.resourceStore.SetStageForResource(ctx, ctr.Name(), "container creating")

	resourceCleaner.Add(ctx, "createCtr: releasing container name "+ctr.Name(), func() error {
		s.ReleaseContainerName(ctx, ctr.Name())
		return nil
	})

	newContainer, err := s.createSandboxContainer(ctx, ctr, sb)
	if err != nil {
		return nil, err
	}
	resourceCleaner.Add(ctx, "createCtr: deleting container "+ctr.ID()+" from storage", func() error {
		if err := s.StorageService().DeleteContainer(ctx, ctr.ID()); err != nil {
			return fmt.Errorf("failed to cleanup container storage: %w", err)
		}
		return nil
	})

	s.addContainer(ctx, newContainer)
	resourceCleaner.Add(ctx, "createCtr: removing container "+newContainer.ID(), func() error {
		s.removeContainer(ctx, newContainer)
		return nil
	})

	if err := s.CtrIDIndex().Add(ctr.ID()); err != nil {
		return nil, err
	}
	resourceCleaner.Add(ctx, "createCtr: deleting container ID "+ctr.ID()+" from idIndex", func() error {
		if err := s.CtrIDIndex().Delete(ctr.ID()); err != nil && !strings.Contains(err.Error(), noSuchID) {
			return err
		}
		return nil
	})

	s.resourceStore.SetStageForResource(ctx, ctr.Name(), "container runtime creation")
	if err := s.createContainerPlatform(ctx, newContainer, sb.CgroupParent()); err != nil {
		return nil, err
	}
	resourceCleaner.Add(ctx, "createCtr: removing container ID "+ctr.ID()+" from runtime", func() error {
		if err := s.Runtime().DeleteContainer(ctx, newContainer); err != nil {
			return fmt.Errorf("failed to delete container in runtime %s: %w", ctr.ID(), err)
		}
		return nil
	})

	if err := s.ContainerStateToDisk(ctx, newContainer); err != nil {
		log.Warnf(ctx, "Unable to write containers %s state to disk: %v", newContainer.ID(), err)
	}

	if isContextError(ctx.Err()) {
		if err := s.resourceStore.Put(ctr.Name(), newContainer, resourceCleaner); err != nil {
			log.Errorf(ctx, "CreateCtr: failed to save progress of container %s: %v", newContainer.ID(), err)
		}
		log.Infof(ctx, "CreateCtr: context was either canceled or the deadline was exceeded: %v", ctx.Err())
		// should not cleanup
		storeResource = true
		return nil, ctx.Err()
	}

	// Since it's not a context error, we can delete the resource from the store, it will be tracked in the server from now on.
	s.resourceStore.Delete(ctr.Name())

	newContainer.SetCreated()

	if err := s.nri.postCreateContainer(ctx, sb, newContainer); err != nil {
		log.Warnf(ctx, "NRI post-create event failed for container %q: %v",
			newContainer.ID(), err)
	}
	s.generateCRIEvent(ctx, newContainer, types.ContainerEventType_CONTAINER_CREATED_EVENT)

	log.Infof(ctx, "Created container %s: %s", newContainer.ID(), newContainer.Description())
	return &types.CreateContainerResponse{
		ContainerId: ctr.ID(),
	}, nil
}

func isInCRIMounts(dst string, mounts []*types.Mount) bool {
	for _, m := range mounts {
		if m.ContainerPath == dst {
			return true
		}
	}
	return false
}
