// Copyright (c) 2022-2023, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package native

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/containers/image/v5/transports"
	"github.com/containers/image/v5/types"
	imgspecv1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/opencontainers/runtime-spec/specs-go"
	"github.com/sylabs/singularity/internal/pkg/build/oci"
	"github.com/sylabs/singularity/internal/pkg/cache"
	"github.com/sylabs/singularity/internal/pkg/runtime/engine/config/oci/generate"
	"github.com/sylabs/singularity/pkg/ocibundle"
	"github.com/sylabs/singularity/pkg/ocibundle/tools"
	"github.com/sylabs/singularity/pkg/sylog"
)

// Bundle is a native OCI bundle, created from imageRef.
type Bundle struct {
	// imageRef is the reference to the OCI image source, e.g. docker://ubuntu:latest.
	imageRef string
	// imageSpec is the OCI image information, CMD, ENTRYPOINT, etc.
	imageSpec *imgspecv1.Image
	// bundlePath is the location where the OCI bundle will be created.
	bundlePath string
	// sysCtx provides containers/image transport configuration (auth etc.)
	sysCtx *types.SystemContext
	// imgCache is a Singularity image cache, which OCI blobs are pulled through.
	// Note that we only use the 'blob' cache section. The 'oci-tmp' cache section holds
	// OCI->SIF conversions, which are not used here.
	imgCache *cache.Handle
	// process is the command to execute, which may override the image's ENTRYPOINT / CMD.
	// Generic bundle properties
	ocibundle.Bundle
}

type Option func(b *Bundle) error

// OptBundlePath sets the path that the bundle will be created at.
func OptBundlePath(bp string) Option {
	return func(b *Bundle) error {
		var err error
		b.bundlePath, err = filepath.Abs(bp)
		if err != nil {
			return fmt.Errorf("failed to determine bundle path: %s", err)
		}
		return nil
	}
}

// OptImageRef sets the image source reference, from which the bundle will be created.
func OptImageRef(ref string) Option {
	return func(b *Bundle) error {
		b.imageRef = ref
		return nil
	}
}

// OptSysCtx sets the OCI client SystemContext holding auth information etc.
func OptSysCtx(sc *types.SystemContext) Option {
	return func(b *Bundle) error {
		b.sysCtx = sc
		return nil
	}
}

// OptImgCache sets the Singularity image cache used to pull through OCI blobs.
func OptImgCache(ic *cache.Handle) Option {
	return func(b *Bundle) error {
		b.imgCache = ic
		return nil
	}
}

// New returns a bundle interface to create/delete an OCI bundle from an OCI image ref.
func New(opts ...Option) (ocibundle.Bundle, error) {
	b := Bundle{
		imageRef: "",
		sysCtx:   &types.SystemContext{},
		imgCache: nil,
	}

	for _, opt := range opts {
		if err := opt(&b); err != nil {
			return nil, fmt.Errorf("while initializing bundle: %w", err)
		}
	}

	return &b, nil
}

// Delete erases OCI bundle created an OCI image ref
func (b *Bundle) Delete() error {
	return tools.DeleteBundle(b.bundlePath)
}

// Create will created the on-disk structures for the OCI bundle, so that it is ready for execution.
func (b *Bundle) Create(ctx context.Context, ociConfig *specs.Spec) error {
	// generate OCI bundle directory and config
	g, err := tools.GenerateBundleConfig(b.bundlePath, ociConfig)
	if err != nil {
		return fmt.Errorf("failed to generate OCI bundle/config: %s", err)
	}

	// Get a reference to an OCI layout source for the image, fetching the image
	// through the cache if it is enabled.
	tmpDir, err := os.MkdirTemp("", "oci-tmp")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	layoutRef, err := oci.FetchLayout(ctx, b.sysCtx, b.imgCache, b.imageRef, tmpDir)
	if err != nil {
		return err
	}

	sylog.Debugf("Original imgref: %s, OCI layout: %s", b.imageRef, transports.ImageName(layoutRef))

	// Get the Image Manifest and ImageSpec
	img, err := layoutRef.NewImage(ctx, b.sysCtx)
	if err != nil {
		return err
	}
	defer img.Close()

	manifestData, mediaType, err := img.Manifest(ctx)
	if err != nil {
		return fmt.Errorf("error obtaining manifest source: %s", err)
	}
	if mediaType != imgspecv1.MediaTypeImageManifest {
		return fmt.Errorf("error verifying manifest media type: %s", mediaType)
	}
	var manifest imgspecv1.Manifest
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		return fmt.Errorf("error parsing manifest: %w", err)
	}

	b.imageSpec, err = img.OCIConfig(ctx)
	if err != nil {
		return err
	}

	// Extract from temp oci layout into bundle rootfs
	if err := oci.UnpackRootfs(ctx, tmpDir, manifest, tools.RootFs(b.bundlePath).Path()); err != nil {
		return err
	}
	return b.writeConfig(g)
}

// Update will update the OCI config for the OCI bundle, so that it is ready for execution.
func (b *Bundle) Update(ctx context.Context, ociConfig *specs.Spec) error {
	// generate OCI bundle directory and config
	g, err := tools.GenerateBundleConfig(b.bundlePath, ociConfig)
	if err != nil {
		return fmt.Errorf("failed to generate OCI bundle/config: %s", err)
	}
	return b.writeConfig(g)
}

// ImageSpec returns the OCI image spec associated with the bundle.
func (b *Bundle) ImageSpec() (imgSpec *imgspecv1.Image) {
	return b.imageSpec
}

// Path returns the bundle's path on disk.
func (b *Bundle) Path() string {
	return b.bundlePath
}

func (b *Bundle) writeConfig(g *generate.Generator) error {
	return tools.SaveBundleConfig(b.bundlePath, g)
}
