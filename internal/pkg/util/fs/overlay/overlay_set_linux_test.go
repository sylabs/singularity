// Copyright (c) 2023, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package overlay

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/sylabs/singularity/v4/internal/pkg/test/tool/require"
	"github.com/sylabs/singularity/v4/internal/pkg/util/fs"
)

func addROItemOrFatal(t *testing.T, s *Set, olStr string) *Item {
	i, err := NewItemFromString(olStr)
	if err != nil {
		t.Fatalf("could not initialize overlay item from string %q: %s", olStr, err)
	}
	s.ReadonlyOverlays = append(s.ReadonlyOverlays, i)

	return i
}

// wrapOverlayTest takes a testing function and wraps it in code that checks if
// the kernel has support for unprivileged overlays. If it does, the underlying
// function will be run twice, once with using kernel overlays and once using
// fuse-overlayfs (if present). Otherwise, only the latter option will be
// attempted.
func wrapOverlayTest(f func(t *testing.T)) func(t *testing.T) {
	unprivOls, unprivOlsErr := UnprivOverlaysSupported()
	return func(t *testing.T) {
		if unprivOlsErr != nil {
			t.Fatalf("while checking for unprivileged overlay support in kernel: %s", unprivOlsErr)
		}

		if unprivOls {
			kernelOverlayFunc := func(t *testing.T) {
				require.Command(t, "fusermount")
				f(t)
			}

			t.Run("kerneloverlay", kernelOverlayFunc)
			unprivOverlays.kernelSupport = false
		}

		fuseOverlayFunc := func(t *testing.T) {
			require.Command(t, "fuse-overlayfs")
			require.Command(t, "fusermount")
			f(t)
		}

		t.Run("fuseoverlayfs", fuseOverlayFunc)
		unprivOverlays.kernelSupport = unprivOls
	}
}

