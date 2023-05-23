// Copyright (c) 2019-2023, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package tools

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/samber/lo"
	"github.com/sylabs/singularity/pkg/sylog"
)

// OverlaySet represents a set of overlay directories which will be overlain on
// top of some filesystem mount point. The actual mount point atop which these
// directories will be overlain is not specified in the OverlaySet; it is left
// implicit, to be chosen by whichever function consumes an OverlaySet. An
// OverlaySet contains two types of directories: zero or more directories which
// will be mounted as read-only overlays atop the (implicit) mount point, and
// one directory which will be mounted as a writable overlay atop all the rest.
// An empty WritableLoc field indicates that no writable overlay is to be
// mounted.
type OverlaySet struct {
	// ReadonlyOverlays is a list of directories to be mounted as read-only
	// overlays. The mount point atop which these will be mounted is left
	// implicit, to be chosen by whichever function consumes the OverlaySet.
	ReadonlyOverlays []*OverlayInfo

	// WritableOverlay is the directory to be mounted as a writable overlay. The
	// mount point atop which this will be mounted is left implicit, to be
	// chosen by whichever function consumes the OverlaySet. Empty value
	// indicates no writable overlay is to be mounted.
	WritableOverlay *OverlayInfo
}

// OverlayInfo represents information about an overlay (as specified, for
// example, in a --overlay argument)
type OverlayInfo struct {
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

// OverlayKind describes whether an overlay is a directory, a squashfs image,
// etc.
type OverlayKind int

// Possible values for OverlayKind
const (
	// OLKindDir represents a directory
	OLKindDir OverlayKind = iota

	// OLKindSquashFS represents a squashfs image file
	OLKindSquashFS

	// OLKindExtFS represents an extfs image file
	OLKindExtFS
)

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

	return ApplyOverlay(
		RootFs(bundlePath).Path(), OverlaySet{WritableOverlay: &OverlayInfo{
			BarePath: overlayDir,
			Kind:     OLKindDir,
			Writable: true,
		}},
	)
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

	err = ApplyOverlay(
		RootFs(bundlePath).Path(), OverlaySet{WritableOverlay: &OverlayInfo{
			BarePath: overlayDir,
			Kind:     OLKindDir,
			Writable: true,
		}},
	)
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

// ApplyOverlay prepares and mounts the specified overlay
func ApplyOverlay(rootFsDir string, ovs OverlaySet) error {
	// Prepare internal structure of writable overlay dir, if necessary
	if ovs.WritableOverlay != nil {
		if err := prepareWritableOverlay(ovs.WritableOverlay); err != nil {
			return err
		}
	}

	// Perform identity mounts for this OverlaySet
	if err := performIdentityMounts(ovs); err != nil {
		return err
	}

	// Perform actual overlay mount
	return performOverlayMount(rootFsDir, overlayOptions(rootFsDir, ovs))
}

// UnmountOverlay umounts an overlay
func UnmountOverlay(rootFsDir string, ovs OverlaySet) error {
	if err := detachMount(rootFsDir); err != nil {
		return err
	}

	return detachIdentityMounts(ovs)
}

// prepareWritableOverlay ensures that the upper and work subdirs of a writable
// overlay dir exist, and if not, creates them.
func prepareWritableOverlay(overlay *OverlayInfo) error {
	switch overlay.Kind {
	case OLKindDir:
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
	}

	return nil
}

// performIdentityMounts creates the writable OverlaySet directory if it does
// not exist, and performs a bind mount & remount of every OverlaySet dir onto
// itself. The pattern of bind mount followed by remount allows application of
// more restrictive mount flags than are in force on the underlying filesystem.
func performIdentityMounts(ovs OverlaySet) error {
	var err error

	overlaysToBind := ovs.ReadonlyOverlays
	if ovs.WritableOverlay != nil {
		overlaysToBind = append(overlaysToBind, ovs.WritableOverlay)
	}

	// Try to do initial bind-mounts
	for _, overlay := range overlaysToBind {
		if (overlay.Kind == OLKindDir) && (len(overlay.DirToMount) < 1) {
			overlay.DirToMount = overlay.BarePath
		}

		if err = ensureOverlayDir(overlay.DirToMount, false, 0); err != nil {
			return fmt.Errorf("error accessing directory %s: %s", overlay.DirToMount, err)
		}

		sylog.Debugf("Performing identity bind-mount of %q", overlay.DirToMount)
		if err = syscall.Mount(overlay.DirToMount, overlay.DirToMount, "", syscall.MS_BIND, ""); err != nil {
			return fmt.Errorf("failed to bind %s: %s", overlay.DirToMount, err)
		}

		// best effort to cleanup mount
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
	}

	return err
}

// detachIdentityMounts detaches mounts created by the bind-mount & remount
// pattern (as implemented in performIdentityMounts())
func detachIdentityMounts(ovs OverlaySet) error {
	overlaysToDetach := ovs.ReadonlyOverlays
	if ovs.WritableOverlay != nil {
		overlaysToDetach = append(overlaysToDetach, ovs.WritableOverlay)
	}

	// Don't stop on the first error; try to clean up as much as possible, and
	// then return the first error encountered.
	errors := []error{}
	for _, overlay := range overlaysToDetach {
		err := detachMount(overlay.DirToMount)
		if err != nil {
			sylog.Errorf("Error encountered trying to detach identity mount %s: %s", overlay.DirToMount, err)
			errors = append(errors, err)
		}
	}

	if len(errors) > 0 {
		return errors[0]
	}

	return nil
}

// overlayOptions creates the options string to be used in an overlay mount
func overlayOptions(rootFsDir string, ovs OverlaySet) string {
	// Create lowerdir argument of options string
	lowerDirs := lo.Map(ovs.ReadonlyOverlays, func(overlay *OverlayInfo, _ int) string {
		return overlay.DirToMount
	})
	lowerDirJoined := strings.Join(append(lowerDirs, rootFsDir), ":")

	if (ovs.WritableOverlay != nil) && (ovs.WritableOverlay.Kind == OLKindDir) {
		return fmt.Sprintf("lowerdir=%s,upperdir=%s,workdir=%s", lowerDirJoined, upperSubdirOf(ovs.WritableOverlay.DirToMount), workSubdirOf(ovs.WritableOverlay.DirToMount))
	}

	return fmt.Sprintf("lowerdir=%s", lowerDirJoined)
}

// performOverlayMount mounts an overlay atop a given rootfs directory
func performOverlayMount(rootFsDir, options string) error {
	// Try to perform actual mount
	sylog.Debugf("Mounting overlay with rootFsDir %q, options: %q", rootFsDir, options)
	if err := syscall.Mount("overlay", rootFsDir, "overlay", 0, options); err != nil {
		return fmt.Errorf("failed to mount %s: %s", rootFsDir, err)
	}

	return nil
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
