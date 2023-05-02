// Copyright (c) 2019, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package tools

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// CreateOverlay creates a writable overlay based on a directory.
func CreateOverlay(bundlePath string) error {
	var err error

	oldumask := syscall.Umask(0)
	defer syscall.Umask(oldumask)

	overlayDir := filepath.Join(bundlePath, "overlay")
	if err = os.Mkdir(overlayDir, 0o700); err != nil {
		return fmt.Errorf("failed to create %s: %s", overlayDir, err)
	}
	// delete overlay directory in case of error
	defer func() {
		if err != nil {
			os.RemoveAll(overlayDir)
		}
	}()

	err = syscall.Mount(overlayDir, overlayDir, "", syscall.MS_BIND, "")
	if err != nil {
		return fmt.Errorf("failed to bind %s: %s", overlayDir, err)
	}
	// best effort to cleanup mount
	defer func() {
		if err != nil {
			syscall.Unmount(overlayDir, syscall.MNT_DETACH)
		}
	}()

	if err = syscall.Mount("", overlayDir, "", syscall.MS_REMOUNT|syscall.MS_BIND, ""); err != nil {
		return fmt.Errorf("failed to remount %s: %s", overlayDir, err)
	}

	err = prepareOverlay(bundlePath, overlayDir)
	return err
}

// CreateOverlay creates a writable overlay based on a tmpfs.
func CreateOverlayTmpfs(bundlePath string, sizeMiB int) error {
	var err error

	oldumask := syscall.Umask(0)
	defer syscall.Umask(oldumask)

	overlayDir := filepath.Join(bundlePath, "overlay")
	if err = os.Mkdir(overlayDir, 0o700); err != nil {
		return fmt.Errorf("failed to create %s: %s", overlayDir, err)
	}
	// delete overlay directory in case of error
	defer func() {
		if err != nil {
			os.RemoveAll(overlayDir)
		}
	}()

	options := fmt.Sprintf("mode=1777,size=%dm", sizeMiB)
	err = syscall.Mount("tmpfs", overlayDir, "tmpfs", syscall.MS_NODEV, options)
	if err != nil {
		return fmt.Errorf("failed to bind %s: %s", overlayDir, err)
	}
	// best effort to cleanup mount
	defer func() {
		if err != nil {
			syscall.Unmount(overlayDir, syscall.MNT_DETACH)
		}
	}()

	err = prepareOverlay(bundlePath, overlayDir)
	return err
}

func prepareOverlay(bundlePath, overlayDir string) error {
	upperDir := filepath.Join(overlayDir, "upper")
	if err := os.Mkdir(upperDir, 0o755); err != nil {
		return fmt.Errorf("failed to create %s: %s", upperDir, err)
	}
	workDir := filepath.Join(overlayDir, "work")
	if err := os.Mkdir(workDir, 0o700); err != nil {
		return fmt.Errorf("failed to create %s: %s", workDir, err)
	}
	rootFsDir := RootFs(bundlePath).Path()

	options := fmt.Sprintf("lowerdir=%s,upperdir=%s,workdir=%s", rootFsDir, upperDir, workDir)
	if err := syscall.Mount("overlay", rootFsDir, "overlay", 0, options); err != nil {
		return fmt.Errorf("failed to mount %s: %s", overlayDir, err)
	}
	return nil
}

// DeleteOverlay deletes overlay
func DeleteOverlay(bundlePath string) error {
	overlayDir := filepath.Join(bundlePath, "overlay")
	rootFsDir := RootFs(bundlePath).Path()

	if err := syscall.Unmount(rootFsDir, syscall.MNT_DETACH); err != nil {
		return fmt.Errorf("failed to unmount %s: %s", rootFsDir, err)
	}
	if err := syscall.Unmount(overlayDir, syscall.MNT_DETACH); err != nil {
		return fmt.Errorf("failed to unmount %s: %s", overlayDir, err)
	}
	if err := os.RemoveAll(overlayDir); err != nil {
		return fmt.Errorf("failed to remove %s: %s", overlayDir, err)
	}
	return nil
}
