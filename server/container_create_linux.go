package server

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/containers/common/pkg/subscriptions"
	"github.com/containers/common/pkg/timezone"
	"github.com/containers/storage/pkg/idtools"
	"github.com/containers/storage/pkg/mount"
	securejoin "github.com/cyphar/filepath-securejoin"
	"github.com/intel/goresctrl/pkg/blockio"
	rspec "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/opencontainers/runtime-tools/generate"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
	kubeletTypes "k8s.io/kubelet/pkg/types"

	"github.com/L-F-Z/TaskC/pkg/bundle"
	"github.com/L-F-Z/cri-t/internal/config/device"
	"github.com/L-F-Z/cri-t/internal/config/node"
	"github.com/L-F-Z/cri-t/internal/config/rdt"
	ctrfactory "github.com/L-F-Z/cri-t/internal/factory/container"
	"github.com/L-F-Z/cri-t/internal/lib/sandbox"
	"github.com/L-F-Z/cri-t/internal/linklogs"
	"github.com/L-F-Z/cri-t/internal/log"
	oci "github.com/L-F-Z/cri-t/internal/oci"
	"github.com/L-F-Z/cri-t/internal/runtimehandlerhooks"
	crioann "github.com/L-F-Z/cri-t/pkg/annotations"
)

const (
	cgroupSysFsPath        = "/sys/fs/cgroup"
	cgroupSysFsSystemdPath = "/sys/fs/cgroup/systemd"
)

// createContainerPlatform performs platform dependent intermediate steps before calling the container's oci.Runtime().CreateContainer().
func (s *Server) createContainerPlatform(ctx context.Context, container *oci.Container, cgroupParent string) error {
	ctx, span := log.StartSpan(ctx)
	defer span.End()
	return s.Runtime().CreateContainer(ctx, container, cgroupParent, false)
}

