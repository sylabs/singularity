// Copyright (c) 2018-2023, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

// Package ociimage provides functions related to retrieving and manipulating
// OCI images, used in pull/push and build operations.
package ociimage

import (
	"context"
	"fmt"
	"io"

	"github.com/containers/image/v5/copy"
	"github.com/containers/image/v5/oci/layout"
	"github.com/containers/image/v5/signature"
	"github.com/containers/image/v5/types"
	"github.com/opencontainers/go-digest"
	"github.com/sylabs/singularity/v4/internal/pkg/cache"
	"github.com/sylabs/singularity/v4/internal/pkg/ocitransport"
	"github.com/sylabs/singularity/v4/pkg/sylog"
)

// CachedImageReference wraps the containers/image ImageReference type, so that
// operations pull through a layout holding a cache of OCI blobs.
type CachedImageReference struct {
	source types.ImageReference
	types.ImageReference
}

// CacheReference converts an ImageReference into a CachedImageReference to cache its blobs.
func CacheReference(ctx context.Context, tOpts *ocitransport.TransportOptions, imgCache *cache.Handle, src types.ImageReference) (types.ImageReference, digest.Digest, error) {
	if imgCache == nil || imgCache.IsDisabled() {
		return nil, "", fmt.Errorf("undefined image cache")
	}
	digest, err := ImageDigest(ctx, tOpts, imgCache, src)
	if err != nil {
		return nil, "", err
	}

	cacheDir, err := imgCache.GetOciCacheDir(cache.OciBlobCacheType)
	if err != nil {
		return nil, "", err
	}
	c, err := layout.ParseReference(cacheDir + ":" + digest.String())
	if err != nil {
		return nil, "", err
	}

	return &CachedImageReference{
		source:         src,
		ImageReference: c,
	}, "", nil
}

// CachedReferenceFromURI parses a uri-like reference to an OCI image (e.g.
// docker://ubuntu) into it's transport:reference combination and then returns
// a CachedImageReference.
func CachedReferenceFromURI(ctx context.Context, tOpts *ocitransport.TransportOptions, imgCache *cache.Handle, uri string) (types.ImageReference, digest.Digest, error) {
	ref, err := ocitransport.ParseImageRef(uri)
	if err != nil {
		return nil, "", fmt.Errorf("unable to parse image name %v: %v", uri, err)
	}

	return CacheReference(ctx, tOpts, imgCache, ref)
}

// NewImageSource wraps the cache's oci-layout ref to first download the real source image to the cache
func (t *CachedImageReference) NewImageSource(ctx context.Context, sys *types.SystemContext) (types.ImageSource, error) {
	return t.newImageSource(ctx, sys, sylog.Writer())
}

func (t *CachedImageReference) newImageSource(ctx context.Context, sys *types.SystemContext, w io.Writer) (types.ImageSource, error) {
	policy := &signature.Policy{Default: []signature.PolicyRequirement{signature.NewPRInsecureAcceptAnything()}}
	policyCtx, err := signature.NewPolicyContext(policy)
	if err != nil {
		return nil, err
	}

	// Check if the image is in the cache layout already
	if _, err = layout.LoadManifestDescriptor(t.ImageReference); err == nil {
		return t.ImageReference.NewImageSource(ctx, sys)
	}

	// Otherwise, we are copying into the cache layout first
	_, err = copy.Image(ctx, policyCtx, t.ImageReference, t.source, &copy.Options{
		ReportWriter: w,
		SourceCtx:    sys,
	})
	if err != nil {
		return nil, err
	}
	return t.ImageReference.NewImageSource(ctx, sys)
}

// NewImage wraps the cache's oci-layout ref to first download the real source image to the cache
func (t *CachedImageReference) NewImage(ctx context.Context, sys *types.SystemContext) (types.ImageCloser, error) {
	return t.newImage(ctx, sys, sylog.Writer())
}

func (t *CachedImageReference) newImage(ctx context.Context, sys *types.SystemContext, w io.Writer) (types.ImageCloser, error) {
	policy := &signature.Policy{Default: []signature.PolicyRequirement{signature.NewPRInsecureAcceptAnything()}}
	policyCtx, err := signature.NewPolicyContext(policy)
	if err != nil {
		return nil, err
	}

	// Check if the image is in the cache layout already
	if _, err = layout.LoadManifestDescriptor(t.ImageReference); err == nil {
		return t.ImageReference.NewImage(ctx, sys)
	}

	// Otherwise, we are copying into the cache layout first
	_, err = copy.Image(ctx, policyCtx, t.ImageReference, t.source, &copy.Options{
		ReportWriter: w,
		SourceCtx:    sys,
	})
	if err != nil {
		return nil, err
	}
	return t.ImageReference.NewImage(ctx, sys)
}
