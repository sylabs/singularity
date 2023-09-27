// Copyright (c) 2019-2023, Sylabs Inc. All rights reserved.
// Copyright (c) 2020, Control Command Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package registry

import (
	"fmt"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/sylabs/singularity/v4/e2e/internal/e2e"
	"github.com/sylabs/singularity/v4/e2e/internal/testhelper"
)

type ctx struct {
	env e2e.TestEnv
}

// registryList tests the functionality of "singularity registry list" command
func (c ctx) registryList(t *testing.T) {
	registry := fmt.Sprintf("oras://%s", c.env.TestRegistry)

	c.env.RunSingularity(
		t,
		e2e.WithProfile(e2e.UserProfile),
		e2e.WithCommand("registry login"),
		e2e.WithArgs("-u", e2e.DefaultUsername, "-p", e2e.DefaultPassword, registry),
		e2e.ExpectExit(0),
	)

	c.env.RunSingularity(
		t,
		e2e.WithProfile(e2e.UserProfile),
		e2e.WithCommand("registry list"),
		e2e.ExpectExit(0,
			e2e.ExpectOutput(
				e2e.ContainMatch,
				strings.Join([]string{
					"URI                     SECURE?",
					registry + "  ✓",
				}, "\n"))),
	)

	c.env.RunSingularity(
		t,
		e2e.WithProfile(e2e.UserProfile),
		e2e.WithCommand("registry logout"),
		e2e.WithArgs(registry),
		e2e.ExpectExit(0),
	)
}

func (c ctx) registryTestHelp(t *testing.T) {
	tests := []struct {
		name           string
		cmdArgs        []string
		expectedOutput string
	}{
		{
			name:           "list help",
			cmdArgs:        []string{"list", "--help"},
			expectedOutput: "List all OCI credentials that are configured",
		},
		{
			name:           "login help",
			cmdArgs:        []string{"login", "--help"},
			expectedOutput: "Login to an OCI/Docker registry",
		},
		{
			name:           "logout help",
			cmdArgs:        []string{"logout", "--help"},
			expectedOutput: "Logout from an OCI/Docker registry",
		},
	}

	for _, tt := range tests {
		c.env.RunSingularity(
			t,
			e2e.AsSubtest(tt.name),
			e2e.WithProfile(e2e.UserProfile),
			e2e.WithCommand("registry"),
			e2e.WithArgs(tt.cmdArgs...),
			e2e.ExpectExit(
				0,
				e2e.ExpectOutput(e2e.RegexMatch, `^`+tt.expectedOutput),
			),
		)
	}
}

func (c ctx) registryLogin(t *testing.T) {
	var (
		registry    = fmt.Sprintf("oras://%s", c.env.TestRegistry)
		badRegistry = "oras://bad_registry:5000"
	)

	tests := []struct {
		name       string
		command    string
		args       []string
		stdin      io.Reader
		expectExit int
	}{
		{
			name:       "login username and empty password",
			command:    "registry login",
			args:       []string{"-u", e2e.DefaultUsername, "-p", "", registry},
			expectExit: 255,
		},
		{
			name:       "login empty username and empty password",
			command:    "registry login",
			args:       []string{"-p", "", registry},
			expectExit: 255,
		},
		{
			name:       "login empty username and password",
			command:    "registry login",
			args:       []string{"-p", "bad", registry},
			expectExit: 255,
		},
		{
			name:       "login without scheme KO",
			command:    "registry login",
			args:       []string{"-u", e2e.DefaultUsername, "-p", e2e.DefaultPassword, c.env.TestRegistry},
			expectExit: 255,
		},
		{
			name:       "login OK",
			command:    "registry login",
			args:       []string{"-u", e2e.DefaultUsername, "-p", e2e.DefaultPassword, registry},
			expectExit: 0,
		},
		{
			name:       "login password-stdin",
			command:    "registry login",
			args:       []string{"-u", e2e.DefaultUsername, "--password-stdin", registry},
			stdin:      strings.NewReader(e2e.DefaultPassword),
			expectExit: 0,
		},
		{
			name:       "logout KO",
			command:    "registry logout",
			args:       []string{badRegistry},
			expectExit: 255,
		},
		{
			name:       "logout OK",
			command:    "registry logout",
			args:       []string{registry},
			expectExit: 0,
		},
	}

	for _, tt := range tests {
		c.env.RunSingularity(
			t,
			e2e.AsSubtest(tt.name),
			e2e.WithProfile(e2e.UserProfile),
			e2e.WithStdin(tt.stdin),
			e2e.WithCommand(tt.command),
			e2e.WithArgs(tt.args...),
			e2e.ExpectExit(tt.expectExit),
		)
	}
}

