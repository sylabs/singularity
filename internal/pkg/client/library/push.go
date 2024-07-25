// Copyright (c) 2020, Control Command Inc. All rights reserved.
// Copyright (c) 2019-2023, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package library

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	scslibrary "github.com/sylabs/scs-library-client/client"
	"github.com/sylabs/sif/v2/pkg/sif"
	"github.com/sylabs/singularity/v4/internal/pkg/client/ocisif"
	"github.com/sylabs/singularity/v4/internal/pkg/client/progress"
	"github.com/sylabs/singularity/v4/internal/pkg/remote/endpoint"
	"github.com/sylabs/singularity/v4/internal/pkg/util/machine"
	"github.com/sylabs/singularity/v4/pkg/sylog"
	"golang.org/x/term"
)

// PushOptions provides options/configuration that determine the behavior of a
// push to the library.
type PushOptions struct {
	// Description sets the optional description for an image pushed via the
	// library's own API.
	Description string
	// Endpoint is the active remote endpoint, against which the OCI registry
	// backing the library can be discovered.
	Endpoint *endpoint.Config
	// LibraryConfig configures operations against the library using its native
	// API, via sylabs/scs-library-client.
	LibraryConfig *scslibrary.Config
	// LayerFormat sets the layer format to use when pushing OCI(-SIF) images only.
	LayerFormat string
	// TmpDir is a temporary directory to be used for an temporary files created
	// during the push.
	TmpDir string
}

// Push will upload an image file to the library.
// Returns the upload completion response on success, if available, containing
// container path and quota usage for v1 libraries.
func Push(ctx context.Context, sourceFile string, destRef *scslibrary.Ref, opts PushOptions) (uploadResponse *scslibrary.UploadImageComplete, err error) {
	f, err := sif.LoadContainerFromPath(sourceFile, sif.OptLoadWithFlag(os.O_RDONLY))
	if err != nil {
		return nil, fmt.Errorf("unable to open: %v: %w", sourceFile, err)
	}
	defer f.UnloadContainer()

	if _, err := f.GetDescriptor(sif.WithDataType(sif.DataOCIRootIndex)); err == nil {
		return nil, pushOCI(ctx, sourceFile, destRef, opts)
	}
	return pushNative(ctx, sourceFile, destRef, opts)
}

// pushNative pushes a non-OCI SIF image, as a SIF, using the library client.
func pushNative(ctx context.Context, sourceFile string, destRef *scslibrary.Ref, opts PushOptions) (uploadResponse *scslibrary.UploadImageComplete, err error) {
	arch, err := machine.SifArch(sourceFile)
	if err != nil {
		return nil, err
	}

	libraryClient, err := scslibrary.NewClient(opts.LibraryConfig)
	if err != nil {
		return nil, fmt.Errorf("error initializing library client: %v", err)
	}

	if destRef.Host != "" && destRef.Host != libraryClient.BaseURL.Host {
		return nil, errors.New("push to location other than current remote is not supported")
	}

	// open image for uploading
	f, err := os.Open(sourceFile)
	if err != nil {
		return nil, fmt.Errorf("error opening image %s for reading: %v", sourceFile, err)
	}
	defer f.Close()

	// Get file size by seeking to end and back
	fSize, err := f.Seek(0, 2)
	if err != nil {
		return nil, err
	}
	if _, err := f.Seek(0, 0); err != nil {
		return nil, err
	}

	var progressBar scslibrary.UploadCallback
	if term.IsTerminal(2) {
		progressBar = &progress.UploadBar{}
	}

	defer func(t time.Time) {
		if err == nil && progressBar == nil {
			sylog.Infof("Uploaded %d bytes in %v\n", fSize, time.Since(t))
		}
	}(time.Now())

	return libraryClient.UploadImage(ctx, f, destRef.Path, arch, destRef.Tags, opts.Description, progressBar)
}

// pushOCI pushes an OCI SIF image, as an OCI image, using the ocisif client.
func pushOCI(ctx context.Context, sourceFile string, destRef *scslibrary.Ref, opts PushOptions) error {
	sylog.Infof("Pushing an OCI-SIF to the library OCI registry. Use `--oci` to pull this image.")
	lr, err := newLibraryRegistry(opts.Endpoint, opts.LibraryConfig)
	if err != nil {
		return err
	}

	pushRef, err := lr.convertRef(*destRef)
	if err != nil {
		return err
	}

	sylog.Debugf("Pushing to OCI registry at: %s", pushRef)
	ocisifOpts := ocisif.PushOptions{
		Auth:        lr.authConfig(),
		AuthFile:    "",
		LayerFormat: opts.LayerFormat,
		TmpDir:      opts.TmpDir,
	}
	return ocisif.PushOCISIF(ctx, sourceFile, pushRef, ocisifOpts)
}
