// Copyright (c) 2018-2019, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package singularity

import (
	"fmt"
	"syscall"

	"github.com/sylabs/singularity/pkg/sylog"
)

// OciExec executes a command in a container
func OciExec(containerID string, cmdArgs []string) error {
	runcArgs := []string{
		"--root=" + OciStateDir,
		"exec",
		containerID,
	}
	runcArgs = append(runcArgs, cmdArgs...)

	sylog.Debugf("Calling runc with args %v", runcArgs)
	if err := syscall.Exec(runc, runcArgs, []string{}); err != nil {
		return fmt.Errorf("while calling runc: %w", err)
	}

	return nil
}
