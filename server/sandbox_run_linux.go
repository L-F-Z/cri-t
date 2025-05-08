package server

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	cnitypes "github.com/containernetworking/cni/pkg/types"
	current "github.com/containernetworking/cni/pkg/types/100"
	json "github.com/json-iterator/go"
	spec "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/opencontainers/runtime-tools/generate"
	"github.com/opencontainers/selinux/go-selinux/label"
	"golang.org/x/sys/unix"
	"k8s.io/apimachinery/pkg/api/resource"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
	kubeletTypes "k8s.io/kubelet/pkg/types"

	"github.com/L-F-Z/cri-t/internal/config/nsmgr"
	ctrfactory "github.com/L-F-Z/cri-t/internal/factory/container"
	"github.com/L-F-Z/cri-t/internal/lib/constants"
	libsandbox "github.com/L-F-Z/cri-t/internal/lib/sandbox"
	"github.com/L-F-Z/cri-t/internal/linklogs"
	"github.com/L-F-Z/cri-t/internal/log"
	"github.com/L-F-Z/cri-t/internal/memorystore"
	oci "github.com/L-F-Z/cri-t/internal/oci"
	"github.com/L-F-Z/cri-t/internal/resourcestore"
	"github.com/L-F-Z/cri-t/internal/runtimehandlerhooks"
	"github.com/L-F-Z/cri-t/internal/storage"
	"github.com/L-F-Z/cri-t/pkg/annotations"
	libconfig "github.com/L-F-Z/cri-t/pkg/config"
	"github.com/L-F-Z/cri-t/utils"
)

