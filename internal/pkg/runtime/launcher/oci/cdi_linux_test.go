// Copyright (c) 2022, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

// Package oci implements a Launcher that will configure and launch a container
// with an OCI runtime. It also provides implementations of OCI state
// transitions that can be called directly, Create/Start/Kill etc.
package oci

import (
	"path/filepath"
	"reflect"
	"sort"
	"testing"

	"github.com/opencontainers/runtime-spec/specs-go"
	"github.com/sylabs/singularity/v4/pkg/util/slice"
	"tags.cncf.io/container-device-interface/pkg/cdi"
)

var specDirs = []string{filepath.Join("..", "..", "..", "..", "..", "test", "cdi")}

type mountsList []specs.Mount

func (a mountsList) Len() int           { return len(a) }
func (a mountsList) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a mountsList) Less(i, j int) bool { return a[i].Destination < a[j].Destination }

func Test_addCDIDevice(t *testing.T) {
	var wantUID uint32 = 1000
	var wantGID uint32 = 1000
	tests := []struct {
		name        string
		devices     []string
		wantDevices []specs.LinuxDevice
		wantMounts  mountsList
		wantErr     bool
		wantEnv     []string
	}{
		{
			name: "ValidOneDeviceKmsg",
			devices: []string{
				"singularityCEtesting.sylabs.io/device=kmsgDevice",
			},
			wantDevices: []specs.LinuxDevice{
				{
					Path:     "/dev/kmsg",
					Type:     "c",
					Major:    1,
					Minor:    11,
					FileMode: nil,
					UID:      &wantUID,
					GID:      &wantGID,
				},
			},
			wantMounts: mountsList{
				{
					Source:      "/tmp",
					Destination: "/tmpmountforkmsg",
					Type:        "",
					Options:     []string{"rw"},
				},
			},
			wantEnv: []string{
				"FOO=VALID_SPEC",
				"BAR=BARVALUE1",
			},
		},
		{
			name: "ValidTmpDevices",
			devices: []string{
				"singularityCEtesting.sylabs.io/device=tmpmountDevice17",
				"singularityCEtesting.sylabs.io/device=tmpmountDevice1",
			},
			wantDevices: []specs.LinuxDevice{},
			wantMounts: mountsList{
				{
					Source:      "/tmp",
					Destination: "/tmpmount1",
					Type:        "",
					Options:     []string{"ro"},
				},
				{
					Source:      "/tmp",
					Destination: "/tmpmount3",
					Type:        "",
					Options:     []string{"rbind", "nosuid", "nodev"},
				},
				{
					Source:      "/tmp",
					Destination: "/tmpmount13",
					Type:        "",
					Options:     []string{"rw"},
				},
				{
					Source:      "/tmp",
					Destination: "/tmpmount17",
					Type:        "",
					Options:     []string{"r"},
				},
			},
			wantEnv: []string{
				"ABCD=QWERTY",
				"EFGH=ASDFGH",
				"IJKL=ZXCVBN",
				"FOO=VALID_SPEC",
				"BAR=BARVALUE1",
			},
		},
		{
			name: "ValidTmpDevicesFromOneJSON",
			devices: []string{
				"singularityCEtesting.sylabs.io/device=tmpmountDevice1",
			},
			wantDevices: []specs.LinuxDevice{},
			wantMounts: mountsList{
				{
					Source:      "/tmp",
					Destination: "/tmpmount1",
					Type:        "",
					Options:     []string{"ro"},
				},
				{
					Source:      "/tmp",
					Destination: "/tmpmount3",
					Type:        "",
					Options:     []string{"rbind", "nosuid", "nodev"},
				},
				{
					Source:      "/tmp",
					Destination: "/tmpmount13",
					Type:        "",
					Options:     []string{"rw"},
				},
			},
			wantEnv: []string{
				"ABCD=QWERTY",
				"EFGH=ASDFGH",
				"IJKL=ZXCVBN",
			},
		},
		{
			name: "ValidMixedDevices",
			devices: []string{
				"singularityCEtesting.sylabs.io/device=tmpmountDevice17",
				"singularityCEtesting.sylabs.io/device=kmsgDevice",
				"singularityCEtesting.sylabs.io/device=tmpmountDevice1",
			},
			wantDevices: []specs.LinuxDevice{
				{
					Path:     "/dev/kmsg",
					Type:     "c",
					Major:    1,
					Minor:    11,
					FileMode: nil,
					UID:      &wantUID,
					GID:      &wantGID,
				},
			},
			wantMounts: mountsList{
				{
					Source:      "/tmp",
					Destination: "/tmpmount1",
					Type:        "",
					Options:     []string{"ro"},
				},
				{
					Source:      "/tmp",
					Destination: "/tmpmount3",
					Type:        "",
					Options:     []string{"rbind", "nosuid", "nodev"},
				},
				{
					Source:      "/tmp",
					Destination: "/tmpmount13",
					Type:        "",
					Options:     []string{"rw"},
				},
				{
					Source:      "/tmp",
					Destination: "/tmpmount17",
					Type:        "",
					Options:     []string{"r"},
				},
				{
					Source:      "/tmp",
					Destination: "/tmpmountforkmsg",
					Type:        "",
					Options:     []string{"rw"},
				},
			},
			wantEnv: []string{
				"ABCD=QWERTY",
				"EFGH=ASDFGH",
				"IJKL=ZXCVBN",
				"FOO=VALID_SPEC",
				"BAR=BARVALUE1",
			},
		},
		{
			name: "InvalidNameOneDevice",
			devices: []string{
				"singularityCEtesting.sylabs.io/device=noSuchDevice",
			},
			wantErr: true,
		},
		{
			name: "InvalidNameSeveralDevices",
			devices: []string{
				"singularityCEtesting.sylabs.io/device=noSuchDevice",
				"singularityCEtesting.sylabs.io/device=noSuchDeviceEither",
			},
			wantErr: true,
		},
		{
			name: "InvalidNameAmongValids",
			devices: []string{
				"singularityCEtesting.sylabs.io/device=tmpmountDevice17",
				"singularityCEtesting.sylabs.io/device=noSuchDevice",
				"singularityCEtesting.sylabs.io/device=tmpmountDevice1",
				"singularityCEtesting.sylabs.io/device=kmsgDevice",
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec := minimalSpec()
			err := addCDIDevices(&spec, tt.devices, cdi.WithSpecDirs(specDirs...))
			if (err != nil) != tt.wantErr {
				t.Errorf("addCDIDevices() mismatched error values; expected %v, got %v.", tt.wantErr, err)
			}

			// We need this if-statement because the comparison below is done with reflection, and so a nil array and a non-nil but zero-length array will be considered different (which is not what we want here)
			if (len(tt.wantMounts) > 0) || (len(spec.Mounts) > 0) {
				// Note that the current implementation of OCI/CDI sorts the mounts generated by the set of mapped devices, therefore we compare against a sorted list.
				sort.Sort(tt.wantMounts)
				if !reflect.DeepEqual(mountsList(spec.Mounts), tt.wantMounts) {
					t.Errorf("addCDIDevices() mismatched mounts; expected %v, got %v.", tt.wantMounts, spec.Mounts)
				}
			}

			envMissing := slice.Subtract(tt.wantEnv, spec.Process.Env)
			if len(envMissing) > 0 {
				t.Errorf("addCDIDevices() mismatched environment variables; expected, but did not find, the following environment variables: %v", envMissing)
			}
		})
	}
}