func (s *Server) createSandboxContainer(ctx context.Context, ctr ctrfactory.Container, sb *sandbox.Sandbox) (cntr *oci.Container, retErr error) {
	ctx, span := log.StartSpan(ctx)
	defer span.End()
	// TODO: simplify this function (cyclomatic complexity here is high)
	// TODO: factor generating/updating the spec into something other projects can vendor

	// eventually, we'd like to access all of these variables through the interface themselves, and do most
	// of the translation between CRI config -> oci/storage container in the container package

	// TODO: eventually, this should be in the container package, but it's going through a lot of churn
	// and SpecAddAnnotations is already being passed too many arguments
	// Filter early so any use of the annotations don't use the wrong values
	if err := s.FilterDisallowedAnnotations(sb.Annotations(), ctr.Config().Annotations, sb.RuntimeHandler()); err != nil {
		return nil, err
	}

	containerID := ctr.ID()
	containerName := ctr.Name()
	containerConfig := ctr.Config()
	if err := ctr.SetPrivileged(); err != nil {
		return nil, err
	}
	setContainerConfigSecurityContext(containerConfig)
	securityContext := containerConfig.Linux.SecurityContext

	specgen := s.getSpecGen(ctr, containerConfig)

	// userRequestedImage is the way to locate the image.
	// When called by Kubelet, it is either the ImageRef as returned by PullImage
	// (for us, always a RegistryImageReference using a repo@digest), or an ImageID as returned by ImageStatus (a full StorageImageID).
	// We accept other inputs, like short names and digest prefixes, because previous CRI-O versions did, just to be conservative against breakage.
	userRequestedImage, err := ctr.UserRequestedImage()
	if err != nil {
		return nil, err
	}

	bundleName, err := bundle.ParseBundleName(userRequestedImage)
	if err != nil {
		return nil, err
	}

	imgResult, err := s.StorageService().ImageStatusByName(bundleName)
	if err != nil {
		return nil, err
	}

	imageID := bundle.BundleId(imgResult.Id)
	someRepoDigest := ""
	if len(imgResult.RepoDigests) > 0 {
		someRepoDigest = imgResult.RepoDigests[0]
	}
	// == Image lookup done.
	// == NEVER USE userRequestedImage (or even someNameOfTheImage) for anything but diagnostic logging past this point; it might
	// resolve to a different image.

	labelOptions, err := ctr.SelinuxLabel(sb.ProcessLabel())
	if err != nil {
		return nil, err
	}

	metadata := containerConfig.Metadata

	s.resourceStore.SetStageForResource(ctx, ctr.Name(), "container storage creation")
	containerInfo, err := s.StorageService().CreateContainer(
		sb.Name(), sb.ID(),
		userRequestedImage, imageID,
		containerName, containerID,
		metadata.Name,
		metadata.Attempt,
		labelOptions,
		ctr.Privileged(),
	)
	if err != nil {
		return nil, err
	}
	defer func() {
		if retErr != nil {
			log.Infof(ctx, "CreateCtrLinux: deleting container %s from storage", containerInfo.ID)
			if err := s.StorageService().DeleteContainer(ctx, containerInfo.ID); err != nil {
				log.Warnf(ctx, "Failed to cleanup container directory: %v", err)
			}
		}
	}()

	mountLabel := containerInfo.MountLabel
	var processLabel string
	if !ctr.Privileged() {
		processLabel = containerInfo.ProcessLabel
	}
	hostIPC := securityContext.NamespaceOptions.Ipc == types.NamespaceMode_NODE
	hostPID := securityContext.NamespaceOptions.Pid == types.NamespaceMode_NODE
	hostNet := securityContext.NamespaceOptions.Network == types.NamespaceMode_NODE

	// Don't use SELinux separation with Host Pid or IPC Namespace or privileged.
	if hostPID || hostIPC {
		processLabel, mountLabel = "", ""
	}

	if hostNet && s.config.RuntimeConfig.HostNetworkDisableSELinux {
		processLabel = ""
	}

	maybeRelabel := false
	if val, present := sb.Annotations()[crioann.TrySkipVolumeSELinuxLabelAnnotation]; present && val == "true" {
		maybeRelabel = true
	}

	skipRelabel := false
	const superPrivilegedType = "spc_t"
	if securityContext.SelinuxOptions.Type == superPrivilegedType || // super privileged container
		(ctr.SandboxConfig().Linux != nil &&
			ctr.SandboxConfig().Linux.SecurityContext != nil &&
			ctr.SandboxConfig().Linux.SecurityContext.SelinuxOptions != nil &&
			ctr.SandboxConfig().Linux.SecurityContext.SelinuxOptions.Type == superPrivilegedType && // super privileged pod
			securityContext.SelinuxOptions.Type == "") {
		skipRelabel = true
	}

	cgroup2RW := node.CgroupIsV2() && sb.Annotations()[crioann.Cgroup2RWAnnotation] == "true"

	s.resourceStore.SetStageForResource(ctx, ctr.Name(), "container volume configuration")
	idMapSupport := s.Runtime().RuntimeSupportsIDMap(sb.RuntimeHandler())
	rroSupport := s.Runtime().RuntimeSupportsRROMounts(sb.RuntimeHandler())
	containerVolumes, ociMounts, err := s.addOCIBindMounts(ctx, ctr, mountLabel, s.config.RuntimeConfig.BindMountPrefix, s.config.AbsentMountSourcesToReject, maybeRelabel, skipRelabel, cgroup2RW, idMapSupport, rroSupport, s.Config().Root)
	if err != nil {
		return nil, err
	}

	s.resourceStore.SetStageForResource(ctx, ctr.Name(), "container device creation")
	err = s.specSetDevices(ctr, sb)
	if err != nil {
		return nil, err
	}

	s.resourceStore.SetStageForResource(ctx, ctr.Name(), "container spec configuration")

	labels := containerConfig.Labels

	if err := validateLabels(labels); err != nil {
		return nil, err
	}

	err = s.specSetApparmorProfile(ctx, specgen, ctr, securityContext)
	if err != nil {
		return nil, err
	}

	err = s.specSetBlockioClass(specgen, metadata.Name, containerConfig.Annotations, sb.Annotations())
	if err != nil {
		log.Warnf(ctx, "Reconfiguring blockio for container %s failed: %v", containerID, err)
	}

	logPath, err := ctr.LogPath(sb.LogDir())
	if err != nil {
		return nil, err
	}

	specgen.SetProcessTerminal(containerConfig.Tty)
	if containerConfig.Tty {
		specgen.AddProcessEnv("TERM", "xterm")
	}

	linux := containerConfig.Linux
	if linux != nil {
		resources := linux.Resources
		if resources != nil {
			containerMinMemory, err := s.Runtime().GetContainerMinMemory(sb.RuntimeHandler())
			if err != nil {
				return nil, err
			}
			err = ctr.SpecSetLinuxContainerResources(resources, containerMinMemory)
			if err != nil {
				return nil, err
			}
		}

		specgen.SetLinuxCgroupsPath(s.config.CgroupManager().ContainerCgroupPath(sb.CgroupParent(), containerID))

		err = ctr.SpecSetPrivileges(ctx, securityContext, &s.config)
		if err != nil {
			return nil, err
		}
	}

	if err := ctr.AddUnifiedResourcesFromAnnotations(sb.Annotations()); err != nil {
		return nil, err
	}

	var nsTargetCtr *oci.Container
	if target := securityContext.NamespaceOptions.TargetId; target != "" {
		nsTargetCtr = s.GetContainer(ctx, target)
	}

	if err := ctr.SpecAddNamespaces(sb, nsTargetCtr, &s.config); err != nil {
		return nil, err
	}

	defer func() {
		if retErr != nil && ctr.PidNamespace() != nil {
			log.Infof(ctx, "CreateCtrLinux: clearing PID namespace for container %s", containerInfo.ID)
			if err := ctr.PidNamespace().Remove(); err != nil {
				log.Warnf(ctx, "Failed to remove PID namespace: %v", err)
			}
		}
	}()

	// If the sandbox is configured to run in the host network, do not create a new network namespace
	if hostNet {
		if !isInCRIMounts("/sys", containerConfig.Mounts) {
			ctr.SpecAddMount(rspec.Mount{
				Destination: "/sys",
				Type:        "sysfs",
				Source:      "sysfs",
				Options:     []string{"nosuid", "noexec", "nodev", "ro"},
			})
			ctr.SpecAddMount(rspec.Mount{
				Destination: cgroupSysFsPath,
				Type:        "cgroup",
				Source:      "cgroup",
				Options:     []string{"nosuid", "noexec", "nodev", "relatime", "ro"},
			})
		}
	}

	if ctr.Privileged() {
		ctr.SpecAddMount(rspec.Mount{
			Destination: "/sys",
			Type:        "sysfs",
			Source:      "sysfs",
			Options:     []string{"nosuid", "noexec", "nodev", "rw", "rslave"},
		})
		ctr.SpecAddMount(rspec.Mount{
			Destination: cgroupSysFsPath,
			Type:        "cgroup",
			Source:      "cgroup",
			Options:     []string{"nosuid", "noexec", "nodev", "rw", "relatime", "rslave"},
		})
	}

	containerImageConfig := containerInfo.Config
	if containerImageConfig == nil {
		err = fmt.Errorf("empty image config for %s", userRequestedImage)
		return nil, err
	}

	if err := ctr.SpecSetProcessArgs(containerImageConfig); err != nil {
		return nil, err
	}

	// When running on cgroupv2, automatically add a cgroup namespace for not privileged containers.
	if !ctr.Privileged() && node.CgroupIsV2() {
		if err := specgen.AddOrReplaceLinuxNamespace(string(rspec.CgroupNamespace), ""); err != nil {
			return nil, err
		}
	}

	ctr.SpecAddMount(rspec.Mount{
		Destination: "/dev/shm",
		Type:        "bind",
		Source:      sb.ShmPath(),
		Options:     []string{"rw", "bind"},
	})

	options := []string{"rw"}
	if ctr.ReadOnly(s.config.ReadOnly) {
		options = []string{"ro"}
	}
	if sb.ResolvPath() != "" {
		if err := securityLabel(sb.ResolvPath(), mountLabel, false, false); err != nil {
			return nil, err
		}
		ctr.SpecAddMount(rspec.Mount{
			Destination: "/etc/resolv.conf",
			Type:        "bind",
			Source:      sb.ResolvPath(),
			Options:     append(options, []string{"bind", "nodev", "nosuid", "noexec"}...),
		})
	}

	if sb.HostnamePath() != "" {
		if err := securityLabel(sb.HostnamePath(), mountLabel, false, false); err != nil {
			return nil, err
		}
		ctr.SpecAddMount(rspec.Mount{
			Destination: "/etc/hostname",
			Type:        "bind",
			Source:      sb.HostnamePath(),
			Options:     append(options, "bind"),
		})
	}

	if sb.ContainerEnvPath() != "" {
		if err := securityLabel(sb.ContainerEnvPath(), mountLabel, false, false); err != nil {
			return nil, err
		}
		ctr.SpecAddMount(rspec.Mount{
			Destination: "/run/.containerenv",
			Type:        "bind",
			Source:      sb.ContainerEnvPath(),
			Options:     append(options, "bind"),
		})
	}

	if !isInCRIMounts("/etc/hosts", containerConfig.Mounts) && hostNet {
		// Only bind mount for host netns and when CRI does not give us any hosts file
		ctr.SpecAddMount(rspec.Mount{
			Destination: "/etc/hosts",
			Type:        "bind",
			Source:      "/etc/hosts",
			Options:     append(options, "bind"),
		})
	}

	if ctr.Privileged() {
		setOCIBindMountsPrivileged(specgen)
	}

	// Set hostname and add env for hostname
	specgen.SetHostname(sb.Hostname())
	specgen.AddProcessEnv("HOSTNAME", sb.Hostname())

	created := time.Now()
	seccompRef := types.SecurityProfile_Unconfined.String()

	if err := s.FilterDisallowedAnnotations(sb.Annotations(), imgResult.Spec.Annotations, sb.RuntimeHandler()); err != nil {
		return nil, fmt.Errorf("filter image annotations: %w", err)
	}

	if s.config.Seccomp().IsDisabled() {
		specgen.Config.Linux.Seccomp = nil
	}

	if !ctr.Privileged() {
		notifier, ref, err := s.config.Seccomp().Setup(
			ctx,
			s.seccompNotifierChan,
			containerID,
			ctr.Config().Metadata.Name,
			sb.Annotations(),
			imgResult.Spec.Annotations,
			specgen,
			securityContext.Seccomp,
		)
		if err != nil {
			return nil, fmt.Errorf("setup seccomp: %w", err)
		}
		if notifier != nil {
			s.seccompNotifiers.Store(containerID, notifier)
		}
		seccompRef = ref
	}

	// Get RDT class
	rdtClass, err := s.Config().Rdt().ContainerClassFromAnnotations(metadata.Name, containerConfig.Annotations, sb.Annotations())
	if err != nil {
		return nil, err
	}
	if rdtClass != "" {
		log.Debugf(ctx, "Setting RDT ClosID of container %s to %q", containerID, rdt.ResctrlPrefix+rdtClass)
		// TODO: patch runtime-tools to support setting ClosID via a helper func similar to SetLinuxIntelRdtL3CacheSchema()
		specgen.Config.Linux.IntelRdt = &rspec.LinuxIntelRdt{ClosID: rdt.ResctrlPrefix + rdtClass}
	}
	// compute the runtime path for a given container
	platform := containerInfo.Config.Platform.OS + "/" + containerInfo.Config.Platform.Architecture
	runtimePath, err := s.Runtime().PlatformRuntimePath(sb.RuntimeHandler(), platform)
	if err != nil {
		return nil, err
	}
	err = ctr.SpecAddAnnotations(ctx, sb, containerVolumes, containerInfo.RootFs, containerImageConfig.Config.StopSignal, imgResult, s.config.CgroupManager().IsSystemd(), seccompRef, runtimePath)
	if err != nil {
		return nil, err
	}

	if err := s.config.Workloads.MutateSpecGivenAnnotations(ctr.Config().Metadata.Name, ctr.Spec(), sb.Annotations()); err != nil {
		return nil, err
	}

	// First add any configured environment variables from crio config.
	// They will get overridden if specified in the image or container config.
	specgen.AddMultipleProcessEnv(s.Config().DefaultEnv)

	// Add environment variables from image the CRI configuration
	envs := mergeEnvs(containerImageConfig, containerConfig.Envs)
	for _, e := range envs {
		parts := strings.SplitN(e, "=", 2)
		specgen.AddProcessEnv(parts[0], parts[1])
	}

	// Setup user and groups
	if linux != nil {
		if err := setupContainerUser(ctx, specgen, containerInfo.RootFs, mountLabel, containerInfo.RunDir, securityContext, containerImageConfig); err != nil {
			return nil, err
		}
	}

	// Add image volumes
	volumeMounts, err := addImageVolumes(ctx, containerInfo.RootFs, s, &containerInfo, mountLabel, specgen)
	if err != nil {
		return nil, err
	}

	// Set working directory
	// Pick it up from image config first and override if specified in CRI
	containerCwd := "/"
	imageCwd := containerImageConfig.Config.WorkingDir
	if imageCwd != "" {
		containerCwd = imageCwd
	}
	runtimeCwd := containerConfig.WorkingDir
	if runtimeCwd != "" {
		containerCwd = runtimeCwd
	}
	specgen.SetProcessCwd(containerCwd)
	if err := setupWorkingDirectory(containerInfo.RootFs, mountLabel, containerCwd); err != nil {
		return nil, err
	}

	rootUID, rootGID := 0, 0

	// Add secrets from the default and override mounts.conf files
	secretMounts := subscriptions.MountsWithUIDGID(
		mountLabel,
		containerInfo.RunDir,
		s.config.DefaultMountsFile,
		containerInfo.RootFs,
		rootUID,
		rootGID,
		false, // not rootless
		ctr.DisableFips(),
	)

	if ctr.DisableFips() && sb.Annotations()[crioann.DisableFIPSAnnotation] == "true" {
		if err := disableFipsForContainer(ctr, containerInfo.RunDir); err != nil {
			return nil, fmt.Errorf("failed to disable FIPS for container %s: %w", containerID, err)
		}
	}

	mounts := []rspec.Mount{}
	mounts = append(mounts, ociMounts...)
	mounts = append(mounts, volumeMounts...)
	mounts = append(mounts, secretMounts...)

	sort.Sort(orderedMounts(mounts))

	for _, m := range mounts {
		rspecMount := rspec.Mount{
			Type:        "bind",
			Options:     append(m.Options, "bind"),
			Destination: m.Destination,
			Source:      m.Source,
			UIDMappings: m.UIDMappings,
			GIDMappings: m.GIDMappings,
		}
		ctr.SpecAddMount(rspecMount)
	}

	if ctr.WillRunSystemd() {
		processLabel, err = InitLabel(processLabel)
		if err != nil {
			return nil, err
		}
		setupSystemd(specgen.Mounts(), *specgen)
	}

	if s.ContainerServer.Hooks != nil {
		newAnnotations := map[string]string{}
		for key, value := range containerConfig.Annotations {
			newAnnotations[key] = value
		}
		for key, value := range sb.Annotations() {
			newAnnotations[key] = value
		}

		if _, err := s.ContainerServer.Hooks.Hooks(specgen.Config, newAnnotations, len(containerConfig.Mounts) > 0); err != nil {
			return nil, err
		}
	}

	// Set up pids limit if pids cgroup is mounted
	if node.CgroupHasPid() {
		specgen.SetLinuxResourcesPidsLimit(s.config.PidsLimit)
	}

	// by default, the root path is an empty string. set it now.
	specgen.SetRootPath(containerInfo.RootFs)

	crioAnnotations := specgen.Config.Annotations

	criMetadata := &types.ContainerMetadata{
		Name:    metadata.Name,
		Attempt: metadata.Attempt,
	}
	ociContainer, err := oci.NewContainer(containerID, containerName, containerInfo.RunDir, logPath, labels, crioAnnotations, ctr.Config().Annotations, userRequestedImage, &bundleName, &imageID, someRepoDigest, criMetadata, sb.ID(), containerConfig.Tty, containerConfig.Stdin, containerConfig.StdinOnce, sb.RuntimeHandler(), containerInfo.Dir, created, containerImageConfig.Config.StopSignal)
	if err != nil {
		return nil, err
	}

	specgen.SetLinuxMountLabel(mountLabel)
	specgen.SetProcessSelinuxLabel(processLabel)

	ociContainer.AddManagedPIDNamespace(ctr.PidNamespace())

	if err := specgen.RemoveLinuxNamespace(string(rspec.UserNamespace)); err != nil {
		return nil, err
	}
	if v := sb.Annotations()[crioann.UmaskAnnotation]; v != "" {
		umaskRegexp := regexp.MustCompile(`^[0-7]{1,4}$`)
		if !umaskRegexp.MatchString(v) {
			return nil, fmt.Errorf("invalid umask string %s", v)
		}
		decVal, err := strconv.ParseUint(sb.Annotations()[crioann.UmaskAnnotation], 8, 32)
		if err != nil {
			return nil, err
		}
		umask := uint32(decVal)
		specgen.Config.Process.User.Umask = &umask
	}

	etcPath := filepath.Join(containerInfo.RootFs, "/etc")

	// Warn users if the container /etc directory path points to a location
	// that is not a regular directory. This could indicate that something
	// might be afoot.
	etc, err := os.Lstat(etcPath)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	if err == nil && !etc.IsDir() {
		log.Warnf(ctx, "Detected /etc path for container %s is not a directory", ctr.ID())
	}

	// The /etc directory can be subjected to various attempts on the path (directory)
	// traversal attacks. As such, we need to ensure that its path will be relative to
	// the base (or root, if you wish) of the container to mitigate a container escape.
	etcPath, err = securejoin.SecureJoin(containerInfo.RootFs, "/etc")
	if err != nil {
		return nil, fmt.Errorf("failed to resolve container /etc directory path: %w", err)
	}

	// Create the /etc directory only when it doesn't exist.
	if _, err := os.Stat(etcPath); err != nil && os.IsNotExist(err) {
		rootPair := idtools.IDPair{UID: 0, GID: 0}
		if err := idtools.MkdirAllAndChown(etcPath, 0o755, rootPair); err != nil {
			return nil, fmt.Errorf("failed to create container /etc directory: %w", err)
		}
	}

	// Add a symbolic link from /proc/mounts to /etc/mtab to keep compatibility with legacy
	// Linux distributions and Docker.
	//
	// We cannot use SecureJoin here, as the /etc/mtab can already be symlinked from somewhere
	// else in some cases, and doing so would resolve an existing mtab path to the symbolic
	// link target location, for example, the /etc/proc/self/mounts, which breaks container
	// creation.
	if err := os.Symlink("/proc/mounts", filepath.Join(etcPath, "mtab")); err != nil && !os.IsExist(err) {
		return nil, err
	}

	// Configure timezone for the container if it is set.
	if err := configureTimezone(s.Runtime().Timezone(), ociContainer.BundlePath(), containerInfo.RootFs, mountLabel, etcPath, ociContainer.ID(), options, ctr); err != nil {
		return nil, fmt.Errorf("failed to configure timezone for container %s: %w", ociContainer.ID(), err)
	}

	if os.Getenv(rootlessEnvName) != "" {
		makeOCIConfigurationRootless(specgen)
	}

	hooks, err := runtimehandlerhooks.GetRuntimeHandlerHooks(ctx, &s.config, sb.RuntimeHandler(), sb.Annotations())
	if err != nil {
		return nil, fmt.Errorf("failed to get runtime handler %q hooks", sb.RuntimeHandler())
	}

	if err := s.nri.createContainer(ctx, specgen, sb, ociContainer); err != nil {
		return nil, err
	}

	defer func() {
		if retErr != nil {
			s.nri.undoCreateContainer(ctx, specgen, sb, ociContainer)
		}
	}()

	if hooks != nil {
		if err := hooks.PreCreate(ctx, specgen, sb, ociContainer); err != nil {
			return nil, fmt.Errorf("failed to run pre-create hook for container %q: %w", ociContainer.ID(), err)
		}
	}

	if emptyDirVolName, ok := sb.Annotations()[crioann.LinkLogsAnnotation]; ok {
		if err := linklogs.LinkContainerLogs(ctx, sb.Labels()[kubeletTypes.KubernetesPodUIDLabel], emptyDirVolName, ctr.ID(), containerConfig.Metadata); err != nil {
			log.Warnf(ctx, "Failed to link container logs: %v", err)
		}
	}

	saveOptions := generate.ExportOptions{}
	if err := specgen.SaveToFile(filepath.Join(containerInfo.Dir, "config.json"), saveOptions); err != nil {
		return nil, err
	}

	if err := specgen.SaveToFile(filepath.Join(containerInfo.RunDir, "config.json"), saveOptions); err != nil {
		return nil, err
	}

	ociContainer.SetSpec(specgen.Config)
	ociContainer.SetMountPoint(containerInfo.RootFs)
	ociContainer.SetSeccompProfilePath(seccompRef)
	if runtimePath != "" {
		ociContainer.SetRuntimePathForPlatform(runtimePath)
	}

	for _, cv := range containerVolumes {
		ociContainer.AddVolume(cv)
	}

	return ociContainer, nil
}

