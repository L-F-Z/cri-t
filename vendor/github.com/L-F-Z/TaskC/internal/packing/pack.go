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

package packing

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"
)

const _PERM = 0700

func Pack(srcDir string, dstDir string, filename string) (err error) {
	fileInfo, err := os.Stat(srcDir)
	if err != nil {
		err = errors.New("unable to stat the directory information when packing " + srcDir)
		return
	}
	if !fileInfo.IsDir() {
		err = errors.New("the given path is not a directory when packing " + srcDir)
		return
	}
	err = os.MkdirAll(dstDir, _PERM)
	if err != nil {
		err = errors.New("unable to create dstDir " + dstDir + ": " + err.Error())
		return
	}
	dstPath := filepath.Join(dstDir, filename)
	file, err := os.OpenFile(dstPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, os.FileMode(_PERM))
	if err != nil {
		err = errors.New("unable to create packing file " + dstPath + ": " + err.Error())
		return
	}
	defer file.Close()

	gzipWriter := gzip.NewWriter(file)
	defer gzipWriter.Close()

	tarWriter := tar.NewWriter(gzipWriter)
	defer tarWriter.Close()

	inodeMap := make(map[uint64]string)

	err = filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return fmt.Errorf("error occured during filepath.Walk: [%v]", err)
		}

		// Skip the root src directory itself.
		if path == srcDir {
			return nil
		}

		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return fmt.Errorf("error occured when get tar file info header %+v: [%v]", info, err)
		}

		header.Name, err = filepath.Rel(srcDir, path)
		if err != nil {
			return fmt.Errorf("error occured when get relative path between %s and %s: [%v]", srcDir, path, err)
		}

		// Check if the file is a symlink
		if info.Mode()&os.ModeSymlink != 0 {
			link, err := os.Readlink(path)
			if err != nil {
				return fmt.Errorf("unable to Readlink %s: [%v]", path, err)
			}
			header.Linkname = link
			header.Typeflag = tar.TypeSymlink
		}

		var stat syscall.Stat_t
		if err := syscall.Lstat(path, &stat); err != nil {
			return fmt.Errorf("unable to syscall Lstat on %s: [%v]", path, err)
		}
		inode := stat.Ino
		if originalPath, ok := inodeMap[inode]; ok {
			header.Typeflag = tar.TypeLink
			header.Linkname = originalPath
			header.Size = 0
		} else {
			inodeMap[inode] = header.Name
		}

		if err := tarWriter.WriteHeader(header); err != nil {
			return fmt.Errorf("unable to write tar header %+v: [%v]", header, err)
		}

		if !info.IsDir() && info.Mode()&os.ModeSymlink == 0 && header.Typeflag != tar.TypeLink {
			file, err := os.Open(path)
			if err != nil {
				return fmt.Errorf("unable to get file %s before copy: [%v]", path, err)
			}
			defer file.Close()
			if _, err := io.Copy(tarWriter, file); err != nil {
				return fmt.Errorf("error occured when copying %s [%v]", path, err)
			}
		}
		return nil
	})
	if err != nil {
		err = fmt.Errorf("error occured during filepath.Walk: [%v]", err)
	}
	return err
}
