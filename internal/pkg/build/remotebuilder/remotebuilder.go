// Copyright (c) 2018-2023, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

//
// NOTE: This package uses a different version of the definition struct and
// definition parser than the rest of the image build system in order to maintain
// compatibility with the remote builder.
//

package remotebuilder

import (
	"bytes"
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"

	golog "github.com/go-log/log"
	"github.com/pkg/errors"
	buildclient "github.com/sylabs/scs-build-client/client"
	client "github.com/sylabs/scs-library-client/client"
	"github.com/sylabs/singularity/v4/internal/pkg/client/library"
	"github.com/sylabs/singularity/v4/pkg/build/types"
	"github.com/sylabs/singularity/v4/pkg/sylog"
	useragent "github.com/sylabs/singularity/v4/pkg/util/user-agent"
)

// RemoteBuilder contains the build request and response
type RemoteBuilder struct {
	BuildClient *buildclient.Client
	ImagePath   string
	LibraryURL  string
	Definition  types.Definition
	BuilderURL  *url.URL
	AuthToken   string
	Force       bool
	IsDetached  bool
	Arch        string
	WebURL      string
}

// New creates a RemoteBuilder with the specified details.
func New(imagePath, libraryURL string, d types.Definition, isDetached, force bool, builderAddr, authToken, buildArch, webURL string) (rb *RemoteBuilder, err error) {
	bc, err := buildclient.NewClient(
		buildclient.OptBaseURL(builderAddr),
		buildclient.OptBearerToken(authToken),
		buildclient.OptUserAgent(useragent.Value()),
	)
	if err != nil {
		return nil, err
	}

	return &RemoteBuilder{
		BuildClient: bc,
		ImagePath:   imagePath,
		Force:       force,
		LibraryURL:  libraryURL,
		Definition:  d,
		IsDetached:  isDetached,
		AuthToken:   authToken,
		Arch:        buildArch,
		WebURL:      webURL,
	}, nil
}

// pathsFromDefinition determines the local paths that should be uploaded to the build service.
func pathsFromDefinition(d types.Definition) ([]string, error) {
	var paths []string

	// There may be mutiple files sections. We only consider files that do not originate from a
	// stage of the build.
	for _, f := range d.BuildData.Files {
		if f.Stage() == "" {
			// Loop through list of files and append source path.
			for _, ft := range f.Files {
				if ft.Src == "" {
					continue
				}

				sylog.Infof("Preparing to upload %v to remote build service...", ft.Src)

				path, err := ft.SourcePath()
				if err != nil {
					return nil, err
				}

				paths = append(paths, path)
			}
		}
	}

	return paths, nil
}

// uploadBuildContext examines the definition for local file references. If no references are
// found, a nil error is returned with an empty digest. Otherwise, an archive containing the local
// files is uploaded to the builder, and its digest is returned.
func (rb *RemoteBuilder) uploadBuildContext(ctx context.Context) (digest string, err error) {
	paths, err := pathsFromDefinition(rb.Definition)
	if err != nil {
		return "", fmt.Errorf("failed to determine paths from definition: %w", err)
	}

	if len(paths) <= 0 {
		return "", nil
	}

	digest, err = rb.BuildClient.UploadBuildContext(ctx, paths)
	if err != nil {
		sylog.Infof("Build context upload failed. This build server may not support the `%%files` section for remote builds.")
	}
	return digest, err
}

// Build is responsible for making the request via scs-build-client to the builder
func (rb *RemoteBuilder) Build(ctx context.Context) (err error) {
	var libraryRef string

	if strings.HasPrefix(rb.ImagePath, "library://") {
		// Image destination is Library.
		libraryRef = rb.ImagePath
	}

	if libraryRef != "" && !client.IsLibraryPushRef(libraryRef) {
		return fmt.Errorf("invalid library reference: %s", rb.ImagePath)
	}

	// Upload build context, if applicable.
	contextDigest, err := rb.uploadBuildContext(ctx)
	if err != nil {
		return fmt.Errorf("failed to upload build context: %w", err)
	}

	bi, err := rb.BuildClient.Submit(ctx, bytes.NewReader(rb.Definition.FullRaw),
		buildclient.OptBuildLibraryRef(libraryRef),
		buildclient.OptBuildLibraryPullBaseURL(rb.LibraryURL),
		buildclient.OptBuildArchitecture(rb.Arch),
		buildclient.OptBuildContext(contextDigest),
	)
	if err != nil {
		return errors.Wrap(err, "failed to post request to remote build service")
	}
	sylog.Debugf("Build response - id: %s, libref: %s", bi.ID(), bi.LibraryRef())

	// If we're doing an detached build, print help on how to download the image
	libraryRefRaw := strings.TrimPrefix(bi.LibraryRef(), "library://")
	if rb.IsDetached {
		fmt.Printf("Build submitted! Once it is complete, the image can be retrieved by running:\n")
		fmt.Printf("\tsingularity pull --library %s library://%s\n\n", bi.LibraryURL(), libraryRefRaw)
		if rb.WebURL != "" {
			fmt.Printf("Alternatively, you can access it from a browser at:\n\t%s/library/%s\n", rb.WebURL, libraryRefRaw)
		}
		return nil
	}

	// We're doing an attached build, stream output and then download the resulting file
	err = rb.BuildClient.GetOutput(ctx, bi.ID(), os.Stdout)
	if err != nil {
		return errors.Wrap(err, "failed to stream output from remote build service")
	}

	// Get build status
	bi, err = rb.BuildClient.GetStatus(ctx, bi.ID())
	if err != nil {
		return errors.Wrap(err, "failed to get status from remote build service")
	}

	// Do not try to download image if not complete or image size is 0
	if !bi.IsComplete() {
		return errors.New("build has not completed")
	}
	if bi.ImageSize() <= 0 {
		return errors.New("build image size <= 0")
	}

	// Now that the build is complete, delete the build context (if applicable.)
	if contextDigest != "" {
		if err := rb.BuildClient.DeleteBuildContext(ctx, contextDigest); err != nil {
			sylog.Warningf("failed to delete build context: %v", err)
		}
	}

	// If image destination is local file, pull image.
	if !strings.HasPrefix(rb.ImagePath, "library://") {
		f, err := os.OpenFile(rb.ImagePath, os.O_CREATE|os.O_TRUNC|os.O_RDWR, 0o777)
		if err != nil {
			return errors.Wrap(err, fmt.Sprintf("unable to open file %s for writing", rb.ImagePath))
		}
		defer f.Close()

		c, err := client.NewClient(&client.Config{
			BaseURL:   bi.LibraryURL(),
			AuthToken: rb.AuthToken,
			Logger:    (golog.Logger)(sylog.DebugLogger{}),
		})
		if err != nil {
			return errors.Wrap(err, fmt.Sprintf("error initializing library client: %v", err))
		}

		imageRef, err := library.NormalizeLibraryRef(bi.LibraryRef())
		if err != nil {
			return errors.Wrap(err, fmt.Sprintf("error parsing library reference: %v", err))
		}

		if err = library.DownloadImageNoProgress(ctx, c, rb.ImagePath, rb.Arch, imageRef); err != nil {
			return errors.Wrap(err, "failed to pull image file")
		}
	}

	return nil
}
