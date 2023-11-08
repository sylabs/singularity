// Copyright (c) 2022-2023, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

// Package oci implements a Launcher that will configure and launch a container
// with an OCI runtime. It also provides implementations of OCI state
// transitions that can be called directly, Create/Start/Kill etc.
package oci

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"

	"github.com/opencontainers/runtime-spec/specs-go"
	"github.com/sylabs/singularity/v4/internal/pkg/runtime/launcher"
	"github.com/sylabs/singularity/v4/internal/pkg/util/user"
	"github.com/sylabs/singularity/v4/pkg/util/bind"
	"github.com/sylabs/singularity/v4/pkg/util/singularityconf"
)

func Test_addBindMount(t *testing.T) {
	tests := []struct {
		name       string
		cfg        launcher.Options
		userbind   bool
		b          bind.Path
		allowSUID  bool
		wantMounts *[]specs.Mount
		wantErr    bool
	}{
		{
			name: "Valid",
			b: bind.Path{
				Source:      "/tmp",
				Destination: "/tmp",
			},
			wantMounts: &[]specs.Mount{
				{
					Source:      "/tmp",
					Destination: "/tmp",
					Type:        "none",
					Options:     []string{"rbind", "nodev", "nosuid"},
				},
			},
		},
		{
			name: "ValidRO",
			b: bind.Path{
				Source:      "/tmp",
				Destination: "/tmp",
				Options:     map[string]*bind.Option{"ro": {}},
			},
			wantMounts: &[]specs.Mount{
				{
					Source:      "/tmp",
					Destination: "/tmp",
					Type:        "none",
					Options:     []string{"rbind", "nodev", "nosuid", "ro"},
				},
			},
		},
		{
			name: "ValidSUID",
			b: bind.Path{
				Source:      "/tmp",
				Destination: "/tmp",
				Options:     map[string]*bind.Option{"ro": {}},
			},
			allowSUID: true,
			wantMounts: &[]specs.Mount{
				{
					Source:      "/tmp",
					Destination: "/tmp",
					Type:        "none",
					Options:     []string{"rbind", "nodev", "ro"},
				},
			},
		},
		{
			name: "BadSource",
			b: bind.Path{
				Source:      "doesnotexist!",
				Destination: "/mnt",
			},
			wantMounts: &[]specs.Mount{},
			wantErr:    true,
		},
		{
			name: "RelDest",
			b: bind.Path{
				Source:      "/tmp",
				Destination: "relative",
			},
			wantMounts: &[]specs.Mount{},
			wantErr:    true,
		},
		{
			name: "ImageID",
			b: bind.Path{
				Source:      "/myimage.sif",
				Destination: "/mnt",
				Options:     map[string]*bind.Option{"id": {Value: "4"}},
			},
			userbind:   true,
			wantMounts: &[]specs.Mount{},
			// Should fail because bind-mounting SIFs not supported in OCI mode
			wantErr: true,
		},
		{
			name: "ImageSrc",
			b: bind.Path{
				Source:      "/myimage.sif",
				Destination: "/mnt",
				Options:     map[string]*bind.Option{"img-src": {Value: "/test"}},
			},
			userbind:   true,
			wantMounts: &[]specs.Mount{},
			// Should fail because bind-mounting SIFs not supported in OCI mode
			wantErr: true,
		},
		{
			name: "Proc",
			b: bind.Path{
				Source:      "/proc",
				Destination: "/proc",
			},
			userbind: true,
			wantMounts: &[]specs.Mount{
				{
					Source:      "/proc",
					Destination: "/proc",
					Type:        "none",
					Options:     []string{"rbind", "nodev", "nosuid", "noexec"},
				},
			},
		},
		{
			name: "Sys",
			b: bind.Path{
				Source:      "/sys",
				Destination: "/sys",
			},
			userbind: true,
			wantMounts: &[]specs.Mount{
				{
					Source:      "/sys",
					Destination: "/sys",
					Type:        "none",
					Options:     []string{"rbind", "nodev", "nosuid", "noexec"},
				},
			},
		},
		{
			name: "Device",
			b: bind.Path{
				Source:      "/dev/null",
				Destination: "/dev/null",
			},
			userbind: true,
			wantMounts: &[]specs.Mount{
				{
					Source:      "/dev/null",
					Destination: "/dev/null",
					Type:        "none",
					Options:     []string{"bind", "nosuid"},
				},
			},
		},
		{
			name: "DeviceBadDest",
			b: bind.Path{
				Source:      "/dev/null",
				Destination: "/notdev/null",
			},
			userbind:   true,
			wantMounts: &[]specs.Mount{},
			wantErr:    true,
		},
	}
	for _, tt := range tests {
		for _, m := range *tt.wantMounts {
			sort.Strings(m.Options)
		}
		t.Run(tt.name, func(t *testing.T) {
			mounts := &[]specs.Mount{}
			l := &Launcher{
				cfg:             tt.cfg,
				singularityConf: &singularityconf.File{},
			}
			if tt.userbind {
				l.singularityConf.UserBindControl = true
			}
			err := l.addBindMount(mounts, tt.b, tt.allowSUID)
			if (err != nil) != tt.wantErr {
				t.Errorf("addBindMount() error = %v, wantErr %v", err, tt.wantErr)
			}
			for _, m := range *mounts {
				sort.Strings(m.Options)
			}
			if !reflect.DeepEqual(mounts, tt.wantMounts) {
				t.Errorf("addBindMount() want %v, got %v", tt.wantMounts, mounts)
			}
		})
	}
}

