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

	ocitypes "github.com/containers/image/v5/types"
	"github.com/sylabs/singularity/internal/pkg/cache"
	"github.com/sylabs/singularity/internal/pkg/util/fs"
	"github.com/sylabs/singularity/pkg/syfs"
	"github.com/sylabs/singularity/pkg/sylog"
	useragent "github.com/sylabs/singularity/pkg/util/user-agent"
)

type PullOptions struct {
	TmpDir     string
	OciAuth    *ocitypes.DockerAuthConfig
	DockerHost string
	NoHTTPS    bool
	NoCleanUp  bool
}

// sysCtx provides authentication and tempDir config for containers/image OCI operations
func sysCtx(opts PullOptions) *ocitypes.SystemContext {
	// DockerInsecureSkipTLSVerify is set only if --no-https is specified to honor
	// configuration from /etc/containers/registries.conf because DockerInsecureSkipTLSVerify
	// can have three possible values true/false and undefined, so we left it as undefined instead
	// of forcing it to false in order to delegate decision to /etc/containers/registries.conf:
	// https://github.com/sylabs/singularity/issues/5172
	sysCtx := &ocitypes.SystemContext{
		OCIInsecureSkipTLSVerify: opts.NoHTTPS,
		DockerAuthConfig:         opts.OciAuth,
		AuthFilePath:             syfs.DockerConf(),
		DockerRegistryUserAgent:  useragent.Value(),
		BigFilesTemporaryDir:     opts.TmpDir,
	}
	if opts.NoHTTPS {
		sysCtx.DockerInsecureSkipTLSVerify = ocitypes.NewOptionalBool(true)
	}
	return sysCtx
}

// Pull will build a SIF image to the cache or direct to a temporary file if cache is disabled
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

	return pullSif(ctx, imgCache, directTo, pullFrom, opts)
}

// PullToFile will build a SIF image from the specified oci URI and place it at the specified dest
func PullToFile(ctx context.Context, imgCache *cache.Handle, pullTo, pullFrom string, opts PullOptions) (imagePath string, err error) {
	directTo := ""
	if imgCache.IsDisabled() {
		directTo = pullTo
		sylog.Debugf("Cache disabled, pulling directly to: %s", directTo)
	}

	src, err := pullSif(ctx, imgCache, directTo, pullFrom, opts)
	if err != nil {
		return "", fmt.Errorf("error fetching image to cache: %v", err)
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