// this function takes a container config and makes sure its SecurityContext
// is not nil. If it is, it makes sure to set default values for every field.
func setContainerConfigSecurityContext(containerConfig *types.ContainerConfig) {
	if containerConfig.Linux == nil {
		containerConfig.Linux = &types.LinuxContainerConfig{}
	}
	if containerConfig.Linux.SecurityContext == nil {
		containerConfig.Linux.SecurityContext = newLinuxContainerSecurityContext()
	}
	if containerConfig.Linux.SecurityContext.NamespaceOptions == nil {
		containerConfig.Linux.SecurityContext.NamespaceOptions = &types.NamespaceOption{}
	}
	if containerConfig.Linux.SecurityContext.SelinuxOptions == nil {
		containerConfig.Linux.SecurityContext.SelinuxOptions = &types.SELinuxOption{}
	}
}

func disableFipsForContainer(ctr ctrfactory.Container, containerDir string) error {
	// Create a unique filename for the FIPS setting file.
	fileName := filepath.Join(containerDir, "sysctl-fips")
	content := []byte("0\n")

	// Write the value '0' to disable FIPS directly to the file.
	if err := os.WriteFile(fileName, content, 0o444); err != nil {
		return fmt.Errorf("failed to write to file: %w", err)
	}
	ctr.SpecAddMount(rspec.Mount{
		Destination: "/proc/sys/crypto/fips_enabled",
		Source:      fileName,
		Type:        "bind",
		Options:     []string{"noexec", "nosuid", "nodev", "ro", "bind"},
	})

	return nil
}