func (c ctx) registryLoginPushPrivate(t *testing.T) {
	e2e.EnsureImage(t, c.env)

	var (
		registry = fmt.Sprintf("oras://%s", c.env.TestRegistry)
		repo     = fmt.Sprintf("oras://%s/private/e2e:1.0.0", c.env.TestRegistry)
	)

	tests := []struct {
		name       string
		command    string
		args       []string
		expectExit int
	}{
		{
			name:       "push before login",
			command:    "push",
			args:       []string{c.env.ImagePath, repo},
			expectExit: 255,
		},
		{
			name:       "login",
			command:    "registry login",
			args:       []string{"-u", e2e.DefaultUsername, "-p", e2e.DefaultPassword, registry},
			expectExit: 0,
		},
		{
			name:       "push after login",
			command:    "push",
			args:       []string{c.env.ImagePath, repo},
			expectExit: 0,
		},
		{
			name:       "logout",
			command:    "registry logout",
			args:       []string{registry},
			expectExit: 0,
		},
	}

	for _, tt := range tests {
		c.env.RunSingularity(
			t,
			e2e.AsSubtest(tt.name),
			e2e.WithProfile(e2e.UserProfile),
			e2e.WithCommand(tt.command),
			e2e.WithArgs(tt.args...),
			e2e.ExpectExit(tt.expectExit),
		)
	}
}

// Repeated logins with same URI should not create duplicate remote.yaml entries.
// If we login twice, and logout once we should not see the URI in list.
// See https://github.com/sylabs/singularity/issues/214
func (c ctx) registryLoginRepeated(t *testing.T) {
	e2e.EnsureImage(t, c.env)

	registry := fmt.Sprintf("oras://%s", c.env.TestRegistry)

	tests := []struct {
		name       string
		command    string
		args       []string
		expectExit int
		resultOp   e2e.SingularityCmdResultOp
	}{
		{
			name:       "FirstLogin",
			command:    "registry login",
			args:       []string{"-u", e2e.DefaultUsername, "-p", e2e.DefaultPassword, registry},
			expectExit: 0,
		},
		{
			name:       "SecondLogin",
			command:    "registry login",
			args:       []string{"-u", e2e.DefaultUsername, "-p", e2e.DefaultPassword, registry},
			expectExit: 0,
		},
		{
			name:       "logout",
			command:    "registry logout",
			args:       []string{registry},
			expectExit: 0,
		},
		{
			name:       "list",
			command:    "registry list",
			expectExit: 0,
			resultOp:   e2e.ExpectOutput(e2e.UnwantedContainMatch, registry),
		},
	}

	for _, tt := range tests {
		c.env.RunSingularity(
			t,
			e2e.AsSubtest(tt.name),
			e2e.WithProfile(e2e.UserProfile),
			e2e.WithCommand(tt.command),
			e2e.WithArgs(tt.args...),
			e2e.ExpectExit(tt.expectExit, tt.resultOp),
		)
	}
}

// Tests authentication with registries, incl.
// https://github.com/sylabs/singularity/issues/2226, among many other flows.
func (c ctx) registryAuth(t *testing.T) {
	t.Run("default", func(t *testing.T) {
		c.registryAuthTester(t, false)
	})
	t.Run("custom", func(t *testing.T) {
		c.registryAuthTester(t, true)
	})
}

