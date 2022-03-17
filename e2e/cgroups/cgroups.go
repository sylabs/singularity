// Copyright (c) 2022, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package cgroups

import (
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/sylabs/singularity/e2e/internal/e2e"
	"github.com/sylabs/singularity/e2e/internal/testhelper"
	"github.com/sylabs/singularity/internal/pkg/test/tool/require"
)

// randomName generates a random name instance or OCI container name based on a UUID.
func randomName(t *testing.T) string {
	t.Helper()

	id, err := uuid.NewRandom()
	if err != nil {
		t.Fatal(err)
	}
	return id.String()
}

type ctx struct {
	env e2e.TestEnv
}

// moved from INSTANCE suite, as testing with systemd cgroup manager requires
// e2e to be run without PID namespace
func (c *ctx) instanceApply(t *testing.T, profile e2e.Profile) {
	e2e.EnsureImage(t, c.env)

	tests := []struct {
		name           string
		createArgs     []string
		execArgs       []string
		startErrorCode int
		startErrorOut  string
		execErrorCode  int
		execErrorOut   string
	}{
		{
			name:           "nonexistent toml",
			createArgs:     []string{"--apply-cgroups", "testdata/cgroups/doesnotexist.toml", c.env.ImagePath},
			startErrorCode: 255,
			// e2e test currently only captures the error from the CLI process, not the error displayed by the
			// starter process, so we check for the generic CLI error.
			startErrorOut: "failed to start instance",
		},
		{
			name:           "invalid toml",
			createArgs:     []string{"--apply-cgroups", "testdata/cgroups/invalid.toml", c.env.ImagePath},
			startErrorCode: 255,
			// e2e test currently only captures the error from the CLI process, not the error displayed by the
			// starter process, so we check for the generic CLI error.
			startErrorOut: "failed to start instance",
		},
		{
			name:       "memory limit",
			createArgs: []string{"--apply-cgroups", "testdata/cgroups/memory_limit.toml", c.env.ImagePath},
			// We get a CLI 255 error code, not the 137 that the starter receives for an OOM kill
			startErrorCode: 255,
		},
		{
			name:           "cpu success",
			createArgs:     []string{"--apply-cgroups", "testdata/cgroups/cpu_success.toml", c.env.ImagePath},
			startErrorCode: 0,
			execArgs:       []string{"/bin/true"},
			execErrorCode:  0,
		},
		{
			name:           "device deny",
			createArgs:     []string{"--apply-cgroups", "testdata/cgroups/deny_device.toml", c.env.ImagePath},
			startErrorCode: 0,
			execArgs:       []string{"cat", "/dev/null"},
			execErrorCode:  1,
			execErrorOut:   "Operation not permitted",
		},
	}

	for _, tt := range tests {
		createExitFunc := []e2e.SingularityCmdResultOp{}
		if tt.startErrorOut != "" {
			createExitFunc = []e2e.SingularityCmdResultOp{e2e.ExpectError(e2e.ContainMatch, tt.startErrorOut)}
		}
		execExitFunc := []e2e.SingularityCmdResultOp{}
		if tt.execErrorOut != "" {
			execExitFunc = []e2e.SingularityCmdResultOp{e2e.ExpectError(e2e.ContainMatch, tt.execErrorOut)}
		}
		// pick up a random name
		instanceName := randomName(t)
		joinName := fmt.Sprintf("instance://%s", instanceName)

		createArgs := append(tt.createArgs, instanceName)
		c.env.RunSingularity(
			t,
			e2e.AsSubtest(tt.name+"/start"),
			e2e.WithProfile(profile),
			e2e.WithCommand("instance start"),
			e2e.WithArgs(createArgs...),
			e2e.ExpectExit(tt.startErrorCode, createExitFunc...),
		)
		if tt.startErrorCode != 0 {
			continue
		}

		execArgs := append([]string{joinName}, tt.execArgs...)
		c.env.RunSingularity(
			t,
			e2e.AsSubtest(tt.name+"/exec"),
			e2e.WithProfile(profile),
			e2e.WithCommand("exec"),
			e2e.WithArgs(execArgs...),
			e2e.ExpectExit(tt.execErrorCode, execExitFunc...),
		)

		c.env.RunSingularity(
			t,
			e2e.AsSubtest(tt.name+"/stop"),
			e2e.WithProfile(profile),
			e2e.WithCommand("instance stop"),
			e2e.WithArgs(instanceName),
			e2e.ExpectExit(0),
		)
	}
}

func (c *ctx) instanceApplyRoot(t *testing.T) {
	require.Cgroups(t)
	c.instanceApply(t, e2e.RootProfile)
}

func (c *ctx) actionApply(t *testing.T, profile e2e.Profile) {
	e2e.EnsureImage(t, c.env)

	tests := []struct {
		name            string
		args            []string
		expectErrorCode int
		expectErrorOut  string
	}{
		{
			name:            "nonexistent toml",
			args:            []string{"--apply-cgroups", "testdata/cgroups/doesnotexist.toml", c.env.ImagePath, "/bin/sleep", "5"},
			expectErrorCode: 255,
			expectErrorOut:  "no such file or directory",
		},
		{
			name:            "invalid toml",
			args:            []string{"--apply-cgroups", "testdata/cgroups/invalid.toml", c.env.ImagePath, "/bin/sleep", "5"},
			expectErrorCode: 255,
			expectErrorOut:  "parsing error",
		},
		{
			name:            "memory limit",
			args:            []string{"--apply-cgroups", "testdata/cgroups/memory_limit.toml", c.env.ImagePath, "/bin/sleep", "5"},
			expectErrorCode: 137,
		},
		{
			name:            "cpu success",
			args:            []string{"--apply-cgroups", "testdata/cgroups/cpu_success.toml", c.env.ImagePath, "/bin/true"},
			expectErrorCode: 0,
		},
		// Device limits are properly applied only in rootful mode. Rootless will ignore them with a warning.
		{
			name:            "device deny",
			args:            []string{"--apply-cgroups", "testdata/cgroups/deny_device.toml", c.env.ImagePath, "cat", "/dev/null"},
			expectErrorCode: 1,
			expectErrorOut:  "Operation not permitted",
		},
	}

	for _, tt := range tests {
		exitFunc := []e2e.SingularityCmdResultOp{}
		if tt.expectErrorOut != "" {
			exitFunc = []e2e.SingularityCmdResultOp{e2e.ExpectError(e2e.ContainMatch, tt.expectErrorOut)}
		}
		c.env.RunSingularity(
			t,
			e2e.AsSubtest(tt.name),
			e2e.WithProfile(profile),
			e2e.WithCommand("exec"),
			e2e.WithArgs(tt.args...),
			e2e.ExpectExit(tt.expectErrorCode, exitFunc...),
		)
	}
}

func (c *ctx) actionApplyRoot(t *testing.T) {
	require.Cgroups(t)
	c.actionApply(t, e2e.RootProfile)
}

// E2ETests is the main func to trigger the test suite
func E2ETests(env e2e.TestEnv) testhelper.Tests {
	c := &ctx{
		env: env,
	}

	np := testhelper.NoParallel

	return testhelper.Tests{
		"instance root cgroups": np(c.instanceApplyRoot),
		"action root cgroups":   np(c.actionApplyRoot),
	}
}
