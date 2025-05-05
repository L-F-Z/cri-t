// Copyright 2025 Fengzhi Li
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package libtc

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/L-F-Z/TaskC/pkg/prefab"
	"github.com/opencontainers/runc/libcontainer"
	_ "github.com/opencontainers/runc/libcontainer/cgroups/devices"
	"github.com/opencontainers/runc/libcontainer/configs"
	_ "github.com/opencontainers/runc/libcontainer/nsenter"
	"github.com/opencontainers/runc/libcontainer/specconv"
	"github.com/opencontainers/runtime-spec/specs-go"
	"github.com/sirupsen/logrus"
)

const CONTAINERS_ROOT = "/run/taskc"

func init() {
	logrus.SetLevel(logrus.Level(2))
	if len(os.Args) > 1 && os.Args[1] == "init" {
		libcontainer.Init()
	}
}

type CreateConfig struct {
	WorkDir        string
	RootFS         string
	GPUEnabled     bool
	PortMapping    string
	CPULimit       string
	MemoryLimit    string
	NameSpacePaths map[configs.NamespaceType]string
}

func Create(cfg CreateConfig) (container *libcontainer.Container, err error) {
	containerDir, err := os.MkdirTemp(cfg.WorkDir, "taskc-")
	if err != nil {
		log.Printf("unable to create container directory %s: [%v]\n", containerDir, err)
		return
	}
	id := filepath.Base(containerDir)

	// generate OCI runtime specification
	spec := DefaultRuntimeSpec()
	spec.Root = &specs.Root{Path: cfg.RootFS}
	spec.Hostname = "TaskC"
	err = addRootFS(spec, cfg.RootFS, containerDir)
	if err != nil {
		err = fmt.Errorf("failed to create rootFS: %v", err)
		return
	}
	err = addResourceLimit(spec, cfg.CPULimit, cfg.MemoryLimit)
	if err != nil {
		err = fmt.Errorf("failed to add resource limit: %v", err)
		return
	}
	if len(cfg.NameSpacePaths) != 0 {
		setNameSpacePaths(spec, cfg.NameSpacePaths)
	} else {
		err = addNetwork(spec, id, cfg.PortMapping)
		if err != nil {
			err = fmt.Errorf("failed to add network: %v", err)
			return
		}
	}

	// convert OCI runtime specification to libcontainer configuration
	config, err := specconv.CreateLibcontainerConfig(&specconv.CreateOpts{
		CgroupName:       id,
		UseSystemdCgroup: true,
		NoPivotRoot:      false,
		NoNewKeyring:     false,
		Spec:             spec,
		RootlessEUID:     os.Geteuid() != 0,
		RootlessCgroups:  false,
	})
	if err != nil {
		err = fmt.Errorf("failed to create libcontainer config: %v", err)
		return
	}
	if cfg.GPUEnabled {
		addNvidiaGPU(config, cfg.RootFS)
	}

	return libcontainer.Create(CONTAINERS_ROOT, id, config)
}

type RunConfig struct {
	Blueprint   *prefab.Blueprint
	Interactive bool
	Input       io.Reader
	Output      io.Writer
}

func Run(container *libcontainer.Container, cfg RunConfig, ctx context.Context) (err error) {
	process, err := genInitProcess(cfg.Blueprint)
	if err != nil {
		return fmt.Errorf("failed to generate Init Process %v", err)
	}

	var tty *tty
	if cfg.Interactive {
		process.Args = []string{"/bin/bash"}
		tty, err = SetupIO(process)
		if err != nil {
			return fmt.Errorf("failed to set up tty: %v", err)
		}
		defer func() {
			tty.ClosePostStart()
			tty.Close()
		}()
	} else {
		if cfg.Input != nil {
			process.Stdin = cfg.Input
		}
		if cfg.Output != nil {
			process.Stdout = cfg.Output
			process.Stderr = cfg.Output
		}
	}

	err = container.Run(process)
	if err != nil {
		return fmt.Errorf("failed to run process in container: %v", err)
	}

	if cfg.Interactive {
		_, err = process.Wait()
		if err != nil {
			log.Println(err)
		}
		err = tty.WaitConsole()
		if err != nil {
			return fmt.Errorf("failed to wait tty: %v", err)
		}
	} else {
		done := make(chan error, 1)
		go func() {
			_, err = process.Wait()
			done <- err
		}()
		select {
		case <-ctx.Done():
			log.Println("Context cancled, shutting down...")
			err := process.Signal(syscall.SIGTERM)
			if err != nil {
				log.Printf("Failed to send SIGTERM to ctrCmd: %v", err)
			}
			<-done
		case err := <-done:
			if err != nil {
				log.Printf("Command finished with error: [%v]\n", err)
			}
		}
	}
	return
}

type UpdateConfig struct {
	CPULimit    string
	MemoryLimit string
}