func (s *Server) runPodSandbox(ctx context.Context, req *types.RunPodSandboxRequest) (resp *types.RunPodSandboxResponse, retErr error) {
	ctx, span := log.StartSpan(ctx)
	defer span.End()
	sbox := libsandbox.NewBuilder()
	if err := sbox.SetConfig(req.Config); err != nil {
		return nil, fmt.Errorf("setting sandbox config: %w", err)
	}

	kubeName := sbox.Config().Metadata.Name
	kubePodUID := sbox.Config().Metadata.Uid
	namespace := sbox.Config().Metadata.Namespace

	sbox.SetNamespace(namespace)
	sbox.SetKubeName(kubeName)
	sbox.SetContainers(memorystore.New[*oci.Container]())

	attempt := sbox.Config().Metadata.Attempt

	// These fields are populated by the Kubelet, but not crictl. Populate if needed.
	sbox.Config().Labels = populateSandboxLabels(sbox.Config().Labels, kubeName, kubePodUID, namespace)
	// we need to fill in the container name, as it is not present in the request. Luckily, it is a constant.
	log.Infof(ctx, "Running pod sandbox: %s%s", oci.LabelsToDescription(sbox.Config().Labels), oci.InfraContainerName)

	if err := sbox.GenerateNameAndID(); err != nil {
		return nil, fmt.Errorf("setting pod sandbox name and id: %w", err)
	}
	sboxID := sbox.ID()
	sboxName := sbox.Name()

	sbox.SetName(sboxName)
	resourceCleaner := resourcestore.NewResourceCleaner()
	// in some cases, it is still necessary to reserve container resources when an error occurs (such as just a request context timeout error)
	storeResource := false
	defer func() {
		// no error or resource need to be stored, no need to cleanup
		if retErr == nil || storeResource {
			return
		}
		if err := resourceCleaner.Cleanup(); err != nil {
			log.Errorf(ctx, "Unable to cleanup: %v", err)
		}
	}()

	if _, err := s.ReservePodName(sboxID, sboxName); err != nil {
		reservedID, getErr := s.PodIDForName(sboxName)
		if getErr != nil {
			return nil, fmt.Errorf("failed to get ID of pod with reserved name (%s), after failing to reserve name with %w: %w", sboxName, getErr, getErr)
		}
		// if we're able to find the sandbox, and it's created, this is actually a duplicate request
		// Just return that sandbox
		if reservedSbox := s.GetSandbox(reservedID); reservedSbox != nil && reservedSbox.Created() {
			return &types.RunPodSandboxResponse{PodSandboxId: reservedID}, nil
		}
		cachedID, resourceErr := s.getResourceOrWait(ctx, sboxName, "sandbox")
		if resourceErr == nil {
			return &types.RunPodSandboxResponse{PodSandboxId: cachedID}, nil
		}
		return nil, fmt.Errorf("%w: %w", resourceErr, err)
	}
	resourceCleaner.Add(ctx, "runSandbox: releasing pod sandbox name: "+sboxName, func() error {
		s.ReleasePodName(sboxName)
		return nil
	})

	// TODO: Pass interface instead of individual field.
	s.resourceStore.SetStageForResource(ctx, sboxName, "sandbox creating")

	securityContext := sbox.Config().Linux.SecurityContext

	if securityContext.NamespaceOptions == nil {
		securityContext.NamespaceOptions = &types.NamespaceOption{}
	}
	hostNetwork := securityContext.NamespaceOptions.Network == types.NamespaceMode_NODE
	sbox.SetHostNetwork(hostNetwork)

	if !hostNetwork {
		if err := s.waitForCNIPlugin(ctx, sboxName); err != nil {
			return nil, err
		}
	}

	// TODO: Pass interface instead of individual field.
	s.resourceStore.SetStageForResource(ctx, sboxName, "sandbox network ready")

	// validate the runtime handler
	runtimeHandler, err := s.runtimeHandler(req)
	if err != nil {
		return nil, err
	}
	sbox.SetRuntimeHandler(runtimeHandler)

	defaultAnnotations, err := s.Runtime().RuntimeDefaultAnnotations(runtimeHandler)
	if err != nil {
		return nil, err
	}
	kubeAnnotations := map[string]string{}
	// Deep copy to prevent writing to the same map in the config
	for k, v := range defaultAnnotations {
		kubeAnnotations[k] = v
	}

	if err := s.FilterDisallowedAnnotations(sbox.Config().Annotations, sbox.Config().Annotations, runtimeHandler); err != nil {
		return nil, err
	}

	// override default annotations with pod spec specified ones
	for k, v := range sbox.Config().Annotations {
		if _, ok := kubeAnnotations[k]; ok {
			log.Debugf(ctx, "Overriding default pod annotation %s for pod %s", k, sbox.ID())
		}
		kubeAnnotations[k] = v
	}

	usernsMode := kubeAnnotations[annotations.UsernsModeAnnotation]
	if usernsMode != "" {
		log.Warnf(ctx, "Annotation 'io.kubernetes.cri-o.userns-mode' is deprecated, and will be replaced with native Kubernetes support for user namespaces in the future")
	}
	sbox.SetUsernsMode(usernsMode)

	containerName, err := s.ReserveSandboxContainerIDAndName(sbox.Config())
	if err != nil {
		return nil, err
	}
	resourceCleaner.Add(ctx, "runSandbox: releasing container name: "+containerName, func() error {
		s.ReleaseContainerName(ctx, containerName)
		return nil
	})

	var labelOptions []string
	selinuxConfig := securityContext.SelinuxOptions
	if selinuxConfig != nil {
		labelOptions = utils.GetLabelOptions(selinuxConfig)
	}

	privileged := s.privilegedSandbox(req)
	sbox.SetPrivileged(privileged)

	// TODO: Pass interface instead of individual field.
	s.resourceStore.SetStageForResource(ctx, sboxName, "sandbox storage creation")
	pauseImage := s.config.ParsePauseImage()
	podContainer, err := s.StorageService().CreatePodSandbox(
		sboxName, sboxID,
		pauseImage,
		containerName,
		kubeName,
		sbox.Config().Metadata.Uid,
		namespace,
		attempt,
		labelOptions,
		privileged,
	)
	if errors.Is(err, storage.ErrDuplicateName) {
		return nil, fmt.Errorf("pod sandbox with name %q already exists", sboxName)
	}
	if err != nil {
		return nil, fmt.Errorf("creating pod sandbox with name %q: %w", sboxName, err)
	}
	resourceCleaner.Add(ctx, "runSandbox: removing pod sandbox from storage: "+sboxID, func() error {
		return s.StorageService().DeleteContainer(ctx, sboxID)
	})

	mountLabel := podContainer.MountLabel
	processLabel := podContainer.ProcessLabel
	sbox.SetProcessLabel(processLabel)
	sbox.SetMountLabel(mountLabel)

	// set log directory
	logDir := sbox.Config().LogDirectory
	if logDir == "" {
		logDir = filepath.Join(s.config.LogDir, sboxID)
	}
	// This should always be absolute from k8s.
	if !filepath.IsAbs(logDir) {
		return nil, fmt.Errorf("requested logDir for sbuilder ID %s is a relative path: %s", sboxID, logDir)
	}
	if err := os.MkdirAll(logDir, 0o700); err != nil {
		return nil, err
	}
	sbox.SetLogDir(logDir)

	// TODO: factor generating/updating the spec into something other projects can vendor.
	if err := sbox.InitInfraContainer(&s.config, &podContainer); err != nil {
		return nil, err
	}

	// add metadata
	metadata := sbox.Config().Metadata
	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return nil, err
	}

	// add labels
	labels := sbox.Config().Labels

	if err := validateLabels(labels); err != nil {
		return nil, err
	}

	// Add special container name label for the infra container
	if labels != nil {
		labels[kubeletTypes.KubernetesContainerNameLabel] = oci.InfraContainerName
	}
	labelsJSON, err := json.Marshal(labels)
	if err != nil {
		return nil, err
	}

	// add annotations
	kubeAnnotationsJSON, err := json.Marshal(kubeAnnotations)
	if err != nil {
		return nil, err
	}

	nsOptsJSON, err := json.Marshal(securityContext.NamespaceOptions)
	if err != nil {
		return nil, err
	}

	hostIPC := securityContext.NamespaceOptions.Ipc == types.NamespaceMode_NODE
	hostPID := securityContext.NamespaceOptions.Pid == types.NamespaceMode_NODE

	// Don't use SELinux separation with Host Pid or IPC Namespace or privileged.
	if hostPID || hostIPC {
		processLabel, mountLabel = "", ""
	}
	g := sbox.Spec()
	g.SetProcessSelinuxLabel(processLabel)
	g.SetLinuxMountLabel(mountLabel)

	// Remove the default /dev/shm mount to ensure we overwrite it
	g.RemoveMount(libsandbox.DevShmPath)

	// create shm mount for the pod containers.
	// TODO: Pass interface instead of individual field.
	s.resourceStore.SetStageForResource(ctx, sboxName, "sandbox shm creation")
	var shmPath string
	if hostIPC {
		shmPath = libsandbox.DevShmPath
	} else {
		shmSize := int64(libsandbox.DefaultShmSize)
		if shmSizeStr, ok := kubeAnnotations[annotations.ShmSizeAnnotation]; ok {
			quantity, err := resource.ParseQuantity(shmSizeStr)
			if err != nil {
				return nil, fmt.Errorf("failed to parse shm size '%s': %w", shmSizeStr, err)
			}
			shmSize = quantity.Value()
		}
		shmPath, err = libsandbox.SetupShm(podContainer.RunDir, mountLabel, shmSize)
		if err != nil {
			return nil, err
		}
		resourceCleaner.Add(ctx, "runSandbox: unmounting shmPath for sandbox "+sboxID, func() error {
			if err := unix.Unmount(shmPath, unix.MNT_DETACH); err != nil {
				return fmt.Errorf("failed to unmount shm for sandbox: %w", err)
			}
			return nil
		})
	}
	sbox.SetShmPath(shmPath)

	// Link logs if requested
	if emptyDirVolName, ok := kubeAnnotations[annotations.LinkLogsAnnotation]; ok {
		if err = linklogs.MountPodLogs(ctx, kubePodUID, emptyDirVolName, namespace, kubeName, mountLabel); err != nil {
			log.Warnf(ctx, "Failed to link logs: %v", err)
		}
	}

	// TODO: Pass interface instead of individual field.
	s.resourceStore.SetStageForResource(ctx, sboxName, "sandbox spec configuration")

	mnt := spec.Mount{
		Type:        "bind",
		Source:      shmPath,
		Destination: libsandbox.DevShmPath,
		Options:     []string{"rw", "bind"},
	}
	// bind mount the pod shm
	g.AddMount(mnt)

	err = s.setPodSandboxMountLabel(ctx, sboxID, mountLabel)
	if err != nil {
		return nil, err
	}

	if err := s.CtrIDIndex().Add(sboxID); err != nil {
		return nil, err
	}
	resourceCleaner.Add(ctx, "runSandbox: deleting container ID from idIndex for sandbox "+sboxID, func() error {
		if err := s.CtrIDIndex().Delete(sboxID); err != nil && !strings.Contains(err.Error(), noSuchID) {
			return fmt.Errorf("could not delete ctr id %s from idIndex: %w", sboxID, err)
		}
		return nil
	})

	// set log path inside log directory
	logPath := filepath.Join(logDir, sboxID+".log")

	// Handle https://issues.k8s.io/44043
	if err := utils.EnsureSaneLogPath(logPath); err != nil {
		return nil, err
	}

	hostname, err := getHostname(sboxID, sbox.Config().Hostname, hostNetwork)
	if err != nil {
		return nil, err
	}
	sbox.SetHostname(hostname)
	g.SetHostname(hostname)

	g.AddAnnotation(annotations.Metadata, string(metadataJSON))
	g.AddAnnotation(annotations.Labels, string(labelsJSON))
	g.AddAnnotation(annotations.Annotations, string(kubeAnnotationsJSON))
	g.AddAnnotation(annotations.LogPath, logPath)
	g.AddAnnotation(annotations.Name, sboxName)
	g.AddAnnotation(annotations.SandboxName, sboxName)
	g.AddAnnotation(annotations.Namespace, namespace)
	g.AddAnnotation(annotations.ContainerType, annotations.ContainerTypeSandbox)
	g.AddAnnotation(annotations.SandboxID, sboxID)
	g.AddAnnotation(annotations.UserRequestedImage, pauseImage.String())
	g.AddAnnotation(annotations.SomeNameOfTheImage, pauseImage.String())
	g.AddAnnotation(annotations.ContainerName, containerName)
	g.AddAnnotation(annotations.ContainerID, sboxID)
	g.AddAnnotation(annotations.ShmPath, shmPath)
	g.AddAnnotation(annotations.PrivilegedRuntime, strconv.FormatBool(privileged))
	g.AddAnnotation(annotations.RuntimeHandler, runtimeHandler)
	g.AddAnnotation(annotations.ResolvPath, sbox.ResolvPath())
	g.AddAnnotation(annotations.HostName, hostname)
	g.AddAnnotation(annotations.NamespaceOptions, string(nsOptsJSON))
	g.AddAnnotation(annotations.KubeName, kubeName)
	g.AddAnnotation(annotations.HostNetwork, strconv.FormatBool(hostNetwork))
	g.AddAnnotation(annotations.ContainerManager, constants.ContainerManagerCRIO)
	if podContainer.Config.Config.StopSignal != "" {
		// this key is defined in image-spec conversion document at https://github.com/opencontainers/image-spec/pull/492/files#diff-8aafbe2c3690162540381b8cdb157112R57
		g.AddAnnotation("org.opencontainers.image.stopSignal", podContainer.Config.Config.StopSignal)
	}

	created := time.Now()
	sbox.SetCreatedAt(created)
	err = sbox.SetCRISandbox(sboxID, labels, kubeAnnotations, metadata)
	if err != nil {
		return nil, err
	}
	g.AddAnnotation(annotations.Created, created.Format(time.RFC3339Nano))

	portMappings := convertPortMappings(sbox.Config().PortMappings)
	portMappingsJSON, err := json.Marshal(portMappings)
	if err != nil {
		return nil, err
	}
	sbox.SetPortMappings(portMappings)

	g.AddAnnotation(annotations.PortMappings, string(portMappingsJSON))
	containerMinMemory, err := s.Runtime().GetContainerMinMemory(runtimeHandler)
	if err != nil {
		return nil, err
	}
	cgroupParent, cgroupPath, err := s.config.CgroupManager().SandboxCgroupPath(sbox.Config().Linux.CgroupParent, sboxID, containerMinMemory)
	if err != nil {
		return nil, err
	}
	if cgroupPath != "" {
		g.SetLinuxCgroupsPath(cgroupPath)
	}

	sbox.SetCgroupParent(cgroupParent)
	g.AddAnnotation(annotations.CgroupParent, cgroupParent)

	overhead := sbox.Config().GetLinux().GetOverhead()
	overheadJSON, err := json.Marshal(overhead)
	if err != nil {
		return nil, err
	}
	sbox.SetPodLinuxOverhead(overhead)
	g.AddAnnotation(annotations.PodLinuxOverhead, string(overheadJSON))

	resources := sbox.Config().GetLinux().GetResources()
	resourcesJSON, err := json.Marshal(resources)
	if err != nil {
		return nil, err
	}
	sbox.SetPodLinuxResources(resources)
	g.AddAnnotation(annotations.PodLinuxResources, string(resourcesJSON))

	seccompRef := types.SecurityProfile_Unconfined.String()
	if !privileged {
		_, ref, err := s.config.Seccomp().Setup(
			ctx,
			nil,
			"",
			"",
			nil,
			nil,
			g,
			securityContext.Seccomp,
		)
		if err != nil {
			return nil, fmt.Errorf("setup seccomp: %w", err)
		}
		seccompRef = ref
	}

	hostnamePath := podContainer.RunDir + "/hostname"
	sbox.SetResolvPath(sbox.ResolvPath())
	sbox.SetDNSConfig(sbox.Config().DnsConfig)
	sbox.SetHostnamePath(hostnamePath)
	sbox.SetNamespaceOptions(securityContext.NamespaceOptions)
	sbox.SetSeccompProfilePath(seccompRef)

	sb, err := sbox.GetSandbox()
	if err != nil {
		return nil, err
	}
	if err := s.addSandbox(ctx, sb); err != nil {
		return nil, err
	}
	resourceCleaner.Add(ctx, "runSandbox: removing pod sandbox "+sboxID, func() error {
		if err := s.removeSandbox(ctx, sboxID); err != nil {
			return fmt.Errorf("could not remove pod sandbox: %w", err)
		}
		return nil
	})

	if err := s.PodIDIndex().Add(sboxID); err != nil {
		return nil, err
	}
	resourceCleaner.Add(ctx, "runSandbox: deleting pod ID "+sboxID+" from idIndex", func() error {
		if err := s.PodIDIndex().Delete(sboxID); err != nil && !strings.Contains(err.Error(), noSuchID) {
			return fmt.Errorf("could not delete pod id %s from idIndex: %w", sboxID, err)
		}
		return nil
	})

	for k, v := range kubeAnnotations {
		g.AddAnnotation(k, v)
	}
	for k, v := range labels {
		g.AddAnnotation(k, v)
	}

	// Add default sysctls given in crio.conf
	sysctls := s.configureGeneratorForSysctls(ctx, g, hostNetwork, hostIPC, req.Config.Linux.Sysctls)

	// set up namespaces
	// TODO: Pass interface instead of individual field.
	s.resourceStore.SetStageForResource(ctx, sboxName, "sandbox namespace creation")
	nsCleanupFuncs, err := s.configureGeneratorForSandboxNamespaces(ctx, hostNetwork, hostIPC, hostPID, sysctls, sb, g)
	// We want to cleanup after ourselves if we are managing any namespaces and fail in this function.
	// However, we don't immediately register this func with resourceCleaner because we need to pair the
	// ns cleanup with networkStop. Otherwise, we could try to cleanup the namespace before the network stop runs,
	// which could put us in a weird state.
	nsCleanupDescription := "runSandbox: cleaning up namespaces after failing to run sandbox " + sboxID
	nsCleanupFunc := func() error {
		for idx := range nsCleanupFuncs {
			if err := nsCleanupFuncs[idx](); err != nil {
				return fmt.Errorf("RunSandbox: failed to cleanup namespace %w", err)
			}
		}
		return nil
	}
	if err != nil {
		resourceCleaner.Add(ctx, nsCleanupDescription, nsCleanupFunc)
		return nil, err
	}

	// now that we have the namespaces, we should create the network if we're managing namespace Lifecycle
	var ips []string
	var result cnitypes.Result

	// TODO: Pass interface instead of individual field.
	s.resourceStore.SetStageForResource(ctx, sboxName, "sandbox network creation")
	ips, result, err = s.networkStart(ctx, sb)
	if err != nil {
		resourceCleaner.Add(ctx, nsCleanupDescription, nsCleanupFunc)
		return nil, err
	}
	resourceCleaner.Add(ctx, "runSandbox: stopping network for sandbox"+sb.ID(), func() error {
		// use a new context to prevent an expired context from preventing a stop
		if err := s.networkStop(context.Background(), sb); err != nil {
			return fmt.Errorf("error stopping network on cleanup: %w", err)
		}

		// Now that we've succeeded in stopping the network, cleanup namespaces
		log.Infof(ctx, "%s", nsCleanupDescription)
		return nsCleanupFunc()
	})
	if result != nil {
		resultCurrent, err := current.NewResultFromResult(result)
		if err != nil {
			return nil, err
		}
		cniResultJSON, err := json.Marshal(resultCurrent)
		if err != nil {
			return nil, err
		}
		g.AddAnnotation(annotations.CNIResult, string(cniResultJSON))
	}
	// TODO: Pass interface instead of individual field.
	s.resourceStore.SetStageForResource(ctx, sboxName, "sandbox storage start")

	// Set OOM score adjust of the infra container to be very low
	// so it doesn't get killed.
	g.SetProcessOOMScoreAdj(PodInfraOOMAdj)

	g.SetLinuxResourcesCPUShares(PodInfraCPUshares)

	// When infra-ctr-cpuset specified, set the infra container CPU set
	if s.config.InfraCtrCPUSet != "" {
		log.Debugf(ctx, "Set the infra container cpuset to %q", s.config.InfraCtrCPUSet)
		g.SetLinuxResourcesCPUCpus(s.config.InfraCtrCPUSet)
	}

	saveOptions := generate.ExportOptions{}
	g.AddAnnotation(annotations.MountPoint, podContainer.RootFs)

	if err := os.WriteFile(hostnamePath, []byte(hostname+"\n"), 0o644); err != nil {
		return nil, err
	}
	if err := label.Relabel(hostnamePath, mountLabel, false); err != nil && !errors.Is(err, unix.ENOTSUP) {
		return nil, err
	}
	mnt = spec.Mount{
		Type:        "bind",
		Source:      hostnamePath,
		Destination: "/etc/hostname",
		Options:     []string{"ro", "bind", "nodev", "nosuid", "noexec"},
	}
	g.AddMount(mnt)
	g.AddAnnotation(annotations.HostnamePath, hostnamePath)
	g.SetRootPath(podContainer.RootFs)

	if os.Getenv(rootlessEnvName) != "" {
		makeOCIConfigurationRootless(g)
	}

	sb.SetNamespaceOptions(securityContext.NamespaceOptions)

	if s.config.Seccomp().IsDisabled() {
		g.Config.Linux.Seccomp = nil
	}

	g.AddAnnotation(annotations.SeccompProfilePath, seccompRef)

	runtimeType, err := s.Runtime().RuntimeType(runtimeHandler)
	if err != nil {
		return nil, err
	}

	// A container is kernel separated if we're using shimv2, or we're using a kata v1 binary
	podIsKernelSeparated := runtimeType == libconfig.RuntimeTypeVM ||
		strings.Contains(strings.ToLower(runtimeHandler), "kata") ||
		(runtimeHandler == "" && strings.Contains(strings.ToLower(s.config.DefaultRuntime), "kata"))

	var container *oci.Container
	// In the case of kernel separated containers, we need the infra container to create the VM for the pod
	if sb.NeedsInfra(s.config.DropInfraCtr) || podIsKernelSeparated {
		log.Debugf(ctx, "Keeping infra container for pod %s", sboxID)
		// pauseImage, as the userRequestedImage parameter, only shows up in CRI values we return.
		container, err = oci.NewContainer(sboxID, containerName, podContainer.RunDir, logPath, labels, g.Config.Annotations, kubeAnnotations, pauseImage.String(), nil, nil, "", nil, sboxID, false, false, false, runtimeHandler, podContainer.Dir, created, podContainer.Config.Config.StopSignal)
		if err != nil {
			return nil, err
		}
		// If using a kernel separated container runtime, the process label should be set to container_kvm_t
		// Keep in mind that kata does *not* apply any process label to containers within the VM
		if podIsKernelSeparated {
			processLabel, err = KVMLabel(processLabel)
			if err != nil {
				return nil, err
			}
			g.SetProcessSelinuxLabel(processLabel)
		}
	} else {
		log.Debugf(ctx, "Dropping infra container for pod %s", sboxID)
		container = oci.NewSpoofedContainer(sboxID, containerName, labels, sboxID, created, podContainer.RunDir)
		g.AddAnnotation(annotations.SpoofedContainer, "true")
		if err := s.config.CgroupManager().CreateSandboxCgroup(cgroupParent, sboxID); err != nil {
			return nil, fmt.Errorf("create dropped infra %s cgroup: %w", sboxID, err)
		}
	}
	container.SetMountPoint(podContainer.RootFs)
	container.SetSpec(g.Config)

	if err := sb.SetInfraContainer(container); err != nil {
		return nil, err
	}

	if err := sb.SetContainerEnvFile(ctx); err != nil {
		return nil, err
	}

	if err = g.SaveToFile(filepath.Join(podContainer.Dir, "config.json"), saveOptions); err != nil {
		return nil, fmt.Errorf("failed to save template configuration for pod sandbox %s(%s): %w", sb.Name(), sboxID, err)
	}
	if err = g.SaveToFile(filepath.Join(podContainer.RunDir, "config.json"), saveOptions); err != nil {
		return nil, fmt.Errorf("failed to write runtime configuration for pod sandbox %s(%s): %w", sb.Name(), sboxID, err)
	}

	s.addInfraContainer(ctx, container)
	resourceCleaner.Add(ctx, "runSandbox: removing infra container "+container.ID(), func() error {
		s.removeInfraContainer(ctx, container)
		return nil
	})
	// TODO: Pass interface instead of individual field.
	s.resourceStore.SetStageForResource(ctx, sboxName, "sandbox container runtime creation")
	if err := s.createContainerPlatform(ctx, container, sb.CgroupParent()); err != nil {
		return nil, err
	}

	hooks, err := runtimehandlerhooks.GetRuntimeHandlerHooks(ctx, &s.config, sb.RuntimeHandler(), sb.Annotations())
	if err != nil {
		return nil, fmt.Errorf("failed to get runtime handler %q hooks", sb.RuntimeHandler())
	}
	if hooks != nil {
		if err := hooks.PreStart(ctx, container, sb); err != nil {
			return nil, fmt.Errorf("failed to run pre-stop hook for container %q: %w", sb.ID(), err)
		}
	}
	s.generateCRIEvent(ctx, sb.InfraContainer(), types.ContainerEventType_CONTAINER_CREATED_EVENT)
	if err := s.Runtime().StartContainer(ctx, container); err != nil {
		return nil, err
	}
	resourceCleaner.Add(ctx, "runSandbox: stopping container "+container.ID(), func() error {
		// Clean-up steps from RemovePodSandbox
		if err := s.stopContainer(ctx, container, stopTimeoutFromContext(ctx)); err != nil {
			return errors.New("failed to stop container for removal")
		}

		log.Infof(ctx, "RunSandbox: deleting container %s", container.ID())
		if err := s.Runtime().DeleteContainer(ctx, container); err != nil {
			return fmt.Errorf("failed to delete container %s in pod sandbox %s: %w", container.Name(), sb.ID(), err)
		}
		log.Infof(ctx, "RunSandbox: writing container %s state to disk", container.ID())
		if err := s.ContainerStateToDisk(ctx, container); err != nil {
			return fmt.Errorf("failed to write container state %s in pod sandbox %s: %w", container.Name(), sb.ID(), err)
		}
		return nil
	})

	if err := s.ContainerStateToDisk(ctx, container); err != nil {
		log.Warnf(ctx, "Unable to write containers %s state to disk: %v", container.ID(), err)
	}

	for idx, ip := range ips {
		g.AddAnnotation(fmt.Sprintf("%s.%d", annotations.IP, idx), ip)
	}
	sb.AddIPs(ips)

	if err := s.nri.runPodSandbox(ctx, sb); err != nil {
		return nil, err
	}

	if isContextError(ctx.Err()) {
		if err := s.resourceStore.Put(sboxName, sb, resourceCleaner); err != nil {
			log.Errorf(ctx, "RunSandbox: failed to save progress of sandbox %s: %v", sboxID, err)
		}
		log.Infof(ctx, "RunSandbox: context was either canceled or the deadline was exceeded: %v", ctx.Err())
		// should not cleanup
		storeResource = true
		return nil, ctx.Err()
	}

	// Since it's not a context error, we can delete the resource from the store, it will be tracked in the server from now on.
	s.resourceStore.Delete(sboxName)

	sb.SetCreated()
	s.generateCRIEvent(ctx, sb.InfraContainer(), types.ContainerEventType_CONTAINER_STARTED_EVENT)

	log.Infof(ctx, "Ran pod sandbox %s with infra container: %s", container.ID(), container.Description())
	resp = &types.RunPodSandboxResponse{PodSandboxId: sboxID}
	return resp, nil
}

