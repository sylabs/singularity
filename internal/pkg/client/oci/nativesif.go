// Copyright (c) 2020, Control Command Inc. All rights reserved.
// Copyright (c) 2018-2023, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package oci

import (
	"context"
	"fmt"

	"github.com/sylabs/singularity/v4/internal/pkg/build"
	"github.com/sylabs/singularity/v4/internal/pkg/cache"
	"github.com/sylabs/singularity/v4/internal/pkg/ociimage"
	"github.com/sylabs/singularity/v4/internal/pkg/ociplatform"
	"github.com/sylabs/singularity/v4/internal/pkg/util/machine"
	buildtypes "github.com/sylabs/singularity/v4/pkg/build/types"
	"github.com/sylabs/singularity/v4/pkg/sylog"
)

// pullNativeSIF will build a SIF image into the cache if directTo="", or a specific file if directTo is set.
func pullNativeSIF(ctx context.Context, imgCache *cache.Handle, directTo, pullFrom string, opts PullOptions) (imagePath string, err error) {
	to := transportOptions(opts)

	hash, err := ociimage.ImageDigest(ctx, to, imgCache, pullFrom)
	if err != nil {
		return "", fmt.Errorf("failed to get checksum for %s: %s", pullFrom, err)
	}

	if directTo != "" {
		sylog.Infof("Converting OCI blobs to SIF format")
		if err := convertOciToSIF(ctx, imgCache, pullFrom, directTo, opts); err != nil {
			return "", fmt.Errorf("while building SIF from layers: %v", err)
		}
		imagePath = directTo
	} else {
		cacheEntry, err := imgCache.GetEntry(cache.OciTempCacheType, hash.String())
		if err != nil {
			return "", fmt.Errorf("unable to check if %v exists in cache: %v", hash, err)
		}
		defer cacheEntry.CleanTmp()
		if !cacheEntry.Exists {
			sylog.Infof("Converting OCI blobs to SIF format")

			if err := convertOciToSIF(ctx, imgCache, pullFrom, cacheEntry.TmpPath, opts); err != nil {
				return "", fmt.Errorf("while building SIF from layers: %v", err)
			}

			err = cacheEntry.Finalize()
			if err != nil {
				return "", err
			}
		} else {
			// Ensure what's retrieved from the cache matches the target platform
			sifArch, err := machine.SifArch(cacheEntry.Path)
			if err != nil {
				return "", err
			}
			sifPlatform, err := ociplatform.PlatformFromArch(sifArch)
			if err != nil {
				return "", fmt.Errorf("could not determine OCI platform from cached image architecture %q: %w", sifArch, err)
			}
			if !sifPlatform.Satisfies(opts.Platform) {
				return "", fmt.Errorf("image (%s) does not satisfy required platform (%s)", sifPlatform, opts.Platform)
			}
			sylog.Infof("Using cached SIF image")
		}
		imagePath = cacheEntry.Path
	}

	return imagePath, nil
}

// convertOciToSIF will convert an OCI source into a SIF using the build routines
func convertOciToSIF(ctx context.Context, imgCache *cache.Handle, image, cachedImgPath string, opts PullOptions) error {
	if imgCache == nil {
		return fmt.Errorf("image cache is undefined")
	}

	b, err := build.NewBuild(
		image,
		build.Config{
			Dest:   cachedImgPath,
			Format: "sif",
			Opts: buildtypes.Options{
				TmpDir:           opts.TmpDir,
				NoCache:          imgCache.IsDisabled(),
				NoTest:           true,
				NoHTTPS:          opts.NoHTTPS,
				DockerDaemonHost: opts.DockerHost,
				ImgCache:         imgCache,
				Platform:         opts.Platform,
				OCIAuthConfig:    opts.OciAuth,
				DockerAuthFile:   opts.ReqAuthFile,
			},
			NoCleanUp: opts.NoCleanUp,
		},
	)
	if err != nil {
		return fmt.Errorf("unable to create new build: %v", err)
	}

	return b.Full(ctx)
}