func configureTimezone(tz, containerRunDir, mountPoint, mountLabel, etcPath, containerID string, options []string, ctr ctrfactory.Container) error {
	localTimePath, err := timezone.ConfigureContainerTimeZone(tz, containerRunDir, mountPoint, etcPath, containerID)
	if err != nil {
		return fmt.Errorf("setting timezone for container %s: %w", containerID, err)
	}
	if localTimePath != "" {
		if err := securityLabel(localTimePath, mountLabel, false, false); err != nil {
			return err
		}
		ctr.SpecAddMount(rspec.Mount{
			Destination: "/etc/localtime",
			Type:        "bind",
			Source:      localTimePath,
			Options:     append(options, []string{"bind", "nodev", "nosuid", "noexec"}...),
		})
	}
	return nil
}

func setupWorkingDirectory(rootfs, mountLabel, containerCwd string) error {
	fp, err := securejoin.SecureJoin(rootfs, containerCwd)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(fp, 0o755); err != nil {
		return err
	}
	if mountLabel != "" {
		if err1 := securityLabel(fp, mountLabel, false, false); err1 != nil {
			return err1
		}
	}
	return nil
}

func setOCIBindMountsPrivileged(g *generate.Generator) {
	spec := g.Config
	// clear readonly for /sys and cgroup
	for i := range spec.Mounts {
		clearReadOnly(&spec.Mounts[i])
	}
	spec.Linux.ReadonlyPaths = nil
	spec.Linux.MaskedPaths = nil
}

