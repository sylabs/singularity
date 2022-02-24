// Copyright (c) 2018-2020, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package singularity

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"

	"github.com/sylabs/singularity/pkg/sylog"
)

// OciCreate creates a container from an OCI bundle
func OciCreate(containerID string, args *OciArgs) error {
	absBundle, err := filepath.Abs(args.BundlePath)
	if err != nil {
		return fmt.Errorf("failed to determine bundle absolute path: %s", err)
	}

	if err := os.Chdir(absBundle); err != nil {
		return fmt.Errorf("failed to change directory to %s: %s", absBundle, err)
	}

	cmdArgs := []string{
		"--root=" + OciStateDir,
		"create",
		"-b", absBundle,
	}
	if args.PidFile != "" {
		cmdArgs = append(cmdArgs, "--pid-file="+args.PidFile)
	}
	cmdArgs = append(cmdArgs, containerID)

	sylog.Debugf("Calling runc with args %v", cmdArgs)
	if err := syscall.Exec(runc, cmdArgs, []string{}); err != nil {
		return fmt.Errorf("while calling runc: %w", err)
	}

	return nil
}
