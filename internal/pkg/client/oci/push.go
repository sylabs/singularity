// Copyright (c) 2023, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package oci

import (
	"context"
	"fmt"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/sylabs/singularity/v4/internal/pkg/client/ocisif"
	"github.com/sylabs/singularity/v4/pkg/image"
)

// PushOptions provides options/configuration that determine the behavior of a
// push to an OCI registry.
type PushOptions struct {
	// Auth provides optional explicit credentials for OCI registry authentication.
	Auth *authn.AuthConfig
	// AuthFile provides a path to a file containing OCI registry credentials.
	AuthFile string
	// LayerFormat sets an explicit layer format to use when pushing an OCI
	// image. Either 'squashfs' or 'tar'. If unset, layers are pushed as
	// squashfs.
	LayerFormat string
	// TmpDir is a temporary directory to be used for an temporary files created
	// during the push.
	TmpDir string
}

// Push pushes an image into an OCI registry, as an OCI image (not an ORAS artifact).
// At present, only OCI-SIF images can be pushed in this manner.
func Push(ctx context.Context, sourceFile string, destRef string, opts PushOptions) error {
	img, err := image.Init(sourceFile, false)
	if err != nil {
		return err
	}
	defer img.File.Close()

	switch img.Type {
	case image.OCISIF:
		ocisifOpts := ocisif.PushOptions{
			Auth:        opts.Auth,
			AuthFile:    opts.AuthFile,
			LayerFormat: opts.LayerFormat,
			TmpDir:      opts.TmpDir,
		}
		return ocisif.PushOCISIF(ctx, sourceFile, destRef, ocisifOpts)
	case image.SIF:
		return fmt.Errorf("non OCI SIF images can only be pushed to OCI registries via oras://")
	}

	return fmt.Errorf("push only supports SIF images")
}