func Update(id string, cfg UpdateConfig) (err error) {
	container, err := libcontainer.Load(CONTAINERS_ROOT, id)
	if err != nil {
		return fmt.Errorf("failed to load container %s: [%v]", id, err)
	}
	config := container.Config()
	if cfg.CPULimit != "" {
		cpuFloat, err := strconv.ParseFloat(cfg.CPULimit, 64)
		if err != nil {
			return fmt.Errorf("invalid CPU limit '%s': %v", cfg.CPULimit, err)
		}
		cpuQuota := int64(cpuFloat * 100000)
		config.Cgroups.CpuQuota = cpuQuota
	}
	if cfg.MemoryLimit != "" {
		memLimit, err := parseMemoryLimit(cfg.MemoryLimit)
		if err != nil {
			return fmt.Errorf("invalid memory limit '%s': %v", cfg.MemoryLimit, err)
		}
		config.Cgroups.Memory = memLimit
	}
	config.Cgroups.SkipDevices = true
	return container.Set(config)
}

func Stop(id string, timeout int64) (err error) {
	container, err := libcontainer.Load(CONTAINERS_ROOT, id)
	if err != nil {
		return fmt.Errorf("failed to load container %s: [%v]", id, err)
	}
	status, err := container.Status()
	if err == nil && status != libcontainer.Running {
		return
	}
	err = container.Signal(syscall.SIGTERM)
	if err != nil {
		return fmt.Errorf("failed to signal SIGTERM: [%v]", err)
	}
	time.Sleep(time.Duration(timeout))
	status, err = container.Status()
	if err == nil && status != libcontainer.Running {
		return
	}
	err = container.Signal(syscall.SIGKILL)
	if err != nil {
		err = fmt.Errorf("failed to signal SIGKILL: [%v]", err)
	}
	return
}

func Remove(id string, workDir string) (err error) {
	container, err := libcontainer.Load(CONTAINERS_ROOT, id)
	if err != nil {
		return fmt.Errorf("failed to load container %s: [%v]", id, err)
	}
	status, _ := container.Status()
	if status == libcontainer.Running {
		_ = container.Signal(syscall.SIGKILL)
	}
	delNetwork(container.Config(), id)
	err = container.Destroy()
	if err != nil {
		return fmt.Errorf("failed to destroy container %s: [%v]", id, err)
	}
	containerDir := filepath.Join(workDir, id)
	return os.RemoveAll(containerDir)
}

const _PERM = 0700

func addRootFS(spec *specs.Spec, rootFS string, containerDir string) (err error) {
	upper := filepath.Join(containerDir, "upper")
	work := filepath.Join(containerDir, "work")
	err = os.MkdirAll(upper, _PERM)
	if err != nil {
		return fmt.Errorf("failed to create rootFS upper directory: [%v]", err)
	}
	err = os.MkdirAll(work, _PERM)
	if err != nil {
		return fmt.Errorf("failed to create rootFS work directory: [%v]", err)
	}
	spec.Mounts = append([]specs.Mount{{
		Destination: "/",
		Type:        "overlay",
		Source:      "overlay",
		Options: []string{
			"lowerdir=" + rootFS,
			"upperdir=" + upper,
			"workdir=" + work,
		},
	}}, spec.Mounts...)
	return
}

func setNameSpacePaths(spec *specs.Spec, nameSpacePaths map[configs.NamespaceType]string) {
	spec.Linux.Namespaces = []specs.LinuxNamespace{}
	for key, path := range nameSpacePaths {
		spec.Linux.Namespaces = append(spec.Linux.Namespaces, specs.LinuxNamespace{
			Type: NSmap[key],
			Path: path,
		})
		if key == configs.NEWCGROUP {
			spec.Linux.CgroupsPath = path
		}
	}
}

var NSmap map[configs.NamespaceType]specs.LinuxNamespaceType = map[configs.NamespaceType]specs.LinuxNamespaceType{
	configs.NEWPID:    specs.PIDNamespace,
	configs.NEWNET:    specs.NetworkNamespace,
	configs.NEWNS:     specs.MountNamespace,
	configs.NEWIPC:    specs.IPCNamespace,
	configs.NEWUTS:    specs.UTSNamespace,
	configs.NEWUSER:   specs.UserNamespace,
	configs.NEWCGROUP: specs.CgroupNamespace,
	configs.NEWTIME:   specs.TimeNamespace,
}

func genInitProcess(bp *prefab.Blueprint) (process *libcontainer.Process, err error) {
	process = &libcontainer.Process{
		Args:   append(bp.EntryPoint, bp.Command...),
		Cwd:    bp.WorkDir,
		Env:    bp.EnvVar,
		User:   bp.User,
		Stdin:  os.Stdin,
		Stdout: os.Stdout,
		Stderr: os.Stderr,
		Init:   true,
	}
	if process.Cwd == "" {
		process.Cwd = "/"
	}
	if process.User == "" {
		process.User = "root"
	}
	if len(process.Env) == 0 {
		process.Env = []string{"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"}
	}
	process.Env = append(process.Env, "PYTHONPATH=/usr/local/lib/python-site-packages:$PYTHONPATH")
	return
}
