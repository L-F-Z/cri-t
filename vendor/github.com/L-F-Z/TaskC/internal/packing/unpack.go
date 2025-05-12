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
	"archive/zip"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/L-F-Z/TaskC/internal/utils"
	"github.com/klauspost/compress/zstd"
	"github.com/ulikunitz/xz"
)

// fileType can be provided like ".tar.gz"
// when fileType is "", type will be get from filename in filePath
// unpackedName is used when archive is ""
// when unpackedName is "", filename will be get from original compressed file's name
func Unpack(filePath string, dstDir string, fileType string, unpackedName string) (err error) {
	if !utils.PathExists(filePath) {
		err = errors.New("file not found when unpacking " + filePath)
		return
	}
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		err = errors.New("unable to stat the file information when unpacking " + filePath)
		return
	}
	if fileInfo.IsDir() {
		err = errors.New("the given path is a directory when unpacking " + filePath)
		return
	}

	fileType = strings.ToLower(fileType)
	if fileType == "" {
		// MUST NOT use filepath.Ext() here
		// it can only get ".gz" for "1.tar.gz"
		fileType = strings.ToLower(filepath.Base(filePath))
	}
	var archive, compress string = "", ""
	if strings.HasSuffix(fileType, ".prefab") {
		archive = "tar"
		compress = "gz"
	} else if strings.HasSuffix(fileType, ".tar.gz") {
		archive = "tar"
		compress = "gz"
	} else if strings.HasSuffix(fileType, ".tgz") {
		archive = "tar"
		compress = "gz"
	} else if strings.HasSuffix(fileType, ".tar.xz") {
		archive = "tar"
		compress = "xz"
	} else if strings.HasSuffix(fileType, ".tar.zst") {
		archive = "tar"
		compress = "zst"
	} else if strings.HasSuffix(fileType, ".gz") {
		compress = "gz"
	} else if strings.HasSuffix(fileType, ".xz") {
		compress = "xz"
	} else if strings.HasSuffix(fileType, ".zst") {
		compress = "zst"
	} else if strings.HasSuffix(fileType, ".tar") {
		archive = "tar"
	} else if strings.HasSuffix(fileType, ".zip") {
		return UnZip(filePath, dstDir)
	} else if strings.HasSuffix(fileType, ".ar") {
		archive = "ar"
	} else {
		err = errors.New("unsupport unpacking type: " + fileType)
		return
	}

	file, err := os.Open(filePath)
	if err != nil {
		err = errors.New("unable to open file when unpacking " + filePath + " error:" + err.Error())
		return
	}
	defer file.Close()

	var decompressed io.Reader
	switch compress {
	case "gz":
		gzReader, err := gzip.NewReader(file)
		if err != nil {
			return err
		}
		defer gzReader.Close()
		decompressed = gzReader
	case "xz":
		xzReader, err := xz.NewReader(file)
		if err != nil {
			return err
		}
		decompressed = xzReader
	case "zst":
		zstReader, err := zstd.NewReader(file)
		if err != nil {
			return err
		}
		defer zstReader.Close()
		decompressed = zstReader
	default:
		decompressed = file
	}

	err = os.MkdirAll(dstDir, _PERM)
	if err != nil {
		return
	}

	switch archive {
	case "tar":
		_, err = UnTar(decompressed, dstDir)
		return
	case "ar":
		return UnAr(decompressed, dstDir)
	default:
		if unpackedName == "" {
			filename := filepath.Base(filePath)
			extension := filepath.Ext(filename)
			unpackedName = strings.TrimSuffix(filename, extension)
		}
		target := filepath.Join(dstDir, unpackedName)
		outFile, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, os.FileMode(_PERM))
		if err != nil {
			return err
		}
		_, err = io.Copy(outFile, decompressed)
		outFile.Close()
		if err != nil {
			return err
		}
	}
	return
}

func UnTarGz(reader io.Reader, dstDir string) (dirSize uint64, err error) {
	gzipReader, err := gzip.NewReader(reader)
	if err != nil {
		return 0, err
	}
	defer gzipReader.Close()
	return UnTar(gzipReader, dstDir)
}

