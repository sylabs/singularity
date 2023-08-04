// Copyright (c) 2022, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package oci

import (
	"os/user"
	"reflect"
	"testing"

	"github.com/sylabs/singularity/v4/internal/pkg/runtime/launcher"
	"github.com/sylabs/singularity/v4/internal/pkg/test"
	"github.com/sylabs/singularity/v4/internal/pkg/util/fs/fuse"
	"github.com/sylabs/singularity/v4/pkg/util/singularityconf"
)

func TestNewLauncher(t *testing.T) {
	test.DropPrivilege(t)
	defer test.ResetPrivilege(t)

	sc, err := singularityconf.GetConfig(nil)
	if err != nil {
		t.Fatalf("while initializing singularityconf: %s", err)
	}
	singularityconf.SetCurrentConfig(sc)

	u, err := user.Current()
	if err != nil {
		t.Fatalf("while getting current user: %s", err)
	}

	tests := []struct {
		name    string
		opts    []launcher.Option
		want    *Launcher
		wantErr bool
	}{
		{
			name: "default",
			want: &Launcher{
				cfg:                     launcher.Options{WritableTmpfs: true},
				singularityConf:         sc,
				homeHost:                u.HomeDir,
				homeSrc:                 "",
				homeDest:                u.HomeDir,
				imageMountsByImagePath:  make(map[string]*fuse.ImageMount),
				imageMountsByMountpoint: make(map[string]*fuse.ImageMount),
			},
		},
		{
			name: "homeDest",
			opts: []launcher.Option{
				launcher.OptHome("/home/dest", true, false),
			},
			want: &Launcher{
				cfg:                     launcher.Options{HomeDir: "/home/dest", CustomHome: true, WritableTmpfs: true},
				singularityConf:         sc,
				homeHost:                u.HomeDir,
				homeSrc:                 "",
				homeDest:                "/home/dest",
				imageMountsByImagePath:  make(map[string]*fuse.ImageMount),
				imageMountsByMountpoint: make(map[string]*fuse.ImageMount),
			},
			wantErr: false,
		},
		{
			name: "homeSrcDest",
			opts: []launcher.Option{
				launcher.OptHome("/home/src:/home/dest", true, false),
			},
			want: &Launcher{
				cfg:                     launcher.Options{HomeDir: "/home/src:/home/dest", CustomHome: true, WritableTmpfs: true},
				singularityConf:         sc,
				homeHost:                u.HomeDir,
				homeSrc:                 "/home/src",
				homeDest:                "/home/dest",
				imageMountsByImagePath:  make(map[string]*fuse.ImageMount),
				imageMountsByMountpoint: make(map[string]*fuse.ImageMount),
			},
			wantErr: false,
		},
		{
			name: "no-compat",
			opts: []launcher.Option{
				launcher.OptNoCompat(true),
			},
			want: &Launcher{
				cfg:                     launcher.Options{NoCompat: true, WritableTmpfs: false},
				singularityConf:         sc,
				homeHost:                u.HomeDir,
				homeSrc:                 "",
				homeDest:                u.HomeDir,
				imageMountsByImagePath:  make(map[string]*fuse.ImageMount),
				imageMountsByMountpoint: make(map[string]*fuse.ImageMount),
			},
			wantErr: false,
		},
		{
			name: "no-compat_writable-tmpfs",
			opts: []launcher.Option{
				launcher.OptNoCompat(true),
				launcher.OptWritableTmpfs(true),
			},
			want: &Launcher{
				cfg:                     launcher.Options{NoCompat: true, WritableTmpfs: true},
				singularityConf:         sc,
				homeHost:                u.HomeDir,
				homeSrc:                 "",
				homeDest:                u.HomeDir,
				imageMountsByImagePath:  make(map[string]*fuse.ImageMount),
				imageMountsByMountpoint: make(map[string]*fuse.ImageMount),
			},
			wantErr: false,
		},
		{
			name: "unsupportedOption",
			opts: []launcher.Option{
				launcher.OptSecurity([]string{"seccomp:example.json"}),
			},
			want:    nil,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewLauncher(tt.opts...)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewLauncher() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("NewLauncher() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_normalizeImageRef(t *testing.T) {
	tests := []struct {
		name     string
		imageRef string
		want     string
		wantErr  bool
	}{
		{
			name:     "ext3 image",
			imageRef: "../../../../../test/images/extfs-for-overlay.img",
			want:     "",
			wantErr:  true,
		},
		{
			name:     "squashfs image",
			imageRef: "../../../../../test/images/squashfs-for-overlay.img",
			want:     "",
			wantErr:  true,
		},
		{
			name:     "sif image",
			imageRef: "../../../../../test/images/empty.sif",
			want:     "sif:../../../../../test/images/empty.sif",
			wantErr:  false,
		},
		{
			name:     "oci ref",
			imageRef: "oci:/my/layout",
			want:     "oci:/my/layout",
			wantErr:  false,
		},
		{
			name:     "oci sif",
			imageRef: "../../../../../test/images/empty.oci.sif",
			want:     "oci-sif:../../../../../test/images/empty.oci.sif",
			wantErr:  false,
		},
		{
			name:     "oci sif prefixed",
			imageRef: "oci-sif:../../../../../test/images/empty.oci.sif",
			want:     "oci-sif:../../../../../test/images/empty.oci.sif",
			wantErr:  false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizeImageRef(tt.imageRef)
			if (err != nil) != tt.wantErr {
				t.Errorf("normalizeImageRef() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("normalizeImageRef() = %v, want %v", got, tt.want)
			}
		})
	}
}
