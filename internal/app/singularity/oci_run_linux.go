// Copyright (c) 2018-2022, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package singularity

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/sylabs/singularity/internal/pkg/util/bin"
	"github.com/sylabs/singularity/pkg/sylog"
)

// OciRun runs a container (equivalent to create/start/delete)
func OciRun(ctx context.Context, containerID string, args *OciArgs) error {
	runc, err := bin.FindBin("runc")
	if err != nil {
		return err
	}
	absBundle, err := filepath.Abs(args.BundlePath)
	if err != nil {
		return fmt.Errorf("failed to determine bundle absolute path: %s", err)
	}

	if err := os.Chdir(absBundle); err != nil {
		return fmt.Errorf("failed to change directory to %s: %s", absBundle, err)
	}

	runcArgs := []string{
		"--root", RuncStateDir,
		"run",
		"-b", absBundle,
	}
	if args.PidFile != "" {
		runcArgs = append(runcArgs, "--pid-file="+args.PidFile)
	}
	runcArgs = append(runcArgs, containerID)
	cmd := exec.Command(runc, runcArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdout
	sylog.Debugf("Calling runc with args %v", runcArgs)
	return cmd.Run()
}
