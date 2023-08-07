// Copyright (c) 2018-2023, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package oci

import (
	"fmt"

	"github.com/sylabs/singularity/v4/internal/pkg/util/fs/overlay"
	"github.com/sylabs/singularity/v4/pkg/ocibundle/tools"
	"github.com/sylabs/singularity/v4/pkg/sylog"
	"github.com/sylabs/singularity/v4/pkg/util/singularityconf"
)

// WrapWithWritableTmpFs runs a function wrapped with prep / cleanup steps for a
// tmpfs. This tmpfs is always writable so that the launcher and runtime are
// able to add content to the container. Whether it is writable from inside the
// container is controlled by the runtime config.
func WrapWithWritableTmpFs(f func() error, bundleDir string, allowSetuid bool) error {
	overlayDir, err := prepareWritableTmpfs(bundleDir, allowSetuid)
	sylog.Debugf("Done with prepareWritableTmpfs; overlayDir is: %q", overlayDir)
	if err != nil {
		return err
	}

	err = f()

	// Cleanup actions log errors, but don't return - so we get as much cleanup done as possible.
	if cleanupErr := cleanupWritableTmpfs(bundleDir, overlayDir); cleanupErr != nil {
		sylog.Errorf("While cleaning up writable tmpfs: %v", cleanupErr)
	}

	// Return any error from the actual container payload - preserve exit code.
	return err
}

// WrapWithOverlays runs a function wrapped with prep / cleanup steps for overlays.
func WrapWithOverlays(f func() error, bundleDir string, overlayPaths []string, allowSetuid bool) error {
	writableOverlayFound := false
	s := overlay.Set{}
	for _, p := range overlayPaths {
		item, err := overlay.NewItemFromString(p)
		if err != nil {
			return err
		}

		item.SetParentDir(bundleDir)

		if allowSetuid {
			item.SetAllowSetuid(true)
		}

		if writableOverlayFound && !item.Readonly {
			return fmt.Errorf("you can't specify more than one writable overlay; %#v has already been specified as a writable overlay; use '--overlay %s:ro' instead", s.WritableOverlay, item.SourcePath)
		}
		if !item.Readonly {
			writableOverlayFound = true
			s.WritableOverlay = item
		} else {
			s.ReadonlyOverlays = append(s.ReadonlyOverlays, item)
		}
	}

	rootFsDir := tools.RootFs(bundleDir).Path()
	err := s.Mount(rootFsDir)
	if err != nil {
		return err
	}

	if writableOverlayFound {
		err = f()
	} else {
		err = WrapWithWritableTmpFs(f, bundleDir, allowSetuid)
	}

	// Cleanup actions log errors, but don't return - so we get as much cleanup done as possible.
	if cleanupErr := s.Unmount(rootFsDir); cleanupErr != nil {
		sylog.Errorf("While unmounting rootfs overlay: %v", cleanupErr)
	}

	// Return any error from the actual container payload - preserve exit code.
	return err
}

func prepareWritableTmpfs(bundleDir string, allowSetuid bool) (string, error) {
	sylog.Debugf("Configuring writable tmpfs overlay for %s", bundleDir)
	c := singularityconf.GetCurrentConfig()
	if c == nil {
		return "", fmt.Errorf("singularity configuration is not initialized")
	}
	return tools.CreateOverlayTmpfs(bundleDir, int(c.SessiondirMaxSize), allowSetuid)
}

func cleanupWritableTmpfs(bundleDir, overlayDir string) error {
	sylog.Debugf("Cleaning up writable tmpfs overlay for %s", bundleDir)
	return tools.DeleteOverlayTmpfs(bundleDir, overlayDir)
}
