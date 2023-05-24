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

	"github.com/sylabs/singularity/internal/pkg/util/bin"
	"github.com/sylabs/singularity/pkg/image"
	"github.com/sylabs/singularity/pkg/sylog"
)

// Item represents information about a single overlay item (as specified,
// for example, in a single --overlay argument)
type Item struct {
	// Type represents what type of overlay this is (from among the values in pkg/image)
	Type int

	// Writable represents whether this is a writable overlay
	Writable bool

	// BarePath is the path of the overlay stripped of any colon-prefixed
	// options (like ":ro")
	BarePath string

	// DirToMount is the path of the directory that will actually be passed to
	// the mount system-call when mounting this overlay
	DirToMount string

	// secureParentDir is the (optional) location of a secure parent-directory
	// in which to create mount directories if needed. If empty, one will be
	// created with os.MkdirTemp()
	secureParentDir string
}

// NewOverlayFromString takes a string argument, as passed to --overlay, and returns
// an overlayInfo struct describing the requested overlay.
func NewOverlayFromString(overlayString string) (*Item, error) {
	overlay := Item{}

	splitted := strings.SplitN(overlayString, ":", 2)
	overlay.BarePath = splitted[0]
	if len(splitted) > 1 {
		if splitted[1] == "ro" {
			overlay.Writable = false
		}
	}

	s, err := os.Stat(overlay.BarePath)
	if (err != nil) && os.IsNotExist(err) {
		return nil, fmt.Errorf("specified overlay %q does not exist", overlay.BarePath)
	}
	if err != nil {
		return nil, err
	}

	if s.IsDir() {
		overlay.Type = image.SANDBOX
	} else if err := analyzeImageFile(&overlay); err != nil {
		return nil, fmt.Errorf("error encountered while examining image file %s: %s", overlay.BarePath, err)
	}

	return &overlay, nil
}

// analyzeImageFile attempts to determine the format of an image file based on
// its header
func analyzeImageFile(overlay *Item) error {
	img, err := image.Init(overlay.BarePath, overlay.Writable)
	if err != nil {
		return fmt.Errorf("error encountered while trying to examine image: %s", err)
	}

	switch img.Type {
	case image.SQUASHFS:
		overlay.Type = image.SQUASHFS
		// squashfs image must be readonly
		overlay.Writable = false
		return nil
	case image.EXT3:
		overlay.Type = image.EXT3
	default:
		return fmt.Errorf("image %s is of a type that is not currently supported as overlay", overlay.BarePath)
	}

	return nil
}

// SetSecureParentDir sets the secure parent-dir in which to create
// overlay-specific mount directories.
func (i *Item) SetSecureParentDir(d string) {
	i.secureParentDir = d
}

// GetSecureParentDir gets a secure parent-dir in which to create
// overlay-specific mount directories. If one has not been set using
// SetSecureParentDir(), one will be created using os.MkdirTemp().
func (i *Item) GetSecureParentDir() (string, error) {
	// Check if we've already been given a SecureParentDir value; if not, create
	// one using os.MkdirTemp()
	if len(i.secureParentDir) > 0 {
		return i.secureParentDir, nil
	}

	d, err := os.MkdirTemp("", "overlay-parent-")
	if err != nil {
		return d, err
	}

	i.secureParentDir = d
	return i.secureParentDir, nil
}

// Mount performs the necessary steps to mount an individual OverlayItem. Note
// that this method does not mount the assembled overlay itself. That happens in
// OverlaySet.Mount().
func (i *Item) Mount() error {
	switch i.Type {
	case image.SANDBOX:
		return i.mountDir()
	case image.SQUASHFS:
		return i.mountWithFuse("squashfuse")
	case image.EXT3:
		return i.mountWithFuse("fuse2fs")
	default:
		return fmt.Errorf("internal error: unrecognized image type in overlay.Item.Mount() (type: %v)", i.Type)
	}
}

