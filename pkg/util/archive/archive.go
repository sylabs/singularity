/*
Contains code adapted from:

   https://github.com/moby/moby/tree/master/pkg/archive

Copyright 2013-2018 Docker, Inc.

   Licensed under the Apache License, Version 2.0 (the "License");
   you may not use this file except in compliance with the License.
   You may obtain a copy of the License at

       https://www.apache.org/licenses/LICENSE-2.0

   Unless required by applicable law or agreed to in writing, software
   distributed under the License is distributed on an "AS IS" BASIS,
   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
   See the License for the specific language governing permissions and
   limitations under the License.
*/

package archive

import (
	"archive/tar"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	mobyarchive "github.com/moby/go-archive"
	mobyuser "github.com/moby/sys/user"
	mobyuserns "github.com/moby/sys/userns"
	"github.com/sylabs/singularity/v4/pkg/sylog"
)

type unpacker struct {
	rootPath string
	destPath string
	destRel  string // destPath relative to rootPath
	root     *os.Root
	options  *mobyarchive.TarOptions
}

// newUnpacker creates a new unpacker which will unpack to destPath. All created
// files will be under destPath, and links must not target outside rootPath. The
// caller must call unpacker.Close() when finished, to release the os.Root.
func newUnpacker(rootPath, destPath string, options *mobyarchive.TarOptions) (*unpacker, error) {
	if options == nil {
		options = &mobyarchive.TarOptions{}
	}
	if options.ExcludePatterns == nil {
		options.ExcludePatterns = []string{}
	}
	if options.WhiteoutFormat != 0 {
		return nil, fmt.Errorf("options.WhiteoutFormat is not supported by unpacker")
	}

	if !filepath.IsAbs(rootPath) {
		return nil, fmt.Errorf("rootPath must be an absolute path: %q", rootPath)
	}
	rootPath = filepath.Clean(rootPath)

	if !filepath.IsAbs(destPath) {
		return nil, fmt.Errorf("destPath must be an absolute path: %q", destPath)
	}
	destPath = filepath.Clean(destPath)

	rel, err := filepath.Rel(rootPath, destPath)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate relative path from rootPath to destPath: %w", err)
	}
	if !filepath.IsLocal(rel) {
		return nil, fmt.Errorf("destPath %q is not under rootPath %q", destPath, rootPath)
	}

	root, err := os.OpenRoot(rootPath)
	if err != nil {
		return nil, err
	}

	return &unpacker{
		rootPath: rootPath,
		destPath: destPath,
		destRel:  rel,
		root:     root,
		options:  options,
	}, nil
}

func (u *unpacker) close() error {
	if u.root != nil {
		return u.root.Close()
	}
	return nil
}

