// Copyright (c) 2023-2025, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package ociimage

import (
	"context"
	"fmt"
	"strings"

	"github.com/docker/docker/client"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/daemon"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/layout"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
	cosignoci "github.com/sigstore/cosign/v2/pkg/oci"
	cosignremote "github.com/sigstore/cosign/v2/pkg/oci/remote"
	"github.com/sylabs/singularity/v4/internal/pkg/client/progress"
	"github.com/sylabs/singularity/v4/internal/pkg/remote/credential/ociauth"
	"github.com/sylabs/singularity/v4/pkg/sylog"
)

type SourceSink int

const (
	UnknownSourceSink SourceSink = iota
	RegistrySourceSink
	OCISourceSink
	TarballSourceSink
	DaemonSourceSink
)

func getDockerImage(ctx context.Context, src string, tOpts *TransportOptions, rt *progress.RoundTripper) (v1.Image, error) {
	var nameOpts []name.Option
	if tOpts != nil && tOpts.Insecure {
		nameOpts = append(nameOpts, name.Insecure)
	}

	srcRef, err := name.ParseReference(src, nameOpts...)
	if err != nil {
		return nil, err
	}

	pullOpts := []remote.Option{
		remote.WithContext(ctx),
	}

	if tOpts != nil {
		pullOpts = append(pullOpts,
			remote.WithPlatform(tOpts.Platform),
			ociauth.AuthOptn(tOpts.AuthConfig, tOpts.AuthFilePath))
	}

	if rt != nil {
		pullOpts = append(pullOpts, remote.WithTransport(rt))
	}

	return remote.Image(srcRef, pullOpts...)
}

func getSignedDockerImage(ctx context.Context, src string, tOpts *TransportOptions, rt *progress.RoundTripper) (cosignoci.SignedImage, error) {
	var nameOpts []name.Option
	if tOpts != nil && tOpts.Insecure {
		nameOpts = append(nameOpts, name.Insecure)
	}

	srcRef, err := name.ParseReference(src, nameOpts...)
	if err != nil {
		return nil, err
	}

	pullOpts := []remote.Option{
		remote.WithContext(ctx),
	}

	if tOpts != nil {
		pullOpts = append(pullOpts,
			remote.WithPlatform(tOpts.Platform),
			ociauth.AuthOptn(tOpts.AuthConfig, tOpts.AuthFilePath))
	}

	if rt != nil {
		pullOpts = append(pullOpts, remote.WithTransport(rt))
	}

	return cosignremote.SignedImage(srcRef, cosignremote.WithRemoteOptions(pullOpts...))
}

// getOCIImage retrieves an image from a layout ref provided in <dir>[@digest] format.
// If no digest is provided, and there is only one image in the layout, it will be returned.
// A digest must be specified when retrieving an image from a layout containing multiple images.
func getOCIImage(src string) (v1.Image, error) {
	refParts := strings.SplitN(src, "@", 2)

	lp, err := layout.FromPath(refParts[0])
	if err != nil {
		return nil, err
	}

	ii, err := lp.ImageIndex()
	if err != nil {
		return nil, err
	}

	im, err := ii.IndexManifest()
	if err != nil {
		return nil, err
	}

	if len(im.Manifests) < 1 {
		return nil, fmt.Errorf("no images found in layout %s", src)
	}

	if len(refParts) < 2 && len(im.Manifests) != 1 {
		return nil, fmt.Errorf("must specify a digest - layout contains multiple images")
	}
	if len(refParts) == 1 {
		return lp.Image(im.Manifests[0].Digest)
	}

	for _, mf := range im.Manifests {
		sylog.Debugf("%v =? %v", mf.Digest.String(), refParts[1])
		if mf.Digest.String() == refParts[1] {
			return ii.Image(mf.Digest)
		}
	}

	return nil, fmt.Errorf("image %q not found in layout", src)
}

func getDaemonImage(ctx context.Context, src string, tOpts *TransportOptions) (v1.Image, error) {
	var nameOpts []name.Option
	if tOpts != nil && tOpts.Insecure {
		nameOpts = append(nameOpts, name.Insecure)
	}

	srcRef, err := name.ParseReference(src, nameOpts...)
	if err != nil {
		return nil, err
	}

	dOpts := []daemon.Option{
		daemon.WithContext(ctx),
	}

	if tOpts != nil && tOpts.DockerDaemonHost != "" {
		dc, err := client.NewClientWithOpts(client.WithHost(tOpts.DockerDaemonHost))
		if err != nil {
			return nil, err
		}
		dOpts = append(dOpts, daemon.WithClient(dc))
	}

	return daemon.Image(srcRef, dOpts...)
}

func (ss SourceSink) Reference(s string, tOpts *TransportOptions) (name.Reference, bool) {
	switch ss {
	case RegistrySourceSink, DaemonSourceSink:
		var nameOpts []name.Option
		if tOpts != nil && tOpts.Insecure {
			nameOpts = append(nameOpts, name.Insecure)
		}
		srcRef, err := name.ParseReference(s, nameOpts...)
		if err != nil {
			return nil, false
		}
		return srcRef, true
	default:
		return nil, false
	}
}

func (ss SourceSink) Image(ctx context.Context, ref string, tOpts *TransportOptions, rt *progress.RoundTripper) (v1.Image, error) {
	switch ss {
	case RegistrySourceSink:
		return getDockerImage(ctx, ref, tOpts, rt)
	case TarballSourceSink:
		return tarball.ImageFromPath(ref, nil)
	case OCISourceSink:
		return getOCIImage(ref)
	case DaemonSourceSink:
		return getDaemonImage(ctx, ref, tOpts)
	case UnknownSourceSink:
		return nil, errUnsupportedTransport
	default:
		return nil, errUnsupportedTransport
	}
}

func (ss SourceSink) SignedImage(ctx context.Context, ref string, tOpts *TransportOptions, rt *progress.RoundTripper) (cosignoci.SignedImage, error) {
	switch ss {
	case RegistrySourceSink:
		return getSignedDockerImage(ctx, ref, tOpts, rt)
	default:
		return nil, errUnsupportedTransport
	}
}

func (ss SourceSink) WriteImage(img v1.Image, dstName string, tOpts *TransportOptions) error {
	switch ss {
	case OCISourceSink:
		lp, err := layout.FromPath(dstName)
		if err != nil {
			lp, err = layout.Write(dstName, empty.Index)
			if err != nil {
				return err
			}
		}
		return lp.AppendImage(img)

	case RegistrySourceSink:
		var nameOpts []name.Option
		if tOpts != nil && tOpts.Insecure {
			nameOpts = append(nameOpts, name.Insecure)
		}
		dstRef, err := name.ParseReference(dstName, nameOpts...)
		if err != nil {
			return err
		}
		remoteOpts := []remote.Option{}
		if tOpts != nil {
			remoteOpts = append(remoteOpts,
				remote.WithPlatform(tOpts.Platform),
				ociauth.AuthOptn(tOpts.AuthConfig, tOpts.AuthFilePath))
		}
		return remote.Write(dstRef, img, remoteOpts...)

	case TarballSourceSink:
		// Only supports writing a single image per tarball.
		dstRef := name.MustParseReference("image")
		return tarball.WriteToFile(dstName, dstRef, img)

	case UnknownSourceSink:
		return errUnsupportedTransport
	default:
		return errUnsupportedTransport
	}
}
