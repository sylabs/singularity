// Copyright (c) 2018-2019, Sylabs Inc. All rights reserved.
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

func TestZypperOpenSuse(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	require.ArchIn(t, []string{"amd64", "arm64"})

	testZypperConveyorPacker(t, fmt.Sprintf("../../../../examples/opensuse-%s/Singularity", runtime.GOARCH))
}

func testZypperConveyorPacker(t *testing.T, defName string) {
	if _, err := exec.LookPath("zypper"); err != nil {
		t.Skip("skipping test, zypper not found")
	}

	test.EnsurePrivilege(t)

	defFile, err := os.Open(defName)
	if err != nil {
		t.Fatalf("unable to open file %s: %v\n", defName, err)
	}
	defer defFile.Close()

	// create bundle to build into
	tmpDir := t.TempDir()
	b, err := types.NewBundle(filepath.Join(tmpDir, "sbuild-zypper"), tmpDir)
	if err != nil {
		return
	}

	b.Recipe, err = parser.ParseDefinitionFile(defFile)
	if err != nil {
		t.Fatalf("failed to parse definition file %s: %v\n", defName, err)
	}

	zcp := &ZypperConveyorPacker{}

	err = zcp.Get(t.Context(), b)
	// clean up tmpfs since assembler isn't called
	defer zcp.b.Remove()
	if err != nil {
		t.Fatalf("failed to Get from %s: %v\n", defName, err)
	}

	_, err = zcp.Pack(t.Context())
	if err != nil {
		t.Fatalf("failed to Pack from %s: %v\n", defName, err)
	}
}
