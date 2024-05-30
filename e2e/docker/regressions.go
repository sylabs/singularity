// Copyright (c) 2019-2022, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package docker

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sylabs/singularity/v4/e2e/internal/e2e"
	"github.com/sylabs/singularity/v4/internal/pkg/test/tool/require"
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
		e2e.WithArgs("--disable-cache", "--force", "/dev/null", image),
		e2e.ExpectExit(0),
	)
}

func (c ctx) issue5172(t *testing.T) {
	u := e2e.UserProfile.HostUser(t)

	// create $HOME/.config/containers/registries.conf
	regImage := c.env.TestRegistryImage
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
		e2e.WithArgs("--disable-cache", "--sandbox", imagePath, regImage),
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
		e2e.WithArgs("--disable-cache", imagePath, regImage),
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

	// Run build with cache disabled, so we can be a parallel test (we are slooow!)
	c.env.RunSingularity(
		t,
		e2e.AsSubtest("build"),
		e2e.WithProfile(e2e.RootProfile),
		e2e.WithCommand("build"),
		e2e.WithArgs("--disable-cache", imagePath, defFile),
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

// https://github.com/sylabs/singularity/issues/1704 Ensure that trailing "n"s
// aren't lopped off by the internal sandbox inspect call that is part of the
// SIF-building process.
func (c ctx) issue1704(t *testing.T) {
	tmpDir, cleanup := e2e.MakeTempDir(t, c.env.TestDir, "issue1704-", "")
	t.Cleanup(func() {
		if !t.Failed() {
			cleanup(t)
		}
	})

	defPath := filepath.Join("..", "test", "defs", "issue1704.def")
	sifPath := filepath.Join(tmpDir, "issue1704.sif")
	bytes, err := os.ReadFile(defPath)
	if err != nil {
		t.Fatalf("could not read contents of def file %q: %s", defPath, err)
	}
	defFileContents := string(bytes)

	c.env.RunSingularity(
		t,
		e2e.AsSubtest("Build"),
		e2e.WithProfile(e2e.RootProfile),
		e2e.WithCommand("build"),
		e2e.WithArgs(sifPath, defPath),
		e2e.ExpectExit(0),
	)

	if t.Failed() {
		return
	}

	c.env.RunSingularity(
		t,
		e2e.AsSubtest("Inspect"),
		e2e.WithProfile(e2e.UserProfile),
		e2e.WithCommand("inspect"),
		e2e.WithArgs("-d", sifPath),
		e2e.ExpectExit(0, e2e.ExpectOutput(e2e.ContainMatch, strings.TrimSpace(defFileContents))),
	)
}

// https://github.com/sylabs/singularity/issues/1286
// Ensure the bare docker://hello-world image runs in all modes
func (c ctx) issue1286(t *testing.T) {
	for _, profile := range e2e.AllProfiles() {
		c.env.RunSingularity(
			t,
			e2e.AsSubtest(profile.String()),
			e2e.WithProfile(profile),
			e2e.WithCommand("run"),
			e2e.WithArgs("docker://hello-world"),
			e2e.ExpectExit(0,
				e2e.ExpectOutput(e2e.ContainMatch, "Hello from Docker!"),
			),
		)
	}
}

// https://github.com/sylabs/singularity/issues/1528
// Check that host's TERM value gets passed to OCI container.
// This test uses fairly fine-grained env vars manipulation which, at the
// present, is beyond what an API like testing.T.Setenv() enables, and so
// the tenv linter is turned off here.
//
//nolint:tenv
func (c ctx) issue1528(t *testing.T) {
	e2e.EnsureOCISIF(t, c.env)

	imageRef := "oci-sif:" + c.env.OCISIFPath

	_, wasHostTermSet := os.LookupEnv("TERM")
	if !wasHostTermSet {
		if err := os.Setenv("TERM", "xterm"); err != nil {
			t.Errorf("could not set TERM environment variable on host")
		}
		defer os.Unsetenv("TERM")
	}

	singEnvTermPrevious, wasHostSingEnvTermSet := os.LookupEnv("SINGULARITYENV_TERM")
	if wasHostSingEnvTermSet {
		if err := os.Unsetenv("SINGULARITYENV_TERM"); err != nil {
			t.Errorf("could not unset SINGULARITYENV_TERM environment variable on host")
		}
		defer os.Setenv("SINGULARITYENV_TERM", singEnvTermPrevious)
	} else {
		defer os.Unsetenv("SINGULARITYENV_TERM")
	}

	envTerm := os.Getenv("TERM")
	wantTermString := fmt.Sprintf("TERM=%s\n", envTerm)
	for _, profile := range e2e.OCIProfiles {
		t.Run(profile.String(), func(t *testing.T) {
			c.env.RunSingularity(
				t,
				e2e.AsSubtest("issue1528"),
				e2e.WithProfile(profile),
				e2e.WithCommand("exec"),
				e2e.WithArgs(imageRef, "env"),
				e2e.ExpectExit(0, e2e.ExpectOutput(e2e.ContainMatch, wantTermString)),
			)
		})
	}

	singEnvTerm := envTerm + "testsuffix"
	if err := os.Setenv("SINGULARITYENV_TERM", singEnvTerm); err != nil {
		t.Errorf("could not set SINGULARITYENV_TERM environment variable on host")
	}
	wantTermString = fmt.Sprintf("TERM=%s\n", singEnvTerm)
	for _, profile := range e2e.OCIProfiles {
		t.Run(profile.String(), func(t *testing.T) {
			c.env.RunSingularity(
				t,
				e2e.AsSubtest("issue1528override"),
				e2e.WithProfile(profile),
				e2e.WithCommand("exec"),
				e2e.WithArgs(imageRef, "env"),
				e2e.ExpectExit(0, e2e.ExpectOutput(e2e.ContainMatch, wantTermString)),
			)
		})
	}
}

// https://github.com/sylabs/singularity/issues/1586
// In OCI mode, ensure that nothing is left in TMPDIR from a docker:// image with restrictive file permissions.
func (c ctx) issue1586(t *testing.T) {
	tmpDir, cleanup := e2e.MakeTempDir(t, c.env.TestDir, "issue1586-", "")
	t.Cleanup(func() {
		if !t.Failed() {
			cleanup(t)
		}
	})

	c.env.RunSingularity(
		t,
		e2e.WithProfile(e2e.OCIUserProfile),
		e2e.WithCommand("exec"),
		e2e.WithArgs("docker://almalinux:9.1-minimal-20230407", "/bin/true"),
		e2e.WithEnv(append(os.Environ(), "TMPDIR="+tmpDir)),
		e2e.ExpectExit(0,
			e2e.ExpectError(e2e.UnwantedContainMatch, "permission denied"),
		),
	)

	d, err := os.Open(tmpDir)
	if err != nil {
		t.Errorf("Couldn't open TMPDIR %s: %v", tmpDir, err)
	}
	defer d.Close()
	if _, err = d.Readdir(1); err != io.EOF {
		t.Errorf("TMPDIR is not empty after singularity exited")
	}
}

// https://github.com/sylabs/singularity/issues/1670
// Check that runc/crun can add directories the rootfs before entering the
// container, by running a container based on busybox that lacks, e.g., /proc
func (c ctx) issue1670(t *testing.T) {
	for _, profile := range e2e.OCIProfiles {
		tmpDir, cleanup := e2e.MakeTempDir(t, c.env.TestDir, fmt.Sprintf("issue1670-%s-", profile.String()), "")
		t.Cleanup(func() {
			if !t.Failed() {
				cleanup(t)
			}
		})

		c.env.RunSingularity(
			t,
			e2e.AsSubtest(profile.String()),
			e2e.WithProfile(profile),
			e2e.WithCommand("exec"),
			e2e.WithArgs("--overlay", fmt.Sprintf("%s:ro", tmpDir), "docker://busybox", "echo", "hi"),
			e2e.ExpectExit(0),
		)
	}
}
