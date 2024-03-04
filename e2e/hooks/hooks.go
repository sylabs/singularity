// Copyright (c) 2024, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package hooks

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sylabs/singularity/v4/e2e/internal/e2e"
	"github.com/sylabs/singularity/v4/e2e/internal/testhelper"
	"github.com/sylabs/singularity/v4/internal/pkg/buildcfg"
	"github.com/sylabs/singularity/v4/internal/pkg/util/fs"
)

type ctx struct {
	env e2e.TestEnv
}

// testHooksUnpriv tests native engine unprivileged hooks
func (c ctx) testPoststart(t *testing.T) {
	e2e.EnsureImage(t, c.env)

	tests := []struct {
		name       string
		hookFiles  []string
		hookUID    int
		hookGID    int
		profile    e2e.Profile
		expectOps  []e2e.SingularityCmdResultOp
		expectExit int
	}{
		// Hook config is invalid
		{
			name:       "invalid",
			hookFiles:  []string{"hooks/testdata/nopath.json"},
			hookUID:    0,
			hookGID:    0,
			profile:    e2e.UserProfile,
			expectExit: 255,
		},
		// Valid hook owned by root, run with setuid user profile
		{
			name:      "unprivilegedSetuid",
			hookFiles: []string{"hooks/testdata/unprivileged.json"},
			hookUID:   0,
			hookGID:   0,
			expectOps: []e2e.SingularityCmdResultOp{
				e2e.ExpectErrorf(e2e.ContainMatch, "uid=%d", e2e.UserProfile.HostUser(t).UID),
			},
			profile:    e2e.UserProfile,
			expectExit: 0,
		},
		// Valid hook owned by root, run with user namespace profile
		{
			name:      "unprivilegedUserNamespace",
			hookFiles: []string{"hooks/testdata/unprivileged.json"},
			hookUID:   int(e2e.UserProfile.HostUser(t).UID),
			hookGID:   int(e2e.UserProfile.HostUser(t).GID),
			expectOps: []e2e.SingularityCmdResultOp{
				e2e.ExpectErrorf(e2e.ContainMatch, "uid=%d", e2e.UserProfile.HostUser(t).UID),
			},
			profile:    e2e.UserNamespaceProfile,
			expectExit: 0,
		},
		// Valid hook not owned by root, can't be run with setuid user profile
		{
			name:       "userOwnerSetuid",
			hookFiles:  []string{"hooks/testdata/unprivileged.json"},
			hookUID:    int(e2e.UserProfile.HostUser(t).UID),
			hookGID:    int(e2e.UserProfile.HostUser(t).GID),
			profile:    e2e.UserProfile,
			expectExit: 255,
		},
		// Valid hook not owned by root, can be run with user namespace profile
		{
			name:      "userOwnerUserNamespace",
			hookFiles: []string{"hooks/testdata/unprivileged.json"},
			hookUID:   int(e2e.UserProfile.HostUser(t).UID),
			hookGID:   int(e2e.UserProfile.HostUser(t).GID),
			expectOps: []e2e.SingularityCmdResultOp{
				e2e.ExpectErrorf(e2e.ContainMatch, "uid=%d", e2e.UserProfile.HostUser(t).UID),
			},
			profile:    e2e.UserNamespaceProfile,
			expectExit: 0,
		},
		// Unprivileged environment
		{
			name:      "unprivilegedEnv",
			hookFiles: []string{"hooks/testdata/unprivilegedEnv.json"},
			hookUID:   0,
			hookGID:   0,
			expectOps: []e2e.SingularityCmdResultOp{
				e2e.ExpectErrorf(e2e.ContainMatch, "SINGULARITY_CONTAINER=%s", c.env.ImagePath),
				e2e.ExpectError(e2e.ContainMatch, "SINGULARITY_CONTAINER_PID="),
				e2e.ExpectError(e2e.UnwantedContainMatch, "SINGULARITY_HOOK_PRIVILEGED=1"),
				e2e.ExpectError(e2e.ContainMatch, "I_AM_A_HOOK=1"),
			},
			profile:    e2e.UserProfile,
			expectExit: 0,
		},
		// Privileged hook run with setuid user profile
		{
			name:      "privileged",
			hookFiles: []string{"hooks/testdata/privileged.json"},
			hookUID:   0,
			hookGID:   0,
			expectOps: []e2e.SingularityCmdResultOp{
				e2e.ExpectError(e2e.ContainMatch, "uid=0"),
			},
			profile:    e2e.UserProfile,
			expectExit: 0,
		},
		// Privileged environment
		{
			name:      "privilegedEnv",
			hookFiles: []string{"hooks/testdata/privilegedEnv.json"},
			hookUID:   0,
			hookGID:   0,
			expectOps: []e2e.SingularityCmdResultOp{
				e2e.ExpectErrorf(e2e.ContainMatch, "SINGULARITY_CONTAINER=%s", c.env.ImagePath),
				e2e.ExpectError(e2e.ContainMatch, "SINGULARITY_CONTAINER_PID="),
				e2e.ExpectError(e2e.ContainMatch, "SINGULARITY_HOOK_PRIVILEGED=1"),
				e2e.ExpectError(e2e.ContainMatch, "I_AM_A_HOOK=1"),
			},
			profile:    e2e.UserProfile,
			expectExit: 0,
		},
		// Privileged & unprivileged hook / setuid user profile - both should run.
		{
			name:      "privilegedAndUnprivileged",
			hookFiles: []string{"hooks/testdata/privileged.json", "hooks/testdata/unprivileged.json"},
			hookUID:   0,
			hookGID:   0,
			expectOps: []e2e.SingularityCmdResultOp{
				e2e.ExpectError(e2e.ContainMatch, "uid=0"),
				e2e.ExpectErrorf(e2e.ContainMatch, "uid=%d", e2e.UserProfile.HostUser(t).UID),
			},
			profile:    e2e.UserProfile,
			expectExit: 0,
		},
		// Privileged & unprivileged hook / user namespace profile - only unpriv hook should run.
		{
			name:      "unprivilegedOnly",
			hookFiles: []string{"hooks/testdata/privileged.json", "hooks/testdata/unprivileged.json"},
			hookUID:   0,
			hookGID:   0,
			expectOps: []e2e.SingularityCmdResultOp{
				e2e.ExpectError(e2e.UnwantedContainMatch, "uid=0"),
				e2e.ExpectErrorf(e2e.ContainMatch, "uid=%d", e2e.UserProfile.HostUser(t).UID),
			},
			profile:    e2e.UserNamespaceProfile,
			expectExit: 0,
		},
	}

	for _, tt := range tests {
		hooksDir := filepath.Join(buildcfg.SINGULARITY_CONFDIR, "native-hooks.d")

		// We will be modifying the global config dir. We *must* define
		// postFn first, so we can call it to clean up if PreFn fails.
		postFn := e2e.Privileged(func(t *testing.T) {
			for _, h := range tt.hookFiles {
				installedHook := filepath.Join(hooksDir, filepath.Base(h))
				if err := os.Remove(installedHook); err != nil {
					t.Errorf("Couldn't remove %s: %v", installedHook, err)
				}
			}
		})

		preFn := e2e.Privileged(func(t *testing.T) {
			for _, h := range tt.hookFiles {
				installedHook := filepath.Join(hooksDir, filepath.Base(h))
				if err := fs.CopyFile(h, installedHook, 0o644); err != nil {
					t.Errorf("Couldn't copy %s to %s: %v", h, installedHook, err)
					postFn(t)
					t.FailNow()
				}
				if err := os.Chown(installedHook, tt.hookUID, tt.hookGID); err != nil {
					t.Errorf("Couldn't chown %s: %v", installedHook, err)
					postFn(t)
					t.FailNow()
				}
			}
		})

		c.env.RunSingularity(
			t,
			e2e.AsSubtest(tt.name),
			e2e.WithProfile(tt.profile),
			e2e.WithCommand("exec"),
			e2e.WithGlobalOptions("--debug"),
			e2e.WithArgs(c.env.ImagePath, "/bin/sleep", "1"),
			e2e.PreRun(preFn),
			e2e.ExpectExit(tt.expectExit, tt.expectOps...),
			e2e.PostRun(postFn),
		)
	}
}

// E2ETests is the main func to trigger the test suite
func E2ETests(env e2e.TestEnv) testhelper.Tests {
	c := ctx{
		env: env,
	}

	np := testhelper.NoParallel

	return testhelper.Tests{
		"poststart": np(c.testPoststart),
	}
}
