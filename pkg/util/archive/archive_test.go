// Copyright (c) 2026, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package archive

import (
	"archive/tar"
	"bytes"
	"os"
	"path/filepath"
	"testing"

	mobyarchive "github.com/moby/go-archive"
	"github.com/sylabs/singularity/v4/internal/pkg/util/fs"
)

func TestUnpackWithRoot(t *testing.T) {
	// An entry cannot be written outside unpacker.destpath.
	t.Run("entryEscape", func(t *testing.T) {
		dst := t.TempDir()
		outside := filepath.Join(filepath.Dir(dst), "escape")

		entries := []tarEntry{{name: "../escape", body: "foo", typeflag: tar.TypeReg}}

		if err := unpackTestArchive(t, entries, dst, dst); err == nil {
			t.Fatal("expected unpack to reject entry outside destination")
		}
		if _, err := os.Lstat(outside); !os.IsNotExist(err) {
			t.Fatalf("outside path was created or could not be checked: %v", err)
		}
	})

	// When replacing a symlink with a tar entry, replace the symlink itself.
	// Don't follow a symlink and replace a target outside the
	// unpacker.destpath.
	t.Run("replaceSymlink", func(t *testing.T) {
		// outside is a symlink to a file outside of unpacker.destpath.
		tmpDir := t.TempDir()
		dst := filepath.Join(tmpDir, "dst")
		outside := filepath.Join(tmpDir, "outside")
		if err := os.Mkdir(dst, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := fs.WriteFileNoFollow(outside, []byte("outside"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(outside, filepath.Join(dst, "file")); err != nil {
			t.Fatal(err)
		}

		entries := []tarEntry{{name: "file", body: "inside"}}

		if err := unpackTestArchive(t, entries, dst, dst); err != nil {
			t.Fatalf("unexpected unpack error: %v", err)
		}

		gotOutside, err := os.ReadFile(outside)
		if err != nil {
			t.Fatal(err)
		}
		if string(gotOutside) != "outside" {
			t.Fatalf("outside file was modified: %q", gotOutside)
		}
		gotInside, err := os.ReadFile(filepath.Join(dst, "file"))
		if err != nil {
			t.Fatal(err)
		}
		if string(gotInside) != "inside" {
			t.Fatalf("destination file content = %q, want inside", gotInside)
		}
		if fs.IsLink(filepath.Join(dst, "file")) {
			t.Fatal("destination should have been replaced with a regular file")
		}
	})

	// Must not traverse a symlink to write a file outside of unpacker.destpath.
	t.Run("parentSymlinkOutside", func(t *testing.T) {
		tmpDir := t.TempDir()
		dst := filepath.Join(tmpDir, "dst")
		outside := filepath.Join(tmpDir, "outside")
		if err := os.Mkdir(dst, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(outside, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(outside, filepath.Join(dst, "dir")); err != nil {
			t.Fatal(err)
		}

		entries := []tarEntry{{name: "dir/file", body: "bad"}}

		if err := unpackTestArchive(t, entries, dst, dst); err == nil {
			t.Fatal("expected unpack through parent symlink to fail")
		}
		if _, err := os.Lstat(filepath.Join(outside, "file")); !os.IsNotExist(err) {
			t.Fatalf("outside file was created or could not be checked: %v", err)
		}
	})

	// Hard link targets cannot be outside of unpacker.rootPath.
	t.Run("hardlinkOutsideRoot", func(t *testing.T) {
		dst := t.TempDir()

		entries := []tarEntry{{name: "link", linkname: "../target", typeflag: tar.TypeLink}}

		if err := unpackTestArchive(t, entries, dst, dst); err == nil {
			t.Fatal("expected hardlink outside destination root to fail")
		}
		if _, err := os.Lstat(filepath.Join(dst, "link")); !os.IsNotExist(err) {
			t.Fatalf("hardlink was created or could not be checked: %v", err)
		}
	})

	// Hard link targets can be outside of unpacker.destpath, if they are under unpacker.rootPath.
	t.Run("hardlinkWithinRootAboveDestination", func(t *testing.T) {
		tmpDir := t.TempDir()
		dst := filepath.Join(tmpDir, "dst")
		if err := os.Mkdir(dst, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := fs.WriteFileNoFollow(filepath.Join(tmpDir, "target"), []byte("target"), 0o644); err != nil {
			t.Fatal(err)
		}

		entries := []tarEntry{{name: "link", linkname: "../target", typeflag: tar.TypeLink}}

		if err := unpackTestArchive(t, entries, dst, tmpDir); err != nil {
			t.Fatalf("unexpected unpack error: %v", err)
		}
		got, err := os.ReadFile(filepath.Join(dst, "link"))
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != "target" {
			t.Fatalf("hardlink content = %q, want target", got)
		}
	})

	// Symlink targets cannot be outside of unpacker.rootPath.
	t.Run("symlinkOutsideRoot", func(t *testing.T) {
		dst := t.TempDir()

		entries := []tarEntry{{name: "link", linkname: "../target", typeflag: tar.TypeSymlink}}

		if err := unpackTestArchive(t, entries, dst, dst); err == nil {
			t.Fatal("expected symlink outside destination root to fail")
		}
		if _, err := os.Lstat(filepath.Join(dst, "link")); !os.IsNotExist(err) {
			t.Fatalf("symlink was created or could not be checked: %v", err)
		}
	})

	// Symlink targets can be outside of unpacker.destpath, if they are under unpacker.rootPath.
	t.Run("symlinkWithinRootAboveDestination", func(t *testing.T) {
		tmpDir := t.TempDir()
		dst := filepath.Join(tmpDir, "dst")
		if err := os.Mkdir(dst, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := fs.WriteFileNoFollow(filepath.Join(tmpDir, "target"), []byte("target"), 0o644); err != nil {
			t.Fatal(err)
		}

		entries := []tarEntry{{name: "link", linkname: "../target", typeflag: tar.TypeSymlink}}

		if err := unpackTestArchive(t, entries, dst, tmpDir); err != nil {
			t.Fatalf("unexpected unpack error: %v", err)
		}
		got, err := os.Readlink(filepath.Join(dst, "link"))
		if err != nil {
			t.Fatal(err)
		}
		if got != "../target" {
			t.Fatalf("symlink target = %q, want ../target", got)
		}
	})
}

type tarEntry struct {
	name     string
	linkname string
	body     string
	typeflag byte
}

func unpackTestArchive(t *testing.T, entries []tarEntry, dest, destRoot string) error {
	t.Helper()

	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	for _, entry := range entries {
		typeflag := entry.typeflag
		if typeflag == 0 {
			typeflag = tar.TypeReg
		}
		hdr := &tar.Header{
			Name:     entry.name,
			Linkname: entry.linkname,
			Mode:     0o644,
			Size:     int64(len(entry.body)),
			Typeflag: typeflag,
		}
		if typeflag != tar.TypeReg {
			hdr.Size = 0
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}
		if typeflag == tar.TypeReg {
			if _, err := tw.Write([]byte(entry.body)); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}

	return unpackWithRoot(bytes.NewReader(buf.Bytes()), dest, destRoot, &mobyarchive.TarOptions{
		NoLchown: true,
	})
}
