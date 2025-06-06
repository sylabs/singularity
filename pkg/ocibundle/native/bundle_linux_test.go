// Copyright (c) 2022-2023, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package native

import (
	"context"
	"os"
	"os/exec"
	"reflect"
	"testing"

	imgspecv1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/sylabs/singularity/v4/internal/pkg/cache"
	"github.com/sylabs/singularity/v4/internal/pkg/test"
	ocitest "github.com/sylabs/singularity/v4/internal/pkg/test/tool/oci"
)

const (
	dockerURI         = "docker://alpine"
	dockerArchiveURI  = "https://s3.amazonaws.com/singularity-ci-public/alpine-docker-save.tar"
	ociArchiveURI     = "https://s3.amazonaws.com/singularity-ci-public/alpine-oci-archive.tar"
	dockerDaemonImage = "alpine:latest"
)

func setupCache(t *testing.T) *cache.Handle {
	dir := t.TempDir()
	h, err := cache.New(cache.Config{ParentDir: dir})
	if err != nil {
		t.Fatalf("failed to create an image cache handle: %s", err)
	}
	return h
}

func TestFromImageRef(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}

	test.EnsurePrivilege(t)

	// Prepare docker-archive source
	dockerArchive, err := ocitest.GetTestImg(dockerArchiveURI)
	if err != nil {
		t.Fatalf("Could not download docker archive test file: %v", err)
	}
	defer os.Remove(dockerArchive)
	// Prepare docker-daemon source
	hasDocker := false
	cmd := exec.Command("docker", "ps")
	err = cmd.Run()
	if err == nil {
		hasDocker = true
		cmd = exec.Command("docker", "pull", dockerDaemonImage)
		err = cmd.Run()
		if err != nil {
			t.Fatalf("could not docker pull %s %v", dockerDaemonImage, err)
			return
		}
	}
	// Prepare oci-archive source
	ociArchive, err := ocitest.GetTestImg(ociArchiveURI)
	if err != nil {
		t.Fatalf("Could not download oci archive test file: %v", err)
	}
	defer os.Remove(ociArchive)
	// Prepare oci source (oci directory layout)
	ociLayout := t.TempDir()
	cmd = exec.Command("tar", "-C", ociLayout, "-xf", ociArchive)
	err = cmd.Run()
	if err != nil {
		t.Fatalf("Error extracting oci archive to layout: %v", err)
	}

	tests := []struct {
		name        string
		imageRef    string
		needsDocker bool
	}{
		{"docker", dockerURI, false},
		{"docker-archive", "docker-archive:" + dockerArchive, false},
		{"docker-daemon", "docker-daemon:" + dockerDaemonImage, true},
		{"oci-archive", "oci-archive:" + ociArchive, false},
		{"oci", "oci:" + ociLayout, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.needsDocker && !hasDocker {
				t.Skipf("docker not available")
			}
			bundleDir := t.TempDir()
			b, err := New(
				OptBundlePath(bundleDir),
				OptImageRef(tt.imageRef),
				OptImgCache(setupCache(t)),
			)
			if err != nil {
				t.Fatalf("While initializing bundle: %s", err)
			}

			if err := b.Create(context.Background(), nil); err != nil {
				t.Errorf("While creating bundle: %s", err)
			}

			if b.ImageSpec() == nil || reflect.DeepEqual(b.ImageSpec(), imgspecv1.Image{}) {
				t.Errorf("ImageSpec is nil / empty.")
			}

			ocitest.ValidateBundle(t, bundleDir)

			if err := b.Delete(context.Background()); err != nil {
				t.Errorf("While deleting bundle: %s", err)
			}
		})
	}
}
