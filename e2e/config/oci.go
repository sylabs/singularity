// Copyright (c) 2019-2023, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package config

import (
	"fmt"
	"testing"

	"github.com/sylabs/singularity/v4/e2e/internal/e2e"
)

//nolint:maintidx
func (c configTests) ociConfigGlobal(t *testing.T) {
	e2e.EnsureOCISIF(t, c.env)
	archiveRef := "oci-sif:" + c.env.OCISIFPath

	setDirective := func(t *testing.T, directive, value string) {
		c.env.RunSingularity(
			t,
			e2e.WithProfile(e2e.RootProfile),
			e2e.WithCommand("config global"),
			e2e.WithArgs("--set", directive, value),
			e2e.ExpectExit(0),
		)
	}
	resetDirective := func(t *testing.T, directive string) {
		c.env.RunSingularity(
			t,
			e2e.WithProfile(e2e.RootProfile),
			e2e.WithCommand("config global"),
			e2e.WithArgs("--reset", directive),
			e2e.ExpectExit(0),
		)
	}

	tests := []struct {
		name              string
		argv              []string
		profile           e2e.Profile
		addRequirementsFn func(*testing.T)
		cwd               string
		directive         string
		directiveValue    string
		exit              int
		resultOp          e2e.SingularityCmdResultOp
	}{
		// {
		// 	name:           "AllowPidNsNo",
		// 	argv:           []string{"--pid", "--no-init", archiveRef, "/bin/sh", "-c", "echo $$"},
		// 	profile:        e2e.OCIUserProfile,
		// 	directive:      "allow pid ns",
		// 	directiveValue: "no",
		// 	exit:           0,
		// 	resultOp:       e2e.ExpectOutput(e2e.UnwantedExactMatch, "1"),
		// },
		// {
		// 	name:           "AllowPidNsYes",
		// 	argv:           []string{"--pid", "--no-init", archiveRef, "/bin/sh", "-c", "echo $$"},
		// 	profile:        e2e.OCIUserProfile,
		// 	directive:      "allow pid ns",
		// 	directiveValue: "yes",
		// 	exit:           0,
		// 	resultOp:       e2e.ExpectOutput(e2e.ExactMatch, "1"),
		// },
		{
			name: "ConfigPasswdNo",
			argv: []string{
				archiveRef, "grep",
				fmt.Sprintf("%s:x:%d", e2e.OCIUserProfile.ContainerUser(t).Name, e2e.OCIUserProfile.ContainerUser(t).UID),
				"/etc/passwd",
			},
			profile:        e2e.OCIUserProfile,
			directive:      "config passwd",
			directiveValue: "no",
			exit:           1,
		},
		{
			name: "ConfigPasswdYes",
			argv: []string{
				archiveRef, "grep",
				fmt.Sprintf("%s:x:%d", e2e.OCIUserProfile.ContainerUser(t).Name, e2e.OCIUserProfile.ContainerUser(t).UID),
				"/etc/passwd",
			},
			profile:        e2e.OCIUserProfile,
			directive:      "config passwd",
			directiveValue: "yes",
			exit:           0,
		},
		{
			name: "ConfigGroupNo",
			argv: []string{
				archiveRef, "grep",
				fmt.Sprintf("x:%d:%s", e2e.OCIUserProfile.ContainerUser(t).GID, e2e.OCIUserProfile.ContainerUser(t).Name),
				"/etc/group",
			},
			profile:        e2e.OCIUserProfile,
			directive:      "config group",
			directiveValue: "no",
			exit:           1,
		},
		{
			name: "ConfigGroupYes",
			argv: []string{
				archiveRef, "grep",
				fmt.Sprintf("x:%d:%s", e2e.OCIUserProfile.ContainerUser(t).GID, e2e.OCIUserProfile.ContainerUser(t).Name),
				"/etc/group",
			},
			profile:        e2e.OCIUserProfile,
			directive:      "config group",
			directiveValue: "yes",
			exit:           0,
		},
		// Test container doesn't have an /etc/resolv.conf, so presence check is okay here.
		{
			name:           "ConfigResolvConfNo",
			argv:           []string{archiveRef, "test", "-f", "/etc/resolv.conf"},
			profile:        e2e.OCIUserProfile,
			directive:      "config resolv_conf",
			directiveValue: "no",
			exit:           1,
		},
		{
			name:           "ConfigResolvConfYes",
			argv:           []string{archiveRef, "test", "-f", "/etc/resolv.conf"},
			profile:        e2e.OCIUserProfile,
			directive:      "config resolv_conf",
			directiveValue: "yes",
			exit:           0,
		},
		{
			name:           "MountProcNo",
			argv:           []string{archiveRef, "test", "-d", "/proc/self"},
			profile:        e2e.OCIUserProfile,
			directive:      "mount proc",
			directiveValue: "no",
			exit:           1,
		},
		{
			name:           "MountProcYes",
			argv:           []string{archiveRef, "test", "-d", "/proc/self"},
			profile:        e2e.OCIUserProfile,
			directive:      "mount proc",
			directiveValue: "yes",
			exit:           0,
		},
		{
			name:           "MountSysNo",
			argv:           []string{archiveRef, "test", "-d", "/sys/kernel"},
			profile:        e2e.OCIUserProfile,
			directive:      "mount sys",
			directiveValue: "no",
			exit:           1,
		},
		{
			name:           "MountSysYes",
			argv:           []string{archiveRef, "test", "-d", "/sys/kernel"},
			profile:        e2e.OCIUserProfile,
			directive:      "mount sys",
			directiveValue: "yes",
			exit:           0,
		},
		{
			name:           "MountDevNo",
			argv:           []string{archiveRef, "test", "-c", "/dev/zero"},
			profile:        e2e.OCIUserProfile,
			directive:      "mount dev",
			directiveValue: "no",
			exit:           255, // Not supported in OCI mode. Runtimes require /dev.
		},
		// Check that in `--no-compat`, `mount dev = minimal` forces a minimal dev.
		{
			name:           "MountDevMinimal",
			argv:           []string{"--no-compat", archiveRef, "test", "-b", "/dev/loop0"},
			profile:        e2e.OCIUserProfile,
			directive:      "mount dev",
			directiveValue: "minimal",
			exit:           1, // No loop device visible in minimal /dev
		},
		{
			name:           "MountDevYes",
			argv:           []string{archiveRef, "test", "-c", "/dev/zero"},
			profile:        e2e.OCIUserProfile,
			directive:      "mount dev",
			directiveValue: "yes",
			exit:           0,
		},
		{
			name:           "MountDevPtsNo",
			argv:           []string{archiveRef, "test", "-d", "/dev/pts"},
			profile:        e2e.OCIUserProfile,
			directive:      "mount devpts",
			directiveValue: "no",
			exit:           255, // Not supported in OCI mode. Runtimes require /dev/pts.
		},
		{
			name:           "MountDevPtsYes",
			argv:           []string{archiveRef, "test", "-d", "/dev/pts"},
			profile:        e2e.OCIUserProfile,
			directive:      "mount devpts",
			directiveValue: "yes",
			exit:           0,
		},
		// We have to check for a mount of $HOME, rather than presence of dir,
		// as runc/crun will create the dir in the container fs if it doesn't
		// exist.
		{
			name:           "MountHomeNo",
			argv:           []string{archiveRef, "grep", e2e.OCIUserProfile.ContainerUser(t).Dir, "/proc/self/mountinfo"},
			profile:        e2e.OCIUserProfile,
			cwd:            "/",
			directive:      "mount home",
			directiveValue: "no",
			exit:           1,
		},
		// Verify that though mount is skipped, $HOME is still set correctly
		// https://github.com/sylabs/singularity/issues/1783
		{
			name:           "MountHomeNoCorrectDir",
			argv:           []string{archiveRef, "sh", "-c", "test $HOME == " + e2e.OCIUserProfile.ContainerUser(t).Dir},
			profile:        e2e.OCIUserProfile,
			cwd:            "/",
			directive:      "mount home",
			directiveValue: "no",
			exit:           0,
		},
		{
			name:           "MountHomeYes",
			argv:           []string{archiveRef, "grep", e2e.OCIUserProfile.ContainerUser(t).Dir, "/proc/self/mountinfo"},
			profile:        e2e.OCIUserProfile,
			cwd:            "/",
			directive:      "mount home",
			directiveValue: "yes",
			exit:           0,
		},
		{
			name:           "MountTmpNo",
			argv:           []string{archiveRef, "grep", " /tmp ", "/proc/self/mountinfo"},
			profile:        e2e.OCIUserProfile,
			directive:      "mount tmp",
			directiveValue: "no",
			exit:           1,
		},
		{
			name:           "MountTmpYes",
			argv:           []string{archiveRef, "grep", " /tmp ", "/proc/self/mountinfo"},
			profile:        e2e.OCIUserProfile,
			directive:      "mount tmp",
			directiveValue: "yes",
			exit:           0,
		},
		//
		// bind path isn't supported at present because we are mimicking
		// --compat behavior in the native runtime. However, we should revisit
		// what makes most sense for users here before 4.0.
		//
		// {
		//  name:           "BindPathPasswd",
		//  argv:           []string{archiveRef, "test", "-f", "/passwd"},
		//  profile:        e2e.OCIUserProfile,
		//  directive:      "bind path",
		//  directiveValue: "/etc/passwd:/passwd",
		//  exit:           0,
		// },
		{
			name:           "UserBindControlNo",
			argv:           []string{"--bind", "/etc/passwd:/passwd", archiveRef, "test", "-f", "/passwd"},
			profile:        e2e.OCIUserProfile,
			directive:      "user bind control",
			directiveValue: "no",
			exit:           1,
		},
		{
			name:           "UserBindControlYes",
			argv:           []string{"--bind", "/etc/passwd:/passwd", archiveRef, "test", "-f", "/passwd"},
			profile:        e2e.OCIUserProfile,
			directive:      "user bind control",
			directiveValue: "yes",
			exit:           0,
		},
	}

	for _, tt := range tests {
		c.env.RunSingularity(
			t,
			e2e.AsSubtest(tt.name),
			e2e.WithProfile(tt.profile),
			e2e.WithDir(tt.cwd),
			e2e.PreRun(func(t *testing.T) {
				if tt.addRequirementsFn != nil {
					tt.addRequirementsFn(t)
				}
				setDirective(t, tt.directive, tt.directiveValue)
			}),
			e2e.PostRun(func(t *testing.T) {
				resetDirective(t, tt.directive)
			}),
			e2e.WithCommand("exec"),
			e2e.WithArgs(tt.argv...),
			e2e.ExpectExit(tt.exit, tt.resultOp),
		)
	}
}
