// Copyright (c) 2019-2021, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package image

import (
	"bytes"
	"os"
	"testing"

	"github.com/sylabs/sif/v2/pkg/sif"
)

//nolint:dupl
func TestOCISIFInitializer(t *testing.T) {
	ociMinimal := func() (sif.DescriptorInput, error) {
		return sif.NewDescriptorInput(sif.DataOCIRootIndex, bytes.NewBufferString("{}\n"))
	}

	tests := []struct {
		name               string
		path               string
		writable           bool
		expectedSuccess    bool
		expectedPartitions int
		expectedSections   int
	}{
		{
			name:               "OCISIF",
			path:               createSIF(t, false, ociMinimal),
			writable:           false,
			expectedSuccess:    true,
			expectedPartitions: 0,
			expectedSections:   0,
		},
		{
			name:               "Empty",
			path:               createSIF(t, false),
			writable:           false,
			expectedSuccess:    false,
			expectedPartitions: 0,
			expectedSections:   0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var err error

			ociSifFmt := new(ociSifFormat)
			mode := ociSifFmt.openMode(tt.writable)

			img := &Image{
				Path: tt.path,
				Name: tt.path,
			}

			img.Writable = tt.writable
			img.File, err = os.OpenFile(tt.path, mode, 0)
			if err != nil {
				t.Fatalf("cannot open image's file: %s\n", err)
			}
			defer img.File.Close()

			fileinfo, err := img.File.Stat()
			if err != nil {
				t.Fatalf("cannot stat the image file: %s\n", err)
			}

			err = ociSifFmt.initializer(img, fileinfo)
			os.Remove(tt.path)

			if (err == nil) != tt.expectedSuccess {
				t.Fatalf("got error %v, expect success %v", err, tt.expectedSuccess)
			} else if tt.expectedPartitions != len(img.Partitions) {
				t.Fatalf("unexpected partitions number: %d instead of %d", len(img.Partitions), tt.expectedPartitions)
			} else if tt.expectedSections != len(img.Sections) {
				t.Fatalf("unexpected sections number: %d instead of %d", len(img.Sections), tt.expectedSections)
			}
		})
	}
}

func TestOCISIFOpenMode(t *testing.T) {
	var ociSifFmt ociSifFormat

	if ociSifFmt.openMode(true) != os.O_RDONLY {
		t.Fatal("openMode(true) returned the wrong value")
	}
	if ociSifFmt.openMode(false) != os.O_RDONLY {
		t.Fatal("openMode(false) returned the wrong value")
	}
}
