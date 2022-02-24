// Copyright (c) 2018-2022, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package singularity

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"syscall"

	"github.com/sylabs/singularity/pkg/sylog"
)

// OciRun runs a container (equivalent to create/start/delete)
func OciRun(ctx context.Context, containerID string, args *OciArgs) error {
	absBundle, err := filepath.Abs(args.BundlePath)
	if err != nil {
		return fmt.Errorf("failed to determine bundle absolute path: %s", err)
	}

	if err := os.Chdir(absBundle); err != nil {
		return fmt.Errorf("failed to change directory to %s: %s", absBundle, err)
	}

	runcArgs := []string{
		"--root=" + OciStateDir,
		"create",
		"-b", absBundle,
	}
	if args.PidFile != "" {
		runcArgs = append(runcArgs, "--pid-file="+args.PidFile)
	}
	runcArgs = append(runcArgs, containerID)

	sylog.Debugf("Calling runc with args %v", runcArgs)
	if err := syscall.Exec(runc, runcArgs, []string{}); err != nil {
		return fmt.Errorf("while calling runc: %w", err)
	}

	return nil
}
