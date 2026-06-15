// Copyright (c) 2018-2026, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package sources

import (
	"context"
	"fmt"
	"path/filepath"

	shub "github.com/sylabs/singularity/v4/internal/pkg/client/shub"
	"github.com/sylabs/singularity/v4/pkg/build/types"
	"github.com/sylabs/singularity/v4/pkg/sylog"
)

// ShubConveyorPacker only needs to hold the conveyor to have the needed data to pack.
type ShubConveyorPacker struct {
	b *types.Bundle
	LocalPacker
}

// Get downloads container from Singularityhub.
func (cp *ShubConveyorPacker) Get(ctx context.Context, b *types.Bundle) (err error) {
	sylog.Debugf("Getting container from Shub")

	cp.b = b

	src := `shub://` + b.Recipe.Header["from"]

	var imagePath string
	if b.Opts.ImgCache.IsDisabled() {
		imageTemp := filepath.Join(b.TmpDir, "library-image")
		imagePath, err = shub.PullToFile(ctx, b.Opts.ImgCache, imageTemp, src, b.Opts.NoHTTPS)
	} else {
		imagePath, err = shub.PullToCache(ctx, b.Opts.ImgCache, src, b.Opts.NoHTTPS)
	}
	if err != nil {
		return fmt.Errorf("while fetching library image: %v", err)
	}

	// insert base metadata before unpacking fs
	if err = makeBaseEnv(cp.b, true); err != nil {
		return fmt.Errorf("while inserting base environment: %v", err)
	}

	cp.LocalPacker, err = GetLocalPacker(ctx, imagePath, cp.b)

	return err
}

// CleanUp removes any tmpfs owned by the conveyorPacker on the filesystem
func (cp *ShubConveyorPacker) CleanUp() {
	cp.b.Remove()
}
