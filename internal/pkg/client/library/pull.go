// Copyright (c) 2020, Control Command Inc. All rights reserved.
// Copyright (c) 2020-2023, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package library

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	gccrv1 "github.com/google/go-containerregistry/pkg/v1"
	keyclient "github.com/sylabs/scs-key-client/client"
	scslibrary "github.com/sylabs/scs-library-client/client"
	"github.com/sylabs/singularity/v4/internal/pkg/cache"
	"github.com/sylabs/singularity/v4/internal/pkg/client/ocisif"
	"github.com/sylabs/singularity/v4/internal/pkg/client/progress"
	"github.com/sylabs/singularity/v4/internal/pkg/remote/endpoint"
	"github.com/sylabs/singularity/v4/internal/pkg/signature"
	"github.com/sylabs/singularity/v4/internal/pkg/util/fs"
	"github.com/sylabs/singularity/v4/pkg/sylog"
	"golang.org/x/term"
)

// ErrLibraryPullUnsigned indicates that the interactive portion of the pull was aborted.
var ErrLibraryPullUnsigned = errors.New("failed to verify container")

// PullOptions provides options/configuration that determine the behavior of a
// pull from a library.
type PullOptions struct {
	// Endpoint is the active remote endpoint, against which the OCI registry
	// backing the library can be discovered.
	Endpoint *endpoint.Config
	// LibraryConfig configures operations against the library using its native
	// API, via sylabs/scs-library-client.
	LibraryConfig *scslibrary.Config
	// KeyClientOpts specifies options for the keyclient that will be used to
	// verify signatures after pulling an image.
	KeyClientOpts []keyclient.Option
	// TmpDir is the path to a directory used for temporary files.
	TmpDir string
	// RequireOciSif should be set true to require that the image pulled is an OCI-SIF.
	// If false a native SIF pull will be attempted, followed by an OCI(-SIF) pull on failure.
	RequireOciSif bool
	// When pulling an OCI-SIF, keep multiple layers if true, squash to single layer otherwise.
	KeepLayers bool
	// Platform specifies the platform of the image to retrieve.
	Platform gccrv1.Platform
}

// pull will pull a library image into the cache if directTo="", or a specific file if directTo is set.
// Attempts a native SIF pull using the library API. If this fails, and the
// error indicates the image is an OCI image, an OCI-SIF pull will be attempted.
func pull(ctx context.Context, imgCache *cache.Handle, directTo string, imageRef *scslibrary.Ref, opts PullOptions) (string, error) {
	c, err := scslibrary.NewClient(opts.LibraryConfig)
	if err != nil {
		return "", fmt.Errorf("unable to initialize client library: %w", err)
	}

	ref := fmt.Sprintf("%s:%s", imageRef.Path, imageRef.Tags[0])

	libraryImage, err := c.GetImage(ctx, opts.Platform.Architecture, ref)
	if err != nil {
		if errors.Is(err, scslibrary.ErrNotFound) {
			return "", fmt.Errorf("image does not exist in the library: %s (%s)", ref, opts.Platform.Architecture)
		}
		// TODO - handle this via a friendlier error in future.
		// Error message comes from server, so this will require changes upstream.
		if strings.Contains(err.Error(), "application/vnd.oci.image.config.v1+json") {
			sylog.Infof("%s is an OCI image, attempting to fetch as an OCI-SIF", ref)
			return pullOCI(ctx, imgCache, directTo, imageRef, opts)
		}
		return "", err
	}

	var progressBar scslibrary.ProgressBar
	if term.IsTerminal(2) {
		progressBar = &progress.DownloadBar{}
	}

	if directTo != "" {
		// Download direct to file
		if err := downloadWrapper(ctx, c, directTo, opts.Platform.Architecture, imageRef, progressBar); err != nil {
			return "", fmt.Errorf("unable to download image: %v", err)
		}
		return directTo, nil
	}

	cacheEntry, err := imgCache.GetEntry(cache.LibraryCacheType, libraryImage.Hash)
	if err != nil {
		return "", fmt.Errorf("unable to check if %v exists in cache: %v", libraryImage.Hash, err)
	}
	defer cacheEntry.CleanTmp()

	if !cacheEntry.Exists {
		if err := downloadWrapper(ctx, c, cacheEntry.TmpPath, opts.Platform.Architecture, imageRef, progressBar); err != nil {
			return "", fmt.Errorf("unable to download image: %v", err)
		}

		if cacheFileHash, err := scslibrary.ImageHash(cacheEntry.TmpPath); err != nil {
			return "", fmt.Errorf("error getting image hash: %v", err)
		} else if cacheFileHash != libraryImage.Hash {
			return "", fmt.Errorf("cached file hash(%s) and expected hash(%s) does not match", cacheFileHash, libraryImage.Hash)
		}

		if err := cacheEntry.Finalize(); err != nil {
			return "", err
		}
	} else {
		sylog.Infof("Using cached image")
	}

	return cacheEntry.Path, nil
}

