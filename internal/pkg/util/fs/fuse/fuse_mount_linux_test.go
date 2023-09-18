// Copyright (c) 2023, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package fuse

import (
	"strings"
	"testing"

	"github.com/samber/lo"
	"github.com/sylabs/singularity/v4/pkg/image"
)

func TestReadonlyOverride(t *testing.T) {
	m1 := ImageMount{
		Type:      image.SQUASHFS,
		Readonly:  true,
		ExtraOpts: []string{"rw"},
	}

	opts := m1.generateMountOpts()
	if lo.ContainsBy(opts, func(s string) bool {
		splitted := strings.SplitN(s, "=", 2)
		return (strings.ToLower(splitted[0]) == "rw")
	}) {
		t.Errorf("Failed to weed out 'rw' mount option; opts: %#v", opts)
	}

	m2 := ImageMount{
		Type:      image.SQUASHFS,
		Readonly:  false,
		ExtraOpts: []string{"ro"},
	}

	opts = m2.generateMountOpts()
	if lo.ContainsBy(opts, func(s string) bool {
		splitted := strings.SplitN(s, "=", 2)
		return (strings.ToLower(splitted[0]) == "ro")
	}) {
		t.Errorf("Failed to weed out 'ro' mount option; opts: %#v", opts)
	}
}

func TestDevOverride(t *testing.T) {
	m := ImageMount{
		Type:      image.SQUASHFS,
		AllowDev:  false,
		ExtraOpts: []string{"dev"},
	}

	opts := m.generateMountOpts()
	if lo.ContainsBy(opts, func(s string) bool {
		splitted := strings.SplitN(s, "=", 2)
		return (strings.ToLower(splitted[0]) == "dev")
	}) {
		t.Errorf("Failed to weed out 'dev' mount option; opts: %#v", opts)
	}
}

func TestSetuidOverride(t *testing.T) {
	m := ImageMount{
		Type:        image.SQUASHFS,
		AllowSetuid: false,
		ExtraOpts:   []string{"suid"},
	}

	opts := m.generateMountOpts()
	if lo.ContainsBy(opts, func(s string) bool {
		splitted := strings.SplitN(s, "=", 2)
		return (strings.ToLower(splitted[0]) == "suid")
	}) {
		t.Errorf("Failed to weed out 'suid' mount option; opts: %#v", opts)
	}
}

func TestAllowOtherOverride(t *testing.T) {
	m := ImageMount{
		Type:       image.SQUASHFS,
		AllowOther: false,
		ExtraOpts:  []string{"allow_other"},
	}

	opts := m.generateMountOpts()
	if lo.ContainsBy(opts, func(s string) bool {
		splitted := strings.SplitN(s, "=", 2)
		return (strings.ToLower(splitted[0]) == "allow_other")
	}) {
		t.Errorf("Failed to weed out 'allow_other' mount option; opts: %#v", opts)
	}
}

func TestAllOverridesAtOnce(t *testing.T) {
	m := ImageMount{
		Type:        image.SQUASHFS,
		Readonly:    true,
		AllowDev:    false,
		AllowSetuid: false,
		AllowOther:  false,
		ExtraOpts:   []string{"suid", "allow_other", "rw", "dev"},
	}

	opts := m.generateMountOpts()
	offendingOpts := lo.Filter(opts, func(s string, _ int) bool {
		splitted := strings.SplitN(s, "=", 2)
		switch splitted[0] {
		case "rw", "dev", "suid", "allow_other":
			return true
		default:
			return false
		}
	})
	if len(offendingOpts) > 0 {
		t.Errorf("Failed to properly filter mount options; opts: %#v (offending options: %#v)", opts, offendingOpts)
	}
}
