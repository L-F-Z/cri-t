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

package utils

// this code is modified from https://github.com/moby/moby/blob/master/daemon/graphdriver/copy/copy.go
import (
	"container/list"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"
)

func Copy(src, dstDir string, chownRoot bool) error {
	fileInfo, err := os.Stat(src)
	if err != nil {
		return fmt.Errorf("unable to stat src %s: [%v]", src, err)
	}

	dstInfo, err := os.Stat(dstDir)
	if err != nil {
		if os.IsNotExist(err) {
			if err := os.MkdirAll(dstDir, 0755); err != nil {
				return fmt.Errorf("unable to create destination directory %s: [%v]", dstDir, err)
			}
		} else {
			return fmt.Errorf("unable to stat destination %s: [%v]", dstDir, err)
		}
	} else {
		if !dstInfo.IsDir() {
			return fmt.Errorf("destination %s is not a directory", dstDir)
		}
	}

	if fileInfo.IsDir() {
		return dirCopy(src, dstDir, chownRoot)
	} else if fileInfo.Mode().IsRegular() {
		return fileCopy(src, dstDir, fileInfo, chownRoot)
	} else {
		return fmt.Errorf("src %s is not a directory or regular file", src)
	}
}

func copyRegular(srcPath, dstPath string, fileinfo os.FileInfo, copyWithFileRange, copyWithFileClone *bool) error {
	srcFile, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	// If the destination file already exists, we shouldn't blow it away
	dstFile, err := os.OpenFile(dstPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, fileinfo.Mode())
	if err != nil {
		return err
	}
	defer dstFile.Close()

	if *copyWithFileClone {
		err = unix.IoctlFileClone(int(dstFile.Fd()), int(srcFile.Fd()))
		if err == nil {
			return nil
		}

		*copyWithFileClone = false
		if err == unix.EXDEV {
			*copyWithFileRange = false
		}
	}
	if *copyWithFileRange {
		err = doCopyWithFileRange(srcFile, dstFile, fileinfo)
		// Trying the file_clone may not have caught the exdev case
		// as the ioctl may not have been available (therefore EINVAL)
		if err == unix.EXDEV || err == unix.ENOSYS {
			*copyWithFileRange = false
		} else {
			return err
		}
	}
	// TODO: moby uses https://github.com/moby/moby/blob/master/pkg/pools/pools.go
	// We need to find out whether it is necessary.
	_, err = io.Copy(dstFile, srcFile)
	return err
}

func doCopyWithFileRange(srcFile, dstFile *os.File, fileinfo os.FileInfo) error {
	amountLeftToCopy := fileinfo.Size()

	for amountLeftToCopy > 0 {
		n, err := unix.CopyFileRange(int(srcFile.Fd()), nil, int(dstFile.Fd()), nil, int(amountLeftToCopy), 0)
		if err != nil {
			return err
		}

		amountLeftToCopy = amountLeftToCopy - int64(n)
	}

	return nil
}

type fileID struct {
	dev uint64
	ino uint64
}

type dirMtimeInfo struct {
	dstPath *string
	stat    *syscall.Stat_t
}

// fileCopy copies one file to dstDir
func fileCopy(srcFile, dstDir string, fileInfo fs.FileInfo, chownRoot bool) error {
	dstPath := filepath.Join(dstDir, filepath.Base(srcFile))
	tmpBool1, tmpBool2 := true, true
	err := copyRegular(srcFile, dstPath, fileInfo, &tmpBool1, &tmpBool2)
	if err != nil {
		return fmt.Errorf("unable to copy %s: [%v]", srcFile, err)
	}
	stat, ok := fileInfo.Sys().(*syscall.Stat_t)
	if !ok {
		return fmt.Errorf("unable to get raw syscall.Stat_t data for %s", srcFile)
	}

	var uid, gid int
	if chownRoot {
		uid, gid = 0, 0
	} else {
		uid, gid = int(stat.Uid), int(stat.Gid)
	}
	if err := os.Lchown(dstPath, uid, gid); err != nil {
		return err
	}

	aTime := time.Unix(stat.Atim.Unix())
	mTime := time.Unix(stat.Mtim.Unix())
	if err := Chtimes(dstPath, aTime, mTime); err != nil {
		return err
	}
	return nil
}

