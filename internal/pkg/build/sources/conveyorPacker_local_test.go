// Copyright (c) 2018-2025, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package sources_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sylabs/singularity/v4/internal/pkg/build/sources"
	"github.com/sylabs/singularity/v4/internal/pkg/test/tool/require"
	"github.com/sylabs/singularity/v4/internal/pkg/util/fs"
	"github.com/sylabs/singularity/v4/internal/pkg/util/fs/squashfs"
	"github.com/sylabs/singularity/v4/pkg/build/types"
)

func TestLocalPackerSquashfs(t *testing.T) {
	require.Command(t, "mksquashfs")
	tempDirPath := t.TempDir()

	// Image root directory
	rootfs := filepath.Join(tempDirPath, "issue_3084_rootfs")

	// Create directories
	if err := os.Mkdir(rootfs, 0o755); err != nil {
		t.Fatalf("while creating directory: %v", err)
	}
	if err := os.Mkdir(filepath.Join(rootfs, "tmp"), 0o755); err != nil {
		t.Fatalf("while creating directory: %v", err)
	}
	if err := os.Mkdir(filepath.Join(rootfs, "var"), 0o755); err != nil {
		t.Fatalf("while creating directory: %v", err)
	}

	// Create symlinks: $rootfs/var/tmp -> /tmp , $rootfs/var/log -> /tmp
	if err := os.Symlink("/tmp", filepath.Join(rootfs, "var", "tmp")); err != nil {
		t.Fatalf("while creating symlink: %v", err)
	}
	if err := os.Symlink("/tmp", filepath.Join(rootfs, "var", "log")); err != nil {
		t.Fatalf("while creating symlink: %v", err)
	}

	// Copy a test file to rootfs and create image.
	testfile := "conveyorPacker_local_test.go"
	data, err := os.ReadFile(testfile)
	if err != nil {
		t.Fatalf("while reading test file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(rootfs, testfile), data, 0o644); err != nil {
		t.Fatalf("while writing test file: %v", err)
	}
	image := filepath.Join(tempDirPath, "issue_3084.img")
	if err := squashfs.Mksquashfs([]string{rootfs}, image); err != nil {
		t.Fatalf("while creating image: %v", err)
	}
	defer os.Remove(image)

	// Creates bundle
	bundleTmp := t.TempDir()
	b, err := types.NewBundle(tempDirPath, bundleTmp)
	if err != nil {
		t.Fatalf("while creating bundle: %v", err)
	}
	b.Recipe, _ = types.NewDefinitionFromURI("localimage://" + image)

	// Creates and execute packer
	lcp := &sources.LocalConveyorPacker{}
	if err := lcp.Get(t.Context(), b); err != nil {
		t.Fatalf("while getting local packer: %v", err)
	}
	if _, err = lcp.Pack(t.Context()); err != nil {
		t.Fatalf("failed to Pack from %s: %v\n", image, err)
	}
	rootfsPath := b.RootfsPath

	// Check if testfile was extracted
	path := filepath.Join(rootfsPath, testfile)
	if exist, _ := fs.PathExists(path); !exist {
		t.Errorf("extraction failed, %s is missing", path)
	}

	// Check directories
	path = filepath.Join(rootfsPath, "tmp")
	if !fs.IsDir(path) {
		t.Errorf("extraction failed, %s is missing", path)
	}
	path = filepath.Join(rootfsPath, "var")
	if !fs.IsDir(path) {
		t.Errorf("extraction failed, %s is missing", path)
	}

	// Check symlinks
	// $rootfsPath/var/tmp -> /tmp
	path = filepath.Join(rootfsPath, "var", "tmp")
	if exist, _ := fs.PathExists(path); !exist {
		t.Errorf("extraction failed, %s is missing", path)
	} else if !fs.IsLink(path) {
		t.Errorf("extraction failed, %s is not a symlink", path)
	} else {
		tgt, _ := os.Readlink(path)
		if tgt != "/tmp" {
			t.Errorf("extraction failed, %s wrongly points to %s", path, tgt)
		}
	}
	// $rootfsPath/var/log -> /tmp
	path = filepath.Join(rootfsPath, "var", "log")
	if exist, _ := fs.PathExists(path); !exist {
		t.Errorf("extraction failed, %s is missing", path)
	} else if !fs.IsLink(path) {
		t.Errorf("extraction failed, %s is not a symlink", path)
	} else {
		tgt, _ := os.Readlink(path)
		if tgt != "/tmp" {
			t.Errorf("extraction failed, %s wrongly points to %s", path, tgt)
		}
	}
}
