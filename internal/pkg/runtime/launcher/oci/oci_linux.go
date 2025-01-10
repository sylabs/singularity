// Copyright (c) 2018-2024, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

// Includes code from https://github.com/containers/podman
// Released under the Apache License Version 2.0

package oci

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"syscall"
	"time"

	securejoin "github.com/cyphar/filepath-securejoin"
	"github.com/sylabs/singularity/v4/internal/pkg/cgroups"
	"github.com/sylabs/singularity/v4/internal/pkg/util/bin"
	"github.com/sylabs/singularity/v4/internal/pkg/util/fs"
	"github.com/sylabs/singularity/v4/internal/pkg/util/rootless"
	"github.com/sylabs/singularity/v4/pkg/syfs"
	"github.com/sylabs/singularity/v4/pkg/sylog"
	"github.com/sylabs/singularity/v4/pkg/util/fs/lock"
)

const (
	// Relative path inside ~/.singularity for conmon and singularity state
	ociPath = "oci"
	// State directory files
	containerPidFile = "container.pid"
	containerLogFile = "container.log"
	runcLogFile      = "runc.log"
	conmonPidFile    = "conmon.pid"
	bundleLink       = "bundle"
	// Files in the OCI bundle root
	bundleLock   = ".singularity-oci.lock"
	attachSocket = "attach"
	// Timeouts
	createTimeout = 30 * time.Second
)

// Runtime returns path to the OCI Runtime - crun (preferred), or runc.
func Runtime() (path string, err error) {
	path, err = bin.FindBin("crun")
	if err == nil {
		return
	}
	sylog.Debugf("While finding crun: %s", err)
	sylog.Debugf("Falling back to runc as OCI runtime.")
	return bin.FindBin("runc")
}

// runtimeStateDir returns path to use for crun/runc's state handling.
func runtimeStateDir() (path string, err error) {
	// Ensure we get correct uid for host if we were re-exec'd in id mapped userns
	u, err := rootless.GetUser()
	if err != nil {
		return "", err
	}

	// Root - use our own /run directory
	if u.Uid == "0" {
		return "/run/singularity-oci", nil
	}

	// Prefer XDG_RUNTIME_DIR for non-root, if set and usable.
	if ok, _ := cgroups.HasXDGRuntimeDir(); ok {
		d := filepath.Join(os.Getenv("XDG_RUNTIME_DIR"), "singularity-oci")
		sylog.Debugf("Using XDG_RUNTIME_DIR for runtime state (%s)", d)
		return d, nil
	}

	// If no XDG_RUNTIME_DIR, then try standard user session directory location.
	runDir := fmt.Sprintf("/run/user/%s/", u.Uid)
	if fs.IsDir(runDir) {
		d := filepath.Join(runDir, "singularity-oci")
		sylog.Debugf("Using /run/user default for runtime state (%s)", d)
		return d, nil
	}

	// If standard user session directory not available, use TMPDIR as a last resort.
	runDir = filepath.Join(os.TempDir(), "singularity-oci-"+u.Uid)
	sylog.Infof("No /run/user session directory for user. Using %q for runtime state.", runDir)

	// Create if not present
	st, err := os.Stat(runDir)
	if os.IsNotExist(err) {
		return runDir, os.Mkdir(runDir, 0o700)
	}
	if err != nil {
		return "", err
	}

	// If it exists, verify it's a directory with correct ownership, perms.
	if !st.IsDir() {
		return "", fmt.Errorf("%s exists, but is not a directory", runDir)
	}
	if st.Sys().(*syscall.Stat_t).Uid != uint32(os.Geteuid()) { //nolint:forcetypeassert
		return "", fmt.Errorf("%s exists, but is not owned by correct user", runDir)
	}
	if st.Mode().Perm() != 0o700 {
		return "", fmt.Errorf("%s exists, but does not have correct permissions (700)", runDir)
	}

	return runDir, nil
}

// stateDir returns the path to container state handled by conmon/singularity
// (as opposed to runc's state in RuncStateDir)
func stateDir(containerID string) (string, error) {
	hostname, err := os.Hostname()
	if err != nil {
		return "", err
	}

	u, err := rootless.GetUser()
	if err != nil {
		return "", err
	}

	configDir, err := syfs.ConfigDirForUsername(u.Username)
	if err != nil {
		return "", err
	}

	rootPath := filepath.Join(configDir, ociPath)
	containerPath := filepath.Join(hostname, containerID)
	path, err := securejoin.SecureJoin(rootPath, containerPath)
	if err != nil {
		return "", err
	}
	return path, err
}

// lockBundle creates a lock file in a bundle directory
func lockBundle(bundlePath string) error {
	bl := path.Join(bundlePath, bundleLock)
	_, err := os.Stat(bl)
	if err == nil {
		return fmt.Errorf("bundle is locked by another process")
	}
	if !os.IsNotExist(err) {
		return fmt.Errorf("while stat-ing lock file: %w", err)
	}

	fd, err := lock.Exclusive(bundlePath)
	if err != nil {
		return fmt.Errorf("while acquiring directory lock: %w", err)
	}
	defer lock.Release(fd)

	err = fs.EnsureFileWithPermission(bl, 0o600)
	if err != nil {
		return fmt.Errorf("while creating lock file: %w", err)
	}
	return nil
}

// releaseBundle removes a lock file in a bundle directory
func releaseBundle(bundlePath string) error {
	bl := path.Join(bundlePath, bundleLock)
	_, err := os.Stat(bl)
	if os.IsNotExist(err) {
		return fmt.Errorf("bundle is not locked")
	}
	if err != nil {
		return fmt.Errorf("while stat-ing lock file: %w", err)
	}

	fd, err := lock.Exclusive(bundlePath)
	if err != nil {
		return fmt.Errorf("while acquiring directory lock: %w", err)
	}
	defer lock.Release(fd)

	err = os.Remove(bl)
	if err != nil {
		return fmt.Errorf("while removing lock file: %w", err)
	}
	return nil
}
