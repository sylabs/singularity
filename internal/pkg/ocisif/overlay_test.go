// Copyright (c) 2024 Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.
package ocisif

import (
	"os"
	"path/filepath"
	"testing"

	ggcrv1 "github.com/google/go-containerregistry/pkg/v1"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/random"
	"github.com/sylabs/sif/v2/pkg/sif"
)

const extfsOverlayPath = "../../../test/images/extfs-for-overlay.img"

var extfsOverlayDigest = v1.Hash{
	Algorithm: "sha256",
	Hex:       "be6a627e7c4226eacd50f530ce82e83f0efb8e0f7d28a9bcee9d6b4bc208cfd1",
}

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

func randomImage(t *testing.T, size int64, layers int) string {
	imgFile := filepath.Join(t.TempDir(), "image.oci.sif")

	addenda := []mutate.Addendum{}
	for i := 0; i < layers; i++ {
		layer, err := random.Layer(size, SquashfsLayerMediaType)
		if err != nil {
			t.Fatal(err)
		}
		addenda = append(addenda, mutate.Addendum{
			Layer: layer,
		})
	}

	im, err := mutate.Append(empty.Image, addenda...)
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
	return imgFile
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
			overlayPath: extfsOverlayPath,
			wantErr:     false,
			wantOverlay: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			imgFile := randomImage(t, 1024, 3)

			err := AddOverlay(imgFile, tt.overlayPath)
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

// Writes 8 bytes of 0xFF, 2048 bytes inside the overlay in imgFile. This is
// beyond the area checked for a valid ext3 header.
func modifyOverlayFF(t *testing.T, imgFile string) {
	t.Helper()
	ok, offset, err := HasOverlay(imgFile)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatalf("no overlay in %s", imgFile)
	}

	f, err := os.OpenFile(imgFile, os.O_WRONLY, 0o755)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	_, err = f.WriteAt([]byte{0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF}, offset+2048)
	if err != nil {
		t.Fatal(err)
	}
}

// Digest of extfsOverlayPath, modified by modifyOverlayFF above
var modifiedOverlayDigest = v1.Hash{
	Algorithm: "sha256",
	Hex:       "1d2b7729ac057ceb9a3b301eec078ef89df5ea54188973e9a9b5f0f9aedf0615",
}

func TestSyncOverlay(t *testing.T) {
	imgFile := randomImage(t, 1024, 1)
	if err := AddOverlay(imgFile, extfsOverlayPath); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name          string
		modifyOverlay func(t *testing.T, imgFile string)
		expectDigest  v1.Hash
	}{
		{
			name:         "Unmodified",
			expectDigest: extfsOverlayDigest,
		},
		{
			name:          "Modified",
			modifyOverlay: modifyOverlayFF,
			expectDigest:  modifiedOverlayDigest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.modifyOverlay != nil {
				tt.modifyOverlay(t, imgFile)
			}

			if err := SyncOverlay(imgFile); err != nil {
				t.Fatalf("unexpected error from SyncOverlay: %v", err)
			}

			fi, err := sif.LoadContainerFromPath(imgFile)
			if err != nil {
				t.Fatal(err)
			}
			defer fi.UnloadContainer()

			// Must have a descriptor with the expected synced digest.
			if _, err := fi.GetDescriptor(sif.WithOCIBlobDigest(tt.expectDigest)); err != nil {
				t.Errorf("no descriptor found for %q: %v", tt.expectDigest, err)
			}

			// Final overlay layer must have the expected digest.
			img, err := GetSingleImage(fi)
			if err != nil {
				t.Fatal(err)
			}
			layers, err := img.Layers()
			if err != nil {
				t.Fatal(err)
			}
			fld, err := layers[len(layers)-1].Digest()
			if err != nil {
				t.Fatal(err)
			}
			if fld != tt.expectDigest {
				t.Errorf("final layer digest is %q, expected %q", fld, tt.expectDigest)
			}
		})
	}
}

func TestSealOverlay(t *testing.T) {
	origLayers := 3
	imgFile := randomImage(t, 1024, origLayers)

	// Seal image with no overlay = error
	err := SealOverlay(imgFile, "")
	if err == nil {
		t.Error("Unexpected success sealing OCI-SIF without overlay.")
	}

	if err := AddOverlay(imgFile, extfsOverlayPath); err != nil {
		t.Fatal(err)
	}

	// Seal image with overlay
	err = SealOverlay(imgFile, "")
	if err != nil {
		t.Errorf("Unexpected error sealing OCI-SIF with overlay: %v", err)
	}

	// After sealing there is no overlay.
	hasOverlay, _, err := HasOverlay(imgFile)
	if err != nil {
		t.Fatal(err)
	}
	if hasOverlay {
		t.Error("Overlay found after OCI-SIF was sealed.")
	}

	// After sealing there are 4 squashfs layers.
	fi, err := sif.LoadContainerFromPath(imgFile)
	if err != nil {
		t.Error(err)
	}
	defer fi.UnloadContainer()
	img, err := GetSingleImage(fi)
	if err != nil {
		t.Error(err)
	}

	layers, err := img.Layers()
	if err != nil {
		t.Error(err)
	}
	if len(layers) != origLayers+1 {
		t.Errorf("Expected %d layers, found %d", origLayers+1, len(layers))
	}

	for i, l := range layers {
		mt, err := l.MediaType()
		if err != nil {
			t.Error(err)
		}
		if mt != SquashfsLayerMediaType {
			t.Errorf("Layer %d is %s. Expected %s.", i, mt, SquashfsLayerMediaType)
		}
	}
}