func (c ctx) registryAuthTester(t *testing.T, withCustomAuthFile bool) {
	tmpdir, tmpdirCleanup := e2e.MakeTempDir(t, c.env.TestDir, "registry-auth", "")
	t.Cleanup(func() {
		if !t.Failed() {
			tmpdirCleanup(t)
		}
	})

	prevCwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("could not get current working directory: %s", err)
	}
	defer os.Chdir(prevCwd)
	if err = os.Chdir(tmpdir); err != nil {
		t.Fatalf("could not change cwd to %q: %s", tmpdir, err)
	}

	localAuthFileName := ""
	if withCustomAuthFile {
		localAuthFileName = "./my_local_authfile"
	}

	authFileArgs := []string{}
	if withCustomAuthFile {
		authFileArgs = []string{"--authfile", localAuthFileName}
	}

	c.env.RunSingularity(
		t,
		e2e.AsSubtest("docker pull before auth"),
		e2e.WithProfile(e2e.UserProfile),
		e2e.WithCommand("pull"),
		e2e.WithArgs(append(authFileArgs, "-F", "--disable-cache", "--no-https", c.env.TestRegistryPrivImage)...),
		e2e.ExpectExit(255),
	)

	t.Cleanup(func() {
		// No need to set up any cleanup when using the custom auth file,
		// because it's stored in a tmpdir for which we already set up cleanup.
		// And trying to do cleanup with a custom auth file if we're not already
		// logged in would cause a test fail.
		if !withCustomAuthFile {
			e2e.PrivateRepoLogout(t, c.env, e2e.UserProfile, localAuthFileName)
		}
	})

	e2e.PrivateRepoLogin(t, c.env, e2e.UserProfile, localAuthFileName)

	c.env.RunSingularity(
		t,
		e2e.AsSubtest("docker pull"),
		e2e.WithProfile(e2e.UserProfile),
		e2e.WithCommand("pull"),
		e2e.WithArgs(append(authFileArgs, "-F", "--disable-cache", "--no-https", c.env.TestRegistryPrivImage)...),
		e2e.ExpectExit(0),
	)

	e2e.PrivateRepoLogout(t, c.env, e2e.UserProfile, localAuthFileName)

	c.env.RunSingularity(
		t,
		e2e.AsSubtest("noauth docker pull"),
		e2e.WithProfile(e2e.UserProfile),
		e2e.WithCommand("pull"),
		e2e.WithArgs(append(authFileArgs, "-F", "--disable-cache", "--no-https", c.env.TestRegistryPrivImage)...),
		e2e.ExpectExit(255),
	)

	e2e.PrivateRepoLogin(t, c.env, e2e.UserProfile, localAuthFileName)

	c.env.RunSingularity(
		t,
		e2e.AsSubtest("docker pull ocisif"),
		e2e.WithProfile(e2e.UserProfile),
		e2e.WithCommand("pull"),
		e2e.WithArgs(append(authFileArgs, "-F", "--oci", "--disable-cache", "--no-https", c.env.TestRegistryPrivImage)...),
		e2e.ExpectExit(0),
	)

	e2e.PrivateRepoLogout(t, c.env, e2e.UserProfile, localAuthFileName)

	orasCustomPushTarget := fmt.Sprintf("oras://%s/authfile-pushtest-oras-alpine:latest", c.env.TestRegistryPrivPath)

	c.env.RunSingularity(
		t,
		e2e.AsSubtest("noauth oras push"),
		e2e.WithProfile(e2e.UserProfile),
		e2e.WithCommand("push"),
		e2e.WithArgs(append(authFileArgs, "my-alpine_latest.sif", orasCustomPushTarget)...),
		e2e.ExpectExit(255),
	)

	e2e.PrivateRepoLogin(t, c.env, e2e.UserProfile, localAuthFileName)

	c.env.RunSingularity(
		t,
		e2e.AsSubtest("oras push"),
		e2e.WithProfile(e2e.UserProfile),
		e2e.WithCommand("push"),
		e2e.WithArgs(append(authFileArgs, "my-alpine_latest.sif", orasCustomPushTarget)...),
		e2e.ExpectExit(0),
	)

	e2e.PrivateRepoLogout(t, c.env, e2e.UserProfile, localAuthFileName)

	dockerCustomPushTarget := fmt.Sprintf("docker://%s/authfile-pushtest-ocisif-alpine:latest", c.env.TestRegistryPrivPath)

	c.env.RunSingularity(
		t,
		e2e.AsSubtest("noauth docker push"),
		e2e.WithProfile(e2e.UserProfile),
		e2e.WithCommand("push"),
		e2e.WithArgs(append(authFileArgs, "my-alpine_latest.oci.sif", dockerCustomPushTarget)...),
		e2e.ExpectExit(255),
	)

	e2e.PrivateRepoLogin(t, c.env, e2e.UserProfile, localAuthFileName)

	c.env.RunSingularity(
		t,
		e2e.AsSubtest("docker push"),
		e2e.WithProfile(e2e.UserProfile),
		e2e.WithCommand("push"),
		e2e.WithArgs(append(authFileArgs, "my-alpine_latest.oci.sif", dockerCustomPushTarget)...),
		e2e.ExpectExit(0),
	)

	e2e.PrivateRepoLogout(t, c.env, e2e.UserProfile, localAuthFileName)

	c.env.RunSingularity(
		t,
		e2e.AsSubtest("noauth oras pull"),
		e2e.WithProfile(e2e.UserProfile),
		e2e.WithCommand("pull"),
		e2e.WithArgs(append(authFileArgs, "-F", "--disable-cache", "--no-https", orasCustomPushTarget)...),
		e2e.ExpectExit(255),
	)

	e2e.PrivateRepoLogin(t, c.env, e2e.UserProfile, localAuthFileName)

	c.env.RunSingularity(
		t,
		e2e.AsSubtest("oras pull"),
		e2e.WithProfile(e2e.UserProfile),
		e2e.WithCommand("pull"),
		e2e.WithArgs(append(authFileArgs, "-F", "--disable-cache", "--no-https", orasCustomPushTarget)...),
		e2e.ExpectExit(0),
	)
}

// E2ETests is the main func to trigger the test suite
func E2ETests(env e2e.TestEnv) testhelper.Tests {
	c := ctx{
		env: env,
	}

	np := testhelper.NoParallel

	return testhelper.Tests{
		"test registry help":          c.registryTestHelp,
		"registry login basic":        np(c.registryLogin),
		"registry login push private": np(c.registryLoginPushPrivate),
		"registry login repeated":     np(c.registryLoginRepeated),
		"registry list":               np(c.registryList),
		"registry auth":               np(c.registryAuth),
	}
}
