// Copyright (c) 2024, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package data

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/sylabs/singularity/v4/e2e/internal/e2e"
	"github.com/sylabs/singularity/v4/e2e/internal/testhelper"
)

type ctx struct {
	env e2e.TestEnv
}

// Check that `data package` creates a valid data container, that can be used.
func (c ctx) testDataPackage(t *testing.T) {
	e2e.EnsureOCISIF(t, c.env)
	// <tmpdir>/innner/file
	outerDir := t.TempDir()
	innerDir := filepath.Join(outerDir, "inner")
	innerFile := filepath.Join(innerDir, "file")
	content := []byte("TEST")
	if err := os.Mkdir(innerDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(innerFile, content, 0o644); err != nil {
		t.Fatal(err)
	}

	// Basic test that we can run the bound in `nvidia-smi` which *should* be on the PATH
	tests := []struct {
		name            string
		packageSrc      string // directory / file to package
		packageExitCode int    // exit code from singularity data package
		boundFile       string // expected location of file in container with data container --bind
	}{
		{
			name:            "InvalidSource",
			packageSrc:      filepath.Join(c.env.TestDir, "/this/does/not/exist"),
			packageExitCode: 255,
		},
		{
			name:            "ValidOuter",
			packageSrc:      outerDir,
			packageExitCode: 0,
			boundFile:       "/data/inner/file",
		},
		{
			name:            "ValidInner",
			packageSrc:      innerDir,
			packageExitCode: 0,
			boundFile:       "/data/file",
		},
		{
			name:            "ValidFile",
			packageSrc:      innerFile,
			packageExitCode: 0,
			boundFile:       "/data/file",
		},
	}

	// Create a data container
	for _, tt := range tests {
		dcPath := filepath.Join(c.env.TestDir, "testDataPackage-"+tt.name)
		c.env.RunSingularity(
			t,
			e2e.AsSubtest(tt.name+"/package"),
			e2e.WithProfile(e2e.UserProfile),
			e2e.WithCommand("data"),
			e2e.WithArgs("package", tt.packageSrc, dcPath),
			e2e.ExpectExit(tt.packageExitCode),
		)

		if tt.boundFile == "" {
			continue
		}

		// Verify that the file is at the expected location, when data container used with `--bind`
		bindSpec := fmt.Sprintf("%s:/data:image-src=/", dcPath)
		c.env.RunSingularity(
			t,
			e2e.AsSubtest(tt.name+"/bind"),
			e2e.WithProfile(e2e.OCIUserProfile),
			e2e.WithCommand("exec"),
			e2e.WithArgs("--bind", bindSpec, c.env.OCISIFPath, "/bin/cat", tt.boundFile),
			e2e.ExpectExit(0,
				e2e.ExpectOutput(e2e.ExactMatch, string(content)),
			),
		)

		// Verify that the file is at the expected location, when data container used with `--data`
		dataSpec := fmt.Sprintf("%s:/data", dcPath)
		c.env.RunSingularity(
			t,
			e2e.AsSubtest(tt.name+"/data"),
			e2e.WithProfile(e2e.OCIUserProfile),
			e2e.WithCommand("exec"),
			e2e.WithArgs("--data", dataSpec, c.env.OCISIFPath, "/bin/cat", tt.boundFile),
			e2e.ExpectExit(0,
				e2e.ExpectOutput(e2e.ExactMatch, string(content)),
			),
		)

		if err := os.Remove(dcPath); err != nil {
			t.Error(err)
		}
	}
}

// E2ETests is the main func to trigger the test suite
func E2ETests(env e2e.TestEnv) testhelper.Tests {
	c := ctx{
		env: env,
	}

	return testhelper.Tests{
		"package": c.testDataPackage,
	}
}
