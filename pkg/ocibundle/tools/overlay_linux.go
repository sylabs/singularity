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
	"syscall"

	"github.com/sylabs/singularity/internal/pkg/util/bin"
	"github.com/sylabs/singularity/pkg/sylog"
)

// OverlayKind describes whether an overlay is a directory, a squashfs image,
// etc.
type OverlayKind int

// Possible values for OverlayKind
const (
	// OLKINDDIR represents a directory
	OLKINDDIR OverlayKind = iota

	// OLKINDSQUASHFS represents a squashfs image file
	OLKINDSQUASHFS

	// OLKINDEXTFS represents an extfs image file
	OLKINDEXTFS
)

// OverlayItem represents information about a single overlay item (as specified,
// for example, in a single --overlay argument)
type OverlayItem struct {
	// Kind represents what kind of overlay this is
	Kind OverlayKind

	// Writable represents whether this is a writable overlay
	Writable bool

	// BarePath is the path of the overlay stripped of any colon-prefixed
	// options (like ":ro")
	BarePath string

	// DirToMount is the path of the directory that will actually be passed to
	// the mount system-call when mounting this overlay
	DirToMount string
}

// Mount performs the necessary steps to mount an individual OverlayItem. It
// acts as a sort of multiplexer, calling the corresponding private method
// depending on the type of overlay involved. Note that this method does not
// mount the overlay itself (which may consist of a series of OverlayItems);
// that happens in OverlaySet.Mount().
func (overlay *OverlayItem) Mount() error {
	switch overlay.Kind {
	case OLKINDDIR:
		return overlay.mountDir()
	case OLKINDSQUASHFS:
		return overlay.mountSquashfs()
	default:
		return fmt.Errorf("internal error: unrecognized image type in prepareWritableOverlay() (kind: %v)", overlay.Kind)
	}
}

// mountDir is the private method for mounting directory-based OverlayItems.
// This involves bind-mounting followed by remounting of the directory onto
// itself. This pattern of bind-mount followed by remount allows application of
// more restrictive mount flags than are in force on the underlying filesystem.
func (overlay *OverlayItem) mountDir() error {
	var err error
	if len(overlay.DirToMount) < 1 {
		overlay.DirToMount = overlay.BarePath
	}

	if err = ensureOverlayDir(overlay.DirToMount, false, 0); err != nil {
		return fmt.Errorf("error accessing directory %s: %s", overlay.DirToMount, err)
	}

	sylog.Debugf("Performing identity bind-mount of %q", overlay.DirToMount)
	if err = syscall.Mount(overlay.DirToMount, overlay.DirToMount, "", syscall.MS_BIND, ""); err != nil {
		return fmt.Errorf("failed to bind %s: %s", overlay.DirToMount, err)
	}

	// Best effort to cleanup mount
	defer func() {
		if err != nil {
			sylog.Debugf("Encountered error with current OverlaySet; attempting to unmount %q", overlay.DirToMount)
			syscall.Unmount(overlay.DirToMount, syscall.MNT_DETACH)
		}
	}()

	// Try to perform remount
	sylog.Debugf("Performing remount of %q", overlay.DirToMount)
	if err = syscall.Mount("", overlay.DirToMount, "", syscall.MS_REMOUNT|syscall.MS_BIND, ""); err != nil {
		return fmt.Errorf("failed to remount %s: %s", overlay.DirToMount, err)
	}

	return nil
}

// mountSquashfs is the private method for mounting squashfs-based OverlayItems.
func (overlay *OverlayItem) mountSquashfs() error {
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
	sqshfsDir, err := os.MkdirTemp("", "squashfuse-for-oci-overlay-")
	if err != nil {
		return fmt.Errorf("failed to create temporary dir %q for OCI-mode squashfs overlay: %s", sqshfsDir, err)
	}

	// Best effort to cleanup temporary dir
	defer func() {
		if err != nil {
			sylog.Debugf("Encountered error with current OverlaySet; attempting to remove %q", sqshfsDir)
			os.Remove(sqshfsDir)
		}
	}()

	execCmd := exec.Command(squashfuseCmd, overlay.BarePath, sqshfsDir)
	execCmd.Stderr = os.Stderr
	_, err = execCmd.Output()
	if err != nil {
		return fmt.Errorf("encountered error while trying to mount squashfs image %s for OCI-mode overlay at %s: %s", overlay.BarePath, sqshfsDir, err)
	}
	overlay.DirToMount = sqshfsDir

	return nil
}

