// Copyright (c) 2018-2025, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package sources

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/sylabs/singularity/v4/internal/pkg/test"
	"github.com/sylabs/singularity/v4/internal/pkg/test/tool/require"
	"github.com/sylabs/singularity/v4/pkg/build/types"
	"github.com/sylabs/singularity/v4/pkg/build/types/parser"
)

func TestYumEL(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	// EL9 uses newer sqlite DB, but older /var/lib/rpm DB path.
	require.RPMMacro(t, "_db_backend", "sqlite")
	require.RPMMacro(t, "_dbpath", "/var/lib/rpm")
	require.ArchIn(t, []string{"amd64", "arm64"})

	testYumConveyorPacker(t, fmt.Sprintf("../../../../examples/almalinux-%s/Singularity", runtime.GOARCH))
}

func TestDnfFedora(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	// Fedora 39+ uses newer sqlite DB, and newer /usr/lib/sysimage/rpmDB path.
	require.RPMMacro(t, "_db_backend", "sqlite")
	require.RPMMacro(t, "_dbpath", "/usr/lib/sysimage/rpm")
	require.ArchIn(t, []string{"amd64", "arm64"})

	testYumConveyorPacker(t, fmt.Sprintf("../../../../examples/fedora-%s/Singularity", runtime.GOARCH))
}

func testYumConveyorPacker(t *testing.T, yumDef string) {
	_, dnfErr := exec.LookPath("dnf")
	_, yumErr := exec.LookPath("yum")
	if dnfErr != nil && yumErr != nil {
		t.Skip("skipping test, neither dnf nor yum found")
	}

	test.EnsurePrivilege(t)

	defFile, err := os.Open(yumDef)
	if err != nil {
		t.Fatalf("unable to open file %s: %v\n", yumDef, err)
	}
	defer defFile.Close()

	// create bundle to build into
	tmpDir := t.TempDir()
	b, err := types.NewBundle(filepath.Join(tmpDir, "sbuild-yum"), tmpDir)
	if err != nil {
		return
	}

	b.Recipe, err = parser.ParseDefinitionFile(defFile)
	if err != nil {
		t.Fatalf("failed to parse definition file %s: %v\n", yumDef, err)
	}

	ycp := &YumConveyorPacker{}

	err = ycp.Get(t.Context(), b)
	// clean up tmpfs since assembler isn't called
	defer ycp.b.Remove()
	if err != nil {
		t.Fatalf("failed to Get from %s: %v\n", yumDef, err)
	}

	_, err = ycp.Pack(t.Context())
	if err != nil {
		t.Fatalf("failed to Pack from %s: %v\n", yumDef, err)
	}
}
