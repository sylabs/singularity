// Copyright (c) 2023, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package remote

import (
	"testing"

	"github.com/sylabs/singularity/e2e/internal/e2e"
	"github.com/sylabs/singularity/internal/pkg/remote"
)

func (c *ctx) issue1948(t *testing.T) {
	c.env.RunSingularity(
		t,
		e2e.AsSubtest("add global issue1948 remote (root)"),
		e2e.WithProfile(e2e.RootProfile),
		e2e.WithCommand("remote"),
		e2e.WithArgs("add", "--global", "issue1948", "https://issue1948.example.com"),
		e2e.ExpectExit(0),
	)

	c.env.RunSingularity(
		t,
		e2e.AsSubtest("use SylabsCloud remote (user)"),
		e2e.WithProfile(e2e.UserProfile),
		e2e.WithCommand("remote"),
		e2e.WithArgs("use", remote.DefaultRemoteName),
		e2e.ExpectExit(0),
	)

	c.env.RunSingularity(
		t,
		e2e.AsSubtest("list remotes (user)"),
		e2e.WithProfile(e2e.UserProfile),
		e2e.WithCommand("remote"),
		e2e.WithArgs("list"),
		e2e.ExpectExit(0),
	)

	c.env.RunSingularity(
		t,
		e2e.AsSubtest("remove global issue1948 remote (root)"),
		e2e.WithProfile(e2e.RootProfile),
		e2e.WithCommand("remote"),
		e2e.WithArgs("remove", "--global", "issue1948"),
		e2e.ExpectExit(0),
	)

	c.env.RunSingularity(
		t,
		e2e.AsSubtest("list remotes again (user)"),
		e2e.WithProfile(e2e.UserProfile),
		e2e.WithCommand("remote"),
		e2e.WithArgs("list"),
		e2e.ExpectExit(
			0,
			e2e.ExpectOutput(e2e.UnwantedContainMatch, "issue1948"),
		),
	)
}
