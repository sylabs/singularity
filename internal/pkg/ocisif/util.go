// Copyright (c) 2024, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package ocisif

import (
	"strings"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	imagespec "github.com/opencontainers/image-spec/specs-go/v1"
	ocitsif "github.com/sylabs/oci-tools/pkg/sif"
	"github.com/sylabs/oci-tools/pkg/sourcesink"
	"github.com/sylabs/sif/v2/pkg/sif"
)

// GetSingleImage returns a v1.Image from an OCI-SIF, that must contain a single
// image. It ignores any cosign images that may be present in the OCI-SIF.
func GetSingleImage(fi *sif.FileImage) (v1.Image, error) {
	ofi, err := ocitsif.FromFileImage(fi)
	if err != nil {
		return nil, err
	}

	return ofi.Image(SkipCosignMatcher)
}

// SkipCosignMatcher matches all images / indices, except those that are related
// to cosign images, as annotated using the sylabs/oci-tools ref.name convention.
func SkipCosignMatcher(d v1.Descriptor) bool {
	if d.Annotations == nil {
		return true
	}
	refName, ok := d.Annotations[imagespec.AnnotationRefName]
	if !ok {
		return true
	}
	if strings.HasPrefix(refName, sourcesink.CosignPlaceholderRepo) {
		return false
	}
	return true
}
