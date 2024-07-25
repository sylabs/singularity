// Copyright (c) 2019-2024, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package squashfs

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/sylabs/singularity/v4/pkg/image"
)

func checkArchive(t *testing.T, path string, files []string, comp string) {
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	fComp, err := image.GetSquashfsComp(b)
	if err != nil {
		t.Error(err)
	}
	if fComp != comp {
		t.Errorf("found compression %s, expected %s", fComp, comp)
	}

	un, err := exec.LookPath("unsquashfs")
	if err != nil {
		t.SkipNow()
	}

	dir := t.TempDir()

	// -no-xattrs avoids priv vs unpriv, and fs specific issues we aren't interested in here.
	cmd := exec.Command(un, "-no-xattrs", "-f", "-d", dir, path)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%v: %v", err, string(out))
	}

	for _, f := range files {
		path := filepath.Join(dir, f)
		if _, err := os.Stat(path); err != nil {
			t.Errorf("squashfs verification failed: %s :%v", path, err)
		}
	}
}

func TestMksquashfs(t *testing.T) {
	testFiles := []string{"mksquashfs_singularity.go", "mksquashfs_singularity_test.go"}

	tests := []struct {
		name          string
		files         []string
		opts          []MksquashfsOpt
		expectError   bool
		expectComp    string
		checkForFiles []string
	}{
		{
			name:          "DefaultFiles",
			files:         testFiles,
			expectError:   false,
			expectComp:    "gzip",
			checkForFiles: testFiles,
		},
		{
			name:          "DefaultDir",
			files:         []string{"."},
			expectError:   false,
			expectComp:    "gzip",
			checkForFiles: testFiles,
		},
		{
			name:        "DoesNotExist",
			files:       []string{"/does/not/exist"},
			expectError: true,
		},
		{
			name:          "OptProcs",
			files:         []string{"."},
			opts:          []MksquashfsOpt{OptProcs(1)},
			expectError:   false,
			expectComp:    "gzip",
			checkForFiles: testFiles,
		},
		{
			name:          "OptMem",
			files:         []string{"."},
			opts:          []MksquashfsOpt{OptMem("64M")},
			expectError:   false,
			expectComp:    "gzip",
			checkForFiles: testFiles,
		},
		{
			name:        "BadMem",
			files:       []string{"."},
			opts:        []MksquashfsOpt{OptMem("64Z")},
			expectError: true,
		},
		{
			name:          "OptComp",
			files:         []string{"."},
			opts:          []MksquashfsOpt{OptComp("xz")},
			expectError:   false,
			expectComp:    "xz",
			checkForFiles: testFiles,
		},
		{
			name:        "BadComp",
			files:       []string{"."},
			opts:        []MksquashfsOpt{OptComp("doesnotexist")},
			expectError: true,
		},
		{
			name:          "OptAllRoot",
			files:         []string{"."},
			opts:          []MksquashfsOpt{OptAllRoot(true)},
			expectError:   false,
			expectComp:    "gzip",
			checkForFiles: testFiles,
		},
	}

	for _, tt := range tests {
		squashImg := filepath.Join(t.TempDir(), "test.sqfs")
		err := Mksquashfs(tt.files, squashImg, tt.opts...)
		if err != nil && !tt.expectError {
			t.Errorf("unexpected error: %v", err)
		}
		if err == nil && tt.expectError {
			t.Error("expected error, but got nil")
		}
		if len(tt.checkForFiles) > 0 {
			checkArchive(t, squashImg, tt.checkForFiles, tt.expectComp)
		}
	}
}
