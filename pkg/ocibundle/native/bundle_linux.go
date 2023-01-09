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
	"strings"

	apexlog "github.com/apex/log"
	"github.com/containers/image/v5/copy"
	"github.com/containers/image/v5/docker"
	dockerarchive "github.com/containers/image/v5/docker/archive"
	dockerdaemon "github.com/containers/image/v5/docker/daemon"
	ociarchive "github.com/containers/image/v5/oci/archive"
	ocilayout "github.com/containers/image/v5/oci/layout"
	"github.com/containers/image/v5/signature"
	"github.com/containers/image/v5/transports"
	"github.com/containers/image/v5/types"
	imgspecv1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/opencontainers/runtime-spec/specs-go"
	"github.com/opencontainers/umoci"
	umocilayer "github.com/opencontainers/umoci/oci/layer"
	"github.com/opencontainers/umoci/pkg/idtools"
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

	// Get a reference to an OCI layout source for the image. If the cache is
	// enabled, we pull through the blob cache layout, otherwise there will be a
	// temp dir and image Copy to it.
	layoutRef, layoutDir, cleanup, err := b.fetchLayout(ctx)
	if err != nil {
		return err
	}
	if cleanup != nil {
		defer cleanup()
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
	if err := b.extractImage(ctx, layoutDir, manifest); err != nil {
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

func (b *Bundle) fetchLayout(ctx context.Context) (layoutRef types.ImageReference, layoutDir string, cleanup func(), err error) {
	if b.sysCtx == nil {
		return nil, "", nil, fmt.Errorf("sysctx must be provided")
	}

	policy := &signature.Policy{Default: []signature.PolicyRequirement{signature.NewPRInsecureAcceptAnything()}}
	policyCtx, err := signature.NewPolicyContext(policy)
	if err != nil {
		return nil, "", nil, err
	}

	parts := strings.SplitN(b.imageRef, ":", 2)
	if len(parts) < 2 {
		return nil, "", nil, fmt.Errorf("could not parse image ref: %s", b.imageRef)
	}
	var srcRef types.ImageReference

	switch parts[0] {
	case "docker":
		srcRef, err = docker.ParseReference(parts[1])
	case "docker-archive":
		srcRef, err = dockerarchive.ParseReference(parts[1])
	case "docker-daemon":
		srcRef, err = dockerdaemon.ParseReference(parts[1])
	case "oci":
		srcRef, err = ocilayout.ParseReference(parts[1])
	case "oci-archive":
		srcRef, err = ociarchive.ParseReference(parts[1])
	default:
		return nil, "", nil, fmt.Errorf("cannot create an OCI container from %s source", parts[0])
	}

	if err != nil {
		return nil, "", nil, fmt.Errorf("invalid image source: %w", err)
	}

	// If the cache is enabled, then we transparently pull through an oci-layout in the cache.
	if b.imgCache != nil && !b.imgCache.IsDisabled() {
		// Grab the modified source ref from the cache
		srcRef, err = oci.ConvertReference(ctx, b.imgCache, srcRef, b.sysCtx)
		if err != nil {
			return nil, "", nil, err
		}
		layoutDir, err := b.imgCache.GetOciCacheDir(cache.OciBlobCacheType)
		if err != nil {
			return nil, "", nil, err
		}
		return srcRef, layoutDir, nil, nil
	}

	// Otherwise we have to stage things in a temporary oci layout.
	tmpDir, err := os.MkdirTemp("", "oci-tmp")
	if err != nil {
		return nil, "", nil, err
	}
	cleanup = func() {
		os.RemoveAll(tmpDir)
	}

	tmpfsRef, err := ocilayout.ParseReference(tmpDir + ":" + "tmp")
	if err != nil {
		cleanup()
		return nil, "", nil, err
	}

	_, err = copy.Image(ctx, policyCtx, tmpfsRef, srcRef, &copy.Options{
		ReportWriter: sylog.Writer(),
		SourceCtx:    b.sysCtx,
	})
	if err != nil {
		cleanup()
		return nil, "", nil, err
	}

	return tmpfsRef, tmpDir, cleanup, nil
}

func (b *Bundle) extractImage(ctx context.Context, layoutDir string, manifest imgspecv1.Manifest) error {
	var mapOptions umocilayer.MapOptions

	loggerLevel := sylog.GetLevel()
	// set the apex log level, for umoci
	if loggerLevel <= int(sylog.ErrorLevel) {
		// silent option
		apexlog.SetLevel(apexlog.ErrorLevel)
	} else if loggerLevel <= int(sylog.LogLevel) {
		// quiet option
		apexlog.SetLevel(apexlog.WarnLevel)
	} else if loggerLevel < int(sylog.DebugLevel) {
		// verbose option(s) or default
		apexlog.SetLevel(apexlog.InfoLevel)
	} else {
		// debug option
		apexlog.SetLevel(apexlog.DebugLevel)
	}

	// Allow unpacking as non-root
	if os.Geteuid() != 0 {
		mapOptions.Rootless = true

		uidMap, err := idtools.ParseMapping(fmt.Sprintf("0:%d:1", os.Geteuid()))
		if err != nil {
			return fmt.Errorf("error parsing uidmap: %s", err)
		}
		mapOptions.UIDMappings = append(mapOptions.UIDMappings, uidMap)

		gidMap, err := idtools.ParseMapping(fmt.Sprintf("0:%d:1", os.Getegid()))
		if err != nil {
			return fmt.Errorf("error parsing gidmap: %s", err)
		}
		mapOptions.GIDMappings = append(mapOptions.GIDMappings, gidMap)
	}

	sylog.Debugf("Extracting manifest %s, from layout %s, to %s", manifest.Config.Digest, layoutDir, b.bundlePath)

	engineExt, err := umoci.OpenLayout(layoutDir)
	if err != nil {
		return fmt.Errorf("error opening layout: %s", err)
	}

	// UnpackRootfs from umoci v0.4.2 expects a path to a non-existing directory
	os.RemoveAll(tools.RootFs(b.bundlePath).Path())

	// Unpack root filesystem
	unpackOptions := umocilayer.UnpackOptions{MapOptions: mapOptions}
	err = umocilayer.UnpackRootfs(ctx, engineExt, tools.RootFs(b.bundlePath).Path(), manifest, &unpackOptions)
	if err != nil {
		return fmt.Errorf("error unpacking rootfs: %s", err)
	}

	return nil
}
