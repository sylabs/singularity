// Copyright (c) 2019-2026, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

//go:build singularity_engine

package bin

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/sylabs/singularity/v4/internal/pkg/buildcfg"
	"github.com/sylabs/singularity/v4/internal/pkg/util/user"
	"github.com/sylabs/singularity/v4/pkg/sylog"
	"github.com/sylabs/singularity/v4/pkg/util/singularityconf"
)

// findOnPath performs a search for the given executable name within the search path
// values specified for host root / non-root in singularity.conf.
func findOnPath(name string) (path string, err error) {
	cfg := singularityconf.GetCurrentConfig()
	if cfg == nil {
		cfg, err = singularityconf.Parse(buildcfg.SINGULARITY_CONF_FILE)
		if err != nil {
			return "", fmt.Errorf("unable to parse singularity configuration file: %w", err)
		}
	}

	// Search paths for host root and non-root / fake root differ.
	u, err := user.CurrentOriginal()
	if err != nil {
		return "", fmt.Errorf("while retrieving current user information: %w", err)
	}
	searchPath := cfg.UserSearchPath
	if u.UID == 0 {
		searchPath = cfg.RootSearchPath
	}

	// Handle $PATH if present in path config.
	searchPath = parsePath(searchPath)

	oldPath := os.Getenv("PATH")
	defer os.Setenv("PATH", oldPath)
	os.Setenv("PATH", searchPath)
	path, err = exec.LookPath(name)
	if err == nil {
		sylog.Debugf("Found %q at %q, searching %q", name, path, searchPath)
	}
	return path, err
}

// findFromConfigOrPath retrieves the path to an executable from singularity.conf,
// or searches PATH if not set there.
func findFromConfigOrPath(name string) (path string, err error) {
	cfg := singularityconf.GetCurrentConfig()
	if cfg == nil {
		cfg, err = singularityconf.Parse(buildcfg.SINGULARITY_CONF_FILE)
		if err != nil {
			return "", fmt.Errorf("unable to parse singularity configuration file: %w", err)
		}
	}

	switch name {
	case "go":
		path = cfg.GoPath
	case "mksquashfs":
		path = cfg.MksquashfsPath
	case "unsquashfs":
		path = cfg.UnsquashfsPath
	default:
		return "", fmt.Errorf("unknown executable name %q", name)
	}

	if path == "" {
		return findOnPath(name)
	}

	sylog.Debugf("Using %q at %q (from singularity.conf)", name, path)

	// Use lookPath with the absolute path to confirm it is accessible & executable
	return exec.LookPath(path)
}

// findFromConfigOnly retrieves the path to an executable from singularity.conf.
// If it's not set there we error.
func findFromConfigOnly(name string) (path string, err error) {
	cfg := singularityconf.GetCurrentConfig()
	if cfg == nil {
		cfg, err = singularityconf.Parse(buildcfg.SINGULARITY_CONF_FILE)
		if err != nil {
			return "", fmt.Errorf("unable to parse singularity configuration file: %w", err)
		}
	}

	switch name {
	case "cryptsetup":
		path = cfg.CryptsetupPath
	case "ldconfig":
		path = cfg.LdconfigPath
	case "nvidia-container-cli":
		path = cfg.NvidiaContainerCliPath
	default:
		return "", fmt.Errorf("unknown executable name %q", name)
	}

	if path == "" {
		return "", fmt.Errorf("path to %q not set in singularity.conf", name)
	}

	sylog.Debugf("Using %q at %q (from singularity.conf)", name, path)

	// Use lookPath with the absolute path to confirm it is accessible & executable
	return exec.LookPath(path)
}

// findSquashfuse returns either the bundled squashfuse_ll (if built), or looks
// for squashfuse_ll / squashfuse on PATH.
func findSquashfuse(name string) (path string, err error) {
	// Bundled squashfuse_ll if it was built
	if buildcfg.SQUASHFUSE_LIBEXEC == 1 {
		return filepath.Join(buildcfg.LIBEXECDIR, "singularity", "bin", "squashfuse_ll"), nil
	}
	// squashfuse_ll if found on PATH
	llPath, err := findOnPath("squashfuse_ll")
	if err == nil {
		return llPath, nil
	}
	// squashfuse if found on PATH
	return findOnPath(name)
}

// findSingularityBuildkitd returns the bundled singularity-buildkitd.
func FindSingularityBuildkitd() (path string, err error) {
	bkd := filepath.Join(buildcfg.LIBEXECDIR, "singularity", "bin", "singularity-buildkitd")
	return exec.LookPath(bkd)
}

// parsePath parses a path string, replacing $PATH with PATH from the environment
func parsePath(p string) string {
	if strings.Contains(p, "$PATH") {
		envPath := os.Getenv("PATH")
		p = strings.Replace(p, "$PATH", envPath, 1)
	}

	// Drop any empty PATH elements, which would be treated as CWD. os.LookPath
	// will return ErrDot on these, but let's avoid CWD lookup entirely.
	parts := filepath.SplitList(p)
	validParts := make([]string, 0, len(parts))
	for _, part := range parts {
		if part != "" {
			validParts = append(validParts, part)
		}
	}
	return strings.Join(validParts, string(os.PathListSeparator))
}