// unpackWithRoot unpacks the decompressedArchive to dest with options.  All
// created files will be under destPath, and links must not target outside
// rootPath.
func unpackWithRoot(decompressedArchive io.Reader, destPath, rootPath string, options *mobyarchive.TarOptions) error {
	tr := tar.NewReader(decompressedArchive)
	trBuf := BufioReader32KPool.Get(nil)
	defer BufioReader32KPool.Put(trBuf)

	var dirs []*tar.Header

	destPath, err := filepath.Abs(destPath)
	if err != nil {
		return err
	}
	rootPath, err = filepath.Abs(rootPath)
	if err != nil {
		return err
	}

	u, err := newUnpacker(rootPath, destPath, options)
	if err != nil {
		return err
	}
	defer u.close()

	// Iterate through the files in the archive.
loop:
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			// end of tar archive
			break
		}
		if err != nil {
			return err
		}

		// ignore XGlobalHeader early to avoid creating parent directories for them
		if hdr.Typeflag == tar.TypeXGlobalHeader {
			sylog.Debugf("PAX Global Extended Headers found for %s and ignored", hdr.Name)
			continue
		}

		// Normalize name, for safety and for a simple is-root check
		// This keeps "../" as-is, but normalizes "/../" to "/". Or Windows:
		// This keeps "..\" as-is, but normalizes "\..\" to "\".
		hdr.Name = filepath.Clean(hdr.Name)

		for _, exclude := range u.options.ExcludePatterns {
			if strings.HasPrefix(hdr.Name, exclude) {
				continue loop
			}
		}

		relPath, err := u.destToRoot(hdr.Name)
		if err != nil {
			return err
		}

		if err := u.createImpliedDirectories(relPath, hdr); err != nil {
			return err
		}

		// If path exits we almost always just want to remove and replace it
		// The only exception is when it is a directory *and* the file from
		// the layer is also a directory. Then we want to merge them (i.e.
		// just apply the metadata from the layer).
		if fi, err := u.root.Lstat(relPath); err == nil {
			if u.options.NoOverwriteDirNonDir && fi.IsDir() && hdr.Typeflag != tar.TypeDir {
				// If NoOverwriteDirNonDir is true then we cannot replace
				// an existing directory with a non-directory from the archive.
				return fmt.Errorf("cannot overwrite directory %q with non-directory %q", filepath.Join(u.rootPath, relPath), u.destPath)
			}

			if u.options.NoOverwriteDirNonDir && !fi.IsDir() && hdr.Typeflag == tar.TypeDir {
				// If NoOverwriteDirNonDir is true then we cannot replace
				// an existing non-directory with a directory from the archive.
				return fmt.Errorf("cannot overwrite non-directory %q with directory %q", filepath.Join(u.rootPath, relPath), u.destPath)
			}

			if fi.IsDir() && hdr.Name == "." {
				continue
			}

			if !fi.IsDir() || !(hdr.Typeflag == tar.TypeDir) { //nolint:staticcheck
				if err := u.root.RemoveAll(relPath); err != nil {
					return err
				}
			}
		} else if !os.IsNotExist(err) {
			return err
		}
		trBuf.Reset(tr)

		if err := remapIDs(u.options.IDMap, hdr); err != nil {
			return err
		}

		if err := u.createTarFile(relPath, hdr, trBuf); err != nil {
			return err
		}

		// Directory mtimes must be handled at the end to avoid further
		// file creation in them to modify the directory mtime
		if hdr.Typeflag == tar.TypeDir {
			dirs = append(dirs, hdr)
		}
	}

	for _, hdr := range dirs {
		relPath, err := u.destToRoot(hdr.Name)
		if err != nil {
			return err
		}

		if err := Chtimes(u.root, relPath, hdr.AccessTime, hdr.ModTime); err != nil {
			return err
		}
	}
	return nil
}

// destToRoot converts a tar entry name (relative to destPath) into a path
// relative to rootPath / the os.Root. The entry must not escape destPath, so
// that all created files remain under destPath.
func (u *unpacker) destToRoot(name string) (string, error) {
	if !filepath.IsLocal(name) {
		return "", fmt.Errorf("%q is outside of destination", name)
	}
	return filepath.Join(u.destRel, name), nil
}

// relToRoot converts a path relative to destPath into a path relative
// to rootPath / the os.Root. Unlike destToRoot, the target may resolve above
// destPath, as long as it remains under rootPath.
func (u *unpacker) relToRoot(name string) (string, error) {
	if filepath.IsAbs(name) {
		return "", fmt.Errorf("%q is outside of root", name)
	}
	relPath := filepath.Join(u.destRel, name)
	if !filepath.IsLocal(relPath) {
		return "", fmt.Errorf("%q is outside of root", name)
	}
	return relPath, nil
}

// createImpliedDirectories will create all parent directories of the current path with default permissions, if they do
// not already exist. This is possible as the tar format supports 'implicit' directories, where their existence is
// defined by the paths of files in the tar, but there are no header entries for the directories themselves, and thus
// we most both create them and choose metadata like permissions.
//
// The caller should have performed filepath.Clean(hdr.Name), so hdr.Name will now be in the filepath format for the OS
// on which the daemon is running. This precondition is required because this function assumes a OS-specific path
// separator when checking that a path is not the root.
func (u *unpacker) createImpliedDirectories(relPath string, hdr *tar.Header) error {
	// Not the root directory, ensure that the parent directory exists
	if !strings.HasSuffix(hdr.Name, string(os.PathSeparator)) {
		parent := filepath.Dir(relPath)
		if parent != "." {
			// RootPair() is confined inside this loop as most cases will not require a call, so we can spend some
			// unneeded function calls in the uncommon case to encapsulate logic -- implied directories are a niche
			// usage that reduces the portability of an image.
			uid, gid := u.options.IDMap.RootPair()

			if err := u.mkdirAllAndChown(parent, mobyarchive.ImpliedDirectoryMode, uid, gid); err != nil {
				return err
			}
		}
	}

	return nil
}

