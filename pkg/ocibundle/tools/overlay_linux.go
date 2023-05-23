// Copyright (c) 2019-2023, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package tools

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"

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

// OverlayItem represents information about an overlay (as specified, for
// example, in a --overlay argument)
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

// prepareWritableOverlay ensures that the upper and work subdirs of a writable
// overlay dir exist, and if not, creates them.
func (overlay *OverlayItem) prepareWritableOverlay() error {
	switch overlay.Kind {
	case OLKINDDIR:
		overlay.DirToMount = overlay.BarePath
		if err := ensureOverlayDir(overlay.DirToMount, true, 0o755); err != nil {
			return err
		}
		sylog.Debugf("Ensuring %q exists", upperSubdirOf(overlay.DirToMount))
		if err := ensureOverlayDir(upperSubdirOf(overlay.DirToMount), true, 0o755); err != nil {
			return fmt.Errorf("err encountered while preparing upper subdir of overlay dir %q: %w", upperSubdirOf(overlay.DirToMount), err)
		}
		sylog.Debugf("Ensuring %q exists", workSubdirOf(overlay.DirToMount))
		if err := ensureOverlayDir(workSubdirOf(overlay.DirToMount), true, 0o700); err != nil {
			return fmt.Errorf("err encountered while preparing work subdir of overlay dir %q: %w", workSubdirOf(overlay.DirToMount), err)
		}
	default:
		return fmt.Errorf("internal error: unrecognized image type in prepareWritableOverlay() (kind: %v)", overlay.Kind)
	}

	return nil
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
	}}.Apply(RootFs(bundlePath).Path())
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
	}}.Apply(RootFs(bundlePath).Path())
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

func upperSubdirOf(overlayDir string) string {
	return filepath.Join(overlayDir, "upper")
}

func workSubdirOf(overlayDir string) string {
	return filepath.Join(overlayDir, "work")
}

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

func detachMount(dir string) error {
	sylog.Debugf("Calling syscall.Unmount() to detach %q", dir)
	if err := syscall.Unmount(dir, syscall.MNT_DETACH); err != nil {
		return fmt.Errorf("failed to detach %s: %s", dir, err)
	}

	return nil
}
