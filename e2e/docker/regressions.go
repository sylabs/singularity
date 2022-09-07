// Copyright (c) 2019-2022, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package docker

import (
	"bytes"
	"os"
	"path"
	"path/filepath"
	"testing"

	"github.com/sylabs/singularity/e2e/internal/e2e"
	"github.com/sylabs/singularity/internal/pkg/test/tool/require"
)

// This test will build a sandbox, as a non-root user from a dockerhub image
// that contains a single folder and file with `000` permission.
// It will verify that with `--fix-perms` we force files to be accessible,
// moveable, removable by the user. We check for `700` and `400` permissions on
// the folder and file respectively.
func (c ctx) issue4524(t *testing.T) {
	sandbox := filepath.Join(c.env.TestDir, "issue_4524")

	c.env.RunSingularity(
		t,
		e2e.WithProfile(e2e.UserProfile),
		e2e.WithCommand("build"),
		e2e.WithArgs("--fix-perms", "--sandbox", sandbox, "docker://sylabsio/issue4524"),
		e2e.PostRun(func(t *testing.T) {
			// If we failed to build the sandbox completely, leave what we have for
			// investigation.
			if t.Failed() {
				t.Logf("Test %s failed, not removing directory %s", t.Name(), sandbox)
				return
			}

			if !e2e.PathPerms(t, path.Join(sandbox, "directory"), 0o700) {
				t.Error("Expected 0700 permissions on 000 test directory in rootless sandbox")
			}
			if !e2e.PathPerms(t, path.Join(sandbox, "file"), 0o600) {
				t.Error("Expected 0600 permissions on 000 test file in rootless sandbox")
			}

			// If the permissions aren't as we expect them to be, leave what we have for
			// investigation.
			if t.Failed() {
				t.Logf("Test %s failed, not removing directory %s", t.Name(), sandbox)
				return
			}

			err := os.RemoveAll(sandbox)
			if err != nil {
				t.Logf("Cannot remove sandbox directory: %#v", err)
			}
		}),
		e2e.ExpectExit(0),
	)
}

func (c ctx) issue4943(t *testing.T) {
	require.Arch(t, "amd64")

	const (
		image = "docker://gitlab-registry.cern.ch/linuxsupport/cc7-base:20191107"
	)

	c.env.RunSingularity(
		t,
		e2e.WithProfile(e2e.UserProfile),
		e2e.WithCommand("build"),
		e2e.WithArgs("--force", "/dev/null", image),
		e2e.ExpectExit(0),
	)
}

func (c ctx) issue5172(t *testing.T) {
	e2e.EnsureRegistry(t)

	u := e2e.UserProfile.HostUser(t)

	// create $HOME/.config/containers/registries.conf
	regImage := "docker://localhost:5000/my-busybox"
	regDir := filepath.Join(u.Dir, ".config", "containers")
	regFile := filepath.Join(regDir, "registries.conf")
	imagePath := filepath.Join(c.env.TestDir, "issue-5172")

	if err := os.MkdirAll(regDir, 0o755); err != nil {
		t.Fatalf("can't create directory %s: %s", regDir, err)
	}

	// add our test registry as insecure and test build/pull
	b := new(bytes.Buffer)
	b.WriteString("[registries.insecure]\nregistries = ['localhost']")
	if err := os.WriteFile(regFile, b.Bytes(), 0o644); err != nil {
		t.Fatalf("can't create %s: %s", regFile, err)
	}
	defer os.RemoveAll(regDir)

	c.env.RunSingularity(
		t,
		e2e.WithProfile(e2e.UserProfile),
		e2e.WithCommand("build"),
		e2e.WithArgs("--sandbox", imagePath, regImage),
		e2e.PostRun(func(t *testing.T) {
			if !t.Failed() {
				os.RemoveAll(imagePath)
			}
		}),
		e2e.ExpectExit(0),
	)

	c.env.RunSingularity(
		t,
		e2e.WithProfile(e2e.UserProfile),
		e2e.WithCommand("pull"),
		e2e.WithArgs(imagePath, regImage),
		e2e.PostRun(func(t *testing.T) {
			if !t.Failed() {
				os.RemoveAll(imagePath)
			}
		}),
		e2e.ExpectExit(0),
	)
}

// https://github.com/sylabs/singularity/issues/274
// The conda profile.d script must be able to be source'd from %environment.
// This has been broken by changes to mvdan.cc/sh interacting badly with our
// custom internalExecHandler.
// The test is quite heavyweight, but is warranted IMHO to ensure that conda
// environment activation works as expected, as this is a common use-case
// for SingularityCE.
func (c ctx) issue274(t *testing.T) {
	imageDir, cleanup := e2e.MakeTempDir(t, c.env.TestDir, "issue274-", "")
	defer cleanup(t)
	imagePath := filepath.Join(imageDir, "container")

	// Create a minimal conda environment on the current miniconda3 base.
	// Source the conda profile.d code and activate the env from `%environment`.
	def := `Bootstrap: docker
From: continuumio/miniconda3:latest

%post

	. /opt/conda/etc/profile.d/conda.sh
	conda create -n env

%environment

	source /opt/conda/etc/profile.d/conda.sh
	conda activate env
`
	defFile, err := e2e.WriteTempFile(imageDir, "deffile", def)
	if err != nil {
		t.Fatalf("Unable to create test definition file: %v", err)
	}

	c.env.RunSingularity(
		t,
		e2e.AsSubtest("build"),
		e2e.WithProfile(e2e.RootProfile),
		e2e.WithCommand("build"),
		e2e.WithArgs(imagePath, defFile),
		e2e.ExpectExit(0),
	)
	// An exec of `conda info` in the container should show environment active, no errors.
	// I.E. the `%environment` section should have worked.
	c.env.RunSingularity(
		t,
		e2e.AsSubtest("exec"),
		e2e.WithProfile(e2e.UserProfile),
		e2e.WithCommand("exec"),
		e2e.WithArgs(imagePath, "conda", "info"),
		e2e.ExpectExit(0,
			e2e.ExpectOutput(e2e.ContainMatch, "active environment : env"),
			e2e.ExpectError(e2e.ExactMatch, ""),
		),
	)
}
