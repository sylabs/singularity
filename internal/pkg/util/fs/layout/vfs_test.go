// Copyright (c) 2018-2026, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package layout

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/sylabs/singularity/v4/internal/pkg/test"
)

func TestVFSEscape(t *testing.T) {
	test.DropPrivilege(t)
	defer test.ResetPrivilege(t)

	rootDir := t.TempDir()
	outsideDir := t.TempDir()
	outsideFile := filepath.Join(outsideDir, "file")
	if err := os.WriteFile(outsideFile, []byte("test"), 0o644); err != nil {
		t.Fatalf("failed to create file outside of root dir: %v", err)
	}
	linkToOutside := filepath.Join(rootDir, "link-to-outside")
	if err := os.Symlink(outsideDir, linkToOutside); err != nil {
		t.Fatalf("failed to create symlink attack path: %v", err)
	}
	insideDir := filepath.Join(rootDir, "inside")
	if err := os.Mkdir(insideDir, 0o755); err != nil {
		t.Fatalf("failed to create directory inside root dir: %v", err)
	}

	v, err := NewRootedVFS(rootDir)
	if err != nil {
		t.Fatalf("failed to create rooted VFS: %v", err)
	}

	mustReject := func(name string, err error) {
		assert.Error(t, err, "%s: expected out-of-root path to be rejected", name)
	}

	// Cannot create anything outside of the root.
	mustReject("Mkdir", v.Mkdir(filepath.Join(linkToOutside+"/foo"), 0o755))
	mustReject("WriteFile", v.WriteFile(filepath.Join(linkToOutside, "/foo"), []byte("x"), 0o644))
	mustReject("Symlink", v.Symlink("target", linkToOutside+"/foo"))

	// RootedVFS operations are relative to the root, like os.Root.
	mustReject("Stat-absolute-inside-root", func() error {
		_, err := v.Stat(insideDir)
		return err
	}())

	// Cannot modify anything outside of the root.
	mustReject("Chown", v.Chown(linkToOutside, os.Getuid(), os.Getgid()))
	mustReject("Lchown", v.Lchown(filepath.Join(linkToOutside, "file"), os.Getuid(), os.Getgid()))

	// Relative traversal paths must also be rejected when they escape root.
	mustReject("Mkdir-relative", v.Mkdir("../outside", 0o755))
	mustReject("WriteFile-relative", v.WriteFile("../outside", []byte("x"), 0o644))
	mustReject("Symlink-relative", v.Symlink("target", "../outside"))
	mustReject("Chown-relative", v.Chown("../outside", os.Getuid(), os.Getgid()))
	mustReject("Lchown-relative", v.Lchown("../outside", os.Getuid(), os.Getgid()))
	mustReject("Stat-relative", func() error {
		_, err := v.Stat("../outside")
		return err
	}())
	mustReject("ReadDir-relative", func() error {
		_, err := v.ReadDir("../outside")
		return err
	}())
	mustReject("Readlink-relative", func() error {
		_, err := v.Readlink("../outside")
		return err
	}())

	// Cannot examine metadata of things outside of the root.
	mustReject("Stat", func() error {
		_, err := v.Stat(filepath.Join(linkToOutside, "file"))
		return err
	}())
	mustReject("ReadDir", func() error {
		_, err := v.ReadDir(linkToOutside)
		return err
	}())
	mustReject("Readlink", func() error {
		_, err := v.Readlink(filepath.Join(linkToOutside, "file"))
		return err
	}())
}
