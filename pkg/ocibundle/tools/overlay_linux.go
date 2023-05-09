// Copyright (c) 2019, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package tools

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

type OverlaySet struct {
	ReadonlyLocs []string
	WritableLoc  string
}

// CreateOverlay creates a writable overlay based on a directory inside the OCI bundle.
func CreateOverlay(bundlePath string) error {
	oldumask := syscall.Umask(0)
	defer syscall.Umask(oldumask)

	overlayDir := filepath.Join(bundlePath, "overlay")
	var err error
	if err = ensureOverlayDir(overlayDir, true, 0o700); err != nil {
		return fmt.Errorf("failed to create %s: %s", overlayDir, err)
	}
	// delete overlay directory in case of error
	defer func() {
		if err != nil {
			os.RemoveAll(overlayDir)
		}
	}()

	return MountOverlay(bundlePath, OverlaySet{WritableLoc: overlayDir})
}

func MountOverlay(bundlePath string, ovs OverlaySet) error {
	var err error

	locsToBind := ovs.ReadonlyLocs
	if len(ovs.WritableLoc) > 0 {
		// Check if writable overlay dir already exists; if it doesn't, try to create it.
		if err = ensureOverlayDir(ovs.WritableLoc, true, 0o755); err != nil {
			return err
		}

		locsToBind = append(locsToBind, ovs.WritableLoc)
	}

	// Try to do initial bind-mounts
	for _, d := range locsToBind {
		if err = ensureOverlayDir(d, false, 0); err != nil {
			return fmt.Errorf("error accessing directory %s: %s", d, err)
		}

		if err = syscall.Mount(d, d, "", syscall.MS_BIND, ""); err != nil {
			return fmt.Errorf("failed to bind %s: %s", d, err)
		}

		// best effort to cleanup mount
		defer func() {
			if err != nil {
				syscall.Unmount(d, syscall.MNT_DETACH)
			}
		}()

		// Try to perform remount
		if err = syscall.Mount("", d, "", syscall.MS_REMOUNT|syscall.MS_BIND, ""); err != nil {
			return fmt.Errorf("failed to remount %s: %s", d, err)
		}
	}

	// Prepare internal structure of overlay dir
	err = prepareOverlay(bundlePath, ovs)
	return err
}

// CreateOverlay creates a writable overlay based on a tmpfs.
func CreateOverlayTmpfs(bundlePath string, sizeMiB int) error {
	var err error

	oldumask := syscall.Umask(0)
	defer syscall.Umask(oldumask)

	overlayDir := filepath.Join(bundlePath, "overlay")
	if err = ensureOverlayDir(overlayDir, true, 0o700); err != nil {
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

	err = prepareOverlay(bundlePath, OverlaySet{WritableLoc: overlayDir})
	return err
}

// ensureOverlayDir checks if a directory already exists; if it doesn't, and writable is true, it attempts to create it with the specified permissions.
func ensureOverlayDir(dir string, createIfMissing bool, createPerm os.FileMode) error {
	_, err := os.Stat(dir)
	if dir == "" {
		panic("ensureOverlayDir on empty dir")
	}

	if err == nil {
		return nil
	}

	if !os.IsNotExist(err) {
		return err
	}

	if !createIfMissing {
		return fmt.Errorf("missing overlay dir %#v", dir)
	}

	// Create the requested dir
	if err := os.Mkdir(dir, createPerm); err != nil {
		return fmt.Errorf("failed to create %#v: %s", dir, err)
	}

	return nil
}

func prepareOverlay(bundlePath string, ovs OverlaySet) error {
	var err error

	rootFsDir := RootFs(bundlePath).Path()
	lowerDirJoined := strings.Join(append(ovs.ReadonlyLocs, rootFsDir), ":")

	// Prepare options string for mount
	var options string
	if len(ovs.WritableLoc) > 0 {
		upperDir := filepath.Join(ovs.WritableLoc, "upper")
		if err = ensureOverlayDir(upperDir, true, 0o755); err != nil {
			return err
		}

		workDir := filepath.Join(ovs.WritableLoc, "work")
		if err = ensureOverlayDir(workDir, true, 0o700); err != nil {
			return err
		}

		options = fmt.Sprintf("lowerdir=%s,upperdir=%s,workdir=%s", lowerDirJoined, upperDir, workDir)
	} else {
		options = fmt.Sprintf("lowerdir=%s", lowerDirJoined)
	}

	// Try to perform actual mount
	if err := syscall.Mount("overlay", rootFsDir, "overlay", 0, options); err != nil {
		return fmt.Errorf("failed to mount %s: %s", rootFsDir, err)
	}

	return nil
}

// DeleteOverlay deletes overlay
func DeleteOverlay(bundlePath string) error {
	if err := UnmountRootFSOverlay(bundlePath); err != nil {
		return err
	}

	overlayDir := filepath.Join(bundlePath, "overlay")
	if err := syscall.Unmount(overlayDir, syscall.MNT_DETACH); err != nil {
		return fmt.Errorf("failed to unmount %s: %s", overlayDir, err)
	}
	if err := os.RemoveAll(overlayDir); err != nil {
		return fmt.Errorf("failed to remove %s: %s", overlayDir, err)
	}
	return nil
}

// UnmountRootFSOverlay umounts a rootfs overlay
func UnmountRootFSOverlay(bundlePath string) error {
	rootFsDir := RootFs(bundlePath).Path()

	if err := syscall.Unmount(rootFsDir, syscall.MNT_DETACH); err != nil {
		return fmt.Errorf("failed to unmount %s: %s", rootFsDir, err)
	}

	return nil
}