func clearReadOnly(m *rspec.Mount) {
	var opt []string
	for _, o := range m.Options {
		if o == "rw" {
			return
		} else if o != "ro" {
			opt = append(opt, o)
		}
	}
	m.Options = opt
	m.Options = append(m.Options, "rw")
}

func (s *Server) addOCIBindMounts(ctx context.Context, ctr ctrfactory.Container, mountLabel, bindMountPrefix string, absentMountSourcesToReject []string, maybeRelabel, skipRelabel, cgroup2RW, idMapSupport, rroSupport bool, storageRoot string) ([]oci.ContainerVolume, []rspec.Mount, error) {
	ctx, span := log.StartSpan(ctx)
	defer span.End()

	volumes := []oci.ContainerVolume{}
	ociMounts := []rspec.Mount{}
	containerConfig := ctr.Config()
	specgen := ctr.Spec()
	mounts := containerConfig.Mounts

	// Sort mounts in number of parts. This ensures that high level mounts don't
	// shadow other mounts.
	sort.Sort(criOrderedMounts(mounts))

	// Copy all mounts from default mounts, except for
	// - mounts overridden by supplied mount;
	// - all mounts under /dev if a supplied /dev is present.
	mountSet := make(map[string]struct{})
	for _, m := range mounts {
		mountSet[filepath.Clean(m.ContainerPath)] = struct{}{}
	}
	defaultMounts := specgen.Mounts()
	specgen.ClearMounts()
	for _, m := range defaultMounts {
		dst := filepath.Clean(m.Destination)
		if _, ok := mountSet[dst]; ok {
			// filter out mount overridden by a supplied mount
			continue
		}
		if _, mountDev := mountSet["/dev"]; mountDev && strings.HasPrefix(dst, "/dev/") {
			// filter out everything under /dev if /dev is a supplied mount
			continue
		}
		if _, mountSys := mountSet["/sys"]; mountSys && strings.HasPrefix(dst, "/sys/") {
			// filter out everything under /sys if /sys is a supplied mount
			continue
		}
		specgen.AddMount(m)
	}

	mountInfos, err := mount.GetMounts()
	if err != nil {
		return nil, nil, err
	}

	imageVolumesPath, err := s.ensureImageVolumesPath(ctx, mounts)
	if err != nil {
		return nil, nil, fmt.Errorf("ensure image volumes path: %w", err)
	}

	for _, m := range mounts {
		dest := m.ContainerPath
		if dest == "" {
			return nil, nil, errors.New("mount.ContainerPath is empty")
		}
		if m.Image != nil && m.Image.Image != "" {
			volume, err := s.mountImage(ctx, specgen, imageVolumesPath, m)
			if err != nil {
				return nil, nil, fmt.Errorf("mount image: %w", err)
			}
			volumes = append(volumes, *volume)
			continue
		}
		if m.HostPath == "" {
			return nil, nil, errors.New("mount.HostPath is empty")
		}
		if m.HostPath == "/" && dest == "/" {
			log.Warnf(ctx, "Configuration specifies mounting host root to the container root.  This is dangerous (especially with privileged containers) and should be avoided.")
		}

		if isSubDirectoryOf(storageRoot, m.HostPath) && m.Propagation == types.MountPropagation_PROPAGATION_PRIVATE {
			log.Infof(ctx, "Mount propogration for the host path %s will be set to HostToContainer as it includes the container storage root", m.HostPath)
			m.Propagation = types.MountPropagation_PROPAGATION_HOST_TO_CONTAINER
		}

		src := filepath.Join(bindMountPrefix, m.HostPath)

		resolvedSrc, err := resolveSymbolicLink(bindMountPrefix, src)
		if err == nil {
			src = resolvedSrc
		} else {
			if !os.IsNotExist(err) {
				return nil, nil, fmt.Errorf("failed to resolve symlink %q: %w", src, err)
			}
			for _, toReject := range absentMountSourcesToReject {
				if filepath.Clean(src) == toReject {
					// special-case /etc/hostname, as we don't want it to be created as a directory
					// This can cause issues with node reboot.
					return nil, nil, fmt.Errorf("cannot mount %s: path does not exist and will cause issues as a directory", toReject)
				}
			}
			if !ctr.Restore() {
				// Although this would also be really helpful for restoring containers
				// it is problematic as during restore external bind mounts need to be
				// a file if the destination is a file. Unfortunately it is not easy
				// to tell if the destination is a file or a directory. Especially if
				// the destination is a nested bind mount. For now we will just not
				// create the missing bind mount source for restore and return an
				// error to the user.
				if err = os.MkdirAll(src, 0o755); err != nil {
					return nil, nil, fmt.Errorf("failed to mkdir %s: %w", src, err)
				}
			}
		}

		options := []string{"rbind"}

		// mount propagation
		switch m.Propagation {
		case types.MountPropagation_PROPAGATION_PRIVATE:
			options = append(options, "rprivate")
			// Since default root propagation in runc is rprivate ignore
			// setting the root propagation
		case types.MountPropagation_PROPAGATION_BIDIRECTIONAL:
			if err := ensureShared(src, mountInfos); err != nil {
				return nil, nil, err
			}
			options = append(options, "rshared")
			if err := specgen.SetLinuxRootPropagation("rshared"); err != nil {
				return nil, nil, err
			}
		case types.MountPropagation_PROPAGATION_HOST_TO_CONTAINER:
			if err := ensureSharedOrSlave(src, mountInfos); err != nil {
				return nil, nil, err
			}
			options = append(options, "rslave")
			if specgen.Config.Linux.RootfsPropagation != "rshared" &&
				specgen.Config.Linux.RootfsPropagation != "rslave" {
				if err := specgen.SetLinuxRootPropagation("rslave"); err != nil {
					return nil, nil, err
				}
			}
		default:
			log.Warnf(ctx, "Unknown propagation mode for hostPath %q", m.HostPath)
			options = append(options, "rprivate")
		}

		// Recursive Read-only (RRO) support requires the mount to be
		// read-only and the mount propagation set to private.
		switch {
		case m.RecursiveReadOnly && m.Readonly:
			if !rroSupport {
				return nil, nil, fmt.Errorf(
					"recursive read-only mount support is not available for hostPath %q",
					m.HostPath,
				)
			}
			if m.Propagation != types.MountPropagation_PROPAGATION_PRIVATE {
				return nil, nil, fmt.Errorf(
					"recursive read-only mount requires private propagation for hostPath %q, got: %s",
					m.HostPath, m.Propagation,
				)
			}
			options = append(options, "rro")
		case m.RecursiveReadOnly:
			return nil, nil, fmt.Errorf(
				"recursive read-only mount conflicts with read-write mount for hostPath %q",
				m.HostPath,
			)
		case m.Readonly:
			options = append(options, "ro")
		default:
			options = append(options, "rw")
		}

		if m.SelinuxRelabel {
			if skipRelabel {
				log.Debugf(ctx, "Skipping relabel for %s because of super privileged container (type: spc_t)", src)
			} else if err := securityLabel(src, mountLabel, false, maybeRelabel); err != nil {
				return nil, nil, err
			}
		} else {
			log.Debugf(ctx, "Skipping relabel for %s because kubelet did not request it", src)
		}

		volumes = append(volumes, oci.ContainerVolume{
			ContainerPath:     dest,
			HostPath:          src,
			Readonly:          m.Readonly,
			RecursiveReadOnly: m.RecursiveReadOnly,
			Propagation:       m.Propagation,
			SelinuxRelabel:    m.SelinuxRelabel,
			Image:             m.Image,
		})

		uidMappings := getOCIMappings(m.UidMappings)
		gidMappings := getOCIMappings(m.GidMappings)
		if (uidMappings != nil || gidMappings != nil) && !idMapSupport {
			return nil, nil, errors.New("idmap mounts specified but OCI runtime does not support them. Perhaps the OCI runtime is too old")
		}
		ociMounts = append(ociMounts, rspec.Mount{
			Source:      src,
			Destination: dest,
			Options:     options,
			UIDMappings: uidMappings,
			GIDMappings: gidMappings,
		})
	}

	if _, mountSys := mountSet["/sys"]; !mountSys {
		m := rspec.Mount{
			Destination: cgroupSysFsPath,
			Type:        "cgroup",
			Source:      "cgroup",
			Options:     []string{"nosuid", "noexec", "nodev", "relatime"},
		}

		if cgroup2RW {
			m.Options = append(m.Options, "rw")
		} else {
			m.Options = append(m.Options, "ro")
		}
		specgen.AddMount(m)
	}

	return volumes, ociMounts, nil
}