func UnTar(reader io.Reader, dstDir string) (dirSize uint64, err error) {
	tarReader := tar.NewReader(reader)
	var hardLinks []struct {
		header *tar.Header
		target string
	}

	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return 0, err
		}

		target := filepath.Join(dstDir, header.Name)

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, _PERM); err != nil {
				return 0, err
			}
			os.Chmod(target, os.FileMode(header.Mode))
			os.Chown(target, header.Uid, header.Gid)
			os.Chtimes(target, header.AccessTime, header.ModTime)
		case tar.TypeReg:
			outFile, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, os.FileMode(_PERM))
			if err != nil {
				return 0, err
			}
			n, err := io.Copy(outFile, tarReader)
			outFile.Close()
			if err != nil {
				return 0, err
			}
			dirSize += uint64(n)
			os.Chmod(target, os.FileMode(header.Mode))
			os.Chown(target, header.Uid, header.Gid)
			os.Chtimes(target, header.AccessTime, header.ModTime)
		case tar.TypeSymlink:
			if err := os.Symlink(header.Linkname, target); err != nil {
				return 0, err
			}
			os.Lchown(target, header.Uid, header.Gid)
		case tar.TypeLink:
			hardLinks = append(hardLinks, struct {
				header *tar.Header
				target string
			}{header, target})
		// case tar.TypeChar, tar.TypeBlock:
		// 	mode := uint32(header.Mode & 07777)
		// 	dev := header.Devmajor<<8 | header.Devminor
		// 	err := syscall.Mknod(target, mode, int(dev))
		// 	if err != nil {
		// 		return err
		// 	}
		// case tar.TypeFifo:
		// 	if err := syscall.Mkfifo(target, uint32(header.Mode)); err != nil {
		// 		return err
		// 	}
		default:
			// Log or handle other file types
			fmt.Printf("Unhandled type: %c in file %s\n", header.Typeflag, header.Name)
		}
	}

	for _, hl := range hardLinks {
		linkTarget := filepath.Join(dstDir, hl.header.Linkname)
		if _, err := os.Stat(linkTarget); os.IsNotExist(err) {
			return 0, fmt.Errorf("hard link target does not exist: %s", linkTarget)
		} else if err != nil {
			return 0, err
		}
		if err := os.Link(linkTarget, hl.target); err != nil {
			return 0, err
		}
		os.Chmod(hl.target, os.FileMode(hl.header.Mode))
		os.Chown(hl.target, hl.header.Uid, hl.header.Gid)
		os.Chtimes(hl.target, hl.header.AccessTime, hl.header.ModTime)
	}

	return dirSize, nil
}

func UnAr(reader io.Reader, dstDir string) (err error) {
	arReader := NewArReader(reader)
	for {
		header, err := arReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		target := filepath.Join(dstDir, header.Name)
		outFile, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, os.FileMode(_PERM))
		if err != nil {
			return err
		}
		_, err = io.Copy(outFile, arReader)
		outFile.Close()
		if err != nil {
			return err
		}
	}
	return
}

// zip format do not support stream reading
func UnZip(filePath string, dstDir string) (err error) {
	r, err := zip.OpenReader(filePath)
	if err != nil {
		return
	}
	defer r.Close()

	for _, f := range r.File {
		fpath := filepath.Join(dstDir, f.Name)
		if f.FileInfo().IsDir() {
			os.MkdirAll(fpath, os.ModePerm)
		} else {
			err = os.MkdirAll(filepath.Dir(fpath), os.ModePerm)
			if err != nil {
				return
			}
			outFile, err := os.OpenFile(fpath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
			if err != nil {
				return err
			}
			rc, err := f.Open()
			if err != nil {
				return err
			}
			_, err = io.Copy(outFile, rc)
			outFile.Close()
			rc.Close()
			if err != nil {
				return err
			}
		}
	}
	return
}
