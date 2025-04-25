package dockerhub

// wget https://github.com/opencontainers/umoci/releases/download/v0.4.7/umoci.amd64
// sudo apt install skopeo
// skopeo copy docker://busybox:stable-glibc oci:busybox:stable-glibc
// sudo ./umoci.amd64 unpack --image busybox:stable-glibc bundle
//
// This code is modified from https://github.com/opencontainers/umoci/tree/main/oci/layer

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

import (
	"archive/tar"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/L-F-Z/TaskC/internal/utils"
	"golang.org/x/sys/unix"
)

// Tarmode takes a Typeflag (from a tar.Header for example) and returns the
// corresponding os.Filemode bit. Unknown typeflags are treated like regular
// files.
func tarmode(typeflag byte) uint32 {
	switch typeflag {
	case tar.TypeSymlink:
		return unix.S_IFLNK
	case tar.TypeChar:
		return unix.S_IFCHR
	case tar.TypeBlock:
		return unix.S_IFBLK
	case tar.TypeFifo:
		return unix.S_IFIFO
	case tar.TypeDir:
		return unix.S_IFDIR
	}
	return 0
}

// ignoreXattrs is a list of xattr names that should be ignored when
// creating a new image layer, because they are host-specific and/or would be a
// bad idea to unpack. They are also excluded from Lclearxattr when extracting
// an archive.
// XXX: Maybe we should make this configurable so users can manually blacklist
//
//	(or even whitelist) xattrs that they actually want included? Like how
//	GNU tar's xattr setup works.
var ignoreXattrs = map[string]struct{}{
	// SELinux doesn't allow you to set SELinux policies generically. They're
	// also host-specific. So just ignore them during extraction.
	"security.selinux": {},

	// NFSv4 ACLs are very system-specific and shouldn't be touched by us, nor
	// should they be included in images.
	"system.nfs4_acl": {},

	// In order to support overlayfs whiteout mode, we shouldn't un-set
	// this after we've set it when writing out the whiteouts.
	"trusted.overlay.opaque": {},

	// We don't want to these xattrs into the image, because they're only
	// relevant based on how the build overlay is constructed and will not
	// be true on the target system once the image is unpacked (e.g. inodes
	// might be different, impure status won't be true, etc.).
	"trusted.overlay.redirect": {},
	"trusted.overlay.origin":   {},
	"trusted.overlay.impure":   {},
	"trusted.overlay.nlink":    {},
	"trusted.overlay.upper":    {},
	"trusted.overlay.metacopy": {},
}

// CleanPath makes a path safe for use with filepath.Join. This is done by not
// only cleaning the path, but also (if the path is relative) adding a leading
// '/' and cleaning it (then removing the leading '/'). This ensures that a
// path resulting from prepending another path will always resolve to lexically
// be a subdirectory of the prefixed path. This is all done lexically, so paths
// that include symlinks won't be safe as a result of using CleanPath.
//
// This function comes from runC (libcontainer/utils/utils.go).
func CleanPath(path string) string {
	// Deal with empty strings nicely.
	if path == "" {
		return ""
	}

	// Ensure that all paths are cleaned (especially problematic ones like
	// "/../../../../../" which can cause lots of issues).
	path = filepath.Clean(path)

	// If the path isn't absolute, we need to do more processing to fix paths
	// such as "../../../../<etc>/some/path". We also shouldn't convert absolute
	// paths to relative ones.
	if !filepath.IsAbs(path) {
		path = filepath.Clean(string(os.PathSeparator) + path)
		// This can't fail, as (by definition) all paths are relative to root.
		// #nosec G104
		path, _ = filepath.Rel(string(os.PathSeparator), path)
	}

	// Clean the path again for good measure.
	return filepath.Clean(path)
}

