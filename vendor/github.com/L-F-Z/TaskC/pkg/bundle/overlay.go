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

package bundle

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unsafe"

	"github.com/L-F-Z/TaskC/internal/utils"
	"golang.org/x/sys/unix"
)

// The parameter of a system call should be limited to one page (4KB)

// for mount/umount Debugger: $ dmesg | tail -n 20
func (bm *BundleManager) mountOverlay(workDir string, lower []string) (mountDir string, err error) {
	mountDir = filepath.Join(workDir, "mount")
	err = os.MkdirAll(mountDir, perm)
	if err != nil {
		err = fmt.Errorf("unable to make dir %s [%v]", mountDir, err)
		return
	}

	linkDir := filepath.Join(workDir, "link")
	err = os.MkdirAll(linkDir, perm)
	if err != nil {
		err = fmt.Errorf("unable to make dir %s [%v]", linkDir, err)
		return
	}
	originalDir, _ := unix.Getwd()
	err = unix.Chdir(linkDir)
	if err != nil {
		err = fmt.Errorf("unable to change directory [%v]", err)
		return
	}
	defer unix.Chdir(originalDir)

	if len(lower) == 0 {
		err = errors.New("no lower directories")
		return
	}
	if len(lower) == 1 {
		empty := filepath.Join(workDir, "empty")
		err = os.MkdirAll(empty, perm)
		if err != nil {
			err = fmt.Errorf("unable to make dir %s [%v]", empty, err)
			return
		}
		lower = append(lower, empty)
	}

	lowerdir := make([]string, len(lower))
	for i, ori := range lower {
		target := filepath.Join(linkDir, intToShortName(i))
		err = os.Symlink(ori, target)
		if err != nil {
			err = fmt.Errorf("unable to create symlink %s->%s [%v]", ori, target, err)
			return
		}
		lowerdir[len(lower)-i-1] = intToShortName(i)
	}

	param := fmt.Sprintf("lowerdir=%s", strings.Join(lowerdir, ":"))

	_p0, err := unix.BytePtrFromString("overlay")
	if err != nil {
		return
	}
	_p1, err := unix.BytePtrFromString(mountDir)
	if err != nil {
		return
	}
	_p2, err := unix.BytePtrFromString("overlay")
	if err != nil {
		return
	}
	_p3, err := unix.BytePtrFromString(param)
	if err != nil {
		return
	}
	_, _, e1 := unix.Syscall6(
		unix.SYS_MOUNT,
		uintptr(unsafe.Pointer(_p0)),
		uintptr(unsafe.Pointer(_p1)),
		uintptr(unsafe.Pointer(_p2)),
		uintptr(0),
		uintptr(unsafe.Pointer(_p3)),
		0,
	)
	if e1 != 0 {
		err = e1
		return
	}
	return
}

func (bm *BundleManager) umountOverlay(workDir string) (err error) {
	mount := filepath.Join(workDir, "mount")
	// check if mount directory exists
	if !utils.PathExists(mount) {
		return fmt.Errorf("dir %s not exists", mount)
	}

	// use syscall to umount the root
	_p0, err := unix.BytePtrFromString(mount)
	if err != nil {
		return
	}
	_, _, e1 := unix.Syscall(unix.SYS_UMOUNT2, uintptr(unsafe.Pointer(_p0)), uintptr(0), 0)
	if e1 != 0 {
		return e1
	}
	return
}

const shortChars = "0123456789abcdefghijklmnopqrstuvwxyz"

func intToShortName(n int) string {
	if n == 0 {
		return "0"
	}
	result := ""
	for n > 0 {
		remainder := n % 36
		result = string(shortChars[remainder]) + result
		n = n / 36
	}
	return result
}