// mountImage adds required image mounts to the provided spec generator and returns a corresponding ContainerVolume.
func (s *Server) mountImage(ctx context.Context, specgen *generate.Generator, imageVolumesPath string, m *types.Mount) (*oci.ContainerVolume, error) {
	if m == nil || m.Image == nil || m.Image.Image == "" || m.ContainerPath == "" {
		return nil, fmt.Errorf("invalid mount specified: %+v", m)
	}

	log.Debugf(ctx, "Image ref to mount: %s", m.Image.Image)
	status, err := s.storageImageStatus(ctx, types.ImageSpec{Image: m.Image.Image})
	if err != nil {
		return nil, fmt.Errorf("get storage image status: %w", err)
	}

	if status == nil {
		// Should not happen because the kubelet ensures the image.
		return nil, fmt.Errorf("image %q does not exist locally", m.Image.Image)
	}

	imageID := status.Id
	log.Debugf(ctx, "Image ID to mount: %v", imageID)

	options := []string{"ro", "noexec", "nosuid", "nodev"}
	mountPoint, err := s.StorageService().MountImage(imageID, options, "")
	if err != nil {
		return nil, fmt.Errorf("mount storage: %w", err)
	}
	log.Infof(ctx, "Image mounted to: %s", mountPoint)

	const overlay = "overlay"
	specgen.AddMount(rspec.Mount{
		Type:        overlay,
		Source:      overlay,
		Destination: m.ContainerPath,
		Options: []string{
			"lowerdir=" + mountPoint + ":" + imageVolumesPath,
		},
		UIDMappings: getOCIMappings(m.UidMappings),
		GIDMappings: getOCIMappings(m.GidMappings),
	})
	log.Debugf(ctx, "Added overlay mount from %s to %s", mountPoint, imageVolumesPath)

	return &oci.ContainerVolume{
		ContainerPath:     m.ContainerPath,
		HostPath:          mountPoint,
		Readonly:          m.Readonly,
		RecursiveReadOnly: m.RecursiveReadOnly,
		Propagation:       m.Propagation,
		SelinuxRelabel:    m.SelinuxRelabel,
		Image:             &types.ImageSpec{Image: imageID},
	}, nil
}

