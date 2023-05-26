// Copyright (c) 2023, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package overlay

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func addROItemOrFatal(t *testing.T, s *Set, olStr string) *Item {
	i, err := NewItemFromString(olStr)
	if err != nil {
		t.Fatalf("could not initialize overlay item from string %q: %s", olStr, err)
	}
	s.ReadonlyOverlays = append(s.ReadonlyOverlays, i)

	return i
}

func TestAllTypesAtOnce(t *testing.T) {
	s := Set{}

	tmpRODir := mkTempDirOrFatal(t)
	addROItemOrFatal(t, &s, tmpRODir+":ro")

	squashfsSupported := false
	if _, err := exec.LookPath("squashfs"); err == nil {
		squashfsSupported = true
		addROItemOrFatal(t, &s, filepath.Join(".", "testdata", "squashfs.img"))
	}

	extfsSupported := false
	if _, err := exec.LookPath("fuse2fs"); err == nil {
		extfsSupported = true
		addROItemOrFatal(t, &s, filepath.Join(".", "testdata", "extfs.img")+":ro")
	}

	tmpRWDir := mkTempDirOrFatal(t)
	i, err := NewItemFromString(tmpRWDir)
	if err != nil {
		t.Fatalf("failed to create writable-dir overlay item (%q): %s", tmpRWDir, err)
	}
	s.WritableOverlay = i

	rootfsDir := mkTempDirOrFatal(t)
	if err := s.Mount(rootfsDir); err != nil {
		t.Fatalf("failed to mount overlay set: %s", err)
	}
	t.Cleanup(func() {
		s.Unmount(rootfsDir)
	})

	var expectStr string
	if extfsSupported {
		expectStr = extfsTestString
	} else if squashfsSupported {
		expectStr = squashfsTestString
	}

	if squashfsSupported || extfsSupported {
		testFileMountedPath := filepath.Join(rootfsDir, testFilePath)
		data, err := os.ReadFile(testFileMountedPath)
		if err != nil {
			t.Fatalf("error while trying to read from file %q: %s", testFileMountedPath, err)
		}
		foundStr := string(data)
		if foundStr != expectStr {
			t.Errorf("incorrect file contents in mounted overlay set: expected %q, but found: %q", expectStr, foundStr)
		}
	}

	if err := s.Unmount(rootfsDir); err != nil {
		t.Errorf("error encountered while trying to unmount overlay set: %s", err)
	}
}

func TestPersistentWriteToDir(t *testing.T) {
	tmpRWDir := mkTempDirOrFatal(t)
	i, err := NewItemFromString(tmpRWDir)
	if err != nil {
		t.Fatalf("failed to create writable-dir overlay item (%q): %s", tmpRWDir, err)
	}
	s := Set{WritableOverlay: i}

	rootfsDir := mkTempDirOrFatal(t)

	// This cleanup will serve adequately for both iterations of the overlay-set
	// mounting, below. If it happens to get called while the set is not
	// mounted, it should fail silently.
	t.Cleanup(func() {
		s.Unmount(rootfsDir)
	})

	// Mount the overlay set, write a string to a file, and unmount.
	if err := s.Mount(rootfsDir); err != nil {
		t.Fatalf("failed to mount overlay set: %s", err)
	}
	expectStr := "my_test_string"
	bytes := []byte(expectStr)
	testFilePath := "my_test_file"
	testFileMountedPath := filepath.Join(rootfsDir, testFilePath)
	if err := os.WriteFile(testFileMountedPath, bytes, 0o644); err != nil {
		t.Fatalf("error encountered while trying to write file inside mounted overlay-set: %s", err)
	}

	if err := s.Unmount(rootfsDir); err != nil {
		t.Fatalf("error encountered while trying to unmount overlay set: %s", err)
	}

	// Mount the same set again, and check that we see the file with the
	// expected contents.
	if err := s.Mount(rootfsDir); err != nil {
		t.Fatalf("failed to mount overlay set: %s", err)
	}
	data, err := os.ReadFile(testFileMountedPath)
	if err != nil {
		t.Fatalf("error while trying to read from file %q: %s", testFileMountedPath, err)
	}
	foundStr := string(data)
	if foundStr != expectStr {
		t.Errorf("incorrect file contents in mounted overlay set: expected %q, but found: %q", expectStr, foundStr)
	}
	if err := s.Unmount(rootfsDir); err != nil {
		t.Errorf("error encountered while trying to unmount overlay set: %s", err)
	}
}

func TestDuplicateItemsInSet(t *testing.T) {
	var rootfsDir string
	var rwI *Item
	var err error

	s := Set{}

	// First, test mounting of an overlay set with only readonly items, one of
	// which is a duplicate of another.
	addROItemOrFatal(t, &s, mkTempDirOrFatal(t)+":ro")
	roI2 := addROItemOrFatal(t, &s, mkTempDirOrFatal(t)+":ro")
	addROItemOrFatal(t, &s, mkTempDirOrFatal(t)+":ro")
	addROItemOrFatal(t, &s, roI2.SourcePath+":ro")
	addROItemOrFatal(t, &s, mkTempDirOrFatal(t)+":ro")

	rootfsDir = mkTempDirOrFatal(t)
	if err := s.Mount(rootfsDir); err == nil {
		t.Errorf("unexpected success: Mounting overlay.Set with duplicate (%q) should have failed", roI2.SourcePath)
		if err := s.Unmount(rootfsDir); err != nil {
			t.Fatalf("could not unmount erroneous successful mount of overlay set: %s", err)
		}
	}

	// Next, test mounting of an overlay set with a writable item as well as
	// several readonly items, one of which is a duplicate of another.
	tmpRWDir := mkTempDirOrFatal(t)
	rwI, err = NewItemFromString(tmpRWDir)
	if err != nil {
		t.Fatalf("failed to create writable-dir overlay item (%q): %s", tmpRWDir, err)
	}
	s.WritableOverlay = rwI

	rootfsDir = mkTempDirOrFatal(t)
	if err := s.Mount(rootfsDir); err == nil {
		t.Errorf("unexpected success: Mounting overlay.Set with duplicate file/dir (%q) should have failed", roI2.SourcePath)
		if err := s.Unmount(rootfsDir); err != nil {
			t.Fatalf("could not unmount erroneous successful mount of overlay set: %s", err)
		}
	}
}