func remapIDs(idMapping mobyuser.IdentityMapping, hdr *tar.Header) error {
	uid, gid, err := idMapping.ToHost(hdr.Uid, hdr.Gid)
	hdr.Uid, hdr.Gid = uid, gid
	return err
}

const paxSchilyXattr = "SCHILY.xattr."

// createTarFile creates a file from a tar record through u.root. Hard links
// and symlinks must target a path under u.rootPath.
func (u *unpacker) createTarFile(path string, hdr *tar.Header, reader io.Reader) error {
	Lchown := !u.options.NoLchown
	bestEffortXattrs := u.options.BestEffortXattrs
	chownOpts := u.options.ChownOpts

	// hdr.Mode is in linux format, which we can use for sycalls,
	// but for os.Foo() calls we need the mode converted to os.FileMode,
	// so use hdrInfo.Mode() (they differ for e.g. setuid bits)
	hdrInfo := hdr.FileInfo()

	switch hdr.Typeflag {
	case tar.TypeDir:
		// Create directory unless it exists as a directory already.
		// In that case we just want to merge the two
		if fi, err := u.root.Lstat(path); err != nil || !fi.IsDir() {
			if err := u.root.Mkdir(path, hdrInfo.Mode()&0o777); err != nil {
				return err
			}
		}

	case tar.TypeReg:
		file, err := u.root.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, hdrInfo.Mode()&0o777)
		if err != nil {
			return err
		}
		if _, err := io.Copy(file, reader); err != nil {
			file.Close()
			return err
		}
		file.Close()

	case tar.TypeBlock, tar.TypeChar:
		sylog.Warningf("Skipping %s - block / char devices are not copied", path)
		return nil

	case tar.TypeFifo:
		// Handle this is an OS-specific way
		sylog.Warningf("Skipping %s - fifos are not copied", filepath.Join(u.rootPath, path))

	case tar.TypeLink:
		targetPath, err := u.relToRoot(filepath.Clean(hdr.Linkname))
		if err != nil {
			return fmt.Errorf("invalid hard link target: %s: %w", hdr.Linkname, err)
		}

		if err := u.root.Link(targetPath, path); err != nil {
			return err
		}

	case tar.TypeSymlink:
		// path is the location of a symlink relative to the unpacker.root.
		// Linkname is the target relative to the location of the symlink. Check
		// it doesn't point outside of the root, if it's relative.
		//
		// The Join here explicitly allows absolute symlink targets, which must
		// be copied as part of a rootfs for it to be functional.
		targetRel := filepath.Join(filepath.Dir(path), hdr.Linkname) //nolint:gosec
		if !filepath.IsLocal(targetRel) {
			return fmt.Errorf("invalid symlink target: %s: %q is outside of root", hdr.Linkname, targetRel)
		}
		if err := u.root.Symlink(hdr.Linkname, path); err != nil {
			return err
		}

	case tar.TypeXGlobalHeader:
		sylog.Debugf("PAX Global Extended Headers found and ignored")
		return nil

	default:
		return fmt.Errorf("unhandled tar header type %d", hdr.Typeflag)
	}

	if Lchown {
		if chownOpts == nil {
			chownOpts = &mobyarchive.ChownOpts{UID: hdr.Uid, GID: hdr.Gid}
		}
		if err := u.root.Lchown(path, chownOpts.UID, chownOpts.GID); err != nil {
			msg := "failed to Lchown %q for UID %d, GID %d"
			if errors.Is(err, syscall.EINVAL) && mobyuserns.RunningInUserNS() {
				msg += " (try increasing the number of subordinate IDs in /etc/subuid and /etc/subgid)"
			}
			return fmt.Errorf("%s %s %d %d: %w", msg, filepath.Join(u.rootPath, path), hdr.Uid, hdr.Gid, err)
		}
	}

	var xattrErrs []string
	for key, value := range hdr.PAXRecords {
		xattr, ok := strings.CutPrefix(key, paxSchilyXattr)
		if !ok {
			continue
		}
		if err := Lsetxattr(filepath.Join(u.rootPath, path), xattr, []byte(value), 0); err != nil {
			if bestEffortXattrs && errors.Is(err, syscall.ENOTSUP) || errors.Is(err, syscall.EPERM) {
				// EPERM occurs if modifying xattrs is not allowed. This can
				// happen when running in userns with restrictions (ChromeOS).
				xattrErrs = append(xattrErrs, err.Error())
				continue
			}
			return err
		}
	}

	if len(xattrErrs) > 0 {
		sylog.Warningf("Ignored xattrs in archive: underlying filesystem doesn't support them: %v", xattrErrs)
	}

	// There is no LChmod, so ignore mode for symlink. Also, this
	// must happen after chown, as that can modify the file mode
	if err := u.handleLChmod(hdr, path, hdrInfo); err != nil {
		return err
	}

	aTime := hdr.AccessTime
	if aTime.Before(hdr.ModTime) {
		// Last access time should never be before last modified time.
		aTime = hdr.ModTime
	}

	// Chtimes doesn't support a NOFOLLOW flag atm
	if hdr.Typeflag == tar.TypeLink {
		if fi, err := u.root.Lstat(path); err == nil && (fi.Mode()&os.ModeSymlink == 0) {
			if err := Chtimes(u.root, path, aTime, hdr.ModTime); err != nil {
				return err
			}
		}
	} else if hdr.Typeflag != tar.TypeSymlink {
		if err := Chtimes(u.root, path, aTime, hdr.ModTime); err != nil {
			return err
		}
	} else {
		ts := []syscall.Timespec{timeToTimespec(aTime), timeToTimespec(hdr.ModTime)}
		// LUtimesNano is path based because the platform API is path based.
		// Creation was rooted above, but this timestamp update is not fd-relative.
		if err := LUtimesNano(filepath.Join(u.rootPath, path), ts); err != nil {
			return err
		}
	}
	return nil
}