func (s *Server) ensureImageVolumesPath(ctx context.Context, mounts []*types.Mount) (string, error) {
	// Check if we need to anything at all
	noop := true
	for _, m := range mounts {
		if m.Image != nil && m.Image.Image != "" {
			noop = false
			break
		}
	}

	if noop {
		return "", nil
	}

	imageVolumesPath := filepath.Join(filepath.Dir(s.Config().ContainerExitsDir), "image-volumes")
	log.Debugf(ctx, "Using image volumes path: %s", imageVolumesPath)

	if err := os.MkdirAll(imageVolumesPath, 0o700); err != nil {
		return "", fmt.Errorf("create image volumes path: %w", err)
	}

	f, err := os.Open(imageVolumesPath)
	if err != nil {
		return "", fmt.Errorf("open image volumes path %s: %w", imageVolumesPath, err)
	}

	_, readErr := f.ReadDir(1)
	if readErr != nil && !errors.Is(readErr, io.EOF) {
		return "", fmt.Errorf("unable to read dir names of image volumes path %s: %w", imageVolumesPath, err)
	}
	if readErr == nil {
		return "", fmt.Errorf("image volumes path %s is not empty", imageVolumesPath)
	}

	return imageVolumesPath, nil
}

func getOCIMappings(m []*types.IDMapping) []rspec.LinuxIDMapping {
	if len(m) == 0 {
		return nil
	}
	ids := make([]rspec.LinuxIDMapping, 0, len(m))
	for _, m := range m {
		ids = append(ids, rspec.LinuxIDMapping{
			ContainerID: m.ContainerId,
			HostID:      m.HostId,
			Size:        m.Length,
		})
	}
	return ids
}

// mountExists returns true if dest exists in the list of mounts.
func mountExists(specMounts []rspec.Mount, dest string) bool {
	for _, m := range specMounts {
		if m.Destination == dest {
			return true
		}
	}
	return false
}

// systemd expects to have /run, /run/lock and /tmp on tmpfs
// It also expects to be able to write to /sys/fs/cgroup/systemd and /var/log/journal.
func setupSystemd(mounts []rspec.Mount, g generate.Generator) {
	options := []string{"rw", "rprivate", "noexec", "nosuid", "nodev"}
	for _, dest := range []string{"/run", "/run/lock"} {
		if mountExists(mounts, dest) {
			continue
		}
		tmpfsMnt := rspec.Mount{
			Destination: dest,
			Type:        "tmpfs",
			Source:      "tmpfs",
			Options:     append(options, "tmpcopyup"),
		}
		g.AddMount(tmpfsMnt)
	}
	for _, dest := range []string{"/tmp", "/var/log/journal"} {
		if mountExists(mounts, dest) {
			continue
		}
		tmpfsMnt := rspec.Mount{
			Destination: dest,
			Type:        "tmpfs",
			Source:      "tmpfs",
			Options:     append(options, "tmpcopyup"),
		}
		g.AddMount(tmpfsMnt)
	}

	if node.CgroupIsV2() {
		g.RemoveMount(cgroupSysFsPath)

		systemdMnt := rspec.Mount{
			Destination: cgroupSysFsPath,
			Type:        "cgroup",
			Source:      "cgroup",
			Options:     []string{"private", "rw"},
		}
		g.AddMount(systemdMnt)
	} else {
		// If the /sys/fs/cgroup is bind mounted from the host,
		// then systemd-mode cgroup should be disabled
		// https://bugzilla.redhat.com/show_bug.cgi?id=2064741
		if !hasCgroupMount(g.Mounts()) {
			systemdMnt := rspec.Mount{
				Destination: cgroupSysFsSystemdPath,
				Type:        "bind",
				Source:      cgroupSysFsSystemdPath,
				Options:     []string{"bind", "nodev", "noexec", "nosuid"},
			}
			g.AddMount(systemdMnt)
		}
		g.AddLinuxMaskedPaths(filepath.Join(cgroupSysFsSystemdPath, "release_agent"))
	}
	g.AddProcessEnv("container", "crio")
}

