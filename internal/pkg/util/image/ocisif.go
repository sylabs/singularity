// Copyright (c) 2023, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package image

import (
	"fmt"

	pkgimage "github.com/sylabs/singularity/v4/pkg/image"
)

func IsOCISIF(filename string) (bool, error) {
	img, err := pkgimage.Init(filename, false)
	if err != nil {
		return false, fmt.Errorf("could not open image %s: %s", filename, err)
	}
	defer img.File.Close()

	return (img.Type == pkgimage.OCISIF), nil
}
