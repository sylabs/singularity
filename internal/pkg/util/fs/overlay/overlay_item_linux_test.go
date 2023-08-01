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

	"github.com/sylabs/singularity/v4/internal/pkg/test/tool/dirs"
	"github.com/sylabs/singularity/v4/internal/pkg/test/tool/require"
	"github.com/sylabs/singularity/v4/internal/pkg/util/fs"
	"github.com/sylabs/singularity/v4/pkg/image"
)

const (
	testFilePath       string = "file-for-testing"
	squashfsTestString string = "squashfs-test-string\n"
	extfsTestString    string = "extfs-test-string\n"
)

var (
	imgsPath        = filepath.Join("..", "..", "..", "..", "..", "test", "images")
	squashfsImgPath = filepath.Join(imgsPath, "squashfs-for-overlay.img")
	extfsImgPath    = filepath.Join(imgsPath, "extfs-for-overlay.img")
)

func mkTempDirOrFatal(t *testing.T) string {
	tmpDir, err := os.MkdirTemp(t.TempDir(), "testoverlayitem-")
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

func mkTempOlDirOrFatal(t *testing.T) string {
	tmpOlDir := mkTempDirOrFatal(t)
	dirs.MkdirOrFatal(t, filepath.Join(tmpOlDir, "upper"), 0o777)
	dirs.MkdirOrFatal(t, filepath.Join(tmpOlDir, "lower"), 0o777)

	return tmpOlDir
}

func TestItemWritableField(t *testing.T) {
	tmpOlDir := mkTempOlDirOrFatal(t)
	rwOverlayStr := tmpOlDir
	roOverlayStr := tmpOlDir + ":ro"

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
	tmpOlDir := mkTempOlDirOrFatal(t)
	rwOverlayStr := tmpOlDir
	roOverlayStr := tmpOlDir + ":ro"

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
	tmpOlDir := mkTempOlDirOrFatal(t)
	rwOverlayStr := tmpOlDir
	roOverlayStr := tmpOlDir + ":ro"

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
		t.Fatalf("while trying to mount dir %q: %s", olStr, err)
	}
	if err := item.Unmount(); err != nil {
		t.Errorf("while trying to unmount dir %q: %s", olStr, err)
	}
}

func TestDirMounts(t *testing.T) {
	dirMountUnmount(t, mkTempOlDirOrFatal(t)+":ro")
	dirMountUnmount(t, mkTempOlDirOrFatal(t))
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

	testFileStagedPath := filepath.Join(item.GetMountDir(), testFilePath)
	checkForStringInOverlay(t, typeStr, testFileStagedPath, expectStr)
}

func TestSquashfsRO(t *testing.T) {
	require.Command(t, "squashfuse")
	require.Command(t, "fusermount")
	tryImageRO(t, squashfsImgPath, image.SQUASHFS, "squashfs", squashfsTestString)
}

func TestExtfsRO(t *testing.T) {
	require.Command(t, "fuse2fs")
	require.Command(t, "fusermount")
	tmpDir := mkTempDirOrFatal(t)
	readonlyExtfsImgPath := filepath.Join(tmpDir, "readonly-extfs.img")
	if err := fs.CopyFile(extfsImgPath, readonlyExtfsImgPath, 0o444); err != nil {
		t.Fatalf("could not copy %q to %q: %s", extfsImgPath, readonlyExtfsImgPath, err)
	}
	tryImageRO(t, readonlyExtfsImgPath+":ro", image.EXT3, "extfs", extfsTestString)
}

func TestExtfsRW(t *testing.T) {
	require.Command(t, "fuse2fs")
	require.Command(t, "fuse-overlayfs")
	require.Command(t, "fusermount")
	tmpDir := mkTempDirOrFatal(t)

	// Create a copy of the extfs test image to be used for testing writable
	// extfs image overlays
	writableExtfsImgPath := filepath.Join(tmpDir, "writable-extfs.img")
	err := fs.CopyFile(extfsImgPath, writableExtfsImgPath, 0o755)
	if err != nil {
		t.Fatalf("could not copy %q to %q: %s", extfsImgPath, writableExtfsImgPath, err)
	}

	item, err := NewItemFromString(writableExtfsImgPath)
	if err != nil {
		t.Fatalf("failed to mount extfs image at %q: %s", writableExtfsImgPath, err)
	}

	if item.Type != image.EXT3 {
		t.Errorf("item.Type is %v (should be %v)", item.Type, image.EXT3)
	}

	if err := item.Mount(); err != nil {
		t.Fatalf("unable to mount extfs image for reading & writing: %s", err)
	}
	t.Cleanup(func() {
		item.Unmount()
	})

	testFileStagedPath := filepath.Join(item.GetMountDir(), testFilePath)
	checkForStringInOverlay(t, "extfs", testFileStagedPath, extfsTestString)
	otherTestFileStagedPath := item.GetMountDir() + "_other"
	otherExtfsTestString := "another string"
	err = os.WriteFile(otherTestFileStagedPath, []byte(otherExtfsTestString), 0o755)
	if err != nil {
		t.Errorf("could not write to file %q in extfs image %q: %s", otherTestFileStagedPath, writableExtfsImgPath, err)
	}
	checkForStringInOverlay(t, "extfs", otherTestFileStagedPath, otherExtfsTestString)
}

func checkForStringInOverlay(t *testing.T, typeStr, stagedPath, expectStr string) {
	data, err := os.ReadFile(stagedPath)
	if err != nil {
		t.Fatalf("error while trying to read from file %q: %s", stagedPath, err)
	}
	foundStr := string(data)
	if foundStr != expectStr {
		t.Errorf("incorrect file contents in %s img: expected %q, but found: %q", typeStr, expectStr, foundStr)
	}
}