func TestAllTypesAtOnce(t *testing.T) {
	wrapOverlayTest(func(t *testing.T) {
		ctx := context.Background()
		s := Set{}

		tmpRoOlDir := mkTempOlDirOrFatal(t)
		addROItemOrFatal(t, &s, tmpRoOlDir+":ro")

		squashfsSupported := false
		if _, err := exec.LookPath("squashfs"); err == nil {
			squashfsSupported = true
			addROItemOrFatal(t, &s, squashfsImgPath)
		}

		extfsSupported := false
		if _, err := exec.LookPath("fuse2fs"); err == nil {
			extfsSupported = true
			tmpDir := mkTempDirOrFatal(t)
			readonlyExtfsImgPath := filepath.Join(tmpDir, "readonly-extfs.img")
			if err := fs.CopyFile(extfsImgPath, readonlyExtfsImgPath, 0o444); err != nil {
				t.Fatalf("could not copy %q to %q: %s", extfsImgPath, readonlyExtfsImgPath, err)
			}
			addROItemOrFatal(t, &s, readonlyExtfsImgPath+":ro")
		}

		tmpRwOlDir := mkTempOlDirOrFatal(t)
		i, err := NewItemFromString(tmpRwOlDir)
		if err != nil {
			t.Fatalf("failed to create writable-dir overlay item (%q): %s", tmpRwOlDir, err)
		}
		s.WritableOverlay = i

		rootfsDir := mkTempDirOrFatal(t)
		if err := s.Mount(ctx, rootfsDir); err != nil {
			t.Fatalf("failed to mount overlay set: %s", err)
		}
		t.Cleanup(func() {
			if t.Failed() {
				s.Unmount(ctx, rootfsDir)
			}
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

		if err := s.Unmount(ctx, rootfsDir); err != nil {
			t.Errorf("while trying to unmount overlay set: %s", err)
		}
	})(t)
}

func TestPersistentWriteToDir(t *testing.T) {
	wrapOverlayTest(func(t *testing.T) {
		tmpRwOlDir := mkTempOlDirOrFatal(t)
		ctx := context.Background()
		i, err := NewItemFromString(tmpRwOlDir)
		if err != nil {
			t.Fatalf("failed to create writable-dir overlay item (%q): %s", tmpRwOlDir, err)
		}
		s := Set{WritableOverlay: i}

		performPersistentWriteTest(ctx, t, s)
	})(t)
}

func TestPersistentWriteToExtfsImg(t *testing.T) {
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

	i, err := NewItemFromString(writableExtfsImgPath)
	if err != nil {
		t.Fatalf("failed to create writable-dir overlay item (%q): %s", writableExtfsImgPath, err)
	}
	s := Set{WritableOverlay: i}

	performPersistentWriteTest(ctx, t, s)
}

func performPersistentWriteTest(ctx context.Context, t *testing.T, s Set) {
	rootfsDir := mkTempDirOrFatal(t)

	// This cleanup will serve adequately for both iterations of the overlay-set
	// mounting, below. If it happens to get called while the set is not
	// mounted, it should fail silently.
	// Mount the overlay set, write a string to a file, and unmount.
	// Mount the same set again, and check that we see the file with the
	// expected contents.
	t.Cleanup(func() {
		if t.Failed() {
			s.Unmount(ctx, rootfsDir)
		}
	})

	if err := s.Mount(ctx, rootfsDir); err != nil {
		t.Fatalf("failed to mount overlay set: %s", err)
	}
	expectStr := "my_test_string"
	bytes := []byte(expectStr)
	testFilePath := "my_test_file"
	testFileMountedPath := filepath.Join(rootfsDir, testFilePath)
	if err := os.WriteFile(testFileMountedPath, bytes, 0o644); err != nil {
		t.Fatalf("while trying to write file inside mounted overlay-set: %s", err)
	}

	if err := s.Unmount(ctx, rootfsDir); err != nil {
		t.Fatalf("while trying to unmount overlay set: %s", err)
	}

	if err := s.Mount(ctx, rootfsDir); err != nil {
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
	if err := s.Unmount(ctx, rootfsDir); err != nil {
		t.Errorf("while trying to unmount overlay set: %s", err)
	}
}

func TestDuplicateItemsInSet(t *testing.T) {
	wrapOverlayTest(func(t *testing.T) {
		var rootfsDir string
		var rwI *Item
		var err error

		ctx := context.Background()
		s := Set{}

		// First, test mounting of an overlay set with only readonly items, one of
		// which is a duplicate of another.
		addROItemOrFatal(t, &s, mkTempOlDirOrFatal(t)+":ro")
		roI2 := addROItemOrFatal(t, &s, mkTempOlDirOrFatal(t)+":ro")
		addROItemOrFatal(t, &s, mkTempOlDirOrFatal(t)+":ro")
		addROItemOrFatal(t, &s, roI2.SourcePath+":ro")
		addROItemOrFatal(t, &s, mkTempOlDirOrFatal(t)+":ro")

		rootfsDir = mkTempDirOrFatal(t)
		if err := s.Mount(ctx, rootfsDir); err == nil {
			t.Errorf("unexpected success: Mounting overlay.Set with duplicate (%q) should have failed", roI2.SourcePath)
			if err := s.Unmount(ctx, rootfsDir); err != nil {
				t.Fatalf("could not unmount erroneous successful mount of overlay set: %s", err)
			}
		}

		// Next, test mounting of an overlay set with a writable item as well as
		// several readonly items, one of which is a duplicate of another.
		tmpRwOlDir := mkTempOlDirOrFatal(t)
		rwI, err = NewItemFromString(tmpRwOlDir)
		if err != nil {
			t.Fatalf("failed to create writable-dir overlay item (%q): %s", tmpRwOlDir, err)
		}
		s.WritableOverlay = rwI

		rootfsDir = mkTempDirOrFatal(t)
		if err := s.Mount(ctx, rootfsDir); err == nil {
			t.Errorf("unexpected success: Mounting overlay.Set with duplicate (%q) should have failed", roI2.SourcePath)
			if err := s.Unmount(ctx, rootfsDir); err != nil {
				t.Fatalf("could not unmount erroneous successful mount of overlay set: %s", err)
			}
		}
	})(t)
}
