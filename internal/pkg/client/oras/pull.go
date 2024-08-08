// Copyright (c) 2020-2023, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package oras

import (
	"context"
	"fmt"
	"os"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/sylabs/singularity/v4/internal/pkg/cache"
	"github.com/sylabs/singularity/v4/internal/pkg/util/fs"
	"github.com/sylabs/singularity/v4/pkg/sylog"
)

// pull will pull an oras image into the cache if directTo="", or a specific file if directTo is set.
func pull(ctx context.Context, imgCache *cache.Handle, directTo, pullFrom string, ociAuth *authn.AuthConfig, reqAuthFile string) (imagePath string, err error) {
	hash, err := RefHash(ctx, pullFrom, ociAuth, reqAuthFile)
	if err != nil {
		return "", fmt.Errorf("failed to get checksum for %s: %s", pullFrom, err)
	}

	if directTo != "" {
		sylog.Infof("Downloading oras image")
		if err := DownloadImage(ctx, directTo, pullFrom, ociAuth, reqAuthFile); err != nil {
			return "", fmt.Errorf("unable to Download Image: %v", err)
		}
		imagePath = directTo
	} else {
		cacheEntry, err := imgCache.GetEntry(cache.OrasCacheType, hash.String())
		if err != nil {
			return "", fmt.Errorf("unable to check if %v exists in cache: %v", hash, err)
		}
		defer cacheEntry.CleanTmp()
		if !cacheEntry.Exists {
			sylog.Infof("Downloading oras image")

			if err := DownloadImage(ctx, cacheEntry.TmpPath, pullFrom, ociAuth, reqAuthFile); err != nil {
				return "", fmt.Errorf("unable to Download Image: %v", err)
			}
			if cacheFileHash, err := ImageHash(cacheEntry.TmpPath); err != nil {
				return "", fmt.Errorf("error getting ImageHash: %v", err)
			} else if cacheFileHash != hash {
				return "", fmt.Errorf("cached file hash(%s) and expected hash(%s) does not match", cacheFileHash, hash)
			}

			err = cacheEntry.Finalize()
			if err != nil {
				return "", err
			}
		} else {
			sylog.Infof("Using cached SIF image")
		}
		imagePath = cacheEntry.Path
	}

	return imagePath, nil
}

// Pull will pull an oras image to the cache or direct to a temporary file if cache is disabled
func Pull(ctx context.Context, imgCache *cache.Handle, pullFrom, tmpDir string, ociAuth *authn.AuthConfig, reqAuthFile string) (imagePath string, err error) {
	directTo := ""

	if imgCache.IsDisabled() {
		file, err := os.CreateTemp(tmpDir, "sbuild-tmp-cache-")
		if err != nil {
			return "", fmt.Errorf("unable to create tmp file: %v", err)
		}
		directTo = file.Name()
		sylog.Infof("Downloading oras image to tmp cache: %s", directTo)
	}

	return pull(ctx, imgCache, directTo, pullFrom, ociAuth, reqAuthFile)
}

// PullToFile will pull an oras image to the specified location, through the cache, or directly if cache is disabled
func PullToFile(ctx context.Context, imgCache *cache.Handle, pullTo, pullFrom string, ociAuth *authn.AuthConfig, reqAuthFile string) (imagePath string, err error) {
	directTo := ""
	if imgCache.IsDisabled() {
		directTo = pullTo
		sylog.Debugf("Cache disabled, pulling directly to: %s", directTo)
	}

	src, err := pull(ctx, imgCache, directTo, pullFrom, ociAuth, reqAuthFile)
	if err != nil {
		return "", fmt.Errorf("error fetching image: %v", err)
	}

	if directTo == "" {
		// mode is before umask if pullTo doesn't exist
		err = fs.CopyFileAtomic(src, pullTo, 0o777)
		if err != nil {
			return "", fmt.Errorf("error copying image out of cache: %v", err)
		}
	}

	return pullTo, nil
}
