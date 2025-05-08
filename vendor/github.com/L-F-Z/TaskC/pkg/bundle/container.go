package bundle

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/L-F-Z/TaskC/internal/utils"
	specs "github.com/opencontainers/image-spec/specs-go/v1"
	"golang.org/x/sys/unix"
)

func (bm *BundleManager) CreateContainerById(bundleId BundleId) (id string, rootFs string, imgConfig specs.ImageConfig, err error) {
	bundle, err := bm.GetById(bundleId)
	if err != nil {
		err = fmt.Errorf("unable to find bundle %s", bundleId)
		return
	}
	return bm.CreateContainer(bundle)
}

func (bm *BundleManager) CreateContainerByName(bundleName BundleName) (id string, rootFs string, imgConfig specs.ImageConfig, err error) {
	bundle, err := bm.Get(bundleName.Name, bundleName.Version)
	if err != nil {
		err = fmt.Errorf("unable to find bundle %s-%s", bundleName.Name, bundleName.Version)
		return
	}
	return bm.CreateContainer(bundle)
}

func (bm *BundleManager) CreateContainer(bundle *Bundle) (id string, rootFs string, imgConfig specs.ImageConfig, err error) {
	containerDir, err := os.MkdirTemp(bm.containerDir, "taskc-")
	if err != nil {
		err = fmt.Errorf("unable to create container directory %s: [%v]", containerDir, err)
		return
	}
	id = filepath.Base(containerDir)
	rootFs, err = mountContainer(containerDir, bundle.PrefabPaths)
	if err != nil {
		err = fmt.Errorf("failed to create rootFS: %v", err)
		return
	}

	imgConfig = specs.ImageConfig{
		User:       bundle.Blueprint.User,
		Env:        bundle.Blueprint.EnvVar,
		Entrypoint: bundle.Blueprint.EntryPoint,
		Cmd:        bundle.Blueprint.Command,
		WorkingDir: bundle.Blueprint.WorkDir,
	}
	specPath := filepath.Join(containerDir, "OCIImageConfig.json")
	file, err := os.Create(specPath)
	if err != nil {
		err = fmt.Errorf("unable to create container config: [%v]", err)
		return
	}
	defer file.Close()
	encoder := json.NewEncoder(file)
	err = encoder.Encode(imgConfig)
	return
}

func (bm *BundleManager) DeleteContainer(id string) (err error) {
	containerDir := filepath.Join(bm.containerDir, id)
	err = umountContainer(containerDir)
	if err != nil {
		return fmt.Errorf("failed to unmount container rootFs: [%v]", err)
	}
	return os.RemoveAll(containerDir)
}

// The parameter of a system call should be limited to one page (4KB)
// for mount/umount Debugger: $ dmesg | tail -n 20
func mountContainer(workDir string, bundleDirs []string) (rootFs string, err error) {
	if len(bundleDirs) == 0 {
		err = errors.New("no lower directories")
		return
	}

	work := filepath.Join(workDir, "work")
	upper := filepath.Join(workDir, "upper")
	link := filepath.Join(workDir, "link")
	rootFs = filepath.Join(workDir, "root")
	for _, path := range []string{work, upper, link, rootFs} {
		err = os.MkdirAll(path, os.FileMode(0700))
		if err != nil {
			err = fmt.Errorf("unable to make dir %s [%v]", path, err)
			return
		}
	}

	lowerdirs := make([]string, len(bundleDirs))
	for i, ori := range bundleDirs {
		target := filepath.Join(link, utils.IntToShortName(i))
		err = os.Symlink(ori, target)
		if err != nil {
			err = fmt.Errorf("unable to create symlink %s->%s [%v]", ori, target, err)
			return
		}
		lowerdirs[len(bundleDirs)-i-1] = utils.IntToShortName(i)
	}

	originalDir, err := unix.Getwd()
	if err != nil {
		err = fmt.Errorf("failed to get current directory [%v]", err)
		return
	}
	err = unix.Chdir(link)
	if err != nil {
		err = fmt.Errorf("failed to change work directory [%v]", err)
		return
	}
	defer unix.Chdir(originalDir)

	param := fmt.Sprintf("lowerdir=%s,upperdir=%s,workdir=%s", strings.Join(lowerdirs, ":"), upper, work)
	err = unix.Mount("overlay", rootFs, "overlay", 0, param)
	if err != nil {
		err = fmt.Errorf("mount overlay failed: %w", err)
	}
	return
}

func umountContainer(workDir string) (err error) {
	rootFs := filepath.Join(workDir, "root")
	if !utils.PathExists(rootFs) {
		return fmt.Errorf("dir %s not exists", rootFs)
	}
	return unix.Unmount(rootFs, 0)
}
