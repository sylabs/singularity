// Copyright (c) 2022, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package singularity

import (
	"context"
	"fmt"
	"os"

	"github.com/sylabs/singularity/v4/internal/pkg/util/fs/squashfs"
	"github.com/sylabs/singularity/v4/pkg/sylog"
)

// CleanupHost cleans up a SIF FUSE mount and temporary directory. It is called
// from a HOST_CLEANUP process that exists in the original host namespaces.
func (e *EngineOperations) CleanupHost(ctx context.Context) (err error) {
	if e.EngineConfig.GetImageFuse() {
		sylog.Infof("Unmounting SIF with FUSE...")
		squashfs.FUSEUnmount(ctx, e.EngineConfig.GetImage())

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
