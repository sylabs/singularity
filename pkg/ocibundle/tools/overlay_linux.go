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

// CreateOverlay creates a writable overlay based on a directory inside the OCI bundle.
func CreateOverlay(bundlePath string) error {
	oldumask := syscall.Umask(0)
	defer syscall.Umask(oldumask)

	overlayPath := filepath.Join(bundlePath, "overlay")
	var err error
	if err = os.Mkdir(overlayPath, 0o700); err != nil {
		return fmt.Errorf("failed to create %s: %s", overlayPath, err)
	}
	// delete overlay directory in case of error
	defer func() {
		if err != nil {
			os.RemoveAll(overlayPath)
		}
	}()

	return CreateOverlayByPath(bundlePath, overlayPath)
}

// CreateOverlayByPath creates a writable overlay based on a directory whose path is specified in the second argument.
func CreateOverlayByPath(bundlePath string, overlayPath string) error {
	var err error

	_, err = os.Stat(overlayPath)
	if err != nil {
		if os.IsNotExist(err) {
			err := os.Mkdir(overlayPath, 0o755)
			if err != nil {
				return fmt.Errorf("failed to create %s: %s", overlayPath, err)
			}
		} else {
			return err
		}
	}

	err = syscall.Mount(overlayPath, overlayPath, "", syscall.MS_BIND, "")
	if err != nil {
		return fmt.Errorf("failed to bind %s: %s", overlayPath, err)
	}
	// best effort to cleanup mount
	defer func() {
		if err != nil {
			syscall.Unmount(overlayPath, syscall.MNT_DETACH)
		}
	}()

	if err = syscall.Mount("", overlayPath, "", syscall.MS_REMOUNT|syscall.MS_BIND, ""); err != nil {
		return fmt.Errorf("failed to remount %s: %s", overlayPath, err)
	}

	err = prepareOverlay(bundlePath, overlayPath)
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
	var err error

	upperDir := filepath.Join(overlayDir, "upper")
	_, err = os.Stat(upperDir)
	if err != nil {
		if os.IsNotExist(err) {
			err := os.Mkdir(upperDir, 0o755)
			if err != nil {
				return fmt.Errorf("failed to create %s: %s", upperDir, err)
			}
		} else {
			return err
		}
	}

	workDir := filepath.Join(overlayDir, "work")
	_, err = os.Stat(workDir)
	if err != nil {
		if os.IsNotExist(err) {
			err := os.Mkdir(workDir, 0o700)
			if err != nil {
				return fmt.Errorf("failed to create %s: %s", workDir, err)
			}
		} else {
			return err
		}
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
