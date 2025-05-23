// Copyright (c) 2019-2025, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package sifbundle

import (
	"path/filepath"
	"runtime"
	"testing"

	"github.com/sylabs/singularity/v4/internal/pkg/runtime/engine/config/oci"
	"github.com/sylabs/singularity/v4/internal/pkg/test"
	ocitest "github.com/sylabs/singularity/v4/internal/pkg/test/tool/oci"
	"github.com/sylabs/singularity/v4/internal/pkg/util/fs"
	"github.com/sylabs/singularity/v4/pkg/ocibundle/tools"
	"github.com/sylabs/singularity/v4/pkg/util/fs/proc"
	useragent "github.com/sylabs/singularity/v4/pkg/util/user-agent"
)

// We need a busybox SIF for these tests. We used to download it each time, but we have one
// around for some e2e tests already.
const busyboxSIF = "../../../e2e/testdata/busybox_" + runtime.GOARCH + ".sif"

func TestFromSif(t *testing.T) {
	useragent.InitValue("TestFromSif", "0.0.0")
	test.EnsurePrivilege(t)

	bundlePath := t.TempDir()
	sifFile := filepath.Join(t.TempDir(), "test.sif")
	if err := fs.CopyFileAtomic(busyboxSIF, sifFile, 0o755); err != nil {
		t.Fatalf("Could not copy test image: %v", err)
	}

	// test with a wrong image path
	bundle, err := FromSif("/blah", bundlePath, false)
	if err != nil {
		t.Errorf("unexpected success while opening non existent image")
	}
	// create OCI bundle from SIF
	if err := bundle.Create(t.Context(), nil); err == nil {
		// check if cleanup occurred
		t.Errorf("unexpected success while creating OCI bundle")
	}

	tests := []struct {
		name     string
		writable bool
	}{
		{"FromSif", false},
		{"FromSifWritable", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.writable {
				requireFilesystem(t, "overlay")
			}

			// create OCI bundle from SIF
			bundle, err = FromSif(sifFile, bundlePath, tt.writable)
			if err != nil {
				t.Fatal(err)
			}
			// generate a default configuration
			g, err := oci.DefaultConfig()
			if err != nil {
				t.Fatal(err)
			}
			// remove seccomp filter for CI
			g.Config.Linux.Seccomp = nil
			g.SetProcessArgs([]string{tools.RunScript, "id"})

			if err := bundle.Create(t.Context(), g.Config); err != nil {
				// check if cleanup occurred
				t.Fatal(err)
			}

			ocitest.ValidateBundle(t, bundle.Path())

			// Clean up
			if err := bundle.Delete(t.Context()); err != nil {
				t.Error(err)
			}
		})
	}
}

// TODO: This is a duplicate from internal/pkg/test/tool/require
// in order avoid needing buildcfg for this unit test, such that
// it can be run directly from the source tree without compilation.
// This bundle code is in `pkg/` so *should not* depend on a compiled
// Singularity (https://github.com/hpcng/singularity/issues/2316).
//
// Ideally we would refactor i/p/t/t/require so requirements that
// don't need a compiled Singularity can be used without compiled
// Singularity.
//
// Filesystem checks that the current test could use the
// corresponding filesystem, if the filesystem is not
// listed in /proc/filesystems, the current test is skipped
// with a message.
func requireFilesystem(t *testing.T, fs string) {
	has, err := proc.HasFilesystem(fs)
	if err != nil {
		t.Fatalf("error while checking filesystem presence: %s", err)
	}
	if !has {
		t.Skipf("%s filesystem seems not supported", fs)
	}
}
