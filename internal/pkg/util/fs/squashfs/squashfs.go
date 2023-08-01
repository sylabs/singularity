// Copyright (c) 2019-2021, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package squashfs

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/sylabs/singularity/v4/internal/pkg/buildcfg"
	"github.com/sylabs/singularity/v4/internal/pkg/util/bin"
	"github.com/sylabs/singularity/v4/pkg/sylog"
	"github.com/sylabs/singularity/v4/pkg/util/singularityconf"
)

func getConfig() (*singularityconf.File, error) {
	// if the caller has set the current config use it
	// otherwise parse the default configuration file
	cfg := singularityconf.GetCurrentConfig()
	if cfg == nil {
		var err error

		configFile := buildcfg.SINGULARITY_CONF_FILE
		cfg, err = singularityconf.Parse(configFile)
		if err != nil {
			return nil, fmt.Errorf("unable to parse singularity.conf file: %s", err)
		}
	}
	return cfg, nil
}

// GetPath figures out where the mksquashfs binary is
// and return an error is not available or not usable.
func GetPath() (string, error) {
	return bin.FindBin("mksquashfs")
}

func GetProcs() (uint, error) {
	c, err := getConfig()
	if err != nil {
		return 0, err
	}
	// proc is either "" or the string value in the conf file
	proc := c.MksquashfsProcs

	return proc, err
}

func GetMem() (string, error) {
	c, err := getConfig()
	if err != nil {
		return "", err
	}
	// mem is either "" or the string value in the conf file
	mem := c.MksquashfsMem

	return mem, err
}

func FUSEMount(ctx context.Context, offset uint64, path, mountPath string) error {
	args := []string{
		"-o", fmt.Sprintf("ro,offset=%d,uid=%d,gid=%d", offset, os.Getuid(), os.Getgid()),
		filepath.Clean(path),
		filepath.Clean(mountPath),
	}

	squashfuse, err := bin.FindBin("squashfuse")
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, squashfuse, args...) //nolint:gosec

	sylog.Debugf("Executing %s %s", squashfuse, strings.Join(args, " "))

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to mount: %w", err)
	}

	return nil
}

func FUSEUnmount(ctx context.Context, mountPath string) error {
	args := []string{
		"-u",
		filepath.Clean(mountPath),
	}

	fusermount, err := bin.FindBin("fusermount")
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, fusermount, args...) //nolint:gosec

	sylog.Debugf("Executing %s %s", fusermount, strings.Join(args, " "))

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to unmount: %w", err)
	}

	return nil
}
