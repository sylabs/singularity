// Copyright (c) 2019-2023, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package overlay

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sylabs/singularity/internal/pkg/test/tool/require"
	"github.com/sylabs/singularity/pkg/image"
)

const (
	testFilePath       string = "file-for-testing"
	squashfsTestString string = "squashfs-test-string\n"
	extfsTestString    string = "extfs-test-string\n"
)

func mkTempDirOrFatal(t *testing.T) string {
	tmpDir, err := os.MkdirTemp("", "testoverlayitem-")
	if err != nil {
		t.Fatalf("failed to create temporary dir: %s", err)
	}
	t.Cleanup(func() {
		if !t.Failed() {
			os.RemoveAll(tmpDir)
		}
	})

	return tmpDir
}

func TestItemWritableField(t *testing.T) {
	tmpDir := mkTempDirOrFatal(t)
	rwOverlayStr := tmpDir
	roOverlayStr := tmpDir + ":ro"

	rwItem, err := NewItemFromString(rwOverlayStr)
	if err != nil {
		t.Fatalf("unexpected error while initializing rwItem from string %q: %s", rwOverlayStr, err)
	}
	roItem, err := NewItemFromString(roOverlayStr)
	if err != nil {
		t.Fatalf("unexpected error while initializing roItem from string %q: %s", roOverlayStr, err)
	}

	if !rwItem.Writable {
		t.Errorf("Writable field of overlay.Item initialized with string %q should be true but is false", rwOverlayStr)
	}

	if roItem.Writable {
		t.Errorf("Writable field of overlay.Item initialized with string %q should be false but is true", roOverlayStr)
	}
}

func TestItemMissing(t *testing.T) {
	const dir string = "/testoverlayitem-this_should_be_missing"
	rwOverlayStr := dir
	roOverlayStr := dir + ":ro"

	if _, err := NewItemFromString(rwOverlayStr); err == nil {
		t.Errorf("unexpected success: initializing overlay.Item with missing file/dir (%q) should have failed", rwOverlayStr)
	}
	if _, err := NewItemFromString(roOverlayStr); err == nil {
		t.Errorf("unexpected success: initializing overlay.Item with missing file/dir (%q) should have failed", roOverlayStr)
	}
}

func verifyAutoParentDir(t *testing.T, item *Item) {
	const autoParentDirStr string = "overlay-parent-"
	if parentDir, err := item.GetParentDir(); err != nil {
		t.Fatalf("unexpected error while calling Item.GetParentDir(): %s", err)
	} else if !strings.Contains(parentDir, autoParentDirStr) {
		t.Errorf("auto-generated parent dir %q does not contain expected identifier string %q", parentDir, autoParentDirStr)
	} else if !strings.HasPrefix(parentDir, "/tmp/") {
		t.Errorf("auto-generated parent dir %q is not in expected location", parentDir)
	}
}

func TestAutofillParentDir(t *testing.T) {
	tmpDir := mkTempDirOrFatal(t)
	rwOverlayStr := tmpDir
	roOverlayStr := tmpDir + ":ro"

	rwItem, err := NewItemFromString(rwOverlayStr)
	if err != nil {
		t.Fatalf("unexpected error while initializing rwItem from string %q: %s", rwOverlayStr, err)
	}
	roItem, err := NewItemFromString(roOverlayStr)
	if err != nil {
		t.Fatalf("unexpected error while initializing roItem from string %q: %s", roOverlayStr, err)
	}

	verifyAutoParentDir(t, rwItem)
	verifyAutoParentDir(t, roItem)
}

func verifyExplicitParentDir(t *testing.T, item *Item, dir string) {
	item.SetParentDir(dir)
	if parentDir, err := item.GetParentDir(); err != nil {
		t.Fatalf("unexpected error while calling Item.GetParentDir(): %s", err)
	} else if parentDir != dir {
		t.Errorf("item returned parent dir %q (expected: %q)", parentDir, dir)
	}
}

