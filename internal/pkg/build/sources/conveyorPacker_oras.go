// Copyright (c) 2020-2024, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package sources

import (
	"context"
	"fmt"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/sylabs/singularity/v4/internal/pkg/client/oras"
	"github.com/sylabs/singularity/v4/pkg/build/types"
	"github.com/sylabs/singularity/v4/pkg/sylog"
)

// OrasConveyorPacker only needs to hold a packer to pack the image it pulls
// as well as extra information about the library it's pulling from.
type OrasConveyorPacker struct {
	LocalPacker
}

// Get downloads container from Singularityhub
func (cp *OrasConveyorPacker) Get(ctx context.Context, b *types.Bundle) (err error) {
	sylog.Debugf("Getting container from registry using ORAS")

	// uri with leading // for oras handlers to consume
	ref := "//" + b.Recipe.Header["from"]
	// full uri for name determination and output
	fullRef := "oras:" + ref

	authConfig := b.Opts.OCIAuthConfig
	if authConfig == nil && b.Opts.DockerAuthConfig != nil {
		authConfig = &authn.AuthConfig{
			Username:      b.Opts.DockerAuthConfig.Username,
			Password:      b.Opts.DockerAuthConfig.Password,
			IdentityToken: b.Opts.DockerAuthConfig.IdentityToken,
		}
	}

	imagePath, err := oras.Pull(ctx, b.Opts.ImgCache, fullRef, b.Opts.TmpDir, authConfig, b.Opts.DockerAuthFile)
	if err != nil {
		return fmt.Errorf("while fetching library image: %v", err)
	}

	// insert base metadata before unpacking fs
	if err = makeBaseEnv(b.RootfsPath, true); err != nil {
		return fmt.Errorf("while inserting base environment: %v", err)
	}

	cp.LocalPacker, err = GetLocalPacker(ctx, imagePath, b)
	return err
}
