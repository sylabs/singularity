// Copyright (c) 2020, Control Command Inc. All rights reserved.
// Copyright (c) 2018-2023, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package oci

import (
	"context"
	"fmt"
	"os"

	"github.com/google/go-containerregistry/pkg/authn"
	gccrv1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/sylabs/singularity/v4/internal/pkg/cache"
	"github.com/sylabs/singularity/v4/internal/pkg/client/ocisif"
	"github.com/sylabs/singularity/v4/internal/pkg/ociimage"
	"github.com/sylabs/singularity/v4/internal/pkg/remote/credential/ociauth"
	"github.com/sylabs/singularity/v4/internal/pkg/util/fs"
	"github.com/sylabs/singularity/v4/pkg/sylog"
	useragent "github.com/sylabs/singularity/v4/pkg/util/user-agent"
)

type PullOptions struct {
	TmpDir      string
	OciAuth     *authn.AuthConfig
	DockerHost  string
	NoHTTPS     bool
	NoCleanUp   bool
	OciSif      bool
	KeepLayers  bool
	Platform    gccrv1.Platform
	ReqAuthFile string
}

// transportOptions maps PullOptions to OCI image transport options
func transportOptions(opts PullOptions) *ociimage.TransportOptions {
	return &ociimage.TransportOptions{
		AuthConfig:       opts.OciAuth,
		AuthFilePath:     ociauth.ChooseAuthFile(opts.ReqAuthFile),
		Insecure:         opts.NoHTTPS,
		TmpDir:           opts.TmpDir,
		UserAgent:        useragent.Value(),
		DockerDaemonHost: opts.DockerHost,
		Platform:         opts.Platform,
	}
}

// Pull will create a SIF / OCI-SIF image to the cache or direct to a temporary file if cache is disabled
func Pull(ctx context.Context, imgCache *cache.Handle, pullFrom string, opts PullOptions) (imagePath string, err error) {
	directTo := ""
	if imgCache.IsDisabled() {
		file, err := os.CreateTemp(opts.TmpDir, "sbuild-tmp-cache-")
		if err != nil {
			return "", fmt.Errorf("unable to create tmp file: %v", err)
		}
		directTo = file.Name()
		sylog.Infof("Downloading library image to tmp cache: %s", directTo)
	}
	if opts.OciSif {
		ocisifOpts := ocisif.PullOptions{
			TmpDir:      opts.TmpDir,
			OciAuth:     opts.OciAuth,
			DockerHost:  opts.DockerHost,
			NoHTTPS:     opts.NoHTTPS,
			NoCleanUp:   opts.NoCleanUp,
			Platform:    opts.Platform,
			ReqAuthFile: opts.ReqAuthFile,
			KeepLayers:  opts.KeepLayers,
		}
		return ocisif.PullOCISIF(ctx, imgCache, directTo, pullFrom, ocisifOpts)
	}

	return pullNativeSIF(ctx, imgCache, directTo, pullFrom, opts)
}

// PullToFile will create a SIF / OCI-SIF image from the specified oci URI and place it at the specified dest
func PullToFile(ctx context.Context, imgCache *cache.Handle, pullTo, pullFrom string, opts PullOptions) (imagePath string, err error) {
	directTo := ""
	if imgCache.IsDisabled() {
		directTo = pullTo
		sylog.Debugf("Cache disabled, pulling directly to: %s", directTo)
	}
	src := ""
	if opts.OciSif {
		ocisifOpts := ocisif.PullOptions{
			TmpDir:      opts.TmpDir,
			OciAuth:     opts.OciAuth,
			DockerHost:  opts.DockerHost,
			NoHTTPS:     opts.NoHTTPS,
			NoCleanUp:   opts.NoCleanUp,
			Platform:    opts.Platform,
			ReqAuthFile: opts.ReqAuthFile,
			KeepLayers:  opts.KeepLayers,
		}
		src, err = ocisif.PullOCISIF(ctx, imgCache, directTo, pullFrom, ocisifOpts)
	} else {
		src, err = pullNativeSIF(ctx, imgCache, directTo, pullFrom, opts)
	}
	if err != nil {
		return "", fmt.Errorf("error fetching image: %v", err)
	}

	if directTo == "" {
		// mode is before umask if pullTo doesn't exist
		err = fs.CopyFileAtomic(src, pullTo, 0o777)
		if err != nil {
			return "", fmt.Errorf("error copying image out of cache (from %q to %q): %w", src, pullTo, err)
		}
	}

	return pullTo, nil
}
