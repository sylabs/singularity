// Copyright (c) 2023, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package ociimage

import (
	"reflect"
	"runtime"
	"testing"

	"github.com/containers/image/v5/types"
	ggcrv1 "github.com/google/go-containerregistry/pkg/v1"
)

func Test_sysCtxToPlatform(t *testing.T) {
	tests := []struct {
		name   string
		sysCtx *types.SystemContext
		want   ggcrv1.Platform
	}{
		{
			name:   "Default",
			sysCtx: &types.SystemContext{},
			want: ggcrv1.Platform{
				OS:           runtime.GOOS,
				Architecture: runtime.GOARCH,
				Variant:      cpuVariant(),
			},
		},
		{
			name: "OverrideOS",
			sysCtx: &types.SystemContext{
				OSChoice: "myOS",
			},
			want: ggcrv1.Platform{
				OS:           "myOS",
				Architecture: runtime.GOARCH,
				Variant:      cpuVariant(),
			},
		},
		{
			name: "OverrideArchitecture",
			sysCtx: &types.SystemContext{
				ArchitectureChoice: "myArch",
			},
			want: ggcrv1.Platform{
				OS:           runtime.GOOS,
				Architecture: "myArch",
				Variant:      cpuVariant(),
			},
		},
		{
			name: "OverrideVariant",
			sysCtx: &types.SystemContext{
				VariantChoice: "myVariant",
			},
			want: ggcrv1.Platform{
				OS:           runtime.GOOS,
				Architecture: runtime.GOARCH,
				Variant:      "myVariant",
			},
		},
		{
			name: "OverrideAll",
			sysCtx: &types.SystemContext{
				OSChoice:           "myOS",
				ArchitectureChoice: "myArch",
				VariantChoice:      "myVariant",
			},
			want: ggcrv1.Platform{
				OS:           "myOS",
				Architecture: "myArch",
				Variant:      "myVariant",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := sysCtxToPlatform(tt.sysCtx); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("sysCtxToPlatform() = %v, want %v", got, tt.want)
			}
		})
	}
}
