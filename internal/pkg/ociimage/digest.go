// Copyright (c) 2018-2024, Sylabs Inc. All rights reserved.
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
	"strings"

	"github.com/google/go-containerregistry/pkg/name"
	ggcrv1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/sylabs/singularity/v4/internal/pkg/cache"
	"github.com/sylabs/singularity/v4/internal/pkg/remote/credential/ociauth"
	"github.com/sylabs/singularity/v4/pkg/sylog"
)

// ImageDigest obtains the digest of the image manifest for a uri-like image
// reference. If the reference points to a multi-arch repository with an image
// index (manifest list), it will traverse this to retrieve the digest of the
// image manifest for the requested architecture specified in tOpts.
func ImageDigest(ctx context.Context, tOpts *TransportOptions, imgCache *cache.Handle, uri string) (ggcrv1.Hash, error) {
	// oci-archive - Perform a tar extraction first, and handle as an oci layout.
	if strings.HasPrefix(uri, "oci-archive:") {
		layoutURI, cleanup, err := extractOCIArchive(uri, tOpts.TmpDir)
		if err != nil {
			return ggcrv1.Hash{}, err
		}
		defer cleanup()
		uri = layoutURI
	}

	srcType, srcRef, err := URItoSourceSinkRef(uri)
	if err != nil {
		return ggcrv1.Hash{}, err
	}

	// For OCI registries (docker://) attempt to use HEAD operation and cached
	// image manifest/image index to avoid hitting GET API limits.
	if srcType == RegistrySourceSink && imgCache != nil && !imgCache.IsDisabled() {
		return cachedRegistryDigest(ctx, tOpts, *imgCache, srcRef)
	}
	return directDigest(ctx, tOpts, srcType, srcRef)
}

// directDigest obtains the image manifest digest for srcRef, by retrieving the
// manifest from the OCI source. If the srcRef points at a multi-arch repository
// with an image index (manifest list), it will traverse this to retrieve the
// digest of the image manifest for the requested architecture specified in
// tOpts.
func directDigest(ctx context.Context, tOpts *TransportOptions, srcType SourceSink, srcRef string) (ggcrv1.Hash, error) {
	img, err := srcType.Image(ctx, srcRef, tOpts, nil)
	if err != nil {
		return ggcrv1.Hash{}, err
	}
	return img.Digest()
}

// cachedRegistryDigest obtains the image manifest digest for a registry
// (docker://) image source, attempting to use a HEAD against the registry and a
// locally cached image index / manifest, to avoid unnecessary GET operations
// that count against Docker Hub API limits.
func cachedRegistryDigest(ctx context.Context, tOpts *TransportOptions, imgCache cache.Handle, srcRef string) (ggcrv1.Hash, error) {
	var nameOpts []name.Option
	if tOpts != nil && tOpts.Insecure {
		nameOpts = append(nameOpts, name.Insecure)
	}
	remoteRef, err := name.ParseReference(srcRef, nameOpts...)
	if err != nil {
		return ggcrv1.Hash{}, err
	}
	remoteOpts := []remote.Option{
		remote.WithContext(ctx),
	}
	if tOpts != nil {
		remoteOpts = append(remoteOpts,
			ociauth.AuthOptn(tOpts.AuthConfig, tOpts.AuthFilePath))
	}

	// remote.HEAD will return a descriptor with the digest indicated by the Docker-Content-Digest header.
	headDesc, err := remote.Head(remoteRef, remoteOpts...)
	if err != nil {
		return registryDigestFallback(ctx, tOpts, srcRef, err)
	}
	sylog.Debugf("HEAD returned digest %v, mediaType %v", headDesc.Digest, headDesc.MediaType)

	// Is the corresponding blob present in the cache?
	r, err := imgCache.GetOciCacheBlob(cache.OciBlobCacheType, headDesc.Digest)
	// Present in cache but couldn't open - warn and fall back.
	if err != nil && !os.IsNotExist(err) {
		return registryDigestFallback(ctx, tOpts, srcRef, err)
	}
	// Present in cache, and opened - read and obtain true image digest.
	if err == nil {
		sylog.Debugf("Found cached image index or manifest for %s", headDesc.Digest)
		defer r.Close()
		mf, err := io.ReadAll(r)
		if err != nil {
			return registryDigestFallback(ctx, tOpts, srcRef, err)
		}
		return digestFromManifestOrIndex(tOpts, mf)
	}
	// Not in cache - GET the index or manifest from the remote, and cache it.
	sylog.Debugf("No cached image index or manifest")
	getDesc, err := remote.Get(remoteRef, remoteOpts...)
	if err != nil {
		return registryDigestFallback(ctx, tOpts, srcRef, err)
	}
	if getDesc.Digest != headDesc.Digest {
		return registryDigestFallback(ctx, tOpts, srcRef, fmt.Errorf("digest changed between HEAD and GET requests: %v != %v", getDesc.Digest, headDesc.Digest))
	}

	sylog.Debugf("Caching image index or manifest %s", getDesc.Digest)
	if err := imgCache.PutOciCacheBlob(cache.OciBlobCacheType, getDesc.Digest, io.NopCloser(bytes.NewBuffer(getDesc.Manifest))); err != nil {
		return registryDigestFallback(ctx, tOpts, srcRef, err)
	}
	return digestFromManifestOrIndex(tOpts, getDesc.Manifest)
}

func registryDigestFallback(ctx context.Context, tOpts *TransportOptions, srcRef string, cause error) (ggcrv1.Hash, error) {
	sylog.Warningf("Couldn't use cached digest for registry: %v", cause)
	sylog.Warningf("Falling back to direct digest.")
	return directDigest(ctx, tOpts, RegistrySourceSink, srcRef)
}

// digestFromManifestOrIndex returns the digest of the provided manifest, or the
// digest of the manifest of an image satisfying sysCtx platform requirements if
// an image index is supplied.
func digestFromManifestOrIndex(tOpts *TransportOptions, manifestOrIndex []byte) (ggcrv1.Hash, error) {
	if tOpts == nil {
		return ggcrv1.Hash{}, fmt.Errorf("internal error: nil TransportOptions")
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
		digest, _, err := ggcrv1.SHA256(bytes.NewBuffer(manifestOrIndex))
		return digest, err
	}

	// If we don't have a manifest, try to parse as an image index, and check for at least one manifest.
	ix, err := ggcrv1.ParseIndexManifest(bytes.NewBuffer(manifestOrIndex))
	if err != nil {
		return ggcrv1.Hash{}, fmt.Errorf("error parsing IndexManifest: %w", err)
	}
	if len(ix.Manifests) == 0 {
		return ggcrv1.Hash{}, fmt.Errorf("not a valid image manifest or image index")
	}

	requiredPlatform := tOpts.Platform
	sylog.Debugf("Content is an image index, finding image for %s", requiredPlatform)
	for _, mf := range ix.Manifests {
		if mf.Platform == nil {
			continue
		}
		if mf.Platform.Satisfies(requiredPlatform) {
			sylog.Debugf("%s (%s) satisfies %s", mf.Digest.String(), mf.Platform.String(), requiredPlatform.String())
			return mf.Digest, nil
		}
	}
	return ggcrv1.Hash{}, fmt.Errorf("no image satisfies requested platform: %s", requiredPlatform.String())
}
