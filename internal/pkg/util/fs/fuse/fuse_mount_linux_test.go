// Copyright (c) 2023, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package fuse

import (
	"testing"
)

func TestExtraOptOverrides(t *testing.T) {
	testOneOverride(t, "rw")
	testOneOverride(t, "ro")
	testOneOverride(t, "dev")
	testOneOverride(t, "suid")
	testOneOverride(t, "allow_other")
}

func testOneOverride(t *testing.T, s string) {
	m := ImageMount{
		ExtraOpts: []string{s},
	}

	opts, err := m.generateMountOpts()
	if err == nil {
		t.Errorf("Failed to weed out %q mount option; opts: %#v", s, opts)
	}
}

func TestAllOverridesAtOnce(t *testing.T) {
	m := ImageMount{
		ExtraOpts: []string{"suid", "allow_other", "rw", "dev"},
	}

	opts, err := m.generateMountOpts()
	if err == nil {
		t.Errorf("Failed to properly filter mount options; opts: %#v", opts)
	}
}
