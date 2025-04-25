package utils

// Copyright (C) 2014-2015 Docker Inc & Go Authors. All rights reserved.
// Copyright (C) 2017-2024 SUSE LLC. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
)

const maxSymlinkLimit = 255

// SecureJoinVFS joins the two given path components (similar to Join) except
// that the returned path is guaranteed to be scoped inside the provided root
// path (when evaluated). Any symbolic links in the path are evaluated with the
// given root treated as the root of the filesystem, similar to a chroot. The
// filesystem state is evaluated through the given VFS interface (if nil, the
// standard os.* family of functions are used).
//
// Note that the guarantees provided by this function only apply if the path
// components in the returned string are not modified (in other words are not
// replaced with symlinks on the filesystem) after this function has returned.
// Such a symlink race is necessarily out-of-scope of SecureJoin.
//
// NOTE: Due to the above limitation, Linux users are strongly encouraged to
// use OpenInRoot instead, which does safely protect against these kinds of
// attacks. There is no way to solve this problem with SecureJoinVFS because
// the API is fundamentally wrong (you cannot return a "safe" path string and
// guarantee it won't be modified afterwards).
//
// Volume names in unsafePath are always discarded, regardless if they are
// provided via direct input or when evaluating symlinks. Therefore:
//
// "C:\Temp" + "D:\path\to\file.txt" results in "C:\Temp\path\to\file.txt"
func SecureJoin(root, unsafePath string) (string, error) {
	unsafePath = filepath.FromSlash(unsafePath)
	var (
		currentPath   string
		remainingPath = unsafePath
		linksWalked   int
	)
	for remainingPath != "" {
		if v := filepath.VolumeName(remainingPath); v != "" {
			remainingPath = remainingPath[len(v):]
		}

		// Get the next path component.
		var part string
		if i := strings.IndexRune(remainingPath, filepath.Separator); i == -1 {
			part, remainingPath = remainingPath, ""
		} else {
			part, remainingPath = remainingPath[:i], remainingPath[i+1:]
		}

		// Apply the component lexically to the path we are building.
		// currentPath does not contain any symlinks, and we are lexically
		// dealing with a single component, so it's okay to do a filepath.Clean
		// here.
		nextPath := filepath.Join(string(filepath.Separator), currentPath, part)
		if nextPath == string(filepath.Separator) {
			currentPath = ""
			continue
		}
		fullPath := root + string(filepath.Separator) + nextPath

		// Figure out whether the path is a symlink.
		fi, err := os.Lstat(fullPath)
		if err != nil && !IsNotExist(err) {
			return "", err
		}
		// Treat non-existent path components the same as non-symlinks (we
		// can't do any better here).
		if IsNotExist(err) || fi.Mode()&os.ModeSymlink == 0 {
			currentPath = nextPath
			continue
		}

		// It's a symlink, so get its contents and expand it by prepending it
		// to the yet-unparsed path.
		linksWalked++
		if linksWalked > maxSymlinkLimit {
			return "", &os.PathError{Op: "SecureJoin", Path: root + string(filepath.Separator) + unsafePath, Err: syscall.ELOOP}
		}

		dest, err := os.Readlink(fullPath)
		if err != nil {
			return "", err
		}
		remainingPath = dest + string(filepath.Separator) + remainingPath
		// Absolute symlinks reset any work we've already done.
		if filepath.IsAbs(dest) {
			currentPath = ""
		}
	}

	// There should be no lexical components like ".." left in the path here,
	// but for safety clean up the path before joining it to the root.
	finalPath := filepath.Join(string(filepath.Separator), currentPath)
	return filepath.Join(root, finalPath), nil
}

// IsNotExist tells you if err is an error that implies that either the path
// accessed does not exist (or path components don't exist). This is
// effectively a more broad version of os.IsNotExist.
func IsNotExist(err error) bool {
	// Check that it's not actually an ENOTDIR, which in some cases is a more
	// convoluted case of ENOENT (usually involving weird paths).
	return errors.Is(err, os.ErrNotExist) || errors.Is(err, syscall.ENOTDIR) || errors.Is(err, syscall.ENOENT)
}

func PathExists(path string) bool {
	_, err := os.Stat(path)
	return !IsNotExist(err)
}

// ensureDir checks if a path exists and is a directory. If the path does not exist,
// it tries to create the directory. If the path is a file, it returns an error.
func EnsureDir(path string, perm fs.FileMode) error {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			// Path does not exist, try creating the directory
			return os.MkdirAll(path, perm)
		}
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("path exists but is not a directory: %s", path)
	}
	return nil
}

func IsDir(path string) bool {
	fileInfo, err := os.Stat(path)
	if err != nil {
		return false
	}
	if fileInfo == nil {
		return false
	}
	return fileInfo.IsDir()
}

func formatFilename(input ...string) string {
	var processedParts []string
	for _, s := range input {
		re := regexp.MustCompile(`[^a-zA-Z0-9]`)
		processed := re.ReplaceAllString(s, "_")
		reUnderscore := regexp.MustCompile(`_+`)
		processed = reUnderscore.ReplaceAllString(processed, "_")
		processedParts = append(processedParts, processed)
	}
	return strings.Join(processedParts, "-")
}

// generate a safe filename for target directory,
// by only preserve the letters and numbers in `parts`
// and add a number if a file with same name already exists in the target directory.
// TODO: safe for parallel calling?
func SafeFilename(dstDir string, ext string, parts ...string) (filename string) {
	pureName := formatFilename(parts...)
	if !strings.HasPrefix(ext, ".") {
		ext = "." + ext
	}
	suffix := ""
	nameCnt := 1
	for PathExists(filepath.Join(dstDir, pureName+suffix+ext)) {
		suffix = fmt.Sprintf("%d", nameCnt)
		nameCnt += 1
	}
	return pureName + suffix + ext
}