// populateSandboxLabels adds some fields that Kubelet specifies by default, but other clients (crictl) does not.
// While CRI-O typically only cares about the kubelet, the cost here is low. Adding this code prevents issues
// with the LogLink feature, as the unmounting relies on the existence of the UID in the sandbox labels.
func populateSandboxLabels(labels map[string]string, kubeName, kubePodUID, namespace string) map[string]string {
	if labels == nil {
		labels = make(map[string]string)
	}
	if _, ok := labels[kubeletTypes.KubernetesPodNameLabel]; !ok {
		labels[kubeletTypes.KubernetesPodNameLabel] = kubeName
	}
	if _, ok := labels[kubeletTypes.KubernetesPodNamespaceLabel]; !ok {
		labels[kubeletTypes.KubernetesPodNamespaceLabel] = namespace
	}
	if _, ok := labels[kubeletTypes.KubernetesPodUIDLabel]; !ok {
		labels[kubeletTypes.KubernetesPodUIDLabel] = kubePodUID
	}
	return labels
}

func (s *Server) configureGeneratorForSysctls(ctx context.Context, g *generate.Generator, hostNetwork, hostIPC bool, sysctls map[string]string) map[string]string {
	ctx, span := log.StartSpan(ctx)
	defer span.End()
	sysctlsToReturn := make(map[string]string)
	defaultSysctls, err := s.config.RuntimeConfig.Sysctls()
	if err != nil {
		log.Warnf(ctx, "Sysctls invalid: %v", err)
	}

	for _, sysctl := range defaultSysctls {
		if err := sysctl.Validate(hostNetwork, hostIPC); err != nil {
			log.Warnf(ctx, "Skipping invalid sysctl specified by config %s: %v", sysctl, err)
			continue
		}
		g.AddLinuxSysctl(sysctl.Key(), sysctl.Value())
		sysctlsToReturn[sysctl.Key()] = sysctl.Value()
	}

	// extract linux sysctls from annotations and pass down to oci runtime
	// Will override any duplicate default systcl from crio.conf
	for key, value := range sysctls {
		sysctl := libconfig.NewSysctl(key, value)
		if err := sysctl.Validate(hostNetwork, hostIPC); err != nil {
			log.Warnf(ctx, "Skipping invalid sysctl specified over CRI %s: %v", sysctl, err)
			continue
		}
		g.AddLinuxSysctl(key, value)
		sysctlsToReturn[key] = value
	}
	return sysctlsToReturn
}