// UnpackLayer unpacks the tar stream representing an OCI layer at the given
// root. It ensures that the state of the root is as close as possible to the
// state used to create the layer. If an error is returned, the state of root
// is undefined (unpacking is not guaranteed to be atomic).
func unpackLayer(root string, layer io.Reader) error {
	// upperPaths are paths that have either been extracted in the execution of
	// this TarExtractor or are ancestors of paths extracted. The purpose of
	// having this stored in-memory is to be able to handle opaque whiteouts as
	// well as some other possible ordering issues with malformed archives (the
	// downside of this approach is that it takes up memory -- we could switch
	// to a trie if necessary). These paths are relative to the tar root but
	// are fully symlink-expanded so no need to worry about that line noise.
	upperPaths := make(map[string]struct{})
	tr := tar.NewReader(layer)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("read next entry: [%w]", err)
		}
		if err := unpackEntry(root, hdr, tr, upperPaths); err != nil {
			return fmt.Errorf("unpack entry: %s: [%w]", hdr.Name, err)
		}
	}
	return nil
}

// restoreMetadata applies the state described in tar.Header to the filesystem
// at the given path. No sanity checking is done of the tar.Header's pathname
// or other information. In addition, no mapping is done of the header.
func restoreMetadata(path string, hdr *tar.Header) error {
	// Some of the tar.Header fields don't match the OS API.
	fi := hdr.FileInfo()

	// Get the _actual_ file info to figure out if the path is a symlink.
	isSymlink := hdr.Typeflag == tar.TypeSymlink
	if realFi, err := os.Lstat(path); err == nil {
		isSymlink = realFi.Mode()&os.ModeSymlink == os.ModeSymlink
	}

	// Apply the owner.
	if err := os.Lchown(path, hdr.Uid, hdr.Gid); err != nil {
		return fmt.Errorf("restore chown metadata: %s [%w]", path, err)
	}

	// We cannot apply hdr.Mode to symlinks, because symlinks don't have a mode
	// of their own (they're special in that way). We have to apply this after
	// we've applied the owner because setuid bits are cleared when changing
	// owner (in rootless we don't care because we're always the owner).
	if !isSymlink {
		if err := os.Chmod(path, fi.Mode()); err != nil {
			return fmt.Errorf("restore chown metadata: %s [%w]", path, err)
		}
	}

	// Apply access and modified time. Note that some archives won't fill the
	// atime and mtime fields, so we have to set them to a more sane value.
	// Otherwise Linux will start screaming at us, and nobody wants that.
	mtime := hdr.ModTime
	if mtime.IsZero() {
		// XXX: Should we instead default to atime if it's non-zero?
		mtime = time.Now()
	}
	atime := hdr.AccessTime
	if atime.IsZero() {
		// Default to the mtime.
		atime = mtime
	}

	// Apply xattrs. In order to make sure that we *only* have the xattr set we
	// want, we first clear the set of xattrs from the file then apply the ones
	// set in the tar.Header.
	err := lclearxattrs(path, ignoreXattrs)
	if err != nil {
		if !errors.Is(err, unix.ENOTSUP) {
			return fmt.Errorf("clear xattr metadata: %s: [%w]", path, err)
		}
	}

	for name, value := range hdr.Xattrs {
		value := []byte(value)

		// Forbidden xattrs should never be touched.
		if _, skip := ignoreXattrs[name]; skip {
			// If the xattr is already set to the requested value, don't bail.
			// The reason for this logic is kinda convoluted, but effectively
			// because restoreMetadata is called with the *on-disk* metadata we
			// run the risk of things like "security.selinux" being included in
			// that metadata (and thus tripping the forbidden xattr error). By
			// only touching xattrs that have a different value we are somewhat
			// more efficient and we don't have to special case parent restore.
			// Of course this will only ever impact ignoreXattrs.
			if oldValue, err := lgetxattr(path, name); err == nil {
				if bytes.Equal(value, oldValue) {
					// log.Debugf("restore xattr metadata: skipping already-set xattr %q: %s", name, hdr.Name)
					continue
				}
			}
			// log.Warnf("xattr{%s} ignoring forbidden xattr: %q", hdr.Name, name)
			continue
		}
		if err := unix.Lsetxattr(path, name, value, 0); err != nil {
			// We cannot do much if we get an ENOTSUP -- this usually means
			// that extended attributes are simply unsupported by the
			// underlying filesystem (such as AUFS or NFS).
			if errors.Is(err, unix.ENOTSUP) {
				continue
			}
			return fmt.Errorf("restore xattr metadata: %s: [%w]", path, err)
		}
	}

	times := []unix.Timespec{
		unix.NsecToTimespec(atime.UnixNano()),
		unix.NsecToTimespec(mtime.UnixNano()),
	}
	err = unix.UtimesNanoAt(unix.AT_FDCWD, path, times, unix.AT_SYMLINK_NOFOLLOW)
	if err != nil {
		err = &os.PathError{Op: "lutimes", Path: path, Err: err}
		return fmt.Errorf("restore lutimes metadata: %s: [%w]", path, err)
	}
	return nil
}

