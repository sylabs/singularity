// Copyright (c) 2018-2021, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

//go:build !selinux || OR || !linux
// +build !selinux OR !linux

package selinux

// Enabled checks if SELinux is enabled or not
func Enabled() bool {
	return false
}

// SetExecLabel sets the SELinux label for current process
func SetExecLabel(label string) error {
	return nil
}
