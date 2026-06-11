// Copyright (c) 2021-2026, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package archive

import (
	"fmt"
	"io"
	"os"

	"github.com/moby/go-archive"
	"github.com/moby/go-archive/compression"
	"github.com/moby/sys/user"
	"github.com/sylabs/singularity/v4/pkg/sylog"
)

// CopyWithTar is a wrapper around the docker pkg/archive/copy CopyWithTar
// function allowing unprivileged use. It forces ownership to the current
// uid/gid in unprivileged situations. No archive entries may be written above
// dst. Hard links to files outside of dst are not allowed. Relative symlinks to
// targets outside of dst are not allowed. Absolute symlink targets are allowed,
// but will not be traversed outside of dst when extracting entries.
func CopyWithTar(src, dst string, disableIDMapping bool) error {
	ar := newRootedArchiver("", disableIDMapping)
	return ar.CopyWithTar(src, dst)
}

// CopyWithTarWithRoots copies files as with CopyWithTar, but uses a custom
// ar.Untar rooted at root, and not dst. This allows copying from src to dst,
// where this will create a hard link or symlink to a location above dst, but
// under dstRoot.
func CopyWithTarWithRoot(src, dst, root string, disableIDMapping bool) error {
	ar := newRootedArchiver(root, disableIDMapping)
	return ar.CopyWithTar(src, dst)
}

// newRootedArchiver returns an Archiver whose Untar is confined to an os.Root at root.
func newRootedArchiver(root string, disableIDMapping bool) *archive.Archiver {
	ar := archive.NewDefaultArchiver()

	// If we are running unprivileged, then squash uid / gid as necessary.
	// TODO: In future, we want to think about preserving effective ownership
	// for fakeroot cases where there will be a mapping allowing non-root, non-user
	// ownership to be preserved.
	euid := os.Geteuid()
	egid := os.Getgid()
	var chownOpts *archive.ChownOpts

	if (euid != 0 || egid != 0) && !disableIDMapping {
		sylog.Debugf("Using unprivileged CopyWithTar (uid=%d, gid=%d)", euid, egid)
		// The docker CopytWithTar function assumes it should create the top-level of dst as the
		// container root user. If we are unprivileged this means setting up an ID mapping
		// from UID/GID 0 to our host UID/GID.
		ar.IDMapping = user.IdentityMapping{
			// Single entry mapping of container root (0) to current uid only
			UIDMaps: []user.IDMap{
				{
					ID:       0,
					ParentID: int64(euid),
					Count:    1,
				},
			},
			// Single entry mapping of container root (0) to current gid only
			GIDMaps: []user.IDMap{
				{
					ID:       0,
					ParentID: int64(egid),
					Count:    1,
				},
			},
		}
		// Actual extraction of files needs to be *always* squashed to our current uid & gid.
		// This requires clearing the IDMaps, and setting a forced UID/GID with ChownOpts for
		// the lower level Untar func called by the archiver.
		chownOpts = &archive.ChownOpts{
			UID: euid,
			GID: egid,
		}
	}

	ar.Untar = func(tarArchive io.Reader, dest string, options *archive.TarOptions) error {
		if tarArchive == nil {
			return fmt.Errorf("empty archive")
		}
		if options == nil {
			options = &archive.TarOptions{}
		}
		if options.ExcludePatterns == nil {
			options.ExcludePatterns = []string{}
		}
		if chownOpts != nil {
			options.IDMap = user.IdentityMapping{}
			options.ChownOpts = chownOpts
		}

		decompressedArchive, err := compression.DecompressStream(tarArchive)
		if err != nil {
			return err
		}
		defer decompressedArchive.Close()

		// For CopyFileWithTar, root is "" and we root at dest, so that a copy
		// of a single file src to dst still succeeds (dest passed to us will be
		// the parent dir for the destination file).
		unpackRoot := root
		if unpackRoot == "" {
			unpackRoot = dest
		}

		return unpackWithRoot(decompressedArchive, dest, unpackRoot, options)
	}

	return ar
}
