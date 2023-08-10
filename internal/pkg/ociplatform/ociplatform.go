// Copyright (c) 2019-2023, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package ociplatform

import (
	"bytes"
	"context"
	"fmt"
	"runtime"

	"github.com/containers/image/v5/types"
	ggcrv1 "github.com/google/go-containerregistry/pkg/v1"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/sylabs/singularity/v4/pkg/sylog"
)

// SysCtxToPlatform translates the xxxChoice values in a containers/image
// types.SytemContext to a go-containerregistry v1.Platform.
func SysCtxToPlatform(sysCtx *types.SystemContext) ggcrv1.Platform {
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
		variant = CPUVariant()
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

	requiredPlatform := SysCtxToPlatform(sysCtx)
	if cf.Platform().Satisfies(requiredPlatform) {
		return nil
	}

	return fmt.Errorf("image (%s) does not satisfy required platform (%s)", cf.Platform(), requiredPlatform)
}

func DefaultPlatform() (*ggcrv1.Platform, error) {
	if runtime.GOOS != "linux" {
		return nil, fmt.Errorf("%q is not a valid platform OS for singularity", runtime.GOOS)
	}
	return &ggcrv1.Platform{
		OS:           runtime.GOOS,
		Architecture: runtime.GOARCH,
		Variant:      CPUVariant(),
	}, nil
}

func PlatformFromString(p string) (*ggcrv1.Platform, error) {
	plat, err := ggcrv1.ParsePlatform(p)
	if err != nil {
		return nil, err
	}
	if plat.OS != "linux" {
		return nil, fmt.Errorf("%q is not a valid platform OS for singularity", plat.OS)
	}
	return plat, nil
}

func PlatformFromArch(a string) (*ggcrv1.Platform, error) {
	if runtime.GOOS != "linux" {
		return nil, fmt.Errorf("%q is not a valid platform OS for singularity", runtime.GOOS)
	}
	return &ggcrv1.Platform{
		OS:           runtime.GOOS,
		Architecture: a,
		Variant:      "",
	}, nil
}
