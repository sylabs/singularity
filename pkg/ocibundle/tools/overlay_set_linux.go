// Copyright (c) 2019-2023, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package tools

import (
	"fmt"
	"strings"
	"syscall"

	"github.com/samber/lo"
	"github.com/sylabs/singularity/pkg/image"
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

// Mount prepares and mounts the entire OverlaySet onto the specified rootfs
// directory.
func (s OverlaySet) Mount(rootFsDir string) error {
	// Prepare internal structure of writable overlay dir, if necessary
	if s.WritableOverlay != nil {
		if err := s.WritableOverlay.prepareWritableOverlay(); err != nil {
			return err
		}
	}

	// Perform identity mounts for this OverlaySet
	if err := s.performIndividualMounts(); err != nil {
		return err
	}

	// Perform actual overlay mount
	return s.performFinalMount(rootFsDir)
}

// UnmountOverlay ummounts an OverlaySet from a specified rootfs directory.
func (s OverlaySet) Unmount(rootFsDir string) error {
	if err := detachMount(rootFsDir); err != nil {
		return err
	}

	return s.detachIndividualMounts()
}

// performIndividualMounts creates the mounts that furnish the individual
// elements of the OverlaySet.
func (s OverlaySet) performIndividualMounts() error {
	overlaysToBind := s.ReadonlyOverlays
	if s.WritableOverlay != nil {
		overlaysToBind = append(overlaysToBind, s.WritableOverlay)
	}

	// Try to do initial bind-mounts
	for _, overlay := range overlaysToBind {
		if err := overlay.Mount(); err != nil {
			return err
		}
	}

	return nil
}

// performFinalMount performs the final step in mounting an OverlaySet, namely
// mounting of the overlay with its full-fledged options string, representing
// all the individual OverlayItems (writable and read-only) that comprise the
// OverlaySet.
func (s OverlaySet) performFinalMount(rootFsDir string) error {
	// Try to perform actual mount
	options := s.options(rootFsDir)
	sylog.Debugf("Mounting overlay with rootFsDir %q, options: %q", rootFsDir, options)
	if err := syscall.Mount("overlay", rootFsDir, "overlay", 0, options); err != nil {
		return fmt.Errorf("failed to mount %s: %s", rootFsDir, err)
	}

	return nil
}

// options creates an options string to be used in an overlay mount,
// representing all the individual OverlayItems (writable and read-only) that
// comprise the OverlaySet.
func (s OverlaySet) options(rootFsDir string) string {
	// Create lowerdir argument of options string
	lowerDirs := lo.Map(s.ReadonlyOverlays, func(overlay *OverlayItem, _ int) string {
		return overlay.DirToMount
	})
	lowerDirJoined := strings.Join(append(lowerDirs, rootFsDir), ":")

	if (s.WritableOverlay != nil) && (s.WritableOverlay.Type == image.SANDBOX) {
		return fmt.Sprintf("lowerdir=%s,upperdir=%s,workdir=%s",
			lowerDirJoined, s.WritableOverlay.Upper(), s.WritableOverlay.Work())
	}

	return fmt.Sprintf("lowerdir=%s", lowerDirJoined)
}

// detachIndividualMounts detaches the bind mounts & remounts created by
// performIndividualMounts, above.
func (s OverlaySet) detachIndividualMounts() error {
	overlaysToDetach := s.ReadonlyOverlays
	if s.WritableOverlay != nil {
		overlaysToDetach = append(overlaysToDetach, s.WritableOverlay)
	}

	// Don't stop on the first error; try to clean up as much as possible, and
	// then return the first error encountered.
	errors := []error{}
	for _, overlay := range overlaysToDetach {
		err := overlay.Unmount()
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
