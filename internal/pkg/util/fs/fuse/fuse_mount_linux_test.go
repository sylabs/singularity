// Copyright (c) 2023, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package fuse

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/sylabs/singularity/v4/pkg/image"
)

var squashfsImgPath = filepath.Join("..", "..", "..", "..", "..", "test", "images", "squashfs-for-overlay.img")

var optsToTest = []string{
	"uid=0",
	"gid=0",
	"uid=23",
	"gid=67",
	"uid=2345",
	"gid=6789",
	"rw",
	"ro",
	"dev",
	"nodev",
	"suid",
	"nosuid",
	"allow_other",
}

func TestExtraOptOverrides(t *testing.T) {
	for _, opt := range optsToTest {
		testOneOverride(t, opt)
	}
}

func testOneOverride(t *testing.T, s string) {
	ctx := context.Background()

	m := ImageMount{
		Type:       image.SQUASHFS,
		SourcePath: squashfsImgPath,
	}

	if err := m.Mount(ctx); err != nil {
		t.Fatalf("Baseline mount of %q failed: %v", m.SourcePath, err)
	}
	if err := m.Unmount(ctx); err != nil {
		t.Fatalf("Baseline unmount of %q failed: %v", m.SourcePath, err)
	}

	m.ExtraOpts = []string{s}

	if err := m.Mount(ctx); err == nil {
		t.Errorf("Failed to block %q mount option.", s)
		if err := m.Unmount(ctx); err != nil {
			t.Fatalf("Post-test unmount of %q failed: %v", m.SourcePath, err)
		}
	}
}

func TestAllOverridesAtOnce(t *testing.T) {
	ctx := context.Background()

	m := ImageMount{
		Type:       image.SQUASHFS,
		SourcePath: squashfsImgPath,
	}

	if err := m.Mount(ctx); err != nil {
		t.Fatalf("Baseline mount of %q failed: %v", m.SourcePath, err)
	}
	if err := m.Unmount(ctx); err != nil {
		t.Fatalf("Baseline unmount of %q failed: %v", m.SourcePath, err)
	}

	m.ExtraOpts = optsToTest[:]

	if err := m.Mount(ctx); err == nil {
		t.Errorf("Failed to block mount options (%q).", m.ExtraOpts)
		if err := m.Unmount(ctx); err != nil {
			t.Fatalf("Post-test unmount of %q failed: %v", m.SourcePath, err)
		}
	}
}
