// Copyright (c) 2024, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package ocisif

import (
	v1 "github.com/google/go-containerregistry/pkg/v1"
	ocitsif "github.com/sylabs/oci-tools/pkg/sif"
	"github.com/sylabs/sif/v2/pkg/sif"
)

// GetSingleImage returns a v1.Image from an OCI-SIF, that must contain a single
// image.
func GetSingleImage(fi *sif.FileImage) (v1.Image, error) {
	ofi, err := ocitsif.FromFileImage(fi)
	if err != nil {
		return nil, err
	}

	return ofi.Image(nil)
}
