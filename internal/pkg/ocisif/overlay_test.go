// Copyright (c) 2024 Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.
package ocisif

import (
	"path/filepath"
	"testing"

	ggcrv1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/random"
)

func TestHasOverlay(t *testing.T) {
	tests := []struct {
		name     string
		genImage func(t *testing.T) ggcrv1.Image
		want     bool
		wantErr  bool
	}{
		{
			name: "NoOverlay",
			genImage: func(t *testing.T) ggcrv1.Image {
				im, err := random.Image(1024, 3)
				if err != nil {
					t.Fatal(err)
				}
				return im
			},
			want:    false,
			wantErr: false,
		},
		{
			name: "HasOverlay",
			genImage: func(t *testing.T) ggcrv1.Image {
				im, err := random.Image(1024, 3)
				if err != nil {
					t.Fatal(err)
				}
				l, err := random.Layer(1024, Ext3LayerMediaType)
				if err != nil {
					t.Fatal(err)
				}
				im, err = mutate.AppendLayers(im, l)
				if err != nil {
					t.Fatal(err)
				}
				return im
			},
			want:    true,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			imgFile := filepath.Join(t.TempDir(), "image.sif")
			iw, err := NewImageWriter(tt.genImage(t), imgFile, t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			if err := iw.Write(); err != nil {
				t.Fatal(err)
			}

			got, _, err := HasOverlay(imgFile)

			if got != tt.want {
				t.Errorf("Expected %v, got %v", tt.want, got)
			}

			if err != nil && !tt.wantErr {
				t.Errorf("Unexpected error: %v", err)
			}

			if err == nil && tt.wantErr {
				t.Error("Expected error, but no error returned.")
			}
		})
	}
}

func TestAddOverlay(t *testing.T) {
	tests := []struct {
		name        string
		overlayPath string
		wantErr     bool
		wantOverlay bool
	}{
		{
			name:        "OverlayNotExist",
			overlayPath: "does/not/exist",
			wantErr:     true,
		},
		{
			name:        "OverlayWrongFS",
			overlayPath: "../../../test/images/squashfs-for-overlay.img",
			wantErr:     true,
			wantOverlay: false,
		},
		{
			name:        "OverlayOK",
			overlayPath: "../../../test/images/extfs-for-overlay.img",
			wantErr:     false,
			wantOverlay: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			imgFile := filepath.Join(t.TempDir(), "image.sif")
			im, err := random.Image(1024, 3)
			if err != nil {
				t.Fatal(err)
			}
			iw, err := NewImageWriter(im, imgFile, t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			if err := iw.Write(); err != nil {
				t.Fatal(err)
			}

			err = AddOverlay(imgFile, tt.overlayPath)
			if err != nil && !tt.wantErr {
				t.Errorf("Unexpected error: %v", err)
			}

			if err == nil && tt.wantErr {
				t.Error("Expected error, but no error returned.")
			}

			hasOverlay, _, err := HasOverlay(imgFile)
			if err != nil {
				t.Fatal(err)
			}
			if hasOverlay != tt.wantOverlay {
				t.Errorf("Image has overlay: %v, want overlay: %v", hasOverlay, tt.wantOverlay)
			}
		})
	}
}
