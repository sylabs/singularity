// Copyright (c) 2022-2023, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package native

import (
	"context"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"testing"

	"github.com/opencontainers/runtime-tools/validate"
	"github.com/sylabs/singularity/internal/pkg/cache"
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

func getTestTar(url string) (path string, err error) {
	dl, err := os.CreateTemp("", "oci-test")
	if err != nil {
		log.Fatal(err)
	}
	defer dl.Close()

	r, err := http.Get(url)
	if err != nil {
		return "", err
	}
	defer r.Body.Close()

	_, err = io.Copy(dl, r.Body)
	if err != nil {
		return "", err
	}

	return dl.Name(), nil
}

func validateBundle(t *testing.T, bundlePath string) {
	v, err := validate.NewValidatorFromPath(bundlePath, false, "linux")
	if err != nil {
		t.Errorf("Could not create bundle validator: %v", err)
	}
	if err := v.CheckAll(); err != nil {
		t.Errorf("Bundle not valid: %v", err)
	}
}

func TestFromImageRef(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}

	// Prepare docker-archive source
	dockerArchive, err := getTestTar(dockerArchiveURI)
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
	ociArchive, err := getTestTar(ociArchiveURI)
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
				t.Fatalf("While creating bundle: %s", err)
			}

			validateBundle(t, bundleDir)
		})
	}
}