func hasCgroupMount(mounts []rspec.Mount) bool {
	for _, m := range mounts {
		if (m.Destination == cgroupSysFsPath || m.Destination == "/sys/fs" || m.Destination == "/sys") && isBindMount(m.Options) {
			return true
		}
	}
	return false
}

func isBindMount(mountOptions []string) bool {
	for _, option := range mountOptions {
		if option == "bind" || option == "rbind" {
			return true
		}
	}
	return false
}

func newLinuxContainerSecurityContext() *types.LinuxContainerSecurityContext {
	return &types.LinuxContainerSecurityContext{
		Capabilities:     &types.Capability{},
		NamespaceOptions: &types.NamespaceOption{},
		SelinuxOptions:   &types.SELinuxOption{},
		RunAsUser:        &types.Int64Value{},
		RunAsGroup:       &types.Int64Value{},
		Seccomp:          &types.SecurityProfile{},
		Apparmor:         &types.SecurityProfile{},
	}
}

// isSubDirectoryOf checks if the base path contains the target path.
// It assumes that paths are Unix-style with forward slashes ("/").
// It ensures that both paths end with a "/" before comparing, so that "/var/lib" will not incorrectly match "/var/libs".

// The function returns true if the base path starts with the target path, providing a way to check if one directory is a subdirectory of another.

// Examples:

// isSubDirectoryOf("/var/lib/containers/storage", "/") returns true
// isSubDirectoryOf("/var/lib/containers/storage", "/var/lib") returns true
// isSubDirectoryOf("/var/lib/containers/storage", "/var/lib/containers") returns true
// isSubDirectoryOf("/var/lib/containers/storage", "/var/lib/containers/storage") returns true
// isSubDirectoryOf("/var/lib/containers/storage", "/var/lib/containers/storage/extra") returns false
// isSubDirectoryOf("/var/lib/containers/storage", "/va") returns false
// isSubDirectoryOf("/var/lib/containers/storage", "/var/tmp/containers") returns false.
func isSubDirectoryOf(base, target string) bool {
	if !strings.HasSuffix(target, "/") {
		target += "/"
	}
	if !strings.HasSuffix(base, "/") {
		base += "/"
	}
	return strings.HasPrefix(base, target)
}

// Returns the spec Generator for the container, with some values set.
func (s *Server) getSpecGen(ctr ctrfactory.Container, containerConfig *types.ContainerConfig) *generate.Generator {
	specgen := ctr.Spec()
	specgen.HostSpecific = true
	specgen.ClearProcessRlimits()

	for _, u := range s.config.Ulimits() {
		specgen.AddProcessRlimits(u.Name, u.Hard, u.Soft)
	}

	specgen.SetRootReadonly(ctr.ReadOnly(s.config.ReadOnly))

	if s.config.ReadOnly {
		// tmpcopyup is a runc extension and is not part of the OCI spec.
		// WORK ON: Use "overlay" mounts as an alternative to tmpfs with tmpcopyup
		// Look at https://github.com/L-F-Z/cri-t/pull/1434#discussion_r177200245 for more info on this
		options := []string{"rw", "noexec", "nosuid", "nodev", "tmpcopyup"}
		mounts := map[string]string{
			"/run":     "mode=0755",
			"/tmp":     "mode=1777",
			"/var/tmp": "mode=1777",
		}
		for target, mode := range mounts {
			if !isInCRIMounts(target, containerConfig.Mounts) {
				ctr.SpecAddMount(rspec.Mount{
					Destination: target,
					Type:        "tmpfs",
					Source:      "tmpfs",
					Options:     append(options, mode),
				})
			}
		}
	}

	return specgen
}

func (s *Server) specSetApparmorProfile(ctx context.Context, specgen *generate.Generator, ctr ctrfactory.Container, securityContext *types.LinuxContainerSecurityContext) error {
	// set this container's apparmor profile if it is set by sandbox
	if s.Config().AppArmor().IsEnabled() && !ctr.Privileged() {
		profile, err := s.Config().AppArmor().Apply(securityContext)
		if err != nil {
			return fmt.Errorf("applying apparmor profile to container %s: %w", ctr.ID(), err)
		}

		log.Debugf(ctx, "Applied AppArmor profile %s to container %s", profile, ctr.ID())
		specgen.SetProcessApparmorProfile(profile)
	}
	return nil
}

func (s *Server) specSetBlockioClass(specgen *generate.Generator, containerName string, containerAnnotations, sandboxAnnotations map[string]string) error {
	// Get blockio class
	if s.Config().BlockIO().Enabled() {
		if blockioClass, err := blockio.ContainerClassFromAnnotations(containerName, containerAnnotations, sandboxAnnotations); blockioClass != "" && err == nil {
			if s.Config().BlockIO().ReloadRequired() {
				if err := s.Config().BlockIO().Reload(); err != nil {
					return err
				}
			}
			if linuxBlockIO, err := blockio.OciLinuxBlockIO(blockioClass); err == nil {
				if specgen.Config.Linux.Resources == nil {
					specgen.Config.Linux.Resources = &rspec.LinuxResources{}
				}
				specgen.Config.Linux.Resources.BlockIO = linuxBlockIO
			}
		}
	}
	return nil
}

func (s *Server) specSetDevices(ctr ctrfactory.Container, sb *sandbox.Sandbox) error {
	configuredDevices := s.config.Devices()

	privilegedWithoutHostDevices, err := s.Runtime().PrivilegedWithoutHostDevices(sb.RuntimeHandler())
	if err != nil {
		return err
	}

	annotationDevices, err := device.DevicesFromAnnotation(sb.Annotations()[crioann.DevicesAnnotation], s.config.AllowedDevices)
	if err != nil {
		return err
	}

	return ctr.SpecAddDevices(configuredDevices, annotationDevices, privilegedWithoutHostDevices, s.config.DeviceOwnershipFromSecurityContext)
}
