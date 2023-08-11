// Copyright (c) 2023, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package ocisif

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	imgspecv1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/opencontainers/runtime-spec/specs-go"
	ocisif "github.com/sylabs/oci-tools/pkg/sif"
	"github.com/sylabs/sif/v2/pkg/sif"
	"github.com/sylabs/singularity/v4/internal/pkg/runtime/engine/config/oci/generate"
	"github.com/sylabs/singularity/v4/internal/pkg/util/fs/squashfs"
	"github.com/sylabs/singularity/v4/pkg/ocibundle"
	"github.com/sylabs/singularity/v4/pkg/ocibundle/tools"
	"github.com/sylabs/singularity/v4/pkg/sylog"
)

// Bundle is a native OCI bundle, created from imageRef.
type Bundle struct {
	// imageRef is the reference to the OCI image source, e.g. oci-sif:alpine.sif
	imageRef string
	// imageSpec is the OCI image information, CMD, ENTRYPOINT, etc.
	imageSpec *imgspecv1.Image
	// bundlePath is the location where the OCI bundle will be created.
	bundlePath string
	// Has the image been mounted onto the bundle rootfs?
	imageMounted bool
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

// New returns a bundle interface to create/delete an OCI bundle from an oci-sif image ref.
func New(opts ...Option) (ocibundle.Bundle, error) {
	b := Bundle{
		imageRef: "",
	}

	for _, opt := range opts {
		if err := opt(&b); err != nil {
			return nil, fmt.Errorf("while initializing bundle: %w", err)
		}
	}

	return &b, nil
}

// Delete erases OCI bundle created an OCI image ref
func (b *Bundle) Delete(ctx context.Context) error {
	sylog.Debugf("Deleting oci-sif bundle at %s", b.bundlePath)

	if b.imageMounted {
		sylog.Debugf("Unmounting squashfs rootfs image from %q", tools.RootFs(b.bundlePath).Path())
		if err := squashfs.FUSEUnmount(ctx, tools.RootFs(b.bundlePath).Path()); err != nil {
			return err
		}
	}

	return tools.DeleteBundle(b.bundlePath)
}

// Create sets up the on-disk structure for an OCI runtime bundle, with rootfs
// mounted from the associated oci-sif image.
func (b *Bundle) Create(ctx context.Context, ociConfig *specs.Spec) error {
	imgFile, err := b.imageFile()
	if err != nil {
		return err
	}

	// Retrieve and check the index manifest, which lists the images in the oci-sif file.
	// We currently only support oci-sif files containing exactly 1 image.
	fi, err := sif.LoadContainerFromPath(imgFile, sif.OptLoadWithFlag(os.O_RDONLY))
	if err != nil {
		return fmt.Errorf("while loading SIF: %w", err)
	}
	ix, err := ocisif.ImageIndexFromFileImage(fi)
	if err != nil {
		return fmt.Errorf("while obtaining image index: %w", err)
	}
	idxManifest, err := ix.IndexManifest()
	if err != nil {
		return fmt.Errorf("while obtaining index manifest: %w", err)
	}
	if len(idxManifest.Manifests) != 1 {
		return fmt.Errorf("only single image oci-sif files are supported")
	}
	imageDigest := idxManifest.Manifests[0].Digest

	// Fetch the image manifest for the single image in the oci-sif.
	img, err := ix.Image(imageDigest)
	if err != nil {
		return fmt.Errorf("while initializing image: %w", err)
	}
	imageManifest, err := img.Manifest()
	if err != nil {
		return fmt.Errorf("while obtaining manifest: %s", err)
	}

	// Verify that the image has a single squashfs layer.
	numLayers := len(imageManifest.Layers)
	if numLayers > 1 {
		return fmt.Errorf("only single-layer oci-sif images are supported - this image has %d layers", numLayers)
	}
	rootfsLayer := imageManifest.Layers[0]
	// TODO - reference the mediatype as a const imported from somewhere?
	if rootfsLayer.MediaType != "application/vnd.sylabs.image.layer.v1.squashfs" {
		tools.DeleteBundle(b.bundlePath)
		return fmt.Errorf("unsupported layer mediaType %q", rootfsLayer.MediaType)
	}

	// The rest of Singularity's OCI handling uses the opencontainers packages, not go-containerregistry.
	// We need parse the image config into the opencontainers type to continue.
	rawConf, err := img.RawConfigFile()
	if err != nil {
		return fmt.Errorf("while retrieving image config: %w", err)
	}
	var imageSpec imgspecv1.Image
	if err := json.Unmarshal(rawConf, &imageSpec); err != nil {
		return fmt.Errorf("while parsing image spec: %w", err)
	}
	b.imageSpec = &imageSpec

	// Generate OCI bundle directory and config
	g, err := tools.GenerateBundleConfig(b.bundlePath, ociConfig)
	if err != nil {
		if errCleanup := b.Delete(ctx); errCleanup != nil {
			sylog.Errorf("While removing temporary bundle: %v", errCleanup)
		}
		return fmt.Errorf("failed to generate OCI bundle/config: %s", err)
	}

	// Initial image mount onto rootfs dir
	sylog.Debugf("Mounting squashfs rootfs from %q to %q", imgFile, tools.RootFs(b.bundlePath).Path())
	if err := mount(ctx, imgFile, tools.RootFs(b.bundlePath).Path(), rootfsLayer.Digest); err != nil {
		if errCleanup := b.Delete(ctx); errCleanup != nil {
			sylog.Errorf("While removing temporary bundle: %v", errCleanup)
		}
		return fmt.Errorf("while mounting squashfs layer: %w", err)
	}
	b.imageMounted = true

	return b.writeConfig(g)
}

// Update will update the OCI config for the OCI bundle, so that it is ready for execution.
func (b *Bundle) Update(_ context.Context, ociConfig *specs.Spec) error {
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

func (b *Bundle) imageFile() (path string, err error) {
	parts := strings.SplitN(b.imageRef, ":", 2)
	if len(parts) < 2 {
		return "", fmt.Errorf("could not parse image ref: %s", b.imageRef)
	}

	if parts[0] != "oci-sif" {
		return "", fmt.Errorf("only 'oci-sif' is supported, cannot parse %q references", parts[0])
	}

	return parts[1], nil
}

func mount(ctx context.Context, path, mountPath string, digest v1.Hash) error {
	f, err := sif.LoadContainerFromPath(path, sif.OptLoadWithFlag(os.O_RDONLY))
	if err != nil {
		return fmt.Errorf("failed to load image: %w", err)
	}
	defer func() { _ = f.UnloadContainer() }()

	d, err := f.GetDescriptor(sif.WithOCIBlobDigest(digest))
	if err != nil {
		return fmt.Errorf("failed to get partition descriptor: %w", err)
	}
	return squashfs.FUSEMount(ctx, uint64(d.Offset()), path, mountPath)
}