func TestExplicitParentDir(t *testing.T) {
	tmpDir := mkTempDirOrFatal(t)
	rwOverlayStr := tmpDir
	roOverlayStr := tmpDir + ":ro"

	rwItem, err := NewItemFromString(rwOverlayStr)
	if err != nil {
		t.Fatalf("unexpected error while initializing rwItem from string %q: %s", rwOverlayStr, err)
	}
	roItem, err := NewItemFromString(roOverlayStr)
	if err != nil {
		t.Fatalf("unexpected error while initializing roItem from string %q: %s", roOverlayStr, err)
	}

	verifyExplicitParentDir(t, rwItem, "/my-special-directory")
	verifyExplicitParentDir(t, roItem, "/my-other-special-directory")
}

func verifyDirExistsAndWritable(t *testing.T, dir string) {
	s, err := os.Stat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			t.Errorf("expected directory %q not found", dir)
		} else {
			t.Fatalf("unexpected error while looking for directory %q: %s", dir, err)
		}
		return
	}

	if !s.IsDir() {
		t.Fatalf("expected %q to be a directory but it is not", dir)
		return
	}

	file, err := os.CreateTemp(dir, "attempt-to-write-a-file")
	if err != nil {
		t.Fatalf("could not create a file inside %q, which should have been writable: %s", dir, err)
	}
	path := file.Name()
	file.Close()
	if err := os.Remove(path); err != nil {
		t.Fatalf("unexpected error while trying to remove temporary file %q: %s", path, err)
	}
}

func TestUpperAndWorkCreation(t *testing.T) {
	tmpDir := mkTempDirOrFatal(t)

	item, err := NewItemFromString(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error while initializing rwItem from string %q: %s", tmpDir, err)
	}

	if err := item.prepareWritableOverlay(); err != nil {
		t.Fatalf("unexpected error while calling prepareWritableOverlay(): %s", err)
	}

	verifyDirExistsAndWritable(t, tmpDir+"/upper")
	verifyDirExistsAndWritable(t, tmpDir+"/work")
}

func dirMountUnmount(t *testing.T, olStr string) {
	item, err := NewItemFromString(olStr)
	if err != nil {
		t.Fatalf("unexpected error while initializing overlay item from string %q: %s", olStr, err)
	}

	if err := item.Mount(); err != nil {
		t.Fatalf("error encountered while trying to mount dir %q: %s", olStr, err)
	}
	if err := item.Unmount(); err != nil {
		t.Errorf("error encountered while trying to unmount dir %q: %s", olStr, err)
	}
}

func TestDirMounts(t *testing.T) {
	dirMountUnmount(t, mkTempDirOrFatal(t)+":ro")
	dirMountUnmount(t, mkTempDirOrFatal(t))
}

func tryImageRO(t *testing.T, olStr string, typeCode int, typeStr, expectStr string) {
	item, err := NewItemFromString(olStr)
	if err != nil {
		t.Fatalf("failed to mount %s image at %q: %s", typeStr, olStr, err)
	}

	if item.Type != typeCode {
		t.Errorf("item.Type is %v (should be %v)", item.Type, typeStr)
	}

	if err := item.Mount(); err != nil {
		t.Fatalf("unable to mount %s image for reading: %s", typeStr, err)
	}
	t.Cleanup(func() {
		item.Unmount()
	})

	testFileStagedPath := filepath.Join(item.StagingDir, testFilePath)
	data, err := os.ReadFile(testFileStagedPath)
	if err != nil {
		t.Fatalf("error while trying to read from file %q: %s", testFileStagedPath, err)
	}
	foundStr := string(data)
	if foundStr != expectStr {
		t.Errorf("incorrect file contents in %s img: expected %q, but found: %q", typeStr, expectStr, foundStr)
	}
}

func TestSquashfsRO(t *testing.T) {
	require.Command(t, "squashfuse")
	require.Command(t, "fusermount")
	tryImageRO(t, filepath.Join("..", "..", "..", "..", "..", "test", "images", "squashfs-for-overlay.img"), image.SQUASHFS, "squashfs", squashfsTestString)
}

func TestExtfsRO(t *testing.T) {
	require.Command(t, "fuse2fs")
	require.Command(t, "fusermount")
	tryImageRO(t, filepath.Join("..", "..", "..", "..", "..", "test", "images", "extfs-for-overlay.img")+":ro", image.EXT3, "extfs", extfsTestString)
}