// dirCopy copies the contents of one directory to another, properly handling soft links
func dirCopy(srcDir, dstDir string, chownRoot bool) error {
	copyWithFileRange := true
	copyWithFileClone := true

	// This is a map of source file inodes to dst file paths
	copiedFiles := make(map[fileID]string)

	dirsToSetMtimes := list.New()
	err := filepath.Walk(srcDir, func(srcPath string, f os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// avoid self loop
		absSrcPath, _ := filepath.Abs(srcPath)
		absDstDir, _ := filepath.Abs(dstDir)
		if absSrcPath == absDstDir || strings.HasPrefix(absSrcPath, absDstDir+string(os.PathSeparator)) {
			return filepath.SkipDir
		}

		// Rebase path
		relPath, err := filepath.Rel(srcDir, srcPath)
		if err != nil {
			return err
		}

		dstPath := filepath.Join(dstDir, relPath)

		stat, ok := f.Sys().(*syscall.Stat_t)
		if !ok {
			return fmt.Errorf("unable to get raw syscall.Stat_t data for %s", srcPath)
		}

		isHardlink := false

		switch mode := f.Mode(); {
		case mode.IsRegular():
			// the type is 32bit on mips
			id := fileID{dev: uint64(stat.Dev), ino: stat.Ino}
			if hardLinkDstPath, ok := copiedFiles[id]; ok {
				isHardlink = true
				if err2 := os.Link(hardLinkDstPath, dstPath); err2 != nil {
					return err2
				}
			} else {
				if err2 := copyRegular(srcPath, dstPath, f, &copyWithFileRange, &copyWithFileClone); err2 != nil {
					return err2
				}
				copiedFiles[id] = dstPath
			}

		case mode.IsDir():
			if err := os.Mkdir(dstPath, f.Mode()); err != nil && !os.IsExist(err) {
				return err
			}

		case mode&os.ModeSymlink != 0:
			link, err := os.Readlink(srcPath)
			if err != nil {
				return err
			}

			if err := os.Symlink(link, dstPath); err != nil {
				return err
			}

		case mode&os.ModeNamedPipe != 0:
			fallthrough
		case mode&os.ModeSocket != 0:
			if err := unix.Mkfifo(dstPath, stat.Mode); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unknown file type (%d / %s) for %s", f.Mode(), f.Mode().String(), srcPath)
		}

		// Everything below is copying metadata from src to dst. All this metadata
		// already shares an inode for hardlinks.
		if isHardlink {
			return nil
		}

		var uid, gid int
		if chownRoot {
			uid, gid = 0, 0
		} else {
			uid, gid = int(stat.Uid), int(stat.Gid)
		}
		if err := os.Lchown(dstPath, uid, gid); err != nil {
			return err
		}

		isSymlink := f.Mode()&os.ModeSymlink != 0

		// There is no LChmod, so ignore mode for symlink. Also, this
		// must happen after chown, as that can modify the file mode
		if !isSymlink {
			if err := os.Chmod(dstPath, f.Mode()); err != nil {
				return err
			}
		}

		// system.Chtimes doesn't support a NOFOLLOW flag atm
		if f.IsDir() {
			dirsToSetMtimes.PushFront(&dirMtimeInfo{dstPath: &dstPath, stat: stat})
		} else if !isSymlink {
			aTime := time.Unix(stat.Atim.Unix())
			mTime := time.Unix(stat.Mtim.Unix())
			if err := Chtimes(dstPath, aTime, mTime); err != nil {
				return err
			}
		} else {
			ts := []syscall.Timespec{stat.Atim, stat.Mtim}
			if err := LUtimesNano(dstPath, ts); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	for e := dirsToSetMtimes.Front(); e != nil; e = e.Next() {
		mtimeInfo := e.Value.(*dirMtimeInfo)
		ts := []syscall.Timespec{mtimeInfo.stat.Atim, mtimeInfo.stat.Mtim}
		if err := LUtimesNano(*mtimeInfo.dstPath, ts); err != nil {
			return err
		}
	}

	return nil
}

// Used by Chtimes
var unixEpochTime, unixMaxTime time.Time

func init() {
	unixEpochTime = time.Unix(0, 0)
	if unsafe.Sizeof(syscall.Timespec{}.Nsec) == 8 {
		// This is a 64 bit timespec
		// os.Chtimes limits time to the following
		//
		// Note that this intentionally sets nsec (not sec), which sets both sec
		// and nsec internally in time.Unix();
		// https://github.com/golang/go/blob/go1.19.2/src/time/time.go#L1364-L1380
		unixMaxTime = time.Unix(0, 1<<63-1)
	} else {
		// This is a 32 bit timespec
		unixMaxTime = time.Unix(1<<31-1, 0)
	}
}

// Chtimes changes the access time and modified time of a file at the given path.
// If the modified time is prior to the Unix Epoch (unixMinTime), or after the
// end of Unix Time (unixEpochTime), os.Chtimes has undefined behavior. In this
// case, Chtimes defaults to Unix Epoch, just in case.
func Chtimes(name string, atime time.Time, mtime time.Time) error {
	if atime.Before(unixEpochTime) || atime.After(unixMaxTime) {
		atime = unixEpochTime
	}

	if mtime.Before(unixEpochTime) || mtime.After(unixMaxTime) {
		mtime = unixEpochTime
	}

	if err := os.Chtimes(name, atime, mtime); err != nil {
		return err
	}
	return nil
}

// LUtimesNano is used to change access and modification time of the specified path.
// It's used for symbol link file because unix.UtimesNano doesn't support a NOFOLLOW flag atm.
func LUtimesNano(path string, ts []syscall.Timespec) error {
	uts := []unix.Timespec{
		unix.NsecToTimespec(syscall.TimespecToNsec(ts[0])),
		unix.NsecToTimespec(syscall.TimespecToNsec(ts[1])),
	}
	err := unix.UtimesNanoAt(unix.AT_FDCWD, path, uts, unix.AT_SYMLINK_NOFOLLOW)
	if err != nil && err != unix.ENOSYS {
		return err
	}

	return nil
}
