// Copyright 2023-2025 Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package oci

import (
	"path/filepath"
	"testing"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/layout"
	"github.com/google/go-containerregistry/pkg/v1/partial"
	"github.com/sylabs/oci-tools/pkg/sif"
)

// ImagePath returns the path to the image in the corpus with the specified name.
func (c *Corpus) ImagePath(name string) string {
	return filepath.Join(c.dir, "images", name)
}

// rootIndex returns a v1.ImageIndex corresponding to the index.json from the OCI Image Layout
// with the specified name in the corpus.
func (c *Corpus) rootIndex(tb testing.TB, name string) v1.ImageIndex {
	tb.Helper()

	ii, err := layout.ImageIndexFromPath(c.ImagePath(name))
	if err != nil {
		tb.Fatalf("failed to get image index: %v", err)
	}

	return ii
}

// ImageIndex returns a v1.ImageIndex corresponding to the OCI Image Layout with the specified name
// in the corpus.
func (c *Corpus) ImageIndex(tb testing.TB, name string) v1.ImageIndex {
	tb.Helper()

	iis, err := partial.FindIndexes(c.rootIndex(tb, name), matchAll)
	if err != nil {
		tb.Fatalf("failed to find indexes: %v", err)
	}

	if got := len(iis); got != 1 {
		tb.Fatalf("got %v manifests, expected 1", got)
	}

	return iis[0]
}

func matchAll(v1.Descriptor) bool { return true }

// Image returns a v1.Image corresponding to the OCI Image Layout with the specified name in the
// corpus.
func (c *Corpus) Image(tb testing.TB, name string) v1.Image {
	tb.Helper()

	ims, err := partial.FindImages(c.rootIndex(tb, name), matchAll)
	if err != nil {
		tb.Fatalf("failed to find images: %v", err)
	}

	if got := len(ims); got != 1 {
		tb.Fatalf("got %v manifests, expected 1", got)
	}

	return ims[0]
}

// OCILayout returns a temporary OCI Image Layout for the test to use, populated from the OCI Image
// Layout with the specified name in the corpus. The directory is automatically removed when the
// test and all its subtests complete.
func (c *Corpus) OCILayout(tb testing.TB, name string) string {
	tb.Helper()

	lp, err := layout.Write(tb.TempDir(), c.ImageIndex(tb, name))
	if err != nil {
		tb.Fatalf("failed to write layout: %v", err)
	}

	return string(lp)
}

// SIF returns a temporary SIF for the test to use, populated from the OCI Image Layout with the
// specified name in the corpus. The SIF is automatically removed when the test and all its
// subtests complete.
func (c *Corpus) SIF(tb testing.TB, name string, opt ...sif.WriteOpt) string {
	tb.Helper()

	path := filepath.Join(tb.TempDir(), "image.sif")

	if err := sif.Write(path, c.rootIndex(tb, name), opt...); err != nil {
		tb.Fatalf("failed to write SIF: %v", err)
	}

	return path
}
