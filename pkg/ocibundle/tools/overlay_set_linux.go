// Copyright (c) 2019-2023, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package tools

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"

	"github.com/samber/lo"
	"github.com/sylabs/singularity/internal/pkg/util/bin"
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
	ReadonlyOverlays []*OverlayItem

	// WritableOverlay is the directory to be mounted as a writable overlay. The
	// mount point atop which this will be mounted is left implicit, to be
	// chosen by whichever function consumes the OverlaySet. Empty value
	// indicates no writable overlay is to be mounted.
	WritableOverlay *OverlayItem
}

// Apply prepares and mounts the OverlaySet
func (ovs OverlaySet) Apply(rootFsDir string) error {
	// Prepare internal structure of writable overlay dir, if necessary
	if ovs.WritableOverlay != nil {
		if err := ovs.WritableOverlay.prepareWritableOverlay(); err != nil {
			return err
		}
	}

	// Perform identity mounts for this OverlaySet
	if err := ovs.performIndividualMounts(); err != nil {
		return err
	}

	// Perform actual overlay mount
	return ovs.mount(rootFsDir)
}

// UnmountOverlay ummounts an OverlaySet from a specified rootfs directory
func (ovs OverlaySet) Unmount(rootFsDir string) error {
	if err := detachMount(rootFsDir); err != nil {
		return err
	}

	return ovs.detachIndividualMounts()
}

// mount mounts an overlay atop a given rootfs directory
func (ovs OverlaySet) mount(rootFsDir string) error {
	// Try to perform actual mount
	options := ovs.options(rootFsDir)
	sylog.Debugf("Mounting overlay with rootFsDir %q, options: %q", rootFsDir, options)
	if err := syscall.Mount("overlay", rootFsDir, "overlay", 0, options); err != nil {
		return fmt.Errorf("failed to mount %s: %s", rootFsDir, err)
	}

	return nil
}

// options creates an options string to be used in an overlay mount based on the
// current OverlaySet and a rootfs path.
func (ovs OverlaySet) options(rootFsDir string) string {
	// Create lowerdir argument of options string
	lowerDirs := lo.Map(ovs.ReadonlyOverlays, func(overlay *OverlayItem, _ int) string {
		return overlay.DirToMount
	})
	lowerDirJoined := strings.Join(append(lowerDirs, rootFsDir), ":")

	if (ovs.WritableOverlay != nil) && (ovs.WritableOverlay.Kind == OLKINDDIR) {
		return fmt.Sprintf("lowerdir=%s,upperdir=%s,workdir=%s", lowerDirJoined, upperSubdirOf(ovs.WritableOverlay.DirToMount), workSubdirOf(ovs.WritableOverlay.DirToMount))
	}

	return fmt.Sprintf("lowerdir=%s", lowerDirJoined)
}

// performIndividualMounts creates the individual mounts that furnish the
// "lowerdir" elements of the eventual overlay mount. In the case of a directory
// overlay, this involves a bind-mount & remount of every OverlaySet the
// directory onto itself. This pattern of bind mount followed by remount allows
// application of more restrictive mount flags than are in force on the
// underlying filesystem.
func (ovs OverlaySet) performIndividualMounts() error {
	var err error

	overlaysToBind := ovs.ReadonlyOverlays
	if ovs.WritableOverlay != nil {
		overlaysToBind = append(overlaysToBind, ovs.WritableOverlay)
	}

	// Try to do initial bind-mounts
	for _, overlay := range overlaysToBind {
		switch overlay.Kind {
		case OLKINDDIR:
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
		case OLKINDSQUASHFS:
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
		default:
			return fmt.Errorf("internal error: unrecognized image type in prepareWritableOverlay() (kind: %v)", overlay.Kind)
		}
	}

	return err
}

// detachIndividualMounts detaches the bind mounts & remounts created by
// performIndividualMounts()
func (ovs OverlaySet) detachIndividualMounts() error {
	overlaysToDetach := ovs.ReadonlyOverlays
	if ovs.WritableOverlay != nil {
		overlaysToDetach = append(overlaysToDetach, ovs.WritableOverlay)
	}

	// Don't stop on the first error; try to clean up as much as possible, and
	// then return the first error encountered.
	errors := []error{}
	for _, overlay := range overlaysToDetach {
		var err error
		switch overlay.Kind {
		case OLKINDDIR:
			err = detachMount(overlay.DirToMount)
		case OLKINDSQUASHFS:
			defer os.Remove(overlay.DirToMount)
			fusermountCmd, innerErr := bin.FindBin("fusermount")
			if innerErr != nil {
				// The code in performIndividualMounts() should not have created
				// a squashfs overlay without fusermount in place
				err = fmt.Errorf("internal error: squashfuse mount created without fusermount installed: %s", innerErr)
				break // This breaks out of the switch-block, not the for-loop
			}
			execCmd := exec.Command(fusermountCmd, "-u", overlay.DirToMount)
			execCmd.Stderr = os.Stderr
			_, innerErr = execCmd.Output()
			if innerErr != nil {
				err = fmt.Errorf("encountered error while trying to unmount squashfs image %s from %s: %s", overlay.BarePath, overlay.DirToMount, innerErr)
			}
		default:
			return fmt.Errorf("internal error: unrecognized image type in prepareWritableOverlay() (kind: %v)", overlay.Kind)
		}
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