// Unmount performs the necessary steps to unmount an individual OverlayItem. It
// acts as a sort of multiplexer, calling the corresponding private method
// depending on the type of overlay involved. Note that this method does not
// unmount the overlay itself (which may consist of a series of OverlayItems);
// that happens in OverlaySet.Unmount().
func (overlay OverlayItem) Unmount() error {
	switch overlay.Kind {
	case OLKINDDIR:
		return overlay.unmountDir()
	case OLKINDSQUASHFS:
		return overlay.unmountSquashfs()
	default:
		return fmt.Errorf("internal error: unrecognized image type in prepareWritableOverlay() (kind: %v)", overlay.Kind)
	}
}

// unmountDir is the private method for unmounting directory-based OverlayItems.
// It does nothing more than wrap the detachMount() function, called on the
// OverlayItem's DirToMount field, in a method. It is therefore a somewhat
// degenerate case, but this wrapping is still maintained for uniformity across
// the different overlay kinds.
func (overlay OverlayItem) unmountDir() error {
	return detachMount(overlay.DirToMount)
}

// unmountSquashfs is the private method for unmounting squashfs-based
// OverlayItems.
func (overlay OverlayItem) unmountSquashfs() error {
	defer os.Remove(overlay.DirToMount)
	fusermountCmd, innerErr := bin.FindBin("fusermount")
	if innerErr != nil {
		// The code in performIndividualMounts() should not have created
		// a squashfs overlay without fusermount in place
		return fmt.Errorf("internal error: squashfuse mount created without fusermount installed: %s", innerErr)
	}
	execCmd := exec.Command(fusermountCmd, "-u", overlay.DirToMount)
	execCmd.Stderr = os.Stderr
	_, innerErr = execCmd.Output()
	if innerErr != nil {
		return fmt.Errorf("encountered error while trying to unmount squashfs image %s from %s: %s", overlay.BarePath, overlay.DirToMount, innerErr)
	}
	return nil
}

// prepareWritableOverlay ensures that the upper and work subdirs of a writable
// overlay dir exist, and if not, creates them.
func (overlay *OverlayItem) prepareWritableOverlay() error {
	switch overlay.Kind {
	case OLKINDDIR:
		overlay.DirToMount = overlay.BarePath
		if err := ensureOverlayDir(overlay.DirToMount, true, 0o755); err != nil {
			return err
		}
		sylog.Debugf("Ensuring %q exists", overlay.Upper())
		if err := ensureOverlayDir(overlay.Upper(), true, 0o755); err != nil {
			return fmt.Errorf("err encountered while preparing upper subdir of overlay dir %q: %w", overlay.Upper(), err)
		}
		sylog.Debugf("Ensuring %q exists", overlay.Work())
		if err := ensureOverlayDir(overlay.Work(), true, 0o700); err != nil {
			return fmt.Errorf("err encountered while preparing work subdir of overlay dir %q: %w", overlay.Work(), err)
		}
	default:
		return fmt.Errorf("internal error: unrecognized image type in prepareWritableOverlay() (kind: %v)", overlay.Kind)
	}

	return nil
}

// Upper returns the "upper"-subdir of the OverlayItem's DirToMount field.
// Useful for computing options strings for overlay-related mount system calls.
func (overlay OverlayItem) Upper() string {
	return filepath.Join(overlay.DirToMount, "upper")
}

// Work returns the "work"-subdir of the OverlayItem's DirToMount field. Useful
// for computing options strings for overlay-related mount system calls.
func (overlay OverlayItem) Work() string {
	return filepath.Join(overlay.DirToMount, "work")
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
		Kind:     OLKINDDIR,
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
		Kind:     OLKINDDIR,
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
