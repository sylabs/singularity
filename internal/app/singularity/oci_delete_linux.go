// Copyright (c) 2018-2020, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package singularity

import (
	"context"
	"fmt"
	"syscall"

	"github.com/sylabs/singularity/pkg/sylog"
)

// OciDelete deletes container resources
func OciDelete(ctx context.Context, containerID string) error {
	runcArgs := []string{
		"--root=" + OciStateDir,
		"delete",
		containerID,
	}

	sylog.Debugf("Calling runc with args %v", runcArgs)
	if err := syscall.Exec(runc, runcArgs, []string{}); err != nil {
		return fmt.Errorf("while calling runc: %w", err)
	}

	return nil
}
