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

// OciStart starts a previously create container
func OciStart(containerID string) error {
	runcArgs := []string{
		"--root=" + OciStateDir,
		"start",
		containerID,
	}

	sylog.Debugf("Calling runc with args %v", runcArgs)
	if err := syscall.Exec(runc, runcArgs, []string{}); err != nil {
		return fmt.Errorf("while calling runc: %w", err)
	}

	return nil
}
