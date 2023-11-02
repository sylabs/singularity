// Copyright (c) 2018-2023, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package oci

import (
	"context"
	"fmt"

	"github.com/sylabs/singularity/v4/internal/pkg/util/fs/overlay"
	"github.com/sylabs/singularity/v4/pkg/image"
	"github.com/sylabs/singularity/v4/pkg/ocibundle/tools"
	"github.com/sylabs/singularity/v4/pkg/sylog"
	"github.com/sylabs/singularity/v4/pkg/util/singularityconf"
)

// WrapWithWritableTmpFs runs a function wrapped with prep / cleanup steps for a
// tmpfs. This tmpfs is always writable so that the launcher and runtime are
// able to add content to the container. Whether it is writable from inside the
// container is controlled by the runtime config.
func WrapWithWritableTmpFs(ctx context.Context, f func() error, bundleDir string, allowSetuid bool) error {
	overlayDir, err := prepareWritableTmpfs(ctx, bundleDir, allowSetuid)
	sylog.Debugf("Done with prepareWritableTmpfs; overlayDir is: %q", overlayDir)
	if err != nil {
		return err
	}

	err = f()

	// Cleanup actions log errors, but don't return - so we get as much cleanup done as possible.
	if cleanupErr := cleanupWritableTmpfs(ctx, bundleDir, overlayDir); cleanupErr != nil {
		sylog.Errorf("While cleaning up writable tmpfs: %v", cleanupErr)
	}

	// Return any error from the actual container payload - preserve exit code.
	return err
}

func prepareWritableTmpfs(ctx context.Context, bundleDir string, allowSetuid bool) (string, error) {
	sylog.Debugf("Configuring writable tmpfs overlay for %s", bundleDir)
	c := singularityconf.GetCurrentConfig()
	if c == nil {
		return "", fmt.Errorf("singularity configuration is not initialized")
	}
	return tools.CreateOverlayTmpfs(ctx, bundleDir, int(c.SessiondirMaxSize), allowSetuid)
}

func cleanupWritableTmpfs(ctx context.Context, bundleDir, overlayDir string) error {
	sylog.Debugf("Cleaning up writable tmpfs overlay for %s", bundleDir)
	return tools.DeleteOverlayTmpfs(ctx, bundleDir, overlayDir)
}

// WrapWithOverlays runs a function wrapped with prep / cleanup steps for the
// overlays specified in overlayPaths. If there is no user-provided writable
// overlay, it adds an ephemeral overlay which is always writable so that the
// launcher and runtime are able to add content to the container. Whether it is
// writable from inside the container is controlled by the runtime config.
func WrapWithOverlays(ctx context.Context, f func() error, bundleDir string, overlayPaths []string, allowSetuid bool) error {
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

		if s.WritableOverlay != nil && !item.Readonly {
			return fmt.Errorf("you can't specify more than one writable overlay; %#v has already been specified as a writable overlay; use '--overlay %s:ro' instead", s.WritableOverlay, item.SourcePath)
		}
		if !item.Readonly {
			s.WritableOverlay = item
		} else {
			s.ReadonlyOverlays = append(s.ReadonlyOverlays, item)
		}
	}

	systemOverlay := ""
	if s.WritableOverlay == nil {
		i, err := prepareSystemOverlay(bundleDir, allowSetuid)
		if err != nil {
			return err
		}
		systemOverlay = i.SourcePath
		s.WritableOverlay = i
	}

	rootFsDir := tools.RootFs(bundleDir).Path()
	err := s.Mount(ctx, rootFsDir)
	if err != nil {
		return err
	}

	err = f()

	// Cleanup actions log errors, but don't return - so we get as much cleanup done as possible.
	if cleanupErr := s.Unmount(ctx, rootFsDir); cleanupErr != nil {
		sylog.Errorf("While unmounting rootfs overlay: %v", cleanupErr)
	}
	if systemOverlay != "" {
		if cleanupErr := cleanupSystemOverlay(systemOverlay); cleanupErr != nil {
			sylog.Errorf("While cleaning up ephemeral writable tmpfs: %v", cleanupErr)
		}
	}

	// Return any error from the actual container payload - preserve exit code.
	return err
}

func prepareSystemOverlay(bundleDir string, allowSetuid bool) (*overlay.Item, error) {
	sylog.Debugf("Configuring ephemeral writable tmpfs overlay for %s", bundleDir)
	c := singularityconf.GetCurrentConfig()
	if c == nil {
		return nil, fmt.Errorf("singularity configuration is not initialized")
	}

	systemOverlay, err := tools.PrepareOverlayTmpfs(bundleDir, int(c.SessiondirMaxSize), allowSetuid)
	if err != nil {
		return nil, err
	}

	i := overlay.Item{
		SourcePath: systemOverlay,
		Type:       image.SANDBOX,
		Readonly:   false,
	}
	i.SetAllowSetuid(allowSetuid)

	return &i, nil
}

func cleanupSystemOverlay(dir string) error {
	sylog.Debugf("Cleaning up ephemeral writable tmpfs overlay for %s", dir)
	return overlay.DetachAndDelete(dir)
}