func ociWhiteout(root string, dir string, file string, upperPaths map[string]struct{}) error {
	isOpaque := file == ".wh..wh..opq"
	file = strings.TrimPrefix(file, ".wh.")

	// We have to be quite careful here. While the most intuitive way of
	// handling whiteouts would be to just RemoveAll without prejudice, We
	// have to be careful here. If there is a whiteout entry for a file
	// *after* a normal entry (in the same layer) then the whiteout must
	// not remove the new entry. We handle this by keeping track of
	// whichpaths have been touched by this layer's extraction (these form
	// the "upperdir"). We also have to handle cases where a directory has
	// been marked for deletion, but a child has been extracted in this
	// layer.

	path := filepath.Join(dir, file)
	if isOpaque {
		path = dir
	}

	// If the root doesn't exist we've got nothing to do.
	// XXX: We currently cannot error out if a layer asks us to remove a
	//      non-existent path with this implementation (because we don't
	//      know if it was implicitly removed by another whiteout). In
	//      future we could add lowerPaths that would help track whether
	//      another whiteout caused the removal to "fail" or if the path
	//      was actually missing -- which would allow us to actually error
	//      out here if the layer is invalid).
	if !utils.PathExists(path) {
		// return errors.New("whiteout target not exists")
		return nil
	}

	// Walk over the path to remove it. We remove a given path as soon as
	// it isn't present in upperPaths (which includes ancestors of paths
	// we've extracted so we only need to look up the one path). Otherwise
	// we iterate over any children and try again. The only difference
	// between opaque whiteouts and regular whiteouts is that we don't
	// delete the directory itself with opaque whiteouts.
	err := filepath.Walk(path, func(subpath string, info os.FileInfo, err error) error {
		// If we are passed an error, bail unless it's ENOENT.
		if err != nil {
			// If something was deleted outside of our knowledge it's not
			// the end of the world. In principle this shouldn't happen
			// though, so we log it for posterity.
			if errors.Is(err, os.ErrNotExist) {
				// log.Debugf("whiteout removal hit already-deleted path: %s", subpath)
				err = filepath.SkipDir
			}
			return err
		}

		// Get the relative form of subpath to root to match
		// te.upperPaths.
		upperPath, err := filepath.Rel(root, subpath)
		if err != nil {
			return fmt.Errorf("find relative-to-root [should never happen]: [%w]", err)
		}

		// Remove the path only if it hasn't been touched.
		if _, ok := upperPaths[upperPath]; !ok {
			// Opaque whiteouts don't remove the directory itself, so skip
			// the top-level directory.
			if isOpaque && CleanPath(path) == CleanPath(subpath) {
				return nil
			}

			// Purge the path. We skip anything underneath (if it's a
			// directory) since we just purged it -- and we don't want to
			// hit ENOENT during iteration for no good reason.
			err := os.RemoveAll(subpath)
			if err == nil && info.IsDir() {
				err = filepath.SkipDir
			}
			if err != nil {
				err = fmt.Errorf("whiteout subpath: [%w]", err)
			}
			return err
		}
		return nil
	})
	// TODO: Should we check here?
	// return fmt.Errorf("whiteout remove: [%w]", err)
	if err != nil {
		return fmt.Errorf("whiteout remove: [%w]", err)
	}
	return nil
}

