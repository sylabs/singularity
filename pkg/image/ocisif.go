// Copyright (c) 2018-2023, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package image

import (
	"bytes"
	"fmt"
	"os"

	"github.com/sylabs/sif/v2/pkg/sif"
)

type ociSifFormat struct{}

// initializer performs minimal detection of oci-sif images only.
// It does not populate any information except img.Type.
func (f *ociSifFormat) initializer(img *Image, fi os.FileInfo) error {
	if fi.IsDir() {
		return debugError("not an oci-sif file image")
	}
	b := make([]byte, bufferSize)
	if n, err := img.File.Read(b); err != nil || n != bufferSize {
		return debugErrorf("can't read first %d bytes: %v", bufferSize, err)
	}
	if !bytes.Contains(b, []byte("SIF_MAGIC")) {
		return debugError("SIF magic not found")
	}

	flag := os.O_RDONLY
	if img.Writable {
		flag = os.O_RDWR
	}

	// Load the SIF file
	fimg, err := sif.LoadContainer(img.File,
		sif.OptLoadWithFlag(flag),
		sif.OptLoadWithCloseOnUnload(false),
	)
	if err != nil {
		return err
	}
	defer fimg.UnloadContainer()

	// It's a SIF, but an OCI-SIF.
	if _, err := fimg.GetDescriptor(sif.WithDataType(sif.DataOCIRootIndex)); err != nil {
		return debugErrorf("image is not an OCI-SIF")
	}

	img.Type = OCISIF

	return nil
}

func (f *ociSifFormat) openMode(bool) int {
	return os.O_RDONLY
}

func (f *ociSifFormat) lock(*Image) error {
	return nil
}

// IsOCISIF receives a path to an image file and returns a boolean indicating
// whether the file is an OCI-SIF image.
func IsOCISIF(filename string) (bool, error) {
	img, err := Init(filename, false)
	if err != nil {
		return false, fmt.Errorf("could not open image %s: %s", filename, err)
	}
	defer img.File.Close()

	return (img.Type == OCISIF), nil
}
