// Copyright (c) 2019-2023, Sylabs Inc. All rights reserved.
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

	"github.com/sylabs/singularity/internal/pkg/buildcfg"
	"github.com/sylabs/singularity/internal/pkg/util/env"
	"github.com/sylabs/singularity/pkg/sylog"
	"github.com/sylabs/singularity/pkg/util/singularityconf"
)

// findOnPath performs a simple search on PATH for the named executable, returning its full path.
// env.DefaultPath` is appended to PATH to ensure standard locations are searched. This
// is necessary as some distributions don't include sbin on user PATH etc.
func findOnPath(name string) (path string, err error) {
	oldPath := os.Getenv("PATH")
	defer os.Setenv("PATH", oldPath)
	os.Setenv("PATH", oldPath+":"+env.DefaultPath)

	path, err = exec.LookPath(name)
	if err == nil {
		sylog.Debugf("Found %q at %q", name, path)
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

// findConmon returns either the bundled conmon (if built), or looks for conmon on PATH.
func findConmon(name string) (path string, err error) {
	if buildcfg.CONMON_LIBEXEC == 1 {
		return filepath.Join(buildcfg.LIBEXECDIR, "singularity", "bin", name), nil
	}
	return findOnPath(name)
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

// findSqfstar returns either the bundled sqfstar (if built), or looks for
// sqfstar / tar2sqfs on PATH.
func findSqfstar(name string) (path string, err error) {
	// Bundled sqfstar if it was built
	if buildcfg.SQFSTAR_LIBEXEC == 1 {
		return filepath.Join(buildcfg.LIBEXECDIR, "singularity", "bin", "sqfstar"), nil
	}
	// sqfstar if found on PATH
	llPath, sqfstarErr := findOnPath("sqfstar")
	if sqfstarErr == nil {
		return llPath, nil
	}

	// tar2sqfs if found on PATH
	llPath, tar2sqfsErr := findOnPath("tar2sqfs")
	if tar2sqfsErr == nil {
		return llPath, nil
	}

	return "", fmt.Errorf("could not find sqfstar or tar2sqfs on path: %w", sqfstarErr)
}
