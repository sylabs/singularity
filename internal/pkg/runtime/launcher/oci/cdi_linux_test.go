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
	"testing"

	"github.com/container-orchestrated-devices/container-device-interface/pkg/cdi"
	"github.com/opencontainers/runtime-spec/specs-go"
	"github.com/samber/lo"
)

var specDirs = []string{filepath.Join("..", "..", "..", "..", "..", "test", "cdi")}

func Test_addCDIDevice(t *testing.T) {
	var wantUID uint32 = 1000
	var wantGID uint32 = 1000
	tests := []struct {
		name        string
		devices     []string
		wantDevices []specs.LinuxDevice
		wantMounts  []specs.Mount
		wantErr     bool
		wantEnv     map[string]bool
	}{
		{
			name: "kmsg",
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
			wantMounts: []specs.Mount{
				{
					Source:      "/tmp",
					Destination: "/mountedtmp",
					Type:        "",
					Options:     []string{"rw"},
				},
			},
			wantEnv: map[string]bool{
				"FOO=VALID_SPEC": true,
				"BAR=BARVALUE1":  true,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec := minimalSpec()
			err := addCDIDevices(&spec, tt.devices, cdi.WithSpecDirs(specDirs...))
			if (err != nil) != tt.wantErr {
				t.Errorf("addCDIDevices() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !reflect.DeepEqual(spec.Mounts, tt.wantMounts) {
				t.Errorf("addCDIDevices() want %v, got %v", tt.wantMounts, spec.Mounts)
			}
			for _, envVar := range spec.Process.Env {
				delete(tt.wantEnv, envVar)
			}
			if len(tt.wantEnv) > 0 {
				t.Errorf("addCDIDevices() expected, but did not find, the following environment variables: %#v", lo.Keys(tt.wantEnv))
			}
		})
	}
}
