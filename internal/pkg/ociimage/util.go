// Copyright (c) 2019-2023, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package ociimage

import (
	"bytes"
	"context"
	"fmt"
	"runtime"
	"strings"

	"github.com/containers/image/v5/docker"
	dockerarchive "github.com/containers/image/v5/docker/archive"
	dockerdaemon "github.com/containers/image/v5/docker/daemon"
	ociarchive "github.com/containers/image/v5/oci/archive"
	ocilayout "github.com/containers/image/v5/oci/layout"
	"github.com/containers/image/v5/signature"
	"github.com/containers/image/v5/types"
	ggcrv1 "github.com/google/go-containerregistry/pkg/v1"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/sylabs/singularity/v4/pkg/sylog"
)

// defaultPolicy is Singularity's default OCI signature verifiction policy - accept anything.
func defaultPolicy() (*signature.PolicyContext, error) {
	policy := &signature.Policy{Default: []signature.PolicyRequirement{signature.NewPRInsecureAcceptAnything()}}
	return signature.NewPolicyContext(policy)
}

// parseImageRef parses a uri-like OCI image reference into a containers/image types.ImageReference.
func ParseImageRef(imageRef string) (types.ImageReference, error) {
	parts := strings.SplitN(imageRef, ":", 2)
	if len(parts) < 2 {
		return nil, fmt.Errorf("could not parse image ref: %s", imageRef)
	}

	var srcRef types.ImageReference
	var err error

	switch parts[0] {
	case "docker":
		srcRef, err = docker.ParseReference(parts[1])
	case "docker-archive":
		srcRef, err = dockerarchive.ParseReference(parts[1])
	case "docker-daemon":
		srcRef, err = dockerdaemon.ParseReference(parts[1])
	case "oci":
		srcRef, err = ocilayout.ParseReference(parts[1])
	case "oci-archive":
		srcRef, err = ociarchive.ParseReference(parts[1])
	default:
		return nil, fmt.Errorf("cannot create an OCI container from %s source", parts[0])
	}
	if err != nil {
		return nil, fmt.Errorf("invalid image source: %v", err)
	}

	return srcRef, nil
}

// sysCtxToPlatform translates the xxxChoice values in a containers/image
// types.SytemContext to a go-containerregistry v1.Platform.
func sysCtxToPlatform(sysCtx *types.SystemContext) ggcrv1.Platform {
	os := sysCtx.OSChoice
	if os == "" {
		os = runtime.GOOS
	}
	arch := sysCtx.ArchitectureChoice
	if arch == "" {
		arch = runtime.GOARCH
	}
	variant := sysCtx.VariantChoice
	if variant == "" {
		variant = cpuVariant()
	}
	return ggcrv1.Platform{
		Architecture: arch,
		Variant:      variant,
		OS:           os,
	}
}

// CheckImageRefPlatform ensures that an image reference satisfies platform requirements in sysCtx
func CheckImageRefPlatform(ctx context.Context, sysCtx *types.SystemContext, imageRef types.ImageReference) error {
	if sysCtx == nil {
		return fmt.Errorf("internal error: sysCtx is nil")
	}
	img, err := imageRef.NewImage(ctx, sysCtx)
	if err != nil {
		return err
	}
	defer img.Close()

	rawConfig, err := img.ConfigBlob(ctx)
	if err != nil {
		return err
	}
	cf, err := v1.ParseConfigFile(bytes.NewBuffer(rawConfig))
	if err != nil {
		return err
	}

	if cf.Platform() == nil {
		sylog.Warningf("OCI image doesn't declare a platform. It may not be compatible with this system.")
		return nil
	}

	requiredPlatform := sysCtxToPlatform(sysCtx)
	if cf.Platform().Satisfies(requiredPlatform) {
		return nil
	}

	return fmt.Errorf("image (%s) does not satisfy required platform (%s)", cf.Platform(), requiredPlatform)
}
