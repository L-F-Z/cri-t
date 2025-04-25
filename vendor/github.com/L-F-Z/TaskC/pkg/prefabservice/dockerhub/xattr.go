/*
 * umoci: Umoci Modifies Open Containers' Images
 * Copyright (C) 2016-2020 SUSE LLC
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *    http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

// copied from https://github.com/opencontainers/umoci/blob/main/pkg/system/xattr_unix.go

package dockerhub

import (
	"bytes"
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

// Llistxattr is a wrapper around unix.Llistattr, to abstract the NUL-splitting
// and resizing of the returned []string.
func llistxattr(path string) ([]string, error) {
	var buffer []byte
	for {
		// Find the size.
		sz, err := unix.Llistxattr(path, nil)
		if err != nil {
			// Could not get the size.
			return nil, err
		}
		buffer = make([]byte, sz)

		// Get the buffer.
		_, err = unix.Llistxattr(path, buffer)
		if err != nil {
			// If we got an ERANGE then we have to resize the buffer because
			// someone raced with us getting the list. Don't you just love C
			// interfaces.
			if err == unix.ERANGE {
				continue
			}
			return nil, err
		}

		break
	}

	// Split the buffer.
	var xattrs []string
	for name := range bytes.SplitSeq(buffer, []byte{'\x00'}) {
		// "" is not a valid xattr (weirdly you get ERANGE -- not EINVAL -- if
		// you try to touch it). So just skip it.
		if len(name) == 0 {
			continue
		}
		xattrs = append(xattrs, string(name))
	}
	return xattrs, nil
}

// Lgetxattr is a wrapper around unix.Lgetattr, to abstract the resizing of the
// returned []string.
func lgetxattr(path string, name string) ([]byte, error) {
	var buffer []byte
	for {
		// Find the size.
		sz, err := unix.Lgetxattr(path, name, nil)
		if err != nil {
			// Could not get the size.
			return nil, err
		}
		buffer = make([]byte, sz)

		// Get the buffer.
		_, err = unix.Lgetxattr(path, name, buffer)
		if err != nil {
			// If we got an ERANGE then we have to resize the buffer because
			// someone raced with us getting the list. Don't you just love C
			// interfaces.
			if err == unix.ERANGE {
				continue
			}
			return nil, err
		}

		break
	}
	return buffer, nil
}

// Lclearxattrs is a wrapper around Llistxattr and Lremovexattr, which attempts
// to remove all xattrs from a given file.
func lclearxattrs(path string, except map[string]struct{}) error {
	names, err := llistxattr(path)
	if err != nil {
		// return fmt.Errorf("lclearxattrs: get list: [%w]", err)
		return err
	}
	for _, name := range names {
		if _, skip := except[name]; skip {
			continue
		}
		if err := unix.Lremovexattr(path, name); err != nil {
			// Ignore permission errors, because hitting a permission error
			// means that it's a security.* xattr label or something similar.
			if errors.Is(err, os.ErrPermission) {
				continue
			}
			// return fmt.Errorf("lclearxattrs: remove xattr: [%w]", err)
			return err
		}
	}
	return nil
}
