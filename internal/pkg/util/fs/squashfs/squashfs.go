// Copyright (c) 2019-2023, Sylabs Inc. All rights reserved.
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

	"github.com/sylabs/singularity/v4/internal/pkg/util/bin"
	"github.com/sylabs/singularity/v4/pkg/sylog"
)

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

	cmd := exec.CommandContext(ctx, squashfuse, args...)
	if outputBytes, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf(
			"failed to mount: %w (cmdline: %q; output: %q)", err,
			strings.Join(append([]string{squashfuse}, args...), " "),
			string(outputBytes),
		)
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
	cmd := exec.CommandContext(ctx, fusermount, args...)

	sylog.Debugf("Executing %s %s", fusermount, strings.Join(args, " "))

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to unmount: %w", err)
	}

	return nil
}
