// Copyright (c) 2018-2024, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package priv

import (
	"runtime"

	"github.com/sylabs/singularity/v4/pkg/sylog"
	"golang.org/x/sys/unix"
)

type DropPrivsFunc func() error

// EscalateRealEffective locks the current goroutine to execute on the current
// OS thread, and then escalates the real and effective uid of the current OS
// thread to root (uid 0). The previous real uid is set as the saved
// set-user-ID. A dropPrivsFunc is returned, which must be called to drop
// privileges and unlock the goroutine at the earliest suitable point.
func EscalateRealEffective() (DropPrivsFunc, error) {
	runtime.LockOSThread()
	uid, _, _ := unix.Getresuid()

	dropPrivsFunc := func() error {
		defer runtime.UnlockOSThread()
		sylog.Debugf("Drop r/e/s: %d/%d/%d", uid, uid, 0)
		return unix.Setresuid(uid, uid, 0)
	}

	sylog.Debugf("Escalate r/e/s: %d/%d/%d", 0, 0, uid)
	// Note - unix.Setresuid makes a direct syscall which performs a single
	// thread escalation. Since Go 1.16, syscall.Setresuid is all-thread.
	return dropPrivsFunc, unix.Setresuid(0, 0, uid)
}
