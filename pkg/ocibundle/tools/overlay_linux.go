// Copyright (c) 2019-2023, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package tools

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

// OverlayItem represents information about a single overlay item (as specified,
// for example, in a single --overlay argument)
type OverlayItem struct {
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

	// SecureParentDir is the (optional) location of a secure parent-directory
	// in which to create mount directories if needed. If empty, one will be
	// created with os.MkdirTemp()
	SecureParentDir string
}

// NewOverlayFromString takes a string argument, as passed to --overlay, and returns
// an overlayInfo struct describing the requested overlay.
func NewOverlayFromString(overlayString string) (*OverlayItem, error) {
	overlay := OverlayItem{}

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
func analyzeImageFile(overlay *OverlayItem) error {
	img, err := image.Init(overlay.BarePath, overlay.Writable)
	if err != nil {
		return fmt.Errorf("error encountered while trying to examine image")
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

// Mount performs the necessary steps to mount an individual OverlayItem. Note
// that this method does not mount the assembled overlay itself. That happens in
// OverlaySet.Mount().
func (o *OverlayItem) Mount() error {
	switch o.Type {
	case image.SANDBOX:
		return o.mountDir()
	case image.SQUASHFS:
		return o.mountSquashfs()
	default:
		return fmt.Errorf("internal error: unrecognized image type in prepareWritableOverlay() (type: %v)", o.Type)
	}
}

// mountDir mounts directory-based OverlayItems. This involves bind-mounting
// followed by remounting of the directory onto itself. This pattern of
// bind-mount followed by remount allows application of more restrictive mount
// flags than are in force on the underlying filesystem.
func (o *OverlayItem) mountDir() error {
	var err error
	if len(o.DirToMount) < 1 {
		o.DirToMount = o.BarePath
	}

	if err = ensureOverlayDir(o.DirToMount, false, 0); err != nil {
		return fmt.Errorf("error accessing directory %s: %s", o.DirToMount, err)
	}

	sylog.Debugf("Performing identity bind-mount of %q", o.DirToMount)
	if err = syscall.Mount(o.DirToMount, o.DirToMount, "", syscall.MS_BIND, ""); err != nil {
		return fmt.Errorf("failed to bind %s: %s", o.DirToMount, err)
	}

	// Best effort to cleanup mount
	defer func() {
		if err != nil {
			sylog.Debugf("Encountered error with current OverlaySet; attempting to unmount %q", o.DirToMount)
			syscall.Unmount(o.DirToMount, syscall.MNT_DETACH)
		}
	}()

	// Try to perform remount
	sylog.Debugf("Performing remount of %q", o.DirToMount)
	if err = syscall.Mount("", o.DirToMount, "", syscall.MS_REMOUNT|syscall.MS_BIND, ""); err != nil {
		return fmt.Errorf("failed to remount %s: %s", o.DirToMount, err)
	}

	return nil
}

// mountSquashfs mounts a squashfs image to a temporary directory using
// squashfuse.
func (o *OverlayItem) mountSquashfs() error {
	var err error
	squashfuseCmd, err := bin.FindBin("squashfuse")
	if err != nil {
		return fmt.Errorf("use of squashfs overlay requires squashfuse to be installed: %s", err)
	}

	// Even though fusermount is not needed for this step, we shouldn't
	// do the squashfuse mount unless we have the necessary tools to
	// eventually unmount it
	_, err = bin.FindBin("fusermount")
	if err != nil {
		return fmt.Errorf("use of squashfs overlay requires fusermount to be installed: %s", err)
	}

	err = o.mkSecureParentDir()
	if err != nil {
		return fmt.Errorf("error while trying to create parent dir for squashfs overlay: %s", err)
	}

	// For security purposes, let's validate that o.SecureParentDir is non-empty
	if len(o.SecureParentDir) < 1 {
		sylog.Fatalf("internal error: o.SecureParentDir should not be empty by this point")
	}

	sqshfsDir, err := os.MkdirTemp(o.SecureParentDir, "squashfuse-for-overlay-")
	if err != nil {
		return fmt.Errorf("failed to create temporary dir %q for squashfs overlay: %s", sqshfsDir, err)
	}

	// Best effort to cleanup temporary dir
	defer func() {
		if err != nil {
			sylog.Debugf("Encountered error with current OverlaySet; attempting to remove %q", sqshfsDir)
			os.Remove(sqshfsDir)
		}
	}()

	execCmd := exec.Command(squashfuseCmd, o.BarePath, sqshfsDir)
	execCmd.Stderr = os.Stderr
	_, err = execCmd.Output()
	if err != nil {
		return fmt.Errorf("encountered error while trying to mount squashfs image %s as overlay at %s: %s", o.BarePath, sqshfsDir, err)
	}
	o.DirToMount = sqshfsDir

	return nil
}

// mkSecureParentDir checks if the SecureParentDir field is empty and, if so,
// changes it to point to a newly-created temporary directory (using
// os.MkdirTemp()).
func (o *OverlayItem) mkSecureParentDir() error {
	// Check if we've already been given a SecureParentDir value; if not, create
	// one using os.MkdirTemp()
	if len(o.SecureParentDir) > 0 {
		return nil
	}

	d, err := os.MkdirTemp("", "overlay-parent-")
	if err != nil {
		return err
	}

	o.SecureParentDir = d
	return nil
}

// Unmount performs the necessary steps to unmount an individual OverlayItem.
// Note that this method does not unmount the overlay itself. That happens in
// OverlaySet.Unmount().
func (o OverlayItem) Unmount() error {
	switch o.Type {
	case image.SANDBOX:
		return o.unmountDir()
	case image.SQUASHFS:
		return o.unmountSquashfs()
	default:
		return fmt.Errorf("internal error: unrecognized image type in prepareWritableOverlay() (type: %v)", o.Type)
	}
}

// unmountDir unmounts directory-based OverlayItems.
func (o OverlayItem) unmountDir() error {
	return detachMount(o.DirToMount)
}

// unmountSquashfs unmounts squashfs-based OverlayItems.
func (o OverlayItem) unmountSquashfs() error {
	defer os.Remove(o.DirToMount)
	fusermountCmd, innerErr := bin.FindBin("fusermount")
	if innerErr != nil {
		// The code in performIndividualMounts() should not have created
		// a squashfs overlay without fusermount in place
		return fmt.Errorf("internal error: squashfuse mount created without fusermount installed: %s", innerErr)
	}
	execCmd := exec.Command(fusermountCmd, "-u", o.DirToMount)
	execCmd.Stderr = os.Stderr
	_, innerErr = execCmd.Output()
	if innerErr != nil {
		return fmt.Errorf("encountered error while trying to unmount squashfs image %s from %s: %s", o.BarePath, o.DirToMount, innerErr)
	}
	return nil
}

// prepareWritableOverlay ensures that the upper and work subdirs of a writable
// overlay dir exist, and if not, creates them.
func (o *OverlayItem) prepareWritableOverlay() error {
	switch o.Type {
	case image.SANDBOX:
		o.DirToMount = o.BarePath
		if err := ensureOverlayDir(o.DirToMount, true, 0o755); err != nil {
			return err
		}
		sylog.Debugf("Ensuring %q exists", o.Upper())
		if err := ensureOverlayDir(o.Upper(), true, 0o755); err != nil {
			return fmt.Errorf("err encountered while preparing upper subdir of overlay dir %q: %w", o.Upper(), err)
		}
		sylog.Debugf("Ensuring %q exists", o.Work())
		if err := ensureOverlayDir(o.Work(), true, 0o700); err != nil {
			return fmt.Errorf("err encountered while preparing work subdir of overlay dir %q: %w", o.Work(), err)
		}
	default:
		return fmt.Errorf("internal error: unrecognized image type in prepareWritableOverlay() (type: %v)", o.Type)
	}

	return nil
}

// Upper returns the "upper"-subdir of the OverlayItem's DirToMount field.
// Useful for computing options strings for overlay-related mount system calls.
func (o OverlayItem) Upper() string {
	return filepath.Join(o.DirToMount, "upper")
}

// Work returns the "work"-subdir of the OverlayItem's DirToMount field. Useful
// for computing options strings for overlay-related mount system calls.
func (o OverlayItem) Work() string {
	return filepath.Join(o.DirToMount, "work")
}

// CreateOverlay creates a writable overlay using a directory inside the OCI
// bundle.
func CreateOverlay(bundlePath string) error {
	oldumask := syscall.Umask(0)
	defer syscall.Umask(oldumask)

	overlayDir := filepath.Join(bundlePath, "overlay")
	var err error
	if err = ensureOverlayDir(overlayDir, true, 0o700); err != nil {
		return fmt.Errorf("failed to create %s: %s", overlayDir, err)
	}
	// delete overlay directory in case of error
	defer func() {
		if err != nil {
			sylog.Debugf("Encountered error in CreateOverlay; attempting to remove overlayDir %q", overlayDir)
			os.RemoveAll(overlayDir)
		}
	}()

	return OverlaySet{WritableOverlay: &OverlayItem{
		BarePath: overlayDir,
		Type:     image.SANDBOX,
		Writable: true,
	}}.Mount(RootFs(bundlePath).Path())
}

// DeleteOverlay deletes an overlay previously created using a directory inside
// the OCI bundle.
func DeleteOverlay(bundlePath string) error {
	overlayDir := filepath.Join(bundlePath, "overlay")
	rootFsDir := RootFs(bundlePath).Path()

	if err := detachMount(rootFsDir); err != nil {
		return err
	}

	return detachAndDelete(overlayDir)
}

// CreateOverlay creates a writable overlay using tmpfs.
func CreateOverlayTmpfs(bundlePath string, sizeMiB int) (string, error) {
	var err error

	oldumask := syscall.Umask(0)
	defer syscall.Umask(oldumask)

	overlayDir := filepath.Join(bundlePath, "overlay")
	err = ensureOverlayDir(overlayDir, true, 0o700)
	if err != nil {
		return "", fmt.Errorf("failed to create %s: %s", overlayDir, err)
	}
	// delete overlay directory in case of error
	defer func() {
		if err != nil {
			sylog.Debugf("Encountered error in CreateOverlay; attempting to remove overlayDir %q", overlayDir)
			os.RemoveAll(overlayDir)
		}
	}()

	options := fmt.Sprintf("mode=1777,size=%dm", sizeMiB)
	err = syscall.Mount("tmpfs", overlayDir, "tmpfs", syscall.MS_NODEV, options)
	if err != nil {
		return "", fmt.Errorf("failed to bind %s: %s", overlayDir, err)
	}
	// best effort to cleanup mount
	defer func() {
		if err != nil {
			sylog.Debugf("Encountered error in CreateOverlayTmpfs; attempting to detach overlayDir %q", overlayDir)
			syscall.Unmount(overlayDir, syscall.MNT_DETACH)
		}
	}()

	err = OverlaySet{WritableOverlay: &OverlayItem{
		BarePath: overlayDir,
		Type:     image.SANDBOX,
		Writable: true,
	}}.Mount(RootFs(bundlePath).Path())
	if err != nil {
		return "", err
	}

	return overlayDir, nil
}

// DeleteOverlayTmpfs deletes an overlay previously created using tmpfs.
func DeleteOverlayTmpfs(bundlePath, overlayDir string) error {
	rootFsDir := RootFs(bundlePath).Path()

	if err := detachMount(rootFsDir); err != nil {
		return err
	}

	// Because CreateOverlayTmpfs() mounts the tmpfs on overlayDir, and then
	// calls ApplyOverlay(), there needs to be an extra unmount in the this case
	if err := detachMount(overlayDir); err != nil {
		return err
	}

	return detachAndDelete(overlayDir)
}

// ensureOverlayDir checks if a directory already exists; if it doesn't, and
// createIfMissing is true, it attempts to create it with the specified
// permissions.
func ensureOverlayDir(dir string, createIfMissing bool, createPerm os.FileMode) error {
	if len(dir) == 0 {
		return fmt.Errorf("internal error: ensureOverlayDir() called with empty dir name")
	}

	_, err := os.Stat(dir)
	if err == nil {
		return nil
	}

	if !os.IsNotExist(err) {
		return err
	}

	if !createIfMissing {
		return fmt.Errorf("missing overlay dir %q", dir)
	}

	// Create the requested dir
	if err := os.Mkdir(dir, createPerm); err != nil {
		return fmt.Errorf("failed to create %q: %s", dir, err)
	}

	return nil
}

// detachAndDelete performs an unmount system call on the specified directory,
// followed by deletion of the directory and all of its contents.
func detachAndDelete(overlayDir string) error {
	sylog.Debugf("Detaching overlayDir %q", overlayDir)
	if err := syscall.Unmount(overlayDir, syscall.MNT_DETACH); err != nil {
		return fmt.Errorf("failed to unmount %s: %s", overlayDir, err)
	}

	sylog.Debugf("Removing overlayDir %q", overlayDir)
	if err := os.RemoveAll(overlayDir); err != nil {
		return fmt.Errorf("failed to remove %s: %s", overlayDir, err)
	}
	return nil
}

// detachMount performs an unmount system call on the specified directory.
func detachMount(dir string) error {
	sylog.Debugf("Calling syscall.Unmount() to detach %q", dir)
	if err := syscall.Unmount(dir, syscall.MNT_DETACH); err != nil {
		return fmt.Errorf("failed to detach %s: %s", dir, err)
	}

	return nil
}
