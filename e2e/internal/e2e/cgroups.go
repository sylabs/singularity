// Copyright (c) 2022 Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package e2e

import (
	"testing"

	"github.com/sylabs/singularity/v4/internal/pkg/test/tool/require"
)

// WithCgroupManagers is a wrapper to call test function f in both the systemd and
// cgroupfs cgroup manager configurations. It *must* be run noparallel, as the
// cgroup manager setting is set / read from global configuration.
func (env TestEnv) WithRootManagers(f func(t *testing.T)) func(t *testing.T) {
	return func(t *testing.T) {
		require.Cgroups(t)

		env.RunSingularity(
			t,
			WithProfile(RootProfile),
			WithCommand("config global"),
			WithArgs("--set", "systemd cgroups", "yes"),
			ExpectExit(0),
		)

		defer env.RunSingularity(
			t,
			WithProfile(RootProfile),
			WithCommand("config global"),
			WithArgs("--reset", "systemd cgroups"),
			ExpectExit(0),
		)

		t.Run("systemd", f)

		env.RunSingularity(
			t,
			WithProfile(RootProfile),
			WithCommand("config global"),
			WithArgs("--set", "systemd cgroups", "no"),
			ExpectExit(0),
		)

		t.Run("cgroupfs", f)
	}
}

// WithRootlessManagers is a wrapper to call test function f if we can satisfy the
// requirement of rootless cgroups (systemd and cgroupsv2)
func (env TestEnv) WithRootlessManagers(f func(t *testing.T)) func(t *testing.T) {
	return func(t *testing.T) {
		require.CgroupsV2Unified(t)

		env.RunSingularity(
			t,
			WithProfile(RootProfile),
			WithCommand("config global"),
			WithArgs("--set", "systemd cgroups", "yes"),
			ExpectExit(0),
		)

		defer env.RunSingularity(
			t,
			WithProfile(RootProfile),
			WithCommand("config global"),
			WithArgs("--reset", "systemd cgroups"),
			ExpectExit(0),
		)

		t.Run("rootless", f)
	}
}
