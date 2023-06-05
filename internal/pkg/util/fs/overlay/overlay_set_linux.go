// Copyright (c) 2019-2023, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package overlay

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

// Set represents a set of overlay directories which will be overlain on top of
// some filesystem mount point. The actual mount point atop which these
// directories will be overlain is not specified in the Set; it is left
// implicit, to be chosen by whichever function consumes a Set. A Set contains
// two types of directories: zero or more directories which will be mounted as
// read-only overlays atop the (implicit) mount point, and one directory which
// will be mounted as a writable overlay atop all the rest. An empty WritableLoc
// field indicates that no writable overlay is to be mounted.
type Set struct {
	// ReadonlyOverlays is a list of directories to be mounted as read-only
	// overlays. The mount point atop which these will be mounted is left
	// implicit, to be chosen by whichever function consumes the Set.
	ReadonlyOverlays []*Item

	// WritableOverlay is the directory to be mounted as a writable overlay. The
	// mount point atop which this will be mounted is left implicit, to be
	// chosen by whichever function consumes the Set. Empty value indicates no
	// writable overlay is to be mounted.
	WritableOverlay *Item
}

// Mount prepares and mounts the entire Set onto the specified rootfs
// directory.
func (s Set) Mount(rootFsDir string) error {
	// Perform identity mounts for this Set
	dups := lo.FindDuplicatesBy(s.ReadonlyOverlays, func(item *Item) string {
		return item.SourcePath
	})
	if len(dups) > 0 {
		return fmt.Errorf("duplicate overlays detected: %v", lo.Map(dups, func(item *Item, _ int) string {
			return item.SourcePath
		}))
	}

	if err := s.performIndividualMounts(); err != nil {
		return err
	}

	// Perform actual overlay mount
	return s.performFinalMount(rootFsDir)
}

// UnmountOverlay ummounts a Set from a specified rootfs directory.
func (s Set) Unmount(rootFsDir string) error {
	unprivOls, err := UnprivOverlaysSupported()
	if err != nil {
		return fmt.Errorf("while checking for unprivileged overlay support in kernel: %w", err)
	}

	if unprivOls {
		err = DetachMount(rootFsDir)
	} else {
		err = UnmountWithFuse(rootFsDir)
	}

	if err != nil {
		return err
	}

	return s.detachIndividualMounts()
}

// performIndividualMounts creates the mounts that furnish the individual
// elements of the Set.
func (s Set) performIndividualMounts() error {
	overlaysToBind := s.ReadonlyOverlays
	if s.WritableOverlay != nil {
		overlaysToBind = append(overlaysToBind, s.WritableOverlay)
	}

	// Try to do initial bind-mounts
	for _, o := range overlaysToBind {
		if err := o.Mount(); err != nil {
			return err
		}
	}

	return nil
}

// performFinalMount performs the final step in mounting a Set, namely mounting
// of the overlay with its full-fledged options string, representing all the
// individual Items (writable and read-only) that comprise the Set.
func (s Set) performFinalMount(rootFsDir string) error {
	// Try to perform actual mount
	options := s.options(rootFsDir)
	unprivOls, err := UnprivOverlaysSupported()
	if err != nil {
		return fmt.Errorf("while checking for unprivileged overlay support in kernel: %w", err)
	}

	if unprivOls {
		sylog.Debugf("Mounting overlay (via syscall) with rootFsDir %q, options: %q", rootFsDir, options)
		if err := syscall.Mount("overlay", rootFsDir, "overlay", syscall.MS_NODEV, options); err != nil {
			return fmt.Errorf("failed to mount %s: %w", rootFsDir, err)
		}
	} else {
		fuseOlFsCmd, err := bin.FindBin("fuse-overlayfs")
		if err != nil {
			return fmt.Errorf("kernel does not support unprivileged overlays, and fuse-overlayfs not available: %w", err)
		}

		// Even though fusermount is not needed for this step, we shouldn't perform
		// the mount unless we have the necessary tools to eventually unmount it
		_, err = bin.FindBin("fusermount")
		if err != nil {
			return fmt.Errorf("kernel does not support unprivileged overlays, and using fuse-overlayfs fallback requires fusermount to be installed: %w", err)
		}

		sylog.Debugf("Mounting overlay (via fuse-overlayfs) with rootFsDir %q, options: %q", rootFsDir, options)
		execCmd := exec.Command(fuseOlFsCmd, "-o", options, rootFsDir)
		execCmd.Stderr = os.Stderr
		_, err = execCmd.Output()
		if err != nil {
			return fmt.Errorf("failed to mount %s: %w", rootFsDir, err)
		}
	}

	return nil
}

// options creates an options string to be used in an overlay mount,
// representing all the individual Items (writable and read-only) that comprise
// the Set.
func (s Set) options(rootFsDir string) string {
	// Create lowerdir argument of options string
	lowerDirs := lo.Map(s.ReadonlyOverlays, func(o *Item, _ int) string {
		return o.StagingDir
	})
	lowerDirJoined := strings.Join(append(lowerDirs, rootFsDir), ":")

	if s.WritableOverlay == nil {
		return fmt.Sprintf("lowerdir=%s", lowerDirJoined)
	}

	return fmt.Sprintf("lowerdir=%s,upperdir=%s,workdir=%s",
		lowerDirJoined, s.WritableOverlay.Upper(), s.WritableOverlay.Work())
}

// detachIndividualMounts detaches the bind mounts & remounts created by
// performIndividualMounts, above.
func (s Set) detachIndividualMounts() error {
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
			sylog.Errorf("Error encountered trying to detach identity mount %s: %s", overlay.StagingDir, err)
			errors = append(errors, err)
		}
	}

	if len(errors) > 0 {
		return errors[0]
	}

	return nil
}
