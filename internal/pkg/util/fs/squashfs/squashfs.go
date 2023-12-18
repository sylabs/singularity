// Copyright (c) 2019-2023, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package squashfs

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/sylabs/singularity/v4/internal/pkg/util/fs/fuse"
	"github.com/sylabs/singularity/v4/pkg/image"
)

func FUSEMount(ctx context.Context, offset uint64, path, mountPath string, allowOther bool) (*fuse.ImageMount, error) {
	im := fuse.ImageMount{
		Type:       image.SQUASHFS,
		UID:        os.Getuid(),
		GID:        os.Getgid(),
		Readonly:   true,
		SourcePath: filepath.Clean(path),
		AllowOther: allowOther,
		ExtraOpts: []string{
			fmt.Sprintf("offset=%d", offset),
		},
	}
	im.SetMountPoint(filepath.Clean(mountPath))

	return &im, im.Mount(ctx)
}

func FUSEUnmount(ctx context.Context, mountPath string) error {
	return fuse.UnmountWithFuse(ctx, mountPath)
}
