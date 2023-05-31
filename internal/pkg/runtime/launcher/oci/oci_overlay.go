// Copyright (c) 2018-2023, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package oci

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"

	"github.com/sylabs/singularity/internal/pkg/util/fs/overlay"
	"github.com/sylabs/singularity/pkg/image"
	"github.com/sylabs/singularity/pkg/ocibundle/tools"
	"github.com/sylabs/singularity/pkg/sylog"
	"github.com/sylabs/singularity/pkg/util/singularityconf"
)

// WrapWithWritableTmpFs runs a function wrapped with prep / cleanup steps for a writable tmpfs.
func WrapWithWritableTmpFs(f func() error, bundleDir string) error {
	// TODO: --oci mode always emulating --compat, which uses --writable-tmpfs.
	//       Provide a way of disabling this, for a read only rootfs.
	overlayDir, err := prepareWritableTmpfs(bundleDir)
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
func WrapWithOverlays(f func() error, bundleDir string, overlayPaths []string) error {
	writableOverlayFound := false
	s := overlay.Set{
		ReadonlyOverlays: []*overlay.Item{
			{
				SourcePath: filepath.Join(bundleDir, "session"),
				Type:       image.SANDBOX,
				Writable:   false,
			},
		},
	}

	for _, p := range overlayPaths {
		item, err := overlay.NewItemFromString(p)
		if err != nil {
			return err
		}

		item.SetParentDir(bundleDir)

		if item.Writable && writableOverlayFound {
			return fmt.Errorf("you can't specify more than one writable overlay; %#v has already been specified as a writable overlay; use '--overlay %s:ro' instead", s.WritableOverlay, item.SourcePath)
		}
		if item.Writable {
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
		err = WrapWithWritableTmpFs(f, bundleDir)
	}

	// Cleanup actions log errors, but don't return - so we get as much cleanup done as possible.
	if cleanupErr := s.Unmount(rootFsDir); cleanupErr != nil {
		sylog.Errorf("While unmounting rootfs overlay: %v", cleanupErr)
	}

	// Return any error from the actual container payload - preserve exit code.
	return err
}

func prepareWritableTmpfs(bundleDir string) (string, error) {
	sylog.Debugf("Configuring writable tmpfs overlay for %s", bundleDir)
	c := singularityconf.GetCurrentConfig()
	if c == nil {
		return "", fmt.Errorf("singularity configuration is not initialized")
	}
	var err error

	oldumask := syscall.Umask(0)
	defer syscall.Umask(oldumask)

	olDir := filepath.Join(bundleDir, "overlay")
	err = overlay.EnsureOverlayDir(olDir, true, 0o700)
	if err != nil {
		return "", fmt.Errorf("failed to create %s: %s", olDir, err)
	}
	// delete overlay directory in case of error
	defer func() {
		if err != nil {
			sylog.Debugf("Encountered error in CreateOverlay; attempting to remove overlay dir %q", olDir)
			os.RemoveAll(olDir)
		}
	}()

	options := fmt.Sprintf("mode=1777,size=%dm", c.SessiondirMaxSize)
	err = syscall.Mount("tmpfs", olDir, "tmpfs", syscall.MS_NODEV, options)
	if err != nil {
		return "", fmt.Errorf("failed to bind %s: %s", olDir, err)
	}
	// best effort to cleanup mount
	defer func() {
		if err != nil {
			sylog.Debugf("Encountered error in CreateOverlayTmpfs; attempting to detach overlay dir %q", olDir)
			syscall.Unmount(olDir, syscall.MNT_DETACH)
		}
	}()

	olSet := overlay.Set{
		WritableOverlay: &overlay.Item{
			SourcePath: olDir,
			Type:       image.SANDBOX,
			Writable:   true,
		},
	}

	err = olSet.Mount(tools.RootFs(bundleDir).Path())
	if err != nil {
		return "", err
	}

	return olDir, nil
}

func cleanupWritableTmpfs(bundleDir, overlayDir string) error {
	sylog.Debugf("Cleaning up writable tmpfs overlay for %s", bundleDir)
	rootFsDir := tools.RootFs(bundleDir).Path()

	if err := overlay.DetachMount(rootFsDir); err != nil {
		return err
	}

	// Because CreateOverlayTmpfs() mounts the tmpfs on olDir, and then
	// calls ApplyOverlay(), there needs to be an extra unmount in the this case
	if err := overlay.DetachMount(overlayDir); err != nil {
		return err
	}

	return overlay.DetachAndDelete(overlayDir)
}
