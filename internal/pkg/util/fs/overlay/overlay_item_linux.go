// Copyright (c) 2019-2023, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package overlay

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/sylabs/singularity/v4/internal/pkg/util/fs"
	fsfuse "github.com/sylabs/singularity/v4/internal/pkg/util/fs/fuse"
	"github.com/sylabs/singularity/v4/pkg/image"
	"github.com/sylabs/singularity/v4/pkg/sylog"
)

// Item represents information about a single overlay item (as specified,
// for example, in a single --overlay argument)
type Item struct {
	// Type represents what type of overlay this is (from among the values in
	// pkg/image)
	Type int

	// Readonly represents whether this is a readonly overlay
	Readonly bool

	// SourcePath is the path of the overlay item, stripped of any
	// colon-prefixed options (like ":ro")
	SourcePath string

	// SourceOffset is the (optional) offset of the overlay filesystem within
	// SourcePath, in bytes.
	SourceOffset int64

	// StagingDir is the directory on which this overlay item is staged, to be
	// used as a source for an overlayfs mount as part of an overlay.Set
	StagingDir string

	// parentDir is the (optional) location of a secure parent-directory in
	// which to create mount directories if needed. If empty, one will be
	// created with os.MkdirTemp()
	parentDir string

	// allowSetuid is set to true to mount the overlay item without the "nosuid" option.
	allowSetuid bool

	// allowDev is set to true to mount the overlay item without the "nodev" option.
	allowDev bool
}

// NewItemFromString takes a string argument, as passed to --overlay, and
// returns an Item struct describing the requested overlay.
func NewItemFromString(overlayString string) (*Item, error) {
	item := Item{Readonly: false}

	var err error
	splitted := strings.SplitN(overlayString, ":", 2)
	item.SourcePath, err = filepath.Abs(splitted[0])
	if err != nil {
		return nil, fmt.Errorf("error while trying to convert overlay path %q to absolute path: %w", splitted[0], err)
	}

	if len(splitted) > 1 {
		if splitted[1] == "ro" {
			item.Readonly = true
		}
	}

	s, err := os.Stat(item.SourcePath)
	if (err != nil) && os.IsNotExist(err) {
		return nil, fmt.Errorf("specified overlay %q does not exist", item.SourcePath)
	}
	if err != nil {
		return nil, err
	}

	if s.IsDir() {
		item.Type = image.SANDBOX
	} else if err := item.analyzeImageFile(); err != nil {
		return nil, fmt.Errorf("while examining image file %s: %w", item.SourcePath, err)
	}

	return &item, nil
}

// analyzeImageFile attempts to determine the format of an image file based on
// its header
func (i *Item) analyzeImageFile() error {
	img, err := image.Init(i.SourcePath, false)
	if err != nil {
		return err
	}

	switch img.Type {
	case image.SQUASHFS:
		i.Type = image.SQUASHFS
		// squashfs image must be readonly
		i.Readonly = true
	case image.EXT3:
		i.Type = image.EXT3
	default:
		return fmt.Errorf("image %s is of a type that is not currently supported as overlay", i.SourcePath)
	}

	return nil
}

// SetParentDir sets the parent-dir in which to create overlay-specific mount
// directories.
func (i *Item) SetParentDir(d string) {
	i.parentDir = d
}

// SetAllowDev sets whether to allow devices on the mount for this item.
func (i *Item) SetAllowDev(a bool) {
	i.allowDev = a
}

// SetAllowSetuid sets whether to allow setuid binaries on the mount for this item.
func (i *Item) SetAllowSetuid(a bool) {
	i.allowSetuid = a
}

// GetParentDir gets a parent-dir in which to create overlay-specific mount
// directories. If one has not been set using SetParentDir(), one will be
// created using os.MkdirTemp().
func (i *Item) GetParentDir() (string, error) {
	// Check if we've already been given a parentDir value; if not, create
	// one using os.MkdirTemp()
	if len(i.parentDir) > 0 {
		return i.parentDir, nil
	}

	d, err := os.MkdirTemp("", "overlay-parent-")
	if err != nil {
		return d, err
	}

	i.parentDir = d
	return i.parentDir, nil
}

// Mount performs the necessary steps to mount an individual Item. Note that
// this method does not mount the assembled overlay itself. That happens in
// Set.Mount().
func (i *Item) Mount(ctx context.Context) error {
	var err error
	switch i.Type {
	case image.SANDBOX:
		err = i.mountDir()

	case image.SQUASHFS, image.EXT3:
		err = i.mountWithFuse(ctx)

	default:
		return fmt.Errorf("internal error: unrecognized image type in overlay.Item.Mount() (type: %v)", i.Type)
	}

	if err != nil {
		return err
	}

	if !i.Readonly {
		return i.prepareWritableOverlay()
	}

	return nil
}

// GetMountDir returns the path to the directory that will actually be mounted
// for this overlay. For squashfs overlays, this is equivalent to the
// Item.StagingDir field. But for all other overlays, it is the "upper"
// subdirectory of Item.StagingDir.
func (i Item) GetMountDir() string {
	switch i.Type {
	case image.SQUASHFS:
		return i.StagingDir

	case image.SANDBOX:
		if (!i.Readonly) || fs.IsDir(i.Upper()) {
			return i.Upper()
		}
		return i.StagingDir

	default:
		return i.Upper()
	}
}

