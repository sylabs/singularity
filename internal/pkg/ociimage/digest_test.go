// Copyright (c) 2018-2023, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.
package ociimage

import (
	"runtime"
	"testing"

	ggcrv1 "github.com/google/go-containerregistry/pkg/v1"
	ggcrempty "github.com/google/go-containerregistry/pkg/v1/empty"
	ggcrmutate "github.com/google/go-containerregistry/pkg/v1/mutate"
	ggcrrandom "github.com/google/go-containerregistry/pkg/v1/random"
	"github.com/sylabs/singularity/v4/internal/pkg/ociplatform"
	"gotest.tools/v3/assert"
)

func imageWithManifest(t *testing.T) (rawManifest []byte, imageDigest ggcrv1.Hash) {
	im, err := ggcrrandom.Image(1024, 3)
	if err != nil {
		t.Fatal(err)
	}
	id, err := im.Digest()
	if err != nil {
		t.Fatal(err)
	}
	rm, err := im.RawManifest()
	if err != nil {
		t.Fatal(err)
	}
	return rm, id
}

func imageWithIndex(t *testing.T) (rawIndex []byte, imageDigest ggcrv1.Hash) {
	im, err := ggcrrandom.Image(1024, 3)
	if err != nil {
		t.Fatal(err)
	}
	ii := ggcrmutate.AppendManifests(ggcrempty.Index, ggcrmutate.IndexAddendum{
		Add: im,
		Descriptor: ggcrv1.Descriptor{
			Platform: &ggcrv1.Platform{
				OS:           runtime.GOOS,
				Architecture: runtime.GOARCH,
				Variant:      ociplatform.CPUVariant(),
			},
		},
	})
	id, err := im.Digest()
	if err != nil {
		t.Fatal(err)
	}
	ri, err := ii.RawManifest()
	if err != nil {
		t.Fatal(err)
	}
	return ri, id
}

func Test_digestFromManifestOrIndex(t *testing.T) {
	manifest, manifestImageDigest := imageWithManifest(t)
	index, indexImageDigest := imageWithIndex(t)

	tests := []struct {
		name             string
		transportOptions *TransportOptions
		manifestOrIndex  []byte
		want             ggcrv1.Hash
		wantErr          bool
	}{
		{
			name:             "ImageManifestDefaultSysCtx",
			transportOptions: &TransportOptions{},
			manifestOrIndex:  manifest,
			want:             manifestImageDigest,
			wantErr:          false,
		},
		{
			name:             "ImageIndexDefaultSysCtx",
			transportOptions: &TransportOptions{},
			manifestOrIndex:  index,
			want:             indexImageDigest,
			wantErr:          false,
		},
		{
			name: "ImageIndexExplicitSysCtx",
			transportOptions: &TransportOptions{
				Platform: ggcrv1.Platform{
					OS:           runtime.GOOS,
					Architecture: runtime.GOARCH,
					Variant:      ociplatform.CPUVariant(),
				},
			},
			manifestOrIndex: index,
			want:            indexImageDigest,
			wantErr:         false,
		},
		{
			name: "ImageIndexBadOS",
			transportOptions: &TransportOptions{
				Platform: ggcrv1.Platform{
					OS:           "myOS",
					Architecture: runtime.GOARCH,
					Variant:      ociplatform.CPUVariant(),
				},
			},
			manifestOrIndex: index,
			want:            ggcrv1.Hash{},
			wantErr:         true,
		},
		{
			name: "ImageIndexBadArch",
			transportOptions: &TransportOptions{
				Platform: ggcrv1.Platform{
					OS:           runtime.GOOS,
					Architecture: "myArch",
					Variant:      ociplatform.CPUVariant(),
				},
			},
			manifestOrIndex: index,
			want:            ggcrv1.Hash{},
			wantErr:         true,
		},
		{
			name: "ImageIndexBadVariant",
			transportOptions: &TransportOptions{
				Platform: ggcrv1.Platform{
					OS:           runtime.GOOS,
					Architecture: runtime.GOARCH,
					Variant:      "myVariant",
				},
			},
			manifestOrIndex: index,
			want:            ggcrv1.Hash{},
			wantErr:         true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := digestFromManifestOrIndex(tt.transportOptions, tt.manifestOrIndex)
			assert.Equal(t, tt.want, got)
			if (err != nil) != tt.wantErr {
				t.Errorf("digestFromManifestOrIndex() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
		})
	}
}
