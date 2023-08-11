// Copyright (c) 2018-2023, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.
package ociimage

import (
	"runtime"
	"testing"

	"github.com/containers/image/v5/types"
	ggcrv1 "github.com/google/go-containerregistry/pkg/v1"
	ggcrempty "github.com/google/go-containerregistry/pkg/v1/empty"
	ggcrmutate "github.com/google/go-containerregistry/pkg/v1/mutate"
	ggcrrandom "github.com/google/go-containerregistry/pkg/v1/random"
	"github.com/opencontainers/go-digest"
	"github.com/sylabs/singularity/v4/internal/pkg/ociplatform"
	"gotest.tools/v3/assert"
)

func imageWithManifest(t *testing.T) (rawManifest []byte, imageDigest digest.Digest) {
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
	return rm, digest.Digest(id.String())
}

func imageWithIndex(t *testing.T) (rawIndex []byte, imageDigest digest.Digest) {
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
	return ri, digest.Digest(id.String())
}

func Test_digestFromManifestOrIndex(t *testing.T) {
	manifest, manifestImageDigest := imageWithManifest(t)
	index, indexImageDigest := imageWithIndex(t)

	tests := []struct {
		name            string
		sysCtx          *types.SystemContext
		manifestOrIndex []byte
		want            digest.Digest
		wantErr         bool
	}{
		{
			name:            "ImageManifestDefaultSysCtx",
			sysCtx:          &types.SystemContext{},
			manifestOrIndex: manifest,
			want:            manifestImageDigest,
			wantErr:         false,
		},
		{
			name:            "ImageIndexDefaultSysCtx",
			sysCtx:          &types.SystemContext{},
			manifestOrIndex: index,
			want:            indexImageDigest,
			wantErr:         false,
		},
		{
			name: "ImageIndexExplicitSysCtx",
			sysCtx: &types.SystemContext{
				OSChoice:           runtime.GOOS,
				ArchitectureChoice: runtime.GOARCH,
				VariantChoice:      ociplatform.CPUVariant(),
			},
			manifestOrIndex: index,
			want:            indexImageDigest,
			wantErr:         false,
		},
		{
			name: "ImageIndexBadOS",
			sysCtx: &types.SystemContext{
				OSChoice:           "myOS",
				ArchitectureChoice: runtime.GOARCH,
				VariantChoice:      ociplatform.CPUVariant(),
			},
			manifestOrIndex: index,
			want:            "",
			wantErr:         true,
		},
		{
			name: "ImageIndexBadArch",
			sysCtx: &types.SystemContext{
				OSChoice:           runtime.GOOS,
				ArchitectureChoice: "myArch",
				VariantChoice:      ociplatform.CPUVariant(),
			},
			manifestOrIndex: index,
			want:            "",
			wantErr:         true,
		},
		{
			name: "ImageIndexBadVariant",
			sysCtx: &types.SystemContext{
				OSChoice:           runtime.GOOS,
				ArchitectureChoice: runtime.GOARCH,
				VariantChoice:      "myVariant",
			},
			manifestOrIndex: index,
			want:            "",
			wantErr:         true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := digestFromManifestOrIndex(tt.sysCtx, tt.manifestOrIndex)
			assert.Equal(t, tt.want, got)
			if (err != nil) != tt.wantErr {
				t.Errorf("digestFromManifestOrIndex() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
		})
	}
}
