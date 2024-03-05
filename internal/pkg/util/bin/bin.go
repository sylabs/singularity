// Copyright (c) 2019-2023, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

// Package bin provides access to external binaries
package bin

import (
	"fmt"
)

// FindBin returns the path to the named binary, or an error if it is not found.
func FindBin(name string) (path string, err error) {
	switch name {
	// Basic system executables that we assume are always on PATH
	case "true", "mkfs.ext3", "cp", "rm", "dd", "truncate":
		return findOnPath(name)
	// Bootstrap related executables that we assume are on PATH
	case "mount", "mknod", "debootstrap", "pacstrap", "dnf", "yum", "rpm", "curl", "uname", "zypper", "SUSEConnect", "rpmkeys", "proot":
		return findOnPath(name)
	// Configurable executables that are found at build time, can be overridden
	// in singularity.conf. If config value is "" will look on PATH.
	case "unsquashfs", "mksquashfs", "go":
		return findFromConfigOrPath(name)
	// distro provided setUID executables that are used in the fakeroot flow to setup subuid/subgid mappings
	case "newuidmap", "newgidmap":
		return findOnPath(name)
	// distro provided OCI runtime
	case "crun", "runc":
		return findOnPath(name)
	// our, or distro provided conmon
	case "conmon":
		// Behavior depends on a buildcfg - whether to use bundled or external conmon
		return findConmon(name)
	// cryptsetup & nvidia-container-cli paths must be explicitly specified
	// They are called as root from the RPC server in a setuid install, so this
	// limits to sysadmin controlled paths.
	// ldconfig is invoked by nvidia-container-cli, so must be trusted also.
	case "cryptsetup", "ldconfig", "nvidia-container-cli":
		return findFromConfigOnly(name)
	// distro provided squashfuse and fusermount for unpriv SIF mount and
	// OCI-mode bare-image overlay
	case "fusermount", "fusermount3":
		return findFusermount()
	case "squashfuse":
		// Behavior depends on buildcfg - whether to use bundled squashfuse_ll or external squashfuse_ll/squashfuse
		return findSquashfuse(name)
	// fuse2fs for OCI-mode bare-image overlay
	case "fuse2fs":
		return findOnPath(name)
	// fuse-overlayfs for mounting overlays without kernel support for
	// unprivileged overlays
	case "fuse-overlayfs":
		return findOnPath(name)
	// tar to squashfs tools for OCI-mode image conversion
	case "tar2sqfs", "sqfstar":
		return findOnPath(name)
	default:
		return "", fmt.Errorf("executable name %q is not known to FindBin", name)
	}
}

// findFusermount looks for fusermount3 or, if that's not found, fusermount, on
// PATH.
func findFusermount() (string, error) {
	// fusermount3 if found on PATH
	path, err := findOnPath("fusermount3")
	if err == nil {
		return path, nil
	}
	// squashfuse if found on PATH
	return findOnPath("fusermount")
}