// configureGeneratorForSandboxNamespaces set the linux namespaces for the generator, based on whether the pod is sharing namespaces with the host,
// as well as whether CRI-O should be managing the namespace lifecycle.
// it returns a slice of cleanup funcs, all of which are the respective NamespaceRemove() for the sandbox.
// The caller should defer the cleanup funcs if there is an error, to make sure each namespace we are managing is properly cleaned up.
func (s *Server) configureGeneratorForSandboxNamespaces(ctx context.Context, hostNetwork, hostIPC, hostPID bool, sysctls map[string]string, sb *libsandbox.Sandbox, g *generate.Generator) (cleanupFuncs []func() error, retErr error) {
	_, span := log.StartSpan(ctx)
	defer span.End()
	// Since we need a process to hold open the PID namespace, CRI-O can't manage the NS lifecycle
	if hostPID {
		if err := g.RemoveLinuxNamespace(string(spec.PIDNamespace)); err != nil {
			return nil, err
		}
	}
	namespaceConfig := &nsmgr.PodNamespacesConfig{
		Sysctls: sysctls,
		Namespaces: []*nsmgr.PodNamespaceConfig{
			{
				Type: nsmgr.IPCNS,
				Host: hostIPC,
			},
			{
				Type: nsmgr.NETNS,
				Host: hostNetwork,
			},
			{
				Type: nsmgr.UTSNS, // there is no option for host UTSNS
			},
		},
	}

	// now that we've configured the namespaces we're sharing, create them
	namespaces, err := s.config.NamespaceManager().NewPodNamespaces(namespaceConfig)
	if err != nil {
		return nil, err
	}

	sb.AddManagedNamespaces(namespaces)

	cleanupFuncs = append(cleanupFuncs, sb.RemoveManagedNamespaces)

	if err := ctrfactory.ConfigureGeneratorGivenNamespacePaths(sb.NamespacePaths(), g); err != nil {
		return cleanupFuncs, err
	}

	return cleanupFuncs, nil
}
