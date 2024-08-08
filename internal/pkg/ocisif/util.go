// Copyright (c) 2024, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package ocisif

import (
	"fmt"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	ocitsif "github.com/sylabs/oci-tools/pkg/sif"
	"github.com/sylabs/sif/v2/pkg/sif"
)

// GetSingleImage returns a v1.Image from an OCI-SIF, that must contain a single
// image.
func GetSingleImage(fi *sif.FileImage) (v1.Image, error) {
	ii, err := ocitsif.ImageIndexFromFileImage(fi)
	if err != nil {
		return nil, fmt.Errorf("while obtaining image index: %w", err)
	}
	ix, err := ii.IndexManifest()
	if err != nil {
		return nil, fmt.Errorf("while obtaining index manifest: %w", err)
	}

	// One image only.
	if len(ix.Manifests) != 1 {
		return nil, fmt.Errorf("only single image data containers are supported, found %d images", len(ix.Manifests))
	}
	imageDigest := ix.Manifests[0].Digest
	return ii.Image(imageDigest)
}
