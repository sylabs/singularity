// Copyright (c) 2022-2026, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package singularity

import (
	"context"
	"fmt"
	"os"
	"slices"

	"github.com/sylabs/singularity/v4/internal/pkg/util/fs"
	"github.com/sylabs/singularity/v4/internal/pkg/util/fs/fuse"
	"github.com/sylabs/singularity/v4/pkg/sylog"
	"github.com/sylabs/singularity/v4/pkg/util/fs/proc"
)

// PostStartHost cleans up a SIF FUSE image mount and the temporary directory
// that holds it. It is called from a POST_START_HOST process that exists in the
// original host namespaces.
func (e *EngineOperations) PostStartHost(ctx context.Context) (err error) {
	if e.EngineConfig.GetImageFuse() && e.EngineConfig.GetDeleteTempDir() != "" {
		return cleanFUSETempDir(ctx, e)
	}
	return nil
}

// CleanupHost cleans up a SIF FUSE image mount and related temporary
// directories. If container creation fails early, in STAGE 1, it will be called
// directly from STAGE 1. Otherwise, it will be called from a CLEANUP_HOST
// process, when the container cleanly exits, or is killed.
func (e *EngineOperations) CleanupHost(ctx context.Context) (err error) {
	if !e.EngineConfig.GetImageFuse() {
		return nil
	}

	// Accumulate errors instead of returning early, so all cleanup steps are attempted.
	errors := []error{}

	// GetDeleteTempDir being set with GetImageFuse also true indicates the
	// rootfs is FUSE mounted on a subdir of GetDeleteTempDir, and should be
	// unmounted and the tempdir removed. It should have been cleaned up with a
	// lazy unmount in PostStartHost, but if something went wrong there, or we
	// didn't ever start the container, we clean up here.
	if tmpDir := e.EngineConfig.GetDeleteTempDir(); tmpDir != "" {
		if fs.IsDir(tmpDir) {
			sylog.Debugf("FUSE mount temporary directory still present in CleanupHost: %s", tmpDir)
			if err := cleanFUSETempDir(ctx, e); err != nil {
				sylog.Errorf("Failed to clean up FUSE mount: %v", err)
				errors = append(errors, err)
			}
		}
	}

	// GetDeletePullTempDir being set indicates the underlying image was
	// implicitly pulled to a temporary directory, due to disabled cache, and
	// this should be removed.
	if tmpDir := e.EngineConfig.GetDeletePullTempDir(); tmpDir != "" {
		sylog.Debugf("Cleaning up image pull temporary directory %s", tmpDir)
		err := os.RemoveAll(tmpDir)
		if err != nil && !os.IsNotExist(err) {
			sylog.Errorf("Failed to delete image pull temporary directory %s: %s", tmpDir, err)
			errors = append(errors, fmt.Errorf("failed to delete image pull temporary directory %s: %w", tmpDir, err))
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("encountered errors during CleanupHost: %v", errors)
	}

	return nil
}

func cleanFUSETempDir(ctx context.Context, e *EngineOperations) error {
	sylog.Debugf("Lazy Unmounting SIF with FUSE: %s", e.EngineConfig.GetImage())
	if err := fuse.UnmountWithFuseLazy(ctx, e.EngineConfig.GetImage()); err != nil {
		// Don't error out if the mount is already gone.
		mounted, mErr := mounted(e.EngineConfig.GetImage())
		if mErr != nil {
			return fmt.Errorf("while checking if fuse directory is still mounted: %w", mErr)
		}
		if mounted {
			return fmt.Errorf("while unmounting fuse directory: %s: %w", e.EngineConfig.GetImage(), err)
		}
	}

	tmpDir := e.EngineConfig.GetDeleteTempDir()
	if tmpDir != "" {
		sylog.Debugf("Removing FUSE mount temporary directory: %s", tmpDir)
		err := os.RemoveAll(tmpDir)
		if err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("failed to delete temporary directory %s: %s", tmpDir, err)
		}
	}
	return nil
}

func mounted(path string) (bool, error) {
	entries, miErr := proc.GetMountInfoEntry("/proc/self/mountinfo")
	if miErr != nil {
		return false, fmt.Errorf("while parsing mountinfo: %w", miErr)
	}
	matchFn := func(entry proc.MountInfoEntry) bool {
		return entry.Point == path
	}
	return slices.ContainsFunc(entries, matchFn), nil
}
