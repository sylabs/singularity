// Copyright (c) 2024, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package priv

import (
	"testing"

	"github.com/sylabs/singularity/v4/internal/pkg/test"
	"golang.org/x/sys/unix"
)

func TestEscalateRealEffective(t *testing.T) {
	test.EnsurePrivilege(t)
	test.DropPrivilege(t)
	defer test.ResetPrivilege(t)

	r, e, s := unix.Getresuid()
	if r == 0 || e == 0 {
		t.Fatalf("real / effective ID must be non-zero before escalation. Got r/e/s %d/%d/%d", r, e, s)
	}
	unprivUID := r

	drop, err := EscalateRealEffective()
	if err != nil {
		t.Fatal(err)
	}

	r, e, s = unix.Getresuid()
	t.Logf("Escalated r/e/s: %d/%d/%d", r, e, s)
	if r != 0 || e != 0 || s != unprivUID {
		t.Fatalf("Expected escalated r/e/s %d/%d/%d, Got r/e/s %d/%d/%d", 0, 0, unprivUID, r, e, s)
	}

	if err := drop(); err != nil {
		t.Fatal(err)
	}

	r, e, s = unix.Getresuid()
	t.Logf("Dropped r/e/s: %d/%d/%d", r, e, s)
	if r != unprivUID || e != unprivUID || s != 0 {
		t.Fatalf("Expected dropped r/e/s %d/%d/%d, Got r/e/s %d/%d/%d", unprivUID, unprivUID, 0, r, e, s)
	}
}
