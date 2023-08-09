// Copyright (c) 2018-2023, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package ociimage

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"

	"github.com/containers/image/v5/docker"
	"github.com/containers/image/v5/types"
	ggcrv1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/opencontainers/go-digest"
	"github.com/sylabs/singularity/v4/internal/pkg/cache"
	"github.com/sylabs/singularity/v4/pkg/sylog"
)

// ImageDigest obtains the digest of the image manifest for an ImageReference.
// If the ImageReference points at a multi-arch repository with an image index
// (manifest list), it will traverse this to retrieve the digest of the image
// manifest for the requested architecture specified in sysCtx.
func ImageDigest(ctx context.Context, sysCtx *types.SystemContext, imgCache *cache.Handle, ref types.ImageReference) (digest.Digest, error) {
	// For OCI registries (docker://) attempt to use HEAD operation and cached
	// image manifest/image index to avoid hitting GET API limits.
	if ref.Transport().Name() == "docker" {
		return dockerDigest(ctx, sysCtx, imgCache, ref)
	}
	return directDigest(ctx, sysCtx, imgCache, ref)
}

// directDigest obtains the image manifest digest for an ImageReference, by
// retrieving the manifest from the OCI source. If the ImageReference points at
// a multi-arch repository with an image index (manifest list), it will traverse
// this to retrieve the digest of the image manifest for the requested
// architecture specified in sysCtx.
func directDigest(ctx context.Context, sysCtx *types.SystemContext, imgCache *cache.Handle, ref types.ImageReference) (digest.Digest, error) {
	source, err := ref.NewImageSource(ctx, sysCtx)
	if err != nil {
		return "", err
	}
	defer func() {
		if closeErr := source.Close(); closeErr != nil {
			err = fmt.Errorf("%w (src: %v)", err, closeErr)
		}
	}()

	mf, _, err := source.GetManifest(ctx, nil)
	if err != nil {
		return "", err
	}

	digest, err := digestFromManifestOrIndex(sysCtx, mf)
	if err != nil {
		return "", err
	}

	if imgCache != nil && !imgCache.IsDisabled() {
		sylog.Debugf("Caching image index or manifest %s", digest.String())
		err := imgCache.PutOciCacheBlob(cache.OciBlobCacheType, digest, io.NopCloser(bytes.NewBuffer(mf)))
		if err != nil {
			sylog.Errorf("While caching image index or manifest: %v", err)
		}
	}

	return digest, nil
}

// dockerDigest obtains the image manifest digest for a registry (docker://)
// image source, attempting to use a HEAD against the registry, and cached image
// index / manifest, to avoid unnecessary GET operations that count against
// Docker Hub API limits.
func dockerDigest(ctx context.Context, sysCtx *types.SystemContext, imgCache *cache.Handle, ref types.ImageReference) (digest.Digest, error) {
	if imgCache == nil || imgCache.IsDisabled() {
		return directDigest(ctx, sysCtx, imgCache, ref)
	}

	d, err := docker.GetDigest(ctx, sysCtx, ref)
	if err != nil {
		// Not all registries send digest in HEAD. Fall back to digest from retrieved manifest.
		sylog.Debugf("Couldn't get digest from HEAD against registry: %v", err)
		return directDigest(ctx, sysCtx, imgCache, ref)
	}
	sylog.Debugf("%s has digest %s via HEAD", ref.DockerReference().String(), d.String())

	// Is the corresponding blob present in the cache?
	r, err := imgCache.GetOciCacheBlob(cache.OciBlobCacheType, d)
	if err != nil {
		if !os.IsNotExist(err) {
			sylog.Warningf("While opening cached image index or manifest: %v", err)
		}
		sylog.Debugf("No cached image index or manifest")
		return directDigest(ctx, sysCtx, imgCache, ref)
	}
	defer r.Close()
	sylog.Debugf("Found cached image index or manifest for %s", d)

	mf, err := io.ReadAll(r)
	if err != nil {
		return "", fmt.Errorf("while reading cached image index or manifest: %w", err)
	}
	return digestFromManifestOrIndex(sysCtx, mf)
}

// digestFromManifestOrIndex returns the digest of the provided manifest, or the
// digest of the manifest of an image satisfying sysCtx platform requirements if
// an image index is supplied.
func digestFromManifestOrIndex(sysCtx *types.SystemContext, manifestOrIndex []byte) (digest.Digest, error) {
	if sysCtx == nil {
		return "", fmt.Errorf("internal error: nil sysCtx")
	}

	// mediaType is only a SHOULD for manifests and image indexes,so we can't
	// rely on it to distinguish betweeen a manifest and image index via ggcr
	// mediaType.IsIndex()/IsImage()
	//
	// Check for an image manifest first, where a Config.Digest is REQUIRED.
	// This would not be present in an image index.
	mf, err := ggcrv1.ParseManifest(bytes.NewBuffer(manifestOrIndex))
	if err == nil && mf.Config.Digest.Hex != "" {
		sylog.Debugf("Content is an image manifest, returning digest.")
		return digest.FromBytes(manifestOrIndex), nil
	}

	// If we don't have a manifest, try to parse as an image index, and check for at least one manifest.
	ix, err := ggcrv1.ParseIndexManifest(bytes.NewBuffer(manifestOrIndex))
	if err != nil {
		return "", fmt.Errorf("error parsing IndexManifest: %w", err)
	}
	if len(ix.Manifests) == 0 {
		return "", fmt.Errorf("not a valid image manifest or image index")
	}

	requiredPlatform := sysCtxToPlatform(sysCtx)
	sylog.Debugf("Content is an image index, finding image for %s", requiredPlatform)
	for _, mf := range ix.Manifests {
		if mf.Platform == nil {
			continue
		}
		if mf.Platform.Satisfies(requiredPlatform) {
			sylog.Debugf("%s (%s) satisfies %s", mf.Digest.String(), mf.Platform.String(), requiredPlatform.String())
			return digest.Digest(mf.Digest.String()), nil
		}
	}
	return "", fmt.Errorf("no image satisfies requested platform: %s", requiredPlatform.String())
}
