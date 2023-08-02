// Copyright (c) 2022, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package oci

import (
	"reflect"
	"testing"

	"github.com/opencontainers/runtime-spec/specs-go"
	"github.com/sylabs/singularity/v4/internal/pkg/runtime/launcher"
	"github.com/sylabs/singularity/v4/internal/pkg/test"
	"github.com/sylabs/singularity/v4/internal/pkg/util/bin"
	"gotest.tools/v3/assert"
)

func Test_addNamespaces(t *testing.T) {
	test.DropPrivilege(t)
	defer test.ResetPrivilege(t)

	defaultPlusPID := append(defaultNamespaces,
		specs.LinuxNamespace{Type: specs.PIDNamespace})
	defaultPlusNetPID := append(defaultNamespaces,
		specs.LinuxNamespace{Type: specs.NetworkNamespace},
		specs.LinuxNamespace{Type: specs.PIDNamespace})
	defaultPlusPIDUTS := append(defaultNamespaces,
		specs.LinuxNamespace{Type: specs.PIDNamespace},
		specs.LinuxNamespace{Type: specs.UTSNamespace})

	tests := []struct {
		name   string
		ns     launcher.Namespaces
		wantNS []specs.LinuxNamespace
	}{
		{
			name:   "none",
			ns:     launcher.Namespaces{},
			wantNS: defaultPlusPID,
		},
		{
			name:   "nopid",
			ns:     launcher.Namespaces{NoPID: true},
			wantNS: defaultNamespaces,
		},
		{
			name:   "pid",
			ns:     launcher.Namespaces{PID: true},
			wantNS: defaultPlusPID,
		},
		{
			name:   "ipc",
			ns:     launcher.Namespaces{IPC: true},
			wantNS: defaultPlusPID,
		},
		{
			name:   "user",
			ns:     launcher.Namespaces{User: true},
			wantNS: defaultPlusPID,
		},
		{
			name:   "net",
			ns:     launcher.Namespaces{Net: true},
			wantNS: defaultPlusNetPID,
		},
		{
			name:   "uts",
			ns:     launcher.Namespaces{UTS: true},
			wantNS: defaultPlusPIDUTS,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ms := minimalSpec()
			spec := &ms
			err := addNamespaces(spec, tt.ns)
			if err != nil {
				t.Errorf("addNamespaces() returned an unexpected error: %v", err)
			}
			newNS := spec.Linux.Namespaces
			if !reflect.DeepEqual(newNS, tt.wantNS) {
				t.Errorf("addNamespaces() got %v, want %v", newNS, tt.wantNS)
			}
		})
	}
}

func Test_noSetgroupsAnnotation(t *testing.T) {
	ms := minimalSpec()

	gotErr := noSetgroupsAnnotation(&ms)

	// crun case - no error, expect annotation
	if _, err := bin.FindBin("crun"); err == nil {
		if err != nil {
			t.Errorf("noSetgroupsAnnotation returned unexpected error when crun available: %s", err)
		}
		assert.DeepEqual(t, ms.Annotations,
			map[string]string{
				"run.oci.keep_original_groups": "1",
			},
		)
		return
	}

	// Otherwise, expect an error
	if gotErr == nil {
		t.Errorf("noSetgroupsAnnotation returned unexpected success when crun not available")
	}
}
