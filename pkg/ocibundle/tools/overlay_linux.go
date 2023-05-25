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

	"github.com/sylabs/singularity/internal/pkg/util/fs/overlay"
	"github.com/sylabs/singularity/pkg/image"
	"github.com/sylabs/singularity/pkg/sylog"
)

// CreateOverlay creates a writable overlay using a directory inside the OCI
// bundle.
func CreateOverlay(bundlePath string) error {
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
		Writable:   true,
	}}

	return olSet.Mount(RootFs(bundlePath).Path())
}

// DeleteOverlay deletes an overlay previously created using a directory inside
// the OCI bundle.
func DeleteOverlay(bundlePath string) error {
	olDir := filepath.Join(bundlePath, "overlay")
	rootFsDir := RootFs(bundlePath).Path()

	if err := overlay.DetachMount(rootFsDir); err != nil {
		return err
	}

	return overlay.DetachAndDelete(olDir)
}

// CreateOverlay creates a writable overlay using tmpfs.
func CreateOverlayTmpfs(bundlePath string, sizeMiB int) (string, error) {
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

	olSet := overlay.Set{WritableOverlay: &overlay.Item{
		SourcePath: olDir,
		Type:       image.SANDBOX,
		Writable:   true,
	}}

	err = olSet.Mount(RootFs(bundlePath).Path())
	if err != nil {
		return "", err
	}

	return olDir, nil
}

// DeleteOverlayTmpfs deletes an overlay previously created using tmpfs.
func DeleteOverlayTmpfs(bundlePath, olDir string) error {
	rootFsDir := RootFs(bundlePath).Path()

	if err := overlay.DetachMount(rootFsDir); err != nil {
		return err
	}

	// Because CreateOverlayTmpfs() mounts the tmpfs on olDir, and then
	// calls ApplyOverlay(), there needs to be an extra unmount in the this case
	if err := overlay.DetachMount(olDir); err != nil {
		return err
	}

	return overlay.DetachAndDelete(olDir)
}
