// Copyright (c) 2023, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package ociplatform

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
				Variant:      CPUVariant(),
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
				Variant:      CPUVariant(),
			},
		},
		{
			name: "OverrideArchitecture",
			sysCtx: &types.SystemContext{
				ArchitectureChoice: "myarch",
			},
			want: ggcrv1.Platform{
				OS:           runtime.GOOS,
				Architecture: "myarch",
				Variant:      CPUVariant(),
			},
		},
		{
			name: "OverrideVariant",
			sysCtx: &types.SystemContext{
				VariantChoice: "myvariant",
			},
			want: ggcrv1.Platform{
				OS:           runtime.GOOS,
				Architecture: runtime.GOARCH,
				Variant:      "myvariant",
			},
		},
		{
			name: "OverrideAll",
			sysCtx: &types.SystemContext{
				OSChoice:           "myos",
				ArchitectureChoice: "myarch",
				VariantChoice:      "myvariant",
			},
			want: ggcrv1.Platform{
				OS:           "myos",
				Architecture: "myarch",
				Variant:      "myvariant",
			},
		},
		{
			name: "Normalize linux/arm64/v8",
			sysCtx: &types.SystemContext{
				OSChoice:           "linux",
				ArchitectureChoice: "arm64",
				VariantChoice:      "v8",
			},
			want: ggcrv1.Platform{
				OS:           "linux",
				Architecture: "arm64",
				Variant:      "",
			},
		},
		{
			name: "Normalize linux/aarch64",
			sysCtx: &types.SystemContext{
				OSChoice:           "linux",
				ArchitectureChoice: "aarch64",
				VariantChoice:      "",
			},
			want: ggcrv1.Platform{
				OS:           "linux",
				Architecture: "arm64",
				Variant:      "",
			},
		},
		{
			name: "Normalize linux/arm32",
			sysCtx: &types.SystemContext{
				OSChoice:           "linux",
				ArchitectureChoice: "arm",
				VariantChoice:      "",
			},
			want: ggcrv1.Platform{
				OS:           "linux",
				Architecture: "arm",
				Variant:      "v7",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SysCtxToPlatform(tt.sysCtx); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("sysCtxToPlatform() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPlatformFromString(t *testing.T) {
	tests := []struct {
		name    string
		plat    string
		want    *ggcrv1.Platform
		wantErr bool
	}{
		{
			name:    "BadString",
			plat:    "os/arch/variant/extra",
			want:    nil,
			wantErr: true,
		},
		{
			name:    "UnsupportedWindows",
			plat:    "windows/amd64",
			want:    nil,
			wantErr: true,
		},
		{
			name:    "GoodAMD64",
			plat:    "linux/amd64",
			want:    &ggcrv1.Platform{OS: "linux", Architecture: "amd64", Variant: ""},
			wantErr: false,
		},
		{
			name:    "NormalizeARM",
			plat:    "linux/arm",
			want:    &ggcrv1.Platform{OS: "linux", Architecture: "arm", Variant: "v7"},
			wantErr: false,
		},
		{
			name:    "NormalizeARM64/v8",
			plat:    "linux/arm64/v8",
			want:    &ggcrv1.Platform{OS: "linux", Architecture: "arm64", Variant: ""},
			wantErr: false,
		},
		{
			name:    "NormalizeAARCH64",
			plat:    "linux/aarch64",
			want:    &ggcrv1.Platform{OS: "linux", Architecture: "arm64", Variant: ""},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := PlatformFromString(tt.plat)
			if (err != nil) != tt.wantErr {
				t.Errorf("PlatformFromString() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("PlatformFromString() = %v, want %v", got, tt.want)
			}
		})
	}
}
