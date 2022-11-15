// Copyright (c) 2022, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package actions

import (
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"testing"

	"github.com/sylabs/singularity/e2e/internal/e2e"
)

const (
	dockerArchiveURI = "https://s3.amazonaws.com/singularity-ci-public/alpine-docker-save.tar"
	ociArchiveURI    = "https://s3.amazonaws.com/singularity-ci-public/alpine-oci-archive.tar"
)

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

func (c actionTests) actionOciRun(t *testing.T) {
	// Prepare docker-archive source
	dockerArchive, err := getTestTar(dockerArchiveURI)
	if err != nil {
		t.Fatalf("Could not download docker archive test file: %v", err)
	}
	defer os.Remove(dockerArchive)
	// Prepare oci-archive source
	ociArchive, err := getTestTar(ociArchiveURI)
	if err != nil {
		t.Fatalf("Could not download oci archive test file: %v", err)
	}
	defer os.Remove(ociArchive)
	// Prepare oci source (oci directory layout)
	ociLayout := t.TempDir()
	cmd := exec.Command("tar", "-C", ociLayout, "-xf", ociArchive)
	err = cmd.Run()
	if err != nil {
		t.Fatalf("Error extracting oci archive to layout: %v", err)
	}

	tests := []struct {
		name     string
		imageRef string
		exit     int
	}{
		{
			name:     "docker-archive",
			imageRef: "docker-archive:" + dockerArchive,
			exit:     0,
		},
		{
			name:     "oci-archive",
			imageRef: "oci-archive:" + ociArchive,
			exit:     0,
		},
		{
			name:     "oci",
			imageRef: "oci:" + ociLayout,
			exit:     0,
		},
	}

	for _, profile := range []e2e.Profile{e2e.OCIRootProfile, e2e.OCIUserProfile} {
		t.Run(profile.String(), func(t *testing.T) {
			for _, tt := range tests {
				c.env.RunSingularity(
					t,
					e2e.AsSubtest(tt.name),
					e2e.WithProfile(e2e.OCIRootProfile),
					e2e.WithCommand("run"),
					// While we don't support args we are entering a /bin/sh interactively.
					e2e.ConsoleRun(e2e.ConsoleSendLine("exit")),
					e2e.WithArgs(tt.imageRef),
					e2e.ExpectExit(tt.exit),
				)
			}
		})
	}
}
