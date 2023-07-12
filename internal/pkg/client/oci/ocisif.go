// Copyright (c) 2023 Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package oci

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"time"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/layout"
	"github.com/google/go-containerregistry/pkg/v1/match"
	ggcrmutate "github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/sylabs/oci-tools/pkg/mutate"
	"github.com/sylabs/oci-tools/pkg/sif"
	buildoci "github.com/sylabs/singularity/internal/pkg/build/oci"
	"github.com/sylabs/singularity/internal/pkg/cache"
	"github.com/sylabs/singularity/internal/pkg/util/fs"
	"github.com/sylabs/singularity/pkg/sylog"
	useragent "github.com/sylabs/singularity/pkg/util/user-agent"
)

// pull will create an OCI-SIF image in the cache if directTo="", or a specific file if directTo is set.
//
//nolint:dupl
func pullOciSif(ctx context.Context, imgCache *cache.Handle, directTo, pullFrom string, opts PullOptions) (imagePath string, err error) {
	sys := sysCtx(opts)
	hash, err := buildoci.ImageDigest(ctx, pullFrom, sys)
	if err != nil {
		return "", fmt.Errorf("failed to get checksum for %s: %s", pullFrom, err)
	}

	if directTo != "" {
		sylog.Infof("Converting OCI image to OCI-SIF format")
		if err := createOciSif(ctx, imgCache, pullFrom, directTo, opts); err != nil {
			return "", fmt.Errorf("while creating OCI-SIF: %v", err)
		}
		imagePath = directTo
	} else {

		cacheEntry, err := imgCache.GetEntry(cache.OciSifCacheType, hash)
		if err != nil {
			return "", fmt.Errorf("unable to check if %v exists in cache: %v", hash, err)
		}
		defer cacheEntry.CleanTmp()
		if !cacheEntry.Exists {
			sylog.Infof("Converting OCI image to OCI-SIF format")

			if err := createOciSif(ctx, imgCache, pullFrom, cacheEntry.TmpPath, opts); err != nil {
				return "", fmt.Errorf("while creating OCI-SIF: %v", err)
			}

			err = cacheEntry.Finalize()
			if err != nil {
				return "", err
			}

		} else {
			sylog.Infof("Using cached OCI-SIF image")
		}
		imagePath = cacheEntry.Path
	}

	return imagePath, nil
}

// createOciSif will convert an OCI source into an OCI-SIF using sylabs/oci-tools
func createOciSif(ctx context.Context, imgCache *cache.Handle, imageSrc, imageDest string, opts PullOptions) error {
	// Step 1 - Pull the OCI config and blobs to a standalone oci layout directory, through the cache if necessary.
	sys := sysCtx(opts)
	layoutDir, err := os.MkdirTemp(opts.TmpDir, "oci-sif-tmp-")
	if err != nil {
		return err
	}
	defer func() {
		sylog.Infof("Cleaning up...")
		if err := fs.ForceRemoveAll(layoutDir); err != nil {
			sylog.Warningf("Couldn't remove oci-sif temporary layout %q: %v", layoutDir, err)
		}
	}()
	sylog.Debugf("Fetching image to temporary layout %q", layoutDir)
	layoutRef, err := buildoci.FetchLayout(ctx, sys, imgCache, imageSrc, layoutDir)
	if err != nil {
		return fmt.Errorf("while fetching OCI image: %w", err)
	}

	// Step 2 - Work from containers/image ImageReference -> gocontainerregistry digest
	layoutSrc, err := layoutRef.NewImageSource(ctx, sys)
	if err != nil {
		return err
	}
	defer layoutSrc.Close()
	imgManifest, _, err := layoutSrc.GetManifest(ctx, nil)
	if err != nil {
		return err
	}
	digest, _, err := v1.SHA256(bytes.NewBuffer(imgManifest))
	if err != nil {
		return err
	}

	// Step 3 - Convert the layout into a squashed, single squashfs-layer oci-sif image
	return layoutToOciSif(layoutDir, digest, imageDest, opts.TmpDir)
}

// layoutToOciSif will convert an image in an OCI layout to a squashed oci-sif with squashfs layer format.
// The OCI layout can contain only a single image.
func layoutToOciSif(layoutDir string, digest v1.Hash, imageDest, tmpDir string) error {
	lp, err := layout.FromPath(layoutDir)
	if err != nil {
		return fmt.Errorf("while opening layout: %w", err)
	}
	img, err := lp.Image(digest)
	if err != nil {
		return fmt.Errorf("while retrieving image: %w", err)
	}

	sylog.Infof("Squashing image to single layer...")
	img, err = mutate.Squash(img)
	if err != nil {
		return fmt.Errorf("while squashing image: %w", err)
	}

	layers, err := img.Layers()
	if err != nil {
		return fmt.Errorf("while retrieving layers: %w", err)
	}
	if len(layers) != 1 {
		return fmt.Errorf("%d > 1 layers remaining after squash operation", len(layers))
	}
	squashfsLayer, err := mutate.SquashfsLayer(layers[0], tmpDir)
	if err != nil {
		return fmt.Errorf("while converting to squashfs format: %w", err)
	}
	img, err = mutate.Apply(img,
		mutate.ReplaceLayers(squashfsLayer),
		mutate.SetHistory(v1.History{
			Created:    v1.Time{time.Now()}, //nolint:govet
			CreatedBy:  useragent.Value(),
			Comment:    "oci-sif created from " + digest.Hex,
			EmptyLayer: false,
		}),
	)
	if err != nil {
		return fmt.Errorf("while replacing layers: %w", err)
	}

	sylog.Infof("Writing oci-sif image...")
	if err := lp.ReplaceImage(img, match.Digests(digest)); err != nil {
		return fmt.Errorf("while replacing image: %w", err)
	}
	ii := ggcrmutate.AppendManifests(empty.Index, ggcrmutate.IndexAddendum{
		Add: img,
	})
	return sif.Write(imageDest, ii)
}