//nolint:maintidx
func TestLauncher_addUserBindMounts(t *testing.T) {
	tests := []struct {
		name       string
		cfg        launcher.Options
		userbind   bool
		allowSUID  bool
		wantMounts *[]specs.Mount
		wantErr    bool
	}{
		{
			name: "Disabled",
			cfg: launcher.Options{
				BindPaths: []string{"/tmp"},
			},
			wantMounts: &[]specs.Mount{},
			wantErr:    false,
		},
		{
			name: "ValidBindSrc",
			cfg: launcher.Options{
				BindPaths: []string{"/tmp"},
			},
			userbind: true,
			wantMounts: &[]specs.Mount{
				{
					Source:      "/tmp",
					Destination: "/tmp",
					Type:        "none",
					Options:     []string{"rbind", "nodev", "nosuid"},
				},
			},
			wantErr: false,
		},
		{
			name: "ValidBindSrcDst",
			cfg: launcher.Options{
				BindPaths: []string{"/tmp:/mnt"},
			},
			userbind: true,
			wantMounts: &[]specs.Mount{
				{
					Source:      "/tmp",
					Destination: "/mnt",
					Type:        "none",
					Options:     []string{"rbind", "nodev", "nosuid"},
				},
			},
			wantErr: false,
		},
		{
			name: "ValidBindRO",
			cfg: launcher.Options{
				BindPaths: []string{"/tmp:/mnt:ro"},
			},
			userbind: true,
			wantMounts: &[]specs.Mount{
				{
					Source:      "/tmp",
					Destination: "/mnt",
					Type:        "none",
					Options:     []string{"rbind", "nodev", "nosuid", "ro"},
				},
			},
			wantErr: false,
		},
		{
			name: "ValidBindSUID",
			cfg: launcher.Options{
				BindPaths: []string{"/tmp:/mnt"},
			},
			userbind:  true,
			allowSUID: true,
			wantMounts: &[]specs.Mount{
				{
					Source:      "/tmp",
					Destination: "/mnt",
					Type:        "none",
					Options:     []string{"rbind", "nodev"},
				},
			},
			wantErr: false,
		},
		{
			name: "InvalidBindSrc",
			cfg: launcher.Options{
				BindPaths: []string{"!doesnotexist"},
			},
			userbind:   true,
			wantMounts: &[]specs.Mount{},
			wantErr:    true,
		},
		{
			name: "RelBindDst",
			cfg: launcher.Options{
				BindPaths: []string{"/tmp:relative"},
			},
			userbind:   true,
			wantMounts: &[]specs.Mount{},
			wantErr:    true,
		},
		{
			name: "UnsupportedBindID",
			cfg: launcher.Options{
				BindPaths: []string{"my.sif:/mnt:id=2"},
			},
			userbind:   true,
			wantMounts: &[]specs.Mount{},
			wantErr:    true,
		},
		{
			name: "UnsupportedBindImgSrc",
			cfg: launcher.Options{
				BindPaths: []string{"my.sif:/mnt:img-src=/test"},
			},
			userbind:   true,
			wantMounts: &[]specs.Mount{},
			wantErr:    true,
		},
		{
			name: "ValidMount",
			cfg: launcher.Options{
				Mounts: []string{"type=bind,source=/tmp,destination=/mnt"},
			},
			userbind: true,
			wantMounts: &[]specs.Mount{
				{
					Source:      "/tmp",
					Destination: "/mnt",
					Type:        "none",
					Options:     []string{"rbind", "nodev", "nosuid"},
				},
			},
			wantErr: false,
		},
		{
			name: "ValidMountRO",
			cfg: launcher.Options{
				Mounts: []string{"type=bind,source=/tmp,destination=/mnt,ro"},
			},
			userbind: true,
			wantMounts: &[]specs.Mount{
				{
					Source:      "/tmp",
					Destination: "/mnt",
					Type:        "none",
					Options:     []string{"rbind", "nodev", "nosuid", "ro"},
				},
			},
			wantErr: false,
		},
		{
			name: "ValidMountSUID",
			cfg: launcher.Options{
				Mounts: []string{"type=bind,source=/tmp,destination=/mnt"},
			},
			userbind: true,
			wantMounts: &[]specs.Mount{
				{
					Source:      "/tmp",
					Destination: "/mnt",
					Type:        "none",
					Options:     []string{"rbind", "nodev"},
				},
			},
			allowSUID: true,
			wantErr:   false,
		},
		{
			name: "UnsupportedMountID",
			cfg: launcher.Options{
				Mounts: []string{"type=bind,source=my.sif,destination=/mnt,id=2"},
			},
			userbind:   true,
			wantMounts: &[]specs.Mount{},
			wantErr:    true,
		},
		{
			name: "UnsupportedMountImgSrc",
			cfg: launcher.Options{
				Mounts: []string{"type=bind,source=my.sif,destination=/mnt,image-src=/test"},
			},
			userbind:   true,
			wantMounts: &[]specs.Mount{},
			wantErr:    true,
		},
		{
			name: "FullDev",
			cfg: launcher.Options{
				BindPaths: []string{"/dev"},
			},
			userbind: true,
			wantMounts: &[]specs.Mount{
				{
					Source:      "/dev",
					Destination: "/dev",
					Type:        "bind",
					Options:     []string{"nosuid", "rbind", "rprivate", "rw"},
				},
				{
					Source:      "devpts",
					Destination: "/dev/pts",
					Type:        "devpts",
					Options:     ptsFlags(t),
				},
			},
			wantErr: false,
		},
		{
			name: "FullDevBadDest",
			cfg: launcher.Options{
				BindPaths: []string{"/dev:/notdev"},
			},
			userbind:   true,
			wantMounts: &[]specs.Mount{},
			wantErr:    true,
		},
		{
			name: "SpecificDevice",
			cfg: launcher.Options{
				BindPaths: []string{"/dev/null"},
			},
			userbind: true,
			wantMounts: &[]specs.Mount{
				{
					Source:      "/dev/null",
					Destination: "/dev/null",
					Type:        "none",
					Options:     []string{"bind", "nosuid"},
				},
			},
			wantErr: false,
		},
		{
			name: "SpecificDeviceBadDest",
			cfg: launcher.Options{
				BindPaths: []string{"/dev/null:/notdev/null"},
			},
			userbind:   true,
			wantMounts: &[]specs.Mount{},
			wantErr:    true,
		},
	}
	for _, tt := range tests {
		for _, m := range *tt.wantMounts {
			sort.Strings(m.Options)
		}
		t.Run(tt.name, func(t *testing.T) {
			l := &Launcher{
				cfg: tt.cfg,
				singularityConf: &singularityconf.File{
					// Required as full `/dev` userbind test involves a devpts mount onto the mounted /dev.
					MountDevPts: true,
				},
			}
			if tt.userbind {
				l.singularityConf.UserBindControl = true
			}
			if tt.allowSUID {
				l.cfg.AllowSUID = true
			}
			mounts := &[]specs.Mount{}
			err := l.addUserBindMounts(mounts)
			for _, m := range *mounts {
				sort.Strings(m.Options)
			}
			if (err != nil) != tt.wantErr {
				t.Errorf("addBindMount() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !reflect.DeepEqual(mounts, tt.wantMounts) {
				t.Errorf("addBindMount() want %v, got %v", tt.wantMounts, mounts)
			}
		})
	}
}

// Flags for /dev/pts depend on whether we are running as root, and what the tty GID is.
func ptsFlags(t *testing.T) []string {
	flags := []string{"nosuid", "noexec", "newinstance", "ptmxmode=0666", "mode=0620"}

	if os.Geteuid() == 0 {
		group, err := user.GetGrNam("tty")
		if err != nil {
			t.Fatalf("while identifying tty gid: %v", err)
		}
		flags = append(flags, fmt.Sprintf("gid=%d", group.GID))
	}

	return flags
}

func TestLauncher_addLibrariesMounts(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "add-libraries-mounts")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if !t.Failed() {
			os.RemoveAll(tmpDir)
		}
	})

	lib1 := filepath.Join(tmpDir, "lib1.so")
	lib2 := filepath.Join(tmpDir, "lib2.so")
	libInvalid := filepath.Join(tmpDir, "invalid")
	if err := os.WriteFile(lib1, []byte("lib1"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lib2, []byte("lib2"), 0o644); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name       string
		cfg        launcher.Options
		userbind   bool
		wantMounts *[]specs.Mount
		wantErr    bool
	}{
		{
			name: "Disabled",
			cfg: launcher.Options{
				ContainLibs: []string{lib1},
			},
			wantMounts: &[]specs.Mount{},
			wantErr:    false,
		},
		{
			name: "Invalid",
			cfg: launcher.Options{
				ContainLibs: []string{libInvalid},
			},
			userbind:   true,
			wantMounts: &[]specs.Mount{},
			wantErr:    true,
		},
		{
			name: "Single",
			cfg: launcher.Options{
				ContainLibs: []string{lib1},
			},
			userbind: true,
			wantMounts: &[]specs.Mount{
				{
					Source:      lib1,
					Destination: "/.singularity.d/libs/lib1.so",
					Type:        "none",
					Options:     []string{"rbind", "nodev", "nosuid", "ro"},
				},
			},
			wantErr: false,
		},
		{
			name: "Multiple",
			cfg: launcher.Options{
				ContainLibs: []string{lib1, lib2},
			},
			userbind: true,
			wantMounts: &[]specs.Mount{
				{
					Source:      lib1,
					Destination: "/.singularity.d/libs/lib1.so",
					Type:        "none",
					Options:     []string{"rbind", "nodev", "nosuid", "ro"},
				},
				{
					Source:      lib2,
					Destination: "/.singularity.d/libs/lib2.so",
					Type:        "none",
					Options:     []string{"rbind", "nodev", "nosuid", "ro"},
				},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		for _, m := range *tt.wantMounts {
			sort.Strings(m.Options)
		}
		t.Run(tt.name, func(t *testing.T) {
			l := &Launcher{
				cfg:             tt.cfg,
				singularityConf: &singularityconf.File{},
			}
			if tt.userbind {
				l.singularityConf.UserBindControl = true
			}
			mounts := &[]specs.Mount{}
			err := l.addLibrariesMounts(mounts)
			for _, m := range *mounts {
				sort.Strings(m.Options)
			}
			if (err != nil) != tt.wantErr {
				t.Errorf("addLibrariesMounts() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !reflect.DeepEqual(mounts, tt.wantMounts) {
				t.Errorf("addLibrariesMounts() want %v, got %v", tt.wantMounts, mounts)
			}
		})
	}
}
