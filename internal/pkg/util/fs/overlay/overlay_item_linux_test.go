// Copyright (c) 2019-2023, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package overlay

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sylabs/singularity/v4/internal/pkg/test/tool/dirs"
	"github.com/sylabs/singularity/v4/internal/pkg/test/tool/require"
	"github.com/sylabs/singularity/v4/internal/pkg/util/fs"
	"github.com/sylabs/singularity/v4/pkg/image"
	"github.com/sylabs/singularity/v4/pkg/util/fs/proc"
	"github.com/sylabs/singularity/v4/pkg/util/slice"
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

	if rwItem.Readonly {
		t.Errorf("Readonly field of overlay.Item initialized with string %q should be false but is true", rwOverlayStr)
	}

	if !roItem.Readonly {
		t.Errorf("Readonly field of overlay.Item initialized with string %q should be true but is false", roOverlayStr)
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

func TestDirMounts(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name            string
		olStr           string
		allowSetUID     bool
		allowDev        bool
		expectMountOpts []string
	}{
		{
			name:            "RO",
			olStr:           mkTempOlDirOrFatal(t) + ":ro",
			allowSetUID:     false,
			allowDev:        false,
			expectMountOpts: []string{"ro", "nosuid", "nodev"},
		},
		{
			name:            "RW",
			olStr:           mkTempOlDirOrFatal(t),
			allowSetUID:     false,
			allowDev:        false,
			expectMountOpts: []string{"rw", "nosuid", "nodev"},
		},
		{
			name:            "AllowSetuid",
			olStr:           mkTempOlDirOrFatal(t),
			allowSetUID:     true,
			allowDev:        false,
			expectMountOpts: []string{"rw", "nodev"},
		},
		{
			name:            "AllowDev",
			olStr:           mkTempOlDirOrFatal(t),
			allowSetUID:     false,
			allowDev:        true,
			expectMountOpts: []string{"rw", "nosuid"},
		},
		{
			name:            "AllowSetuidDev",
			olStr:           mkTempOlDirOrFatal(t),
			allowSetUID:     true,
			allowDev:        true,
			expectMountOpts: []string{"rw"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			item, err := NewItemFromString(tt.olStr)
			if err != nil {
				t.Fatalf("unexpected error while initializing overlay item from string %q: %s", tt.olStr, err)
			}

			item.SetAllowSetuid(tt.allowSetUID)
			item.SetAllowDev(tt.allowDev)

			if err := item.Mount(ctx); err != nil {
				t.Fatalf("while trying to mount dir %q: %s", tt.olStr, err)
			}

			checkMountOpts(t, item.StagingDir, tt.expectMountOpts)

			if err := item.Unmount(ctx); err != nil {
				t.Errorf("while trying to unmount dir %q: %s", tt.olStr, err)
			}
		})
	}
}

func TestImageRO(t *testing.T) {
	require.Command(t, "fusermount")
	ctx := context.Background()

	tests := []struct {
		name            string
		fusebin         string
		olStr           string
		typeCode        int
		typeStr         string
		allowSetUID     bool
		allowDev        bool
		expectStr       string
		expectMountOpts []string
	}{
		{
			name:            "squashfs",
			fusebin:         "squashfuse",
			olStr:           squashfsImgPath,
			typeCode:        image.SQUASHFS,
			typeStr:         "squashfs",
			expectStr:       squashfsTestString,
			expectMountOpts: []string{"ro", "nosuid", "nodev"},
		},
		{
			name:      "extfs",
			fusebin:   "fuse2fs",
			olStr:     extfsImgPath + ":ro",
			typeCode:  image.EXT3,
			typeStr:   "extfs",
			expectStr: extfsTestString,
			// NOTE - fuse2fs mount shows a "rw" mount option even when mounted "ro".
			// However, it does actually restrict read-only.
			expectMountOpts: []string{"rw", "nosuid", "nodev"},
		},
		{
			name:            "AllowSetuid",
			fusebin:         "squashfuse",
			olStr:           squashfsImgPath,
			typeCode:        image.SQUASHFS,
			typeStr:         "squashfs",
			allowSetUID:     true,
			allowDev:        false,
			expectStr:       squashfsTestString,
			expectMountOpts: []string{"ro", "nodev"},
		},
		{
			name:            "AllowDev",
			fusebin:         "squashfuse",
			olStr:           squashfsImgPath,
			typeCode:        image.SQUASHFS,
			typeStr:         "squashfs",
			allowSetUID:     false,
			allowDev:        true,
			expectStr:       squashfsTestString,
			expectMountOpts: []string{"ro", "nosuid"},
		},
		{
			name:            "AllowSetuidDev",
			fusebin:         "squashfuse",
			olStr:           squashfsImgPath,
			typeCode:        image.SQUASHFS,
			typeStr:         "squashfs",
			allowSetUID:     true,
			allowDev:        true,
			expectStr:       squashfsTestString,
			expectMountOpts: []string{"ro"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Command(t, tt.fusebin)

			item, err := NewItemFromString(tt.olStr)
			if err != nil {
				t.Fatalf("unexpected error while initializing overlay item from string %q: %s", tt.olStr, err)
			}
			item.SetAllowSetuid(tt.allowSetUID)
			item.SetAllowDev(tt.allowDev)

			if item.Type != tt.typeCode {
				t.Errorf("item.Type is %v (should be %v)", item.Type, tt.typeCode)
			}

			if err := item.Mount(ctx); err != nil {
				t.Fatalf("unable to mount image for reading: %s", err)
			}
			t.Cleanup(func() {
				item.Unmount(ctx)
			})

			testFileStagedPath := filepath.Join(item.GetMountDir(), testFilePath)
			checkForStringInOverlay(t, tt.typeStr, testFileStagedPath, tt.expectStr)
			checkMountOpts(t, item.StagingDir, tt.expectMountOpts)
		})
	}
}

func TestExtfsRW(t *testing.T) {
	require.Command(t, "fuse2fs")
	require.Command(t, "fuse-overlayfs")
	require.Command(t, "fusermount")
	tmpDir := mkTempDirOrFatal(t)
	ctx := context.Background()

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

	if err := item.Mount(ctx); err != nil {
		t.Fatalf("unable to mount extfs image for reading & writing: %s", err)
	}
	t.Cleanup(func() {
		item.Unmount(ctx)
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
	checkMountOpts(t, item.StagingDir, []string{"rw", "nosuid", "nodev", "relatime"})
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

func checkMountOpts(t *testing.T, mountPoint string, wantOpts []string) {
	entries, err := proc.GetMountInfoEntry("/proc/self/mountinfo")
	if err != nil {
		t.Fatal(err)
	}

	for _, e := range entries {
		if e.Point != mountPoint {
			continue
		}

		// Drop "relatime" as it may or may not be a default depending on distro, and is of no practical relevance to us.
		haveOpts := slice.Subtract(e.Options, []string{"relatime"})

		if len(slice.Subtract(haveOpts, wantOpts)) != 0 {
			t.Errorf("Mount %q has options %q, expected %q", mountPoint, e.Options, wantOpts)
		}
		return
	}

	t.Errorf("Mount %q not found", mountPoint)
}