func (u *unpacker) handleLChmod(hdr *tar.Header, path string, hdrInfo os.FileInfo) error {
	if hdr.Typeflag == tar.TypeLink {
		if fi, err := u.root.Lstat(path); err == nil && (fi.Mode()&os.ModeSymlink == 0) {
			if err := u.root.Chmod(path, hdrInfo.Mode()); err != nil {
				return err
			}
		}
	} else if hdr.Typeflag != tar.TypeSymlink {
		if err := u.root.Chmod(path, hdrInfo.Mode()); err != nil {
			return err
		}
	}
	return nil
}

func (u *unpacker) mkdirAllAndChown(path string, perm os.FileMode, uid, gid int) error {
	var current string
	for _, part := range strings.Split(path, string(os.PathSeparator)) {
		if part == "" || part == "." {
			continue
		}
		current = filepath.Join(current, part)
		if fi, err := u.root.Lstat(current); err == nil {
			if !fi.IsDir() {
				return fmt.Errorf("%s exists and is not a directory", current)
			}
			continue
		} else if !os.IsNotExist(err) {
			return err
		}

		if err := u.root.Mkdir(current, perm&0o777); err != nil {
			if !os.IsExist(err) {
				return err
			}
			continue
		}
		if err := u.root.Lchown(current, uid, gid); err != nil {
			return err
		}
	}
	return nil
}

func timeToTimespec(time time.Time) (ts syscall.Timespec) {
	if time.IsZero() {
		// Return UTIME_OMIT special value
		ts.Sec = 0
		ts.Nsec = (1 << 30) - 2
		return
	}
	return syscall.NsecToTimespec(time.UnixNano())
}
