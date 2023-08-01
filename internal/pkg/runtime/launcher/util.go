// Copyright (c) 2022, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package launcher

import (
	"fmt"
	"os"
	"strings"

	"github.com/sylabs/singularity/v4/pkg/sylog"
	"github.com/sylabs/singularity/v4/pkg/util/fs/proc"
)

// WithPrivilege calls fn if cond is satisfied, and we are uid 0.
func WithPrivilege(cond bool, desc string, fn func() error) error {
	if !cond {
		return nil
	}
	if os.Getuid() != 0 {
		return fmt.Errorf("%s requires root privileges", desc)
	}
	return fn()
}

// HidepidProc checks if hidepid is set on the /proc mount point.
//
// If this is set then an instance started in the with setuid workflow cannot be
// joined later or stopped correctly.
func HidepidProc() bool {
	entries, err := proc.GetMountInfoEntry("/proc/self/mountinfo")
	if err != nil {
		sylog.Warningf("while reading /proc/self/mountinfo: %s", err)
		return false
	}
	for _, e := range entries {
		if e.Point == "/proc" {
			for _, o := range e.SuperOptions {
				if strings.HasPrefix(o, "hidepid=") {
					return true
				}
			}
		}
	}
	return false
}