// downloadWrapper calls DownloadImage() and outputs download summary if progressBar not specified.
func downloadWrapper(ctx context.Context, c *scslibrary.Client, imagePath, arch string, libraryRef *scslibrary.Ref, pb scslibrary.ProgressBar) error {
	sylog.Infof("Downloading library image")

	defer func(t time.Time) {
		if pb == nil {
			if fi, err := os.Stat(imagePath); err == nil {
				// Progress bar interface not specified; output summary to stdout
				sylog.Infof("Downloaded %d bytes in %v\n", fi.Size(), time.Since(t))
			}
		}
	}(time.Now())

	return DownloadImage(ctx, c, imagePath, arch, libraryRef, pb)
}

// pullOCI pulls a single layer squashfs OCI image from the library into an OCI-SIF file.
func pullOCI(ctx context.Context, imgCache *cache.Handle, directTo string, pullFrom *scslibrary.Ref, opts PullOptions) (imagePath string, err error) {
	lr, err := newLibraryRegistry(opts.Endpoint, opts.LibraryConfig)
	if err != nil {
		return "", err
	}

	pullRef, err := lr.convertRef(*pullFrom)
	if err != nil {
		return "", err
	}

	authConf := lr.authConfig()
	ocisifOpts := ocisif.PullOptions{
		TmpDir:     opts.TmpDir,
		OciAuth:    authConf,
		Platform:   opts.Platform,
		KeepLayers: opts.KeepLayers,
	}
	return ocisif.PullOCISIF(ctx, imgCache, directTo, pullRef, ocisifOpts)
}

// Pull will pull a library image to the cache or direct to a temporary file if cache is disabled
func Pull(ctx context.Context, imgCache *cache.Handle, pullFrom *scslibrary.Ref, opts PullOptions) (imagePath string, err error) {
	directTo := ""

	if imgCache.IsDisabled() {
		file, err := os.CreateTemp(opts.TmpDir, "sbuild-tmp-cache-")
		if err != nil {
			return "", fmt.Errorf("unable to create tmp file: %v", err)
		}
		directTo = file.Name()
		sylog.Infof("Downloading library image to tmp cache: %s", directTo)
	}

	if opts.RequireOciSif {
		return pullOCI(ctx, imgCache, directTo, pullFrom, opts)
	}

	return pull(ctx, imgCache, directTo, pullFrom, opts)
}

// PullToFile will pull a library image to the specified location, through the cache, or directly if cache is disabled
func PullToFile(ctx context.Context, imgCache *cache.Handle, pullTo string, pullFrom *scslibrary.Ref, opts PullOptions) (imagePath string, err error) {
	directTo := ""
	if imgCache.IsDisabled() {
		directTo = pullTo
		sylog.Debugf("Cache disabled, pulling directly to: %s", directTo)
	}

	src := ""
	if opts.RequireOciSif {
		src, err = pullOCI(ctx, imgCache, directTo, pullFrom, opts)
	} else {
		src, err = pull(ctx, imgCache, directTo, pullFrom, opts)
	}
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

	if err := signature.Verify(ctx, pullTo, signature.OptVerifyWithPGP(opts.KeyClientOpts...)); err != nil {
		sylog.Warningf("%v", err)
		return pullTo, ErrLibraryPullUnsigned
	}

	return pullTo, nil
}