// UnpackEntry extracts the given tar.Header to the provided root, ensuring
// that the layer state is consistent with the layer state that produced the
// tar archive being iterated over. This does handle whiteouts, so a tar.Header
// that represents a whiteout will result in the path being removed.
func unpackEntry(root string, hdr *tar.Header, r io.Reader, upperPaths map[string]struct{}) (Err error) {
	// Make the paths safe.
	hdr.Name = CleanPath(hdr.Name)
	root = filepath.Clean(root)

	// Get directory and filename, but we have to safely get the directory
	// component of the path. SecureJoin will evaluate the path itself,
	// which we don't want (we're clever enough to handle the actual path being
	// a symlink).
	unsafeDir, file := filepath.Split(hdr.Name)
	if filepath.Join("/", hdr.Name) == "/" {
		// If we got an entry for the root, then unsafeDir is the full path.
		unsafeDir, file = hdr.Name, "."
		// If we're being asked to change the root type, bail because they may
		// change it to a symlink which we could inadvertently follow.
		if hdr.Typeflag != tar.TypeDir {
			return errors.New("malicious tar entry -- refusing to change type of root directory")
		}
	}
	dir, err := utils.SecureJoin(root, unsafeDir)
	if err != nil {
		return fmt.Errorf("sanitise symlinks in root: [%w]", err)
	}
	path := filepath.Join(dir, file)

	// Before we do anything, get the state of dir. Because we might be adding
	// or removing files, our parent directory might be modified in the
	// process. As a result, we want to be able to restore the old state
	// (because we only apply state that we find in the archive we're iterating
	// over). We can safely ignore an error here, because a non-existent
	// directory will be fixed by later archive entries.
	if dirFi, err := os.Lstat(dir); err == nil && path != dir {
		// FIXME: This is really stupid.
		// #nosec G104
		link, _ := os.Readlink(dir)
		dirHdr, err := tar.FileInfoHeader(dirFi, link)
		if err != nil {
			return fmt.Errorf("convert dirFi to dirHdr: [%w]", err)
		}

		// More faking to trick restoreMetadata to actually restore the directory.
		dirHdr.Typeflag = tar.TypeDir
		dirHdr.Linkname = ""

		// os.Lstat doesn't get the list of xattrs by default. We need to fill
		// this explicitly. Note that while Go's "archive/tar" takes strings,
		// in Go strings can be arbitrary byte sequences so this doesn't
		// restrict the possible values.
		// TODO: Move this to a separate function so we can share it with
		//       tar_generate.go.
		xattrs, err := llistxattr(dir)
		if err != nil {
			if !errors.Is(err, unix.ENOTSUP) {
				return fmt.Errorf("get dirHdr.Xattrs: [%w]", err)
			}
		}
		if len(xattrs) > 0 {
			dirHdr.PAXRecords = map[string]string{}
			for _, xattr := range xattrs {
				value, err := lgetxattr(dir, xattr)
				if err != nil {
					return fmt.Errorf("get xattr: [%w]", err)
				}
				dirHdr.PAXRecords[xattr] = string(value)
			}
		}

		// Ensure that after everything we correctly re-apply the old metadata.
		// We don't map this header because we're restoring files that already
		// existed on the filesystem, not from a tar layer.
		defer func() {
			// Only overwrite the error if there wasn't one already.
			if err := restoreMetadata(dir, dirHdr); err != nil {
				if Err == nil {
					Err = fmt.Errorf("restore parent directory: [%w]", err)
				}
			}
		}()
	}

	// Currently the spec doesn't specify what the hdr.Typeflag of whiteout
	// files is meant to be. We specifically only produce regular files
	// ('\x00') but it could be possible that someone produces a different
	// Typeflag, expecting that the path is the only thing that matters in a
	// whiteout entry.
	if strings.HasPrefix(file, ".wh.") {
		return ociWhiteout(root, dir, file, upperPaths)
	}

	// Get information about the path. This has to be done after we've dealt
	// with whiteouts because it turns out that lstat(2) will return EPERM if
	// you try to stat a whiteout on AUFS.
	fi, err := os.Lstat(path)
	if err != nil {
		// File doesn't exist, just switch fi to the file header.
		fi = hdr.FileInfo()
	}

	// Attempt to create the parent directory of the path we're unpacking.
	// We do a MkdirAll here because even though you need to have a tar entry
	// for every component of a new path, applyMetadata will correct any
	// inconsistencies.
	// FIXME: We have to make this consistent, since if the tar archive doesn't
	//        have entries for some of these components we won't be able to
	//        verify that we have consistent results during unpacking.
	if err := os.MkdirAll(dir, 0777); err != nil {
		return fmt.Errorf("mkdir parent: [%w]", err)
	}

	// We remove whatever existed at the old path to clobber it so that
	// creating a new path will not break. The only exception is if the path is
	// a directory in both the layer and the current filesystem, in which case
	// we don't delete it for obvious reasons. In all other cases we clobber.
	//
	// Note that this will cause hard-links in the "lower" layer to not be able
	// to point to "upper" layer inodes even if the extracted type is the same
	// as the old one, however it is not clear whether this is something a user
	// would expect anyway. In addition, this will incorrectly deal with a
	// TarLink that is present before the "upper" entry in the layer but the
	// "lower" file still exists (so the hard-link would point to the old
	// inode). It's not clear if such an archive is actually valid though.
	if !fi.IsDir() || hdr.Typeflag != tar.TypeDir {
		if err := os.RemoveAll(path); err != nil {
			return fmt.Errorf("clobber old path: [%w]", err)
		}
	}

	// Now create or otherwise modify the state of the path. Right now, either
	// the type of path matches hdr or the path doesn't exist. Note that we
	// don't care about umasks or the initial mode here, since applyMetadata
	// will fix all of that for us.
	switch hdr.Typeflag {
	// regular file
	case tar.TypeReg, tar.TypeRegA:
		// Create a new file, then just copy the data.
		fh, err := os.Create(path)
		if err != nil {
			return fmt.Errorf("create regular: [%w]", err)
		}
		defer fh.Close()

		// We need to make sure that we copy all of the bytes.
		n, err := copy(fh, r)
		if int64(n) != hdr.Size {
			if err != nil {
				return fmt.Errorf("short write: [%w]", err)
			} else {
				err = io.ErrShortWrite
			}
		}
		if err != nil {
			return fmt.Errorf("unpack to regular file: [%w]", err)
		}

		// Force close here so that we don't affect the metadata.
		if err := fh.Close(); err != nil {
			return fmt.Errorf("close unpacked regular file: [%w]", err)
		}

	// directory
	case tar.TypeDir:
		// Attempt to create the directory. We do a MkdirAll here because even
		// though you need to have a tar entry for every component of a new
		// path, applyMetadata will correct any inconsistencies.
		if err := os.MkdirAll(path, 0777); err != nil {
			return fmt.Errorf("mkdirall: [%w]", err)
		}

	// hard link, symbolic link
	case tar.TypeLink, tar.TypeSymlink:
		linkname := hdr.Linkname

		// Hardlinks and symlinks act differently when it comes to the scoping.
		// In both cases, we have to just unlink and then re-link the given
		// path. But the function used and the argument are slightly different.
		switch hdr.Typeflag {
		case tar.TypeLink:
			// Because hardlinks are inode-based we need to scope the link to
			// the rootfs using SecureJoinVFS. As before, we need to be careful
			// that we don't resolve the last part of the link path (in case
			// the user actually wanted to hardlink to a symlink).
			unsafeLinkDir, linkFile := filepath.Split(CleanPath(linkname))
			linkDir, err := utils.SecureJoin(root, unsafeLinkDir)
			if err != nil {
				return fmt.Errorf("sanitise hardlink target in root: [%w]", err)
			}
			linkname = filepath.Join(linkDir, linkFile)
			// Link the new one.
			// We need to explicitly pass 0 as a flag because POSIX allows the default
			// behaviour of link(2) when it comes to target being a symlink to be
			// implementation-defined. Only linkat(2) allows us to guarantee the right
			// behaviour.
			//  <https://pubs.opengroup.org/onlinepubs/9699919799/functions/link.html>
			if err := unix.Linkat(unix.AT_FDCWD, linkname, unix.AT_FDCWD, path, 0); err != nil {
				// FIXME: Currently this can break if tar hardlink entries occur
				//        before we hit the entry those hardlinks link to. I have a
				//        feeling that such archives are invalid, but the correct
				//        way of handling this is to delay link creation until the
				//        very end. Unfortunately this won't work with symlinks
				//        (which can link to directories).
				return fmt.Errorf("link: [%w]", err)
			}
		case tar.TypeSymlink:
			if err := os.Symlink(linkname, path); err != nil {
				return fmt.Errorf("link: [%w]", err)
			}
		}

	// character device node, block device node, fifo node
	case tar.TypeChar, tar.TypeBlock, tar.TypeFifo:
		// We have to remove and then create the device. In the FIFO case we
		// could choose not to do so, but we do it anyway just to be on the
		// safe side.

		mode := tarmode(hdr.Typeflag)
		dev := unix.Mkdev(uint32(hdr.Devmajor), uint32(hdr.Devminor))

		// Create the node.
		if err := unix.Mknod(path, uint32(os.FileMode(int64(mode)|hdr.Mode)), int(dev)); err != nil {
			return fmt.Errorf("mknod: [%w]", err)
		}

	// We should never hit any other headers (Go abstracts them away from us),
	// and we can't handle any custom Tar extensions. So just error out.
	default:
		return fmt.Errorf("unpack entry: %s: unknown typeflag '\\x%x'", hdr.Name, hdr.Typeflag)
	}

	// Apply the metadata, which will apply any mappings necessary. We don't
	// apply metadata for hardlinks, because hardlinks don't have any separate
	// metadata from their link (and the tar headers might not be filled).

	if hdr.Typeflag != tar.TypeLink {
		// Apply the state described in tar.Header to the filesystem at
		// the given path, using the state of the TarExtractor to remap information
		// within the header. This should only be used with headers from a tar layer
		// (not from the filesystem). No sanity checking is done of the tar.Header's
		// pathname or other information.

		// Modify the header.
		hdr.Uid = 0
		hdr.Gid = 0

		// Restore it on the filesystme.
		if err := restoreMetadata(path, hdr); err != nil {
			return fmt.Errorf("restore hdr metadata: [%w]", err)
		}

	}

	// Everything is done -- the path now exists. Add it (and all its
	// ancestors) to the set of upper paths. We first have to figure out the
	// proper path corresponding to hdr.Name though.
	upperPath, err := filepath.Rel(root, path)
	if err != nil {
		// Really shouldn't happen because of the guarantees of SecureJoinVFS.
		return fmt.Errorf("find relative-to-root [should never happen]: [%w]", err)
	}
	for pth := upperPath; pth != filepath.Dir(pth); pth = filepath.Dir(pth) {
		upperPaths[pth] = struct{}{}
	}
	return nil
}

// Copy has identical semantics to io.Copy except it will automatically resume
// the copy after it receives an EINTR error.
func copy(dst io.Writer, src io.Reader) (int64, error) {
	// Make a buffer so io.Copy doesn't make one for each iteration.
	var buf []byte
	size := 32 * 1024
	if lr, ok := src.(*io.LimitedReader); ok && lr.N < int64(size) {
		if lr.N < 1 {
			size = 1
		} else {
			size = int(lr.N)
		}
	}
	buf = make([]byte, size)

	var written int64
	for {
		n, err := io.CopyBuffer(dst, src, buf)
		written += n // n is always non-negative
		if errors.Is(err, unix.EINTR) {
			continue
		}
		return written, err
	}
}
