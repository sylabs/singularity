// Copyright (c) 2022, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package actions

import (
	"os"
	"testing"

	"github.com/pkg/errors"
	"github.com/sylabs/singularity/e2e/internal/e2e"
	"github.com/sylabs/singularity/internal/pkg/test/tool/require"
)

func (c actionTests) ociBundle(t *testing.T) (string, func()) {
	require.Seccomp(t)
	require.Filesystem(t, "overlay")

	bundleDir, err := os.MkdirTemp(c.env.TestDir, "bundle-")
	if err != nil {
		err = errors.Wrapf(err, "creating temporary bundle directory at %q", c.env.TestDir)
		t.Fatalf("failed to create bundle directory: %+v", err)
	}
	c.env.RunSingularity(
		t,
		e2e.WithProfile(e2e.RootProfile),
		e2e.WithCommand("oci mount"),
		e2e.WithArgs(c.env.ImagePath, bundleDir),
		e2e.ExpectExit(0),
	)

	cleanup := func() {
		c.env.RunSingularity(
			t,
			e2e.WithProfile(e2e.RootProfile),
			e2e.WithCommand("oci umount"),
			e2e.WithArgs(bundleDir),
			e2e.ExpectExit(0),
		)
		os.RemoveAll(bundleDir)
	}

	return bundleDir, cleanup
}

func (c actionTests) actionOciRun(t *testing.T) {
	e2e.EnsureImage(t, c.env)

	bundle, cleanup := c.ociBundle(t)
	defer cleanup()

	tests := []struct {
		name string
		argv []string
		exit int
	}{
		{
			name: "NoCommand",
			argv: []string{bundle},
			exit: 0,
		},
	}

	for _, tt := range tests {
		c.env.RunSingularity(
			t,
			e2e.AsSubtest(tt.name),
			e2e.WithProfile(e2e.OCIRootProfile),
			e2e.WithCommand("run"),
			// While we don't support args we are entering a /bin/sh interactively, so we need to exit.
			e2e.ConsoleRun(e2e.ConsoleSendLine("exit")),
			e2e.WithArgs(tt.argv...),
			e2e.ExpectExit(tt.exit),
		)
	}
}