// mountDir mounts directory-based Items. This involves bind-mounting followed
// by remounting of the directory onto itself. This pattern of bind-mount
// followed by remount allows application of more restrictive mount flags than
// are in force on the underlying filesystem.
func (i *Item) mountDir() error {
	var err error
	if len(i.StagingDir) < 1 {
		i.StagingDir = i.SourcePath
	}

	if err = EnsureOverlayDir(i.StagingDir, false, 0); err != nil {
		return fmt.Errorf("error accessing directory %s: %w", i.StagingDir, err)
	}

	sylog.Debugf("Performing identity bind-mount of %q", i.StagingDir)
	if err = syscall.Mount(i.StagingDir, i.StagingDir, "", syscall.MS_BIND, ""); err != nil {
		return fmt.Errorf("failed to bind %s: %w", i.StagingDir, err)
	}

	// Best effort to cleanup mount
	defer func() {
		if err != nil {
			sylog.Debugf("Encountered error with current overlay set; attempting to unmount %q", i.StagingDir)
			syscall.Unmount(i.StagingDir, syscall.MNT_DETACH)
		}
	}()

	// Try to perform remount to apply restrictive flags.
	var remountOpts uintptr = syscall.MS_REMOUNT | syscall.MS_BIND
	if i.Readonly {
		// Not strictly necessary as will be read-only in assembled overlay,
		// however this stops any erroneous writes through the stagingDir.
		remountOpts |= syscall.MS_RDONLY
	}
	if !i.allowDev {
		remountOpts |= syscall.MS_NODEV
	}
	if !i.allowSetuid {
		remountOpts |= syscall.MS_NOSUID
	}
	sylog.Debugf("Performing remount of %q", i.StagingDir)
	if err = syscall.Mount("", i.StagingDir, "", remountOpts, ""); err != nil {
		return fmt.Errorf("failed to remount %s: %w", i.StagingDir, err)
	}

	return nil
}

// mountWithFuse mounts an image to a temporary directory
func (i *Item) mountWithFuse(ctx context.Context) error {
	parentDir, err := i.GetParentDir()
	if err != nil {
		return err
	}

	im := fsfuse.ImageMount{
		Type:         i.Type,
		Readonly:     i.Readonly,
		SourcePath:   i.SourcePath,
		EnclosingDir: parentDir,
		AllowSetuid:  i.allowSetuid,
		AllowDev:     i.allowDev,
	}

	if i.SourceOffset != 0 {
		im.ExtraOpts = []string{fmt.Sprintf("offset=%d", i.SourceOffset)}
	}

	if err := im.Mount(ctx); err != nil {
		return err
	}

	i.StagingDir = im.GetMountPoint()

	return nil
}

// Unmount performs the necessary steps to unmount an individual Item. Note that
// this method does not unmount the overlay itself. That happens in
// Set.Unmount().
func (i Item) Unmount(ctx context.Context) error {
	switch i.Type {
	case image.SANDBOX:
		return i.unmountDir(ctx)

	case image.SQUASHFS, image.EXT3:
		return i.unmountFuse(ctx)

	default:
		return fmt.Errorf("internal error: unrecognized image type in overlay.Item.Unmount() (type: %v)", i.Type)
	}
}

// unmountDir unmounts directory-based Items.
func (i Item) unmountDir(ctx context.Context) error {
	return DetachMount(ctx, i.StagingDir)
}

// unmountFuse unmounts FUSE-based Items.
func (i Item) unmountFuse(ctx context.Context) error {
	defer os.Remove(i.StagingDir)
	err := fsfuse.UnmountWithFuse(ctx, i.StagingDir)
	if err != nil {
		return fmt.Errorf("error while trying to unmount image %q from %s: %w", i.SourcePath, i.StagingDir, err)
	}
	return nil
}

// PrepareWritableOverlay ensures that the upper and work subdirs of a writable
// overlay dir exist, and if not, creates them.
func (i *Item) prepareWritableOverlay() error {
	switch i.Type {
	case image.SANDBOX:
		i.StagingDir = i.SourcePath
		fallthrough
	case image.EXT3:
		if err := EnsureOverlayDir(i.StagingDir, true, 0o755); err != nil {
			return err
		}
		sylog.Debugf("Ensuring %q exists", i.Upper())
		if err := EnsureOverlayDir(i.Upper(), true, 0o755); err != nil {
			sylog.Errorf("Could not create overlay upper dir. If using an overlay image ensure it contains 'upper' and 'work' directories")
			return fmt.Errorf("err encountered while preparing upper subdir of overlay dir %q: %w", i.Upper(), err)
		}
		sylog.Debugf("Ensuring %q exists", i.Work())
		if err := EnsureOverlayDir(i.Work(), true, 0o700); err != nil {
			sylog.Errorf("Could not create overlay work dir. If using an overlay image ensure it contains 'upper' and 'work' directories")
			return fmt.Errorf("err encountered while preparing work subdir of overlay dir %q: %w", i.Work(), err)
		}
	default:
		return fmt.Errorf("unsupported image type in prepareWritableOverlay() (type: %v)", i.Type)
	}

	return nil
}

// Upper returns the "upper"-subdir of the Item's DirToMount field.
// Useful for computing options strings for overlay-related mount system calls.
func (i Item) Upper() string {
	return filepath.Join(i.StagingDir, "upper")
}

// Work returns the "work"-subdir of the Item's DirToMount field. Useful
// for computing options strings for overlay-related mount system calls.
func (i Item) Work() string {
	return filepath.Join(i.StagingDir, "work")
}
