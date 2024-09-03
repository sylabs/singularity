// Copyright (c) 2024 Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.
package ocisif

import (
	"archive/tar"
	"bytes"
	"io"
	"path/filepath"
	"strconv"
	"testing"

	ggcrv1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
	useragent "github.com/sylabs/singularity/v4/pkg/util/user-agent"
)

func testImage(t *testing.T) ggcrv1.Image {
	addenda := []mutate.Addendum{}

	for i := 0; i < 3; i++ {
		filename := "file" + strconv.Itoa(i)
		content := []byte("LAYER " + strconv.Itoa(i))

		var testTar bytes.Buffer
		tw := tar.NewWriter(&testTar)
		if err := tw.WriteHeader(&tar.Header{
			Name:     filename,
			Size:     int64(len(content)),
			Typeflag: tar.TypeReg,
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write(content); err != nil {
			t.Fatal(err)
		}
		if err := tw.Close(); err != nil {
			t.Fatal(err)
		}

		opener := func() (io.ReadCloser, error) {
			return io.NopCloser(&testTar), nil
		}
		layer, err := tarball.LayerFromOpener(opener)
		if err != nil {
			t.Fatal(err)
		}

		addenda = append(addenda, mutate.Addendum{Layer: layer})
	}

	img, err := mutate.Append(empty.Image, addenda...)
	if err != nil {
		t.Fatal(err)
	}
	return img
}

func TestNewImageWriter(t *testing.T) {
	useragent.InitValue("TestNewImageWriter", "0.0.0")
	tmpDir := t.TempDir()
	tImg := testImage(t)

	tests := []struct {
		name      string
		srcImg    ggcrv1.Image
		dest      string
		workDir   string
		opts      []ImageWriterOpt
		wantError bool
	}{
		{
			name:      "NoWorkDir",
			srcImg:    tImg,
			dest:      filepath.Join(tmpDir, "dest.oci.sif"),
			workDir:   "",
			wantError: true,
		},
		{
			name:      "NoDest",
			srcImg:    tImg,
			dest:      "",
			workDir:   tmpDir,
			wantError: true,
		},
		{
			name:      "ValidNoOpts",
			srcImg:    tImg,
			dest:      filepath.Join(tmpDir, "dest.oci.sif"),
			workDir:   tmpDir,
			wantError: false,
		},
		{
			name:      "ValidOpts",
			srcImg:    tImg,
			dest:      filepath.Join(tmpDir, "dest.oci.sif"),
			workDir:   tmpDir,
			opts:      []ImageWriterOpt{WithSquash(true), WithSquashFSLayers(true)},
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewImageWriter(tt.srcImg, tt.dest, tt.workDir, tt.opts...)
			if !tt.wantError && err != nil {
				t.Fatal(err)
			}

			if (err != nil) != tt.wantError {
				t.Fatalf("got error %v, want error %v", err, tt.wantError)
			}
		})
	}
}

func TestWrite(t *testing.T) {
	useragent.InitValue("TestNewImageWriter", "0.0.0")
	tmpDir := t.TempDir()
	tImg := testImage(t)

	tests := []struct {
		name    string
		srcImg  ggcrv1.Image
		dest    string
		workDir string
		opts    []ImageWriterOpt
	}{
		{
			name:    "Defaults",
			srcImg:  tImg,
			dest:    filepath.Join(tmpDir, "default.oci.sif"),
			workDir: tmpDir,
		},
		{
			name:    "Squash",
			srcImg:  tImg,
			dest:    filepath.Join(tmpDir, "default.oci.sif"),
			workDir: tmpDir,
			opts:    []ImageWriterOpt{WithSquash(true)},
		},
		{
			name:    "SquashFS",
			srcImg:  tImg,
			dest:    filepath.Join(tmpDir, "default.oci.sif"),
			workDir: tmpDir,
			opts:    []ImageWriterOpt{WithSquashFSLayers(true)},
		},
		{
			name:    "SquashSquashFS",
			srcImg:  tImg,
			dest:    filepath.Join(tmpDir, "default.oci.sif"),
			workDir: tmpDir,
			opts:    []ImageWriterOpt{WithSquash(true), WithSquashFSLayers(true)},
		},
		{
			name:    "ArtifactType",
			srcImg:  tImg,
			dest:    filepath.Join(tmpDir, "default.oci.sif"),
			workDir: tmpDir,
			opts:    []ImageWriterOpt{WithArtifactType(DataContainerArtifactType)},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := NewImageWriter(tt.srcImg, tt.dest, tt.workDir, tt.opts...)
			if err != nil {
				t.Fatal(err)
			}
			if err := p.Write(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
