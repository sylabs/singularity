// Copyright (c) 2023, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package ocisif

import (
	"context"
	"testing"

	"github.com/sylabs/singularity/v4/internal/pkg/test"
	ocitest "github.com/sylabs/singularity/v4/internal/pkg/test/tool/oci"
)

const (
	ociSifURI = "https://s3.amazonaws.com/singularity-ci-public/alpine-oci-sif-squashfs.sif"
)

func TestFromImageRef(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}

	test.EnsurePrivilege(t)

	// Prepare oci-sif source
	ociSif, err := ocitest.GetTestImg(ociSifURI)
	if err != nil {
		t.Fatalf("Could not download oci-sif test file: %v", err)
	}

	tests := []struct {
		name     string
		imageRef string
	}{
		{"oci-sif", "oci-sif:" + ociSif},
		{"oci-sif-no-transport", ociSif},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bundleDir := t.TempDir()
			b, err := New(
				OptBundlePath(bundleDir),
				OptImageRef(tt.imageRef),
			)
			if err != nil {
				t.Fatalf("While initializing bundle: %s", err)
			}

			if err := b.Create(context.Background(), nil); err != nil {
				t.Fatalf("While creating bundle: %s", err)
			}

			ocitest.ValidateBundle(t, bundleDir)

			if err := b.Delete(context.Background()); err != nil {
				t.Errorf("While deleting bundle: %s", err)
			}
		})
	}
}
