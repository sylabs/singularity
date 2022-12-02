// Copyright (c) 2022, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package oci

import (
	"reflect"
	"testing"

	"github.com/opencontainers/runtime-spec/specs-go"
	"github.com/sylabs/singularity/internal/pkg/runtime/launcher"
	"github.com/sylabs/singularity/internal/pkg/test"
)

func Test_addNamespaces(t *testing.T) {
	test.DropPrivilege(t)
	defer test.ResetPrivilege(t)

	tests := []struct {
		name   string
		ns     launcher.Namespaces
		wantNS []specs.LinuxNamespace
	}{
		{
			name:   "none",
			ns:     launcher.Namespaces{},
			wantNS: defaultNamespaces,
		},
		{
			name:   "pid",
			ns:     launcher.Namespaces{PID: true},
			wantNS: defaultNamespaces,
		},
		{
			name:   "ipc",
			ns:     launcher.Namespaces{IPC: true},
			wantNS: defaultNamespaces,
		},
		{
			name:   "user",
			ns:     launcher.Namespaces{User: true},
			wantNS: defaultNamespaces,
		},
		{
			name:   "net",
			ns:     launcher.Namespaces{Net: true},
			wantNS: append(defaultNamespaces, specs.LinuxNamespace{Type: specs.NetworkNamespace}),
		},
		{
			name:   "uts",
			ns:     launcher.Namespaces{UTS: true},
			wantNS: append(defaultNamespaces, specs.LinuxNamespace{Type: specs.UTSNamespace}),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec := minimalSpec()
			newSpec := addNamespaces(spec, tt.ns)
			newNS := newSpec.Linux.Namespaces
			if !reflect.DeepEqual(newNS, tt.wantNS) {
				t.Errorf("addNamespaces() got %v, want %v", newNS, tt.wantNS)
			}
		})
	}
}
