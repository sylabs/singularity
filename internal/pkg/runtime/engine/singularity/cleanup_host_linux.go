// Copyright (c) 2022, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package singularity

import (
	"context"
	"fmt"
	"os"

	sifuser "github.com/sylabs/sif/v2/pkg/user"
	"github.com/sylabs/singularity/internal/pkg/util/bin"
	"github.com/sylabs/singularity/pkg/sylog"
)

// CleanupHost cleans up a SIF FUSE mount and temporary directory. It is called
// from a HOST_CLEANUP process that exists in the original host namespaces.
func (e *EngineOperations) CleanupHost(ctx context.Context) (err error) {
	if e.EngineConfig.GetImageFuse() {
		sylog.Infof("Unmounting SIF with FUSE...")
		fusermountPath, err := bin.FindBin("fusermount")
		if err != nil {
			return fmt.Errorf("while unmounting fuse directory: %s: %w", e.EngineConfig.GetImage(), err)
		}

		err = sifuser.Unmount(ctx, e.EngineConfig.GetImage(),
			sifuser.OptUnmountStdout(os.Stdout),
			sifuser.OptUnmountStderr(os.Stderr),
			sifuser.OptUnmountFusermountPath(fusermountPath))
		if err != nil {
			return fmt.Errorf("while unmounting fuse directory: %s: %w", e.EngineConfig.GetImage(), err)
		}

		if tempDir := e.EngineConfig.GetDeleteTempDir(); tempDir != "" {
			sylog.Infof("Removing image tempDir %s", tempDir)
			err := os.RemoveAll(tempDir)
			if err != nil {
				return fmt.Errorf("failed to delete container image tempDir %s: %w", tempDir, err)
			}
		}
	}
	return nil
}
