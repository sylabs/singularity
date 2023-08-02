// Copyright (c) 2019-2023, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package overlay

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/sylabs/singularity/v4/internal/pkg/util/bin"
	"github.com/sylabs/singularity/v4/internal/pkg/util/fs"
	"github.com/sylabs/singularity/v4/pkg/image"
	"github.com/sylabs/singularity/v4/pkg/sylog"
)

// Item represents information about a single overlay item (as specified,
// for example, in a single --overlay argument)
type Item struct {
	// Type represents what type of overlay this is (from among the values in
	// pkg/image)
	Type int

	// Writable represents whether this is a writable overlay
	Writable bool

	// SourcePath is the path of the overlay item stripped of any colon-prefixed
	// options (like ":ro")
	SourcePath string

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
	item := Item{Writable: true}

	var err error
	splitted := strings.SplitN(overlayString, ":", 2)
	item.SourcePath, err = filepath.Abs(splitted[0])
	if err != nil {
		return nil, fmt.Errorf("error while trying to convert overlay path %q to absolute path: %w", splitted[0], err)
	}

	if len(splitted) > 1 {
		if splitted[1] == "ro" {
			item.Writable = false
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
		i.Writable = false
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
func (i *Item) Mount() error {
	var err error
	switch i.Type {
	case image.SANDBOX:
		err = i.mountDir()
	case image.SQUASHFS:
		err = i.mountWithFuse("squashfuse")
	case image.EXT3:
		i.mountWithFuse("fuse2fs")
	default:
		return fmt.Errorf("internal error: unrecognized image type in overlay.Item.Mount() (type: %v)", i.Type)
	}

	if err != nil {
		return err
	}

	if i.Writable {
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
		if i.Writable || fs.IsDir(i.Upper()) {
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
	if !i.Writable {
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

// mountWithFuse mounts an image to a temporary directory using a specified fuse
// tool. It also verifies that fusermount is present before performing the
// mount.
func (i *Item) mountWithFuse(fuseMountTool string) error {
	var err error
	fuseMountCmd, err := bin.FindBin(fuseMountTool)
	if err != nil {
		return fmt.Errorf("use of image %q as overlay requires %s to be installed: %w", i.SourcePath, fuseMountTool, err)
	}

	// Even though fusermount is not needed for this step, we shouldn't perform
	// the mount unless we have the necessary tools to eventually unmount it
	_, err = bin.FindBin("fusermount")
	if err != nil {
		return fmt.Errorf("use of image %q as overlay requires fusermount to be installed: %w", i.SourcePath, err)
	}

	// Obtain parent directory in which to create overlay-related mount
	// directories. See https://github.com/apptainer/singularity/pull/5575 for
	// related discussion.
	parentDir, err := i.GetParentDir()
	if err != nil {
		return fmt.Errorf("error while trying to create parent dir for overlay %q: %w", i.SourcePath, err)
	}
	fuseMountDir, err := os.MkdirTemp(parentDir, "overlay-mountpoint-")
	if err != nil {
		return fmt.Errorf("failed to create temporary dir %q for overlay %q: %w", fuseMountDir, i.SourcePath, err)
	}

	// Best effort to cleanup temporary dir
	defer func() {
		if err != nil {
			sylog.Debugf("Encountered error with current overlay set; attempting to remove %q", fuseMountDir)
			os.Remove(fuseMountDir)
		}
	}()

	args := make([]string, 0, 4)

	// TODO: Think through what makes sense for file ownership in FUSE-mounted
	// images, vis a vis id-mappings and user-namespaces.
	opts := "uid=0,gid=0"
	if !i.Writable {
		// Not strictly necessary as will be read-only in assembled overlay,
		// however this stops any erroneous writes through the stagingDir.
		opts += ",ro"
	}
	// FUSE defaults to nosuid,nodev - attempt to reverse if AllowDev/Setuid requested.
	if i.allowDev {
		opts += ",dev"
	}
	if i.allowSetuid {
		opts += ",suid"
	}
	args = append(args, "-o", opts)

	args = append(args, i.SourcePath)
	args = append(args, fuseMountDir)
	sylog.Debugf("Executing FUSE mount command: %s %s", fuseMountCmd, strings.Join(args, " "))
	execCmd := exec.Command(fuseMountCmd, args...)
	execCmd.Stderr = os.Stderr
	_, err = execCmd.Output()
	if err != nil {
		return fmt.Errorf("encountered error while trying to mount image %q as overlay at %s: %w", i.SourcePath, fuseMountDir, err)
	}
	i.StagingDir = fuseMountDir

	return nil
}

// Unmount performs the necessary steps to unmount an individual Item. Note that
// this method does not unmount the overlay itself. That happens in
// Set.Unmount().
func (i Item) Unmount() error {
	switch i.Type {
	case image.SANDBOX:
		return i.unmountDir()

	case image.SQUASHFS:
		fallthrough
	case image.EXT3:
		return i.unmountFuse()

	default:
		return fmt.Errorf("internal error: unrecognized image type in overlay.Item.Unmount() (type: %v)", i.Type)
	}
}

// unmountDir unmounts directory-based Items.
func (i Item) unmountDir() error {
	return DetachMount(i.StagingDir)
}

// unmountFuse unmounts FUSE-based Items.
func (i Item) unmountFuse() error {
	defer os.Remove(i.StagingDir)
	err := UnmountWithFuse(i.StagingDir)
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
