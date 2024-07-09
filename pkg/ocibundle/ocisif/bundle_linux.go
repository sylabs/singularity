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
	"strconv"
	"strings"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	imgspecv1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/opencontainers/runtime-spec/specs-go"
	ocitsif "github.com/sylabs/oci-tools/pkg/sif"
	"github.com/sylabs/sif/v2/pkg/sif"
	"github.com/sylabs/singularity/v4/internal/pkg/ocisif"
	"github.com/sylabs/singularity/v4/internal/pkg/runtime/engine/config/oci/generate"
	"github.com/sylabs/singularity/v4/internal/pkg/util/fs/overlay"
	"github.com/sylabs/singularity/v4/internal/pkg/util/fs/squashfs"
	"github.com/sylabs/singularity/v4/pkg/ocibundle"
	"github.com/sylabs/singularity/v4/pkg/ocibundle/tools"
	"github.com/sylabs/singularity/v4/pkg/sylog"
)

// UnavailableError is used to wrap an Underlying error, while indicating that
// it is not currently possible to setup an OCI bundle using direct mount(s)
// from an OCI-SIF. This is intended to permit fall-back paths in the caller
// when squashfuse is unavailable / failing.
//
// TODO - replace with native Go error wrapping at major version increment.
type UnavailableError struct {
	Underlying error
}

func (e UnavailableError) Error() string {
	return e.Underlying.Error()
}

// Bundle is a native OCI bundle, created from imageRef.
type Bundle struct {
	// imageRef is the reference to the OCI image source, e.g. oci-sif:alpine.sif
	imageRef string
	// imageSpec is the OCI image information, CMD, ENTRYPOINT, etc.
	imageSpec *imgspecv1.Image
	// bundlePath is the location where the OCI bundle will be created.
	bundlePath string
	// paths to squashfs layers that have been mounted
	mountedLayers []string
	// assembled rootfs, from overlay mount of mountedLayers
	rootfsOverlaySet overlay.Set
	// Has the image been mounted onto the bundle rootfs?
	rootfsMounted bool
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

	if b.rootfsMounted {
		rootfsPath := tools.RootFs(b.bundlePath).Path()
		sylog.Debugf("Unmounting rootfs overlay from %q", rootfsPath)
		if err := b.rootfsOverlaySet.Unmount(ctx, rootfsPath); err != nil {
			return err
		}
	}

	for _, layerPath := range b.mountedLayers {
		sylog.Debugf("Unmounting layer fs from %q", layerPath)
		if err := squashfs.FUSEUnmount(ctx, layerPath); err != nil {
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
	ix, err := ocitsif.ImageIndexFromFileImage(fi)
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

	img, err := ix.Image(imageDigest)
	if err != nil {
		return fmt.Errorf("while initializing image: %w", err)
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

	// Mount layers from image
	sylog.Debugf("Mounting squashfs layers from %q to %q", imgFile, b.bundlePath)
	if err := b.mountLayers(ctx, img, imgFile); err != nil {
		if errCleanup := b.Delete(ctx); errCleanup != nil {
			sylog.Errorf("While removing temporary bundle: %v", errCleanup)
		}
		return UnavailableError{Underlying: fmt.Errorf("while mounting squashfs layers: %w", err)}
	}

	// Assemble rootfs overlay mount
	sylog.Debugf("Mounting rootfs overlay to %q", tools.RootFs(b.bundlePath).Path())
	if err := b.mountRootfs(ctx); err != nil {
		if errCleanup := b.Delete(ctx); errCleanup != nil {
			sylog.Errorf("While removing temporary bundle: %v", errCleanup)
		}
		return fmt.Errorf("while mounting rootfs overlay: %w", err)
	}

	return b.writeConfig(g)
}

func (b *Bundle) mountLayers(ctx context.Context, img v1.Image, imgFile string) error {
	layers, err := img.Layers()
	if err != nil {
		return fmt.Errorf("while obtaining layers: %s", err)
	}

	for i, l := range layers {
		mt, err := l.MediaType()
		if err != nil {
			return fmt.Errorf("while checking layer: %w", err)
		}
		// An ext3 final layer is an overlay, and handled separately from the rootfs assembly.
		if mt == ocisif.Ext3LayerMediaType && i == len(layers)-1 {
			continue
		}
		if mt != ocisif.SquashfsLayerMediaType {
			return fmt.Errorf("unsupported layer mediaType %q", mt)
		}

		offset, err := l.(*ocitsif.Layer).Offset()
		if err != nil {
			return fmt.Errorf("while finding layer offset: %w", err)
		}

		layerPath := filepath.Join(tools.Layers(b.bundlePath).Path(), strconv.Itoa(i))
		sylog.Debugf("Mounting layer %d fs from %q to %q", i, imgFile, layerPath)
		if err := os.Mkdir(layerPath, 0o755); err != nil {
			return fmt.Errorf("while creating layer directory: %w", err)
		}

		if _, err := squashfs.FUSEMount(ctx, uint64(offset), imgFile, layerPath, false); err != nil {
			return UnavailableError{Underlying: fmt.Errorf("while mounting squashfs layer: %w", err)}
		}
		b.mountedLayers = append(b.mountedLayers, layerPath)
	}
	return nil
}

func (b *Bundle) mountRootfs(ctx context.Context) error {
	for i := len(b.mountedLayers) - 1; i >= 0; i-- {
		item, err := overlay.NewItemFromString(b.mountedLayers[i])
		if err != nil {
			return err
		}
		item.Readonly = true
		item.SetParentDir(b.bundlePath)
		b.rootfsOverlaySet.ReadonlyOverlays = append(b.rootfsOverlaySet.ReadonlyOverlays, item)
	}

	rootFsDir := tools.RootFs(b.bundlePath).Path()
	if err := b.rootfsOverlaySet.Mount(ctx, rootFsDir); err != nil {
		return fmt.Errorf("while mounting rootfs overlay: %w", err)
	}
	b.rootfsMounted = true
	return nil
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
		sylog.Debugf("Image ref %q lacks transport prefix; assuming OCI-SIF.", b.imageRef)
		return parts[0], nil
	}

	if parts[0] != "oci-sif" {
		return "", fmt.Errorf("only 'oci-sif' is supported, cannot parse %q references", parts[0])
	}

	return parts[1], nil
}
