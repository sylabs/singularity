// Copyright (c) 2019-2023, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"syscall"

	"github.com/sylabs/singularity/v4/internal/pkg/util/fs/overlay"
	"github.com/sylabs/singularity/v4/pkg/image"
	"github.com/sylabs/singularity/v4/pkg/sylog"
)

// CreateOverlay creates a writable overlay using a directory inside the OCI
// bundle.
func CreateOverlay(ctx context.Context, bundlePath string) error {
	oldumask := syscall.Umask(0)
	defer syscall.Umask(oldumask)

	olDir := filepath.Join(bundlePath, "overlay")
	var err error
	if err = overlay.EnsureOverlayDir(olDir, true, 0o700); err != nil {
		return fmt.Errorf("failed to create %s: %s", olDir, err)
	}
	// delete overlay directory in case of error
	defer func() {
		if err != nil {
			sylog.Debugf("Encountered error in CreateOverlay; attempting to remove overlay dir %q", olDir)
			os.RemoveAll(olDir)
		}
	}()

	olSet := overlay.Set{WritableOverlay: &overlay.Item{
		SourcePath: olDir,
		Type:       image.SANDBOX,
		Readonly:   false,
	}}

	return olSet.Mount(ctx, RootFs(bundlePath).Path())
}

// DeleteOverlay deletes an overlay previously created using a directory inside
// the OCI bundle.
func DeleteOverlay(ctx context.Context, bundlePath string) error {
	olDir := filepath.Join(bundlePath, "overlay")
	rootFsDir := RootFs(bundlePath).Path()

	if err := overlay.DetachMount(ctx, rootFsDir); err != nil {
		return err
	}

	return overlay.DetachAndDelete(olDir)
}

// PrepareOverlayTmpfs creates a tmpfs, mounted to the overlay/ directory in the
// bundle, but does not perform an overlay mount using it.
func PrepareOverlayTmpfs(bundlePath string, sizeMiB int, allowSetuid bool) (string, error) {
	var err error

	oldumask := syscall.Umask(0)
	defer syscall.Umask(oldumask)

	olDir := filepath.Join(bundlePath, "overlay")
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

	options := fmt.Sprintf("mode=1777,size=%dm", sizeMiB)

	var flags uintptr = syscall.MS_NODEV
	if !allowSetuid {
		flags |= syscall.MS_NOSUID
	}

	err = syscall.Mount("tmpfs", olDir, "tmpfs", flags, options)
	if err != nil {
		return "", fmt.Errorf("failed to bind %s: %s", olDir, err)
	}
	return olDir, nil
}

// CreateOverlay creates a writable overlay using tmpfs, mounting it over the bundle rootfs.
func CreateOverlayTmpfs(ctx context.Context, bundlePath string, sizeMiB int, allowSetuid bool) (string, error) {
	olDir, err := PrepareOverlayTmpfs(bundlePath, sizeMiB, allowSetuid)
	if err != nil {
		return "", err
	}

	oi := overlay.Item{
		SourcePath: olDir,
		Type:       image.SANDBOX,
		Readonly:   false,
	}
	oi.SetAllowSetuid(allowSetuid)
	olSet := overlay.Set{WritableOverlay: &oi}

	err = olSet.Mount(ctx, RootFs(bundlePath).Path())
	if err != nil {
		// best effort to cleanup tmpfs mount
		sylog.Debugf("Encountered error in CreateOverlayTmpfs; attempting to detach overlay dir %q", olDir)
		syscall.Unmount(olDir, syscall.MNT_DETACH)
		return "", err
	}

	return olDir, nil
}

// DeleteOverlayTmpfs unmounts and deletes a tmpfs backed overlay created with CreateOverlayTmpfs.
func DeleteOverlayTmpfs(ctx context.Context, bundlePath, olDir string) error {
	rootFsDir := RootFs(bundlePath).Path()

	if err := overlay.DetachMount(ctx, rootFsDir); err != nil {
		return err
	}

	// Because CreateOverlayTmpfs() mounts the tmpfs on olDir, and then
	// calls ApplyOverlay(), there needs to be an extra unmount in the this case
	if err := overlay.DetachMount(ctx, olDir); err != nil {
		return err
	}

	return overlay.DetachAndDelete(olDir)
}
