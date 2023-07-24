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
	"syscall"

	"github.com/containers/image/v5/transports"
	"github.com/containers/image/v5/types"
	imgspecv1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/opencontainers/runtime-spec/specs-go"
	"github.com/sylabs/singularity/internal/pkg/cache"
	"github.com/sylabs/singularity/internal/pkg/ociimage"
	"github.com/sylabs/singularity/internal/pkg/runtime/engine/config/oci/generate"
	"github.com/sylabs/singularity/internal/pkg/util/fs"
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
	// tmpDir is the location for any temporary files that will be created outside of the
	// assembled runtime bundle directory.
	tmpDir string
	// rootfsParentDir is the parent directory of the pristine rootfs, that will be mounted into the bundle.
	// It is created and mounted to the bundlePath/rootfs during Create() and umounted and removed during Delete().
	rootfsParentDir string
	// rootfsMounted is set to true when the pristine rootfs has been bind mounted into the bundle.
	rootfsMounted bool
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

// OptTempDir sets the parent temporary directory for temporary files generated
// outside of the assembled bundle.
func OptTmpDir(tmpDir string) Option {
	return func(b *Bundle) error {
		b.tmpDir = tmpDir
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
func (b *Bundle) Delete(ctx context.Context) error {
	if b.rootfsMounted {
		sylog.Debugf("Unmounting bundle rootfs %q", tools.RootFs(b.bundlePath).Path())
		if err := syscall.Unmount(tools.RootFs(b.bundlePath).Path(), syscall.MNT_DETACH); err != nil {
			return fmt.Errorf("while unmounting bundle rootfs bind mount: %w", err)
		}
	}

	if b.rootfsParentDir != "" {
		sylog.Debugf("Removing pristine rootfs %q", b.rootfsParentDir)
		if err := fs.ForceRemoveAll(b.rootfsParentDir); err != nil {
			return fmt.Errorf("while removing pristine rootfs %q: %w", b.rootfsParentDir, err)
		}
	}

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
	tmpLayout, err := os.MkdirTemp(b.tmpDir, "oci-tmp-layout")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpLayout)

	layoutRef, err := ociimage.FetchLayout(ctx, b.sysCtx, b.imgCache, b.imageRef, tmpLayout)
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

	// Extract from temp oci layout into a temporary pristine rootfs dir, outside of the bundle.
	// The rootfs must be nested inside a parent directory, so extracting it does not
	// open the tmpdir permissions.
	b.rootfsParentDir, err = os.MkdirTemp(b.tmpDir, "oci-tmp-rootfs")
	if err != nil {
		return err
	}
	pristineRootfs := filepath.Join(b.rootfsParentDir, "rootfs")

	if err := ociimage.UnpackRootfs(ctx, tmpLayout, manifest, pristineRootfs); err != nil {
		return err
	}

	bundleRootfs := tools.RootFs(b.bundlePath).Path()
	if err := os.Mkdir(bundleRootfs, 0o755); err != nil && !os.IsExist(err) {
		return err
	}

	sylog.Debugf("Performing bind mount of pristine rootfs %q to bundle %q", pristineRootfs, bundleRootfs)
	if err = syscall.Mount(pristineRootfs, bundleRootfs, "", syscall.MS_BIND, ""); err != nil {
		return fmt.Errorf("failed to bind pristine rootfs to %q: %w", bundleRootfs, err)
	}

	defer func() {
		if err != nil {
			sylog.Debugf("Encountered error with rootfs bind mount; attempting to unmount %q", bundleRootfs)
			syscall.Unmount(bundleRootfs, syscall.MNT_DETACH)
		}
	}()

	sylog.Debugf("Performing remount of bundle rootfs %q", bundleRootfs)
	if err = syscall.Mount("", bundleRootfs, "", syscall.MS_REMOUNT|syscall.MS_BIND|syscall.MS_RDONLY|syscall.MS_NODEV|syscall.MS_NOSUID, ""); err != nil {
		return fmt.Errorf("failed to remount bundle rootfs %q: %w", bundleRootfs, err)
	}

	b.rootfsMounted = true

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
