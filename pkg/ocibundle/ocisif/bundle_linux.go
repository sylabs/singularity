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
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/containers/image/v5/types"
	"github.com/opencontainers/go-digest"
	imgspecv1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/opencontainers/runtime-spec/specs-go"
	sifimage "github.com/sylabs/sif/v2/pkg/image"
	"github.com/sylabs/sif/v2/pkg/sif"
	"github.com/sylabs/singularity/internal/pkg/runtime/engine/config/oci/generate"
	"github.com/sylabs/singularity/pkg/ocibundle"
	"github.com/sylabs/singularity/pkg/ocibundle/tools"
	"github.com/sylabs/singularity/pkg/sylog"
)

// Bundle is a native OCI bundle, created from imageRef.
type Bundle struct {
	// imageRef is the reference to the OCI image source, e.g. oci-sif:alpine.sif
	imageRef string
	// imageSpec is the OCI image information, CMD, ENTRYPOINT, etc.
	imageSpec *imgspecv1.Image
	// bundlePath is the location where the OCI bundle will be created.
	bundlePath string
	// sysCtx provides containers/image transport configuration (auth etc.)
	sysCtx *types.SystemContext
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

// OptSysCtx sets the OCI client SystemContext holding auth information etc.
func OptSysCtx(sc *types.SystemContext) Option {
	return func(b *Bundle) error {
		b.sysCtx = sc
		return nil
	}
}

// New returns a bundle interface to create/delete an OCI bundle from an oci-sif image ref.
func New(opts ...Option) (ocibundle.Bundle, error) {
	b := Bundle{
		imageRef: "",
		sysCtx:   &types.SystemContext{},
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
	sylog.Debugf("Deleting oci-sif bundle at %s", b.bundlePath)

	if b.imageMounted {
		sylog.Debugf("Unmounting squashfs rootfs image")
		if err := unmountSquashFS(context.TODO(), tools.RootFs(b.bundlePath).Path()); err != nil {
			return err
		}
	}

	return tools.DeleteBundle(b.bundlePath)
}

// Create will created the on-disk structures for the OCI bundle, so that it is ready for execution.
func (b *Bundle) Create(ctx context.Context, ociConfig *specs.Spec) error {
	srcRef, err := b.getImageReference()
	if err != nil {
		return fmt.Errorf("while parsing image reference %q: %w", srcRef, err)
	}

	// Get the Image Manifest and ImageSpec
	img, err := srcRef.NewImage(ctx, b.sysCtx)
	if err != nil {
		return fmt.Errorf("while initializing image: %w", err)
	}
	defer img.Close()

	manifestData, mediaType, err := img.Manifest(ctx)
	if err != nil {
		return fmt.Errorf("while obtaininr manifest: %s", err)
	}
	if mediaType != imgspecv1.MediaTypeImageManifest {
		return fmt.Errorf("unsupported manifest media type: %s", mediaType)
	}
	var manifest imgspecv1.Manifest
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		return fmt.Errorf("while parsing manifest: %w", err)
	}

	numLayers := len(manifest.Layers)
	if numLayers > 1 {
		return fmt.Errorf("only single-layer oci-sif images are supported - this image has %d layers", numLayers)
	}

	rootfsLayer := manifest.Layers[0]

	if rootfsLayer.MediaType != "application/vnd.sylabs.image.layer.v1.squashfs" {
		tools.DeleteBundle(b.bundlePath)
		return fmt.Errorf("unsupported layer mediaType %q", rootfsLayer.MediaType)
	}

	b.imageSpec, err = img.OCIConfig(ctx)
	if err != nil {
		return fmt.Errorf("while retrieving image config: %w", err)
	}

	// generate OCI bundle directory and config
	g, err := tools.GenerateBundleConfig(b.bundlePath, ociConfig)
	if err != nil {
		if errCleanup := b.Delete(); errCleanup != nil {
			sylog.Errorf("While removing temporary bundle: %v", errCleanup)
		}
		return fmt.Errorf("failed to generate OCI bundle/config: %s", err)
	}

	// Initial image mount onto rootfs dir
	if err := mount(ctx, srcRef.StringWithinTransport(), tools.RootFs(b.bundlePath).Path(), rootfsLayer.Digest); err != nil {
		if errCleanup := b.Delete(); errCleanup != nil {
			sylog.Errorf("While removing temporary bundle: %v", errCleanup)
		}
		return fmt.Errorf("while mounting squashfs layer: %w", err)
	}
	b.imageMounted = true

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

func (b *Bundle) getImageReference() (srcRef types.ImageReference, err error) {

	parts := strings.SplitN(b.imageRef, ":", 2)
	if len(parts) < 2 {
		return nil, fmt.Errorf("could not parse image ref: %s", b.imageRef)
	}

	if parts[0] != "oci-sif" {
		return nil, fmt.Errorf("only 'oci-sif' is supported, cannot parse %q references", parts[0])
	}

	return sifimage.Transport.ParseReference(parts[1])
}

func withDigest(want digest.Digest) sif.DescriptorSelectorFunc {
	return func(d sif.Descriptor) (bool, error) {
		got, err := digest.Canonical.FromReader(d.GetReader())
		if err != nil {
			return false, err
		}

		return got == want, nil
	}
}

func mount(ctx context.Context, path, mountPath string, digest digest.Digest) error {
	f, err := sif.LoadContainerFromPath(path, sif.OptLoadWithFlag(os.O_RDONLY))
	if err != nil {
		return fmt.Errorf("failed to load image: %w", err)
	}
	defer func() { _ = f.UnloadContainer() }()

	d, err := f.GetDescriptor(withDigest(digest))
	if err != nil {
		return fmt.Errorf("failed to get partition descriptor: %w", err)
	}
	return mountSquashFS(ctx, d.Offset(), path, mountPath)
}

func mountSquashFS(ctx context.Context, offset int64, path, mountPath string) error {
	args := []string{
		"-o", fmt.Sprintf("ro,offset=%d,uid=%d,gid=%d", offset, os.Getuid(), os.Getgid()),
		filepath.Clean(path),
		filepath.Clean(mountPath),
	}
	//nolint:gosec // note (gosec exclusion) - we require callers to be able to specify squashfuse not on PATH
	cmd := exec.CommandContext(ctx, "squashfuse", args...)

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to mount: %w", err)
	}

	return nil
}

func unmountSquashFS(ctx context.Context, mountPath string) error {
	args := []string{
		"-u",
		filepath.Clean(mountPath),
	}
	cmd := exec.CommandContext(ctx, "fusermount", args...) //nolint:gosec

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to unmount: %w", err)
	}

	return nil
}