// mountDir mounts directory-based OverlayItems. This involves bind-mounting
// followed by remounting of the directory onto itself. This pattern of
// bind-mount followed by remount allows application of more restrictive mount
// flags than are in force on the underlying filesystem.
func (i *Item) mountDir() error {
	var err error
	if len(i.DirToMount) < 1 {
		i.DirToMount = i.BarePath
	}

	if err = EnsureOverlayDir(i.DirToMount, false, 0); err != nil {
		return fmt.Errorf("error accessing directory %s: %s", i.DirToMount, err)
	}

	sylog.Debugf("Performing identity bind-mount of %q", i.DirToMount)
	if err = syscall.Mount(i.DirToMount, i.DirToMount, "", syscall.MS_BIND, ""); err != nil {
		return fmt.Errorf("failed to bind %s: %s", i.DirToMount, err)
	}

	// Best effort to cleanup mount
	defer func() {
		if err != nil {
			sylog.Debugf("Encountered error with current OverlaySet; attempting to unmount %q", i.DirToMount)
			syscall.Unmount(i.DirToMount, syscall.MNT_DETACH)
		}
	}()

	// Try to perform remount
	sylog.Debugf("Performing remount of %q", i.DirToMount)
	if err = syscall.Mount("", i.DirToMount, "", syscall.MS_REMOUNT|syscall.MS_BIND, ""); err != nil {
		return fmt.Errorf("failed to remount %s: %s", i.DirToMount, err)
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
		return fmt.Errorf("use of image %q as overlay requires %s to be installed: %s", i.BarePath, fuseMountTool, err)
	}

	// Even though fusermount is not needed for this step, we shouldn't perform
	// the mount unless we have the necessary tools to eventually unmount it
	_, err = bin.FindBin("fusermount")
	if err != nil {
		return fmt.Errorf("use of image %q as overlay requires fusermount to be installed: %s", i.BarePath, err)
	}

	// Obtain parent directory in which to create overlay-related mount
	// directories. See https://github.com/apptainer/singularity/pull/5575 for
	// related discussion.
	parentDir, err := i.GetSecureParentDir()
	if err != nil {
		return fmt.Errorf("error while trying to create parent dir for overlay %q: %s", i.BarePath, err)
	}
	fuseMountDir, err := os.MkdirTemp(parentDir, "overlay-mountpoint-")
	if err != nil {
		return fmt.Errorf("failed to create temporary dir %q for overlay %q: %s", fuseMountDir, i.BarePath, err)
	}

	// Best effort to cleanup temporary dir
	defer func() {
		if err != nil {
			sylog.Debugf("Encountered error with current OverlaySet; attempting to remove %q", fuseMountDir)
			os.Remove(fuseMountDir)
		}
	}()

	execCmd := exec.Command(fuseMountCmd, i.BarePath, fuseMountDir)
	execCmd.Stderr = os.Stderr
	_, err = execCmd.Output()
	if err != nil {
		return fmt.Errorf("encountered error while trying to mount image %q as overlay at %s: %s", i.BarePath, fuseMountDir, err)
	}
	i.DirToMount = fuseMountDir

	return nil
}

// Unmount performs the necessary steps to unmount an individual OverlayItem.
// Note that this method does not unmount the overlay itself. That happens in
// OverlaySet.Unmount().
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

// unmountDir unmounts directory-based OverlayItems.
func (i Item) unmountDir() error {
	return DetachMount(i.DirToMount)
}

// unmountFuse unmounts FUSE-based OverlayItems.
func (i Item) unmountFuse() error {
	defer os.Remove(i.DirToMount)
	fusermountCmd, innerErr := bin.FindBin("fusermount")
	if innerErr != nil {
		// The code in performIndividualMounts() should not have created
		// a FUSE-based overlay without fusermount in place
		return fmt.Errorf("internal error: FUSE-based mount created without fusermount installed: %s", innerErr)
	}
	execCmd := exec.Command(fusermountCmd, "-u", i.DirToMount)
	execCmd.Stderr = os.Stderr
	_, innerErr = execCmd.Output()
	if innerErr != nil {
		return fmt.Errorf("encountered error while trying to unmount image %q from %s: %s", i.BarePath, i.DirToMount, innerErr)
	}
	return nil
}

// PrepareWritableOverlay ensures that the upper and work subdirs of a writable
// overlay dir exist, and if not, creates them.
func (i *Item) prepareWritableOverlay() error {
	switch i.Type {
	case image.SANDBOX:
		i.DirToMount = i.BarePath
		if err := EnsureOverlayDir(i.DirToMount, true, 0o755); err != nil {
			return err
		}
		sylog.Debugf("Ensuring %q exists", i.Upper())
		if err := EnsureOverlayDir(i.Upper(), true, 0o755); err != nil {
			return fmt.Errorf("err encountered while preparing upper subdir of overlay dir %q: %w", i.Upper(), err)
		}
		sylog.Debugf("Ensuring %q exists", i.Work())
		if err := EnsureOverlayDir(i.Work(), true, 0o700); err != nil {
			return fmt.Errorf("err encountered while preparing work subdir of overlay dir %q: %w", i.Work(), err)
		}
	default:
		return fmt.Errorf("internal error: unrecognized image type in prepareWritableOverlay() (type: %v)", i.Type)
	}

	return nil
}

// Upper returns the "upper"-subdir of the OverlayItem's DirToMount field.
// Useful for computing options strings for overlay-related mount system calls.
func (i Item) Upper() string {
	return filepath.Join(i.DirToMount, "upper")
}

// Work returns the "work"-subdir of the OverlayItem's DirToMount field. Useful
// for computing options strings for overlay-related mount system calls.
func (i Item) Work() string {
	return filepath.Join(i.DirToMount, "work")
}
