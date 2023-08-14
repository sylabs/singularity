// Copyright (c) 2019-2023, Sylabs Inc. All rights reserved.
// Copyright (c) 2020, Control Command Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package keyserver

import (
	"fmt"
	"log"
	"os"
	"strings"
	"testing"

	"github.com/sylabs/singularity/v4/e2e/internal/e2e"
	"github.com/sylabs/singularity/v4/e2e/internal/testhelper"
)

type ctx struct {
	env e2e.TestEnv
}

func (c ctx) keyserver(t *testing.T) {
	config, err := os.CreateTemp(c.env.TestDir, "testConfig-")
	if err != nil {
		log.Fatal(err)
	}
	t.Cleanup(func() {
		if !t.Failed() {
			os.Remove(config.Name()) // clean up
		}
	})

	var (
		sylabsKeyserver = "https://keys.sylabs.io"
		testKeyserver   = "http://localhost:11371"
		addKeyserver    = fmt.Sprintf("keyserver --config %s add", config.Name())
		removeKeyserver = fmt.Sprintf("keyserver --config %s remove", config.Name())
	)

	argv := []string{"--config", config.Name(), "add", "--no-login", "--no-default", "OtherCloud", "cloud.sylabs.io"}
	c.env.RunSingularity(
		t,
		e2e.WithProfile(e2e.UserProfile),
		e2e.WithCommand("remote"),
		e2e.WithArgs(argv...),
		e2e.ExpectExit(0),
	)

	tests := []struct {
		name       string
		command    string
		args       []string
		listLines  []string
		expectExit int
		profile    e2e.Profile
	}{
		{
			name:       "add non privileged",
			command:    addKeyserver,
			args:       []string{testKeyserver},
			expectExit: 255,
			profile:    e2e.UserProfile,
		},
		{
			name:    "add without order",
			command: addKeyserver,
			args:    []string{"--insecure", testKeyserver},
			listLines: []string{
				"SylabsCloud *^",
				"   #1  https://keys.sylabs.io  TLS     ",
				"   #2  http://localhost:11371  no TLS  ",
			},
			expectExit: 0,
			profile:    e2e.RootProfile,
		},
		{
			name:       "remove previous",
			command:    removeKeyserver,
			args:       []string{testKeyserver},
			expectExit: 0,
			profile:    e2e.RootProfile,
		},
		{
			name:       "remove non-existent",
			command:    removeKeyserver,
			args:       []string{testKeyserver},
			expectExit: 255,
			profile:    e2e.RootProfile,
		},
		{
			name:       "add with order 0",
			command:    addKeyserver,
			args:       []string{"--order", "0", testKeyserver},
			expectExit: 255,
			profile:    e2e.RootProfile,
		},
		{
			name:    "add with order 1",
			command: addKeyserver,
			args:    []string{"--order", "1", testKeyserver},
			listLines: []string{
				"SylabsCloud *^",
				"   #1  http://localhost:11371  TLS  ",
				"   #2  https://keys.sylabs.io  TLS  ",
			},
			expectExit: 0,
			profile:    e2e.RootProfile,
		},
		{
			name:       "add duplicate",
			command:    addKeyserver,
			args:       []string{testKeyserver},
			expectExit: 255,
			profile:    e2e.RootProfile,
		},
		{
			name:    "remove sylabs",
			command: removeKeyserver,
			args:    []string{sylabsKeyserver},
			listLines: []string{
				"SylabsCloud *^",
				"   #1  http://localhost:11371  TLS  ",
			},
			expectExit: 0,
			profile:    e2e.RootProfile,
		},
		{
			name:       "remove primary KO",
			command:    removeKeyserver,
			args:       []string{testKeyserver},
			expectExit: 255,
			profile:    e2e.RootProfile,
		},
		{
			name:    "add restore sylabs",
			command: addKeyserver,
			args:    []string{sylabsKeyserver},
			listLines: []string{
				"SylabsCloud *^",
				"   #1  http://localhost:11371  TLS  ",
				"   #2  https://keys.sylabs.io  TLS  ",
			},
			expectExit: 0,
			profile:    e2e.RootProfile,
		},
		{
			name:    "remove primary OK",
			command: removeKeyserver,
			args:    []string{testKeyserver},
			listLines: []string{
				"SylabsCloud *^",
				"   #1  https://keys.sylabs.io  TLS  ",
			},
			expectExit: 0,
			profile:    e2e.RootProfile,
		},
		{
			name:       "add out of order",
			command:    addKeyserver,
			args:       []string{"--order", "100", testKeyserver},
			expectExit: 255,
			profile:    e2e.RootProfile,
		},
	}

	for _, tt := range tests {
		c.env.RunSingularity(
			t,
			e2e.AsSubtest(tt.name),
			e2e.WithProfile(tt.profile),
			e2e.WithCommand(tt.command),
			e2e.WithArgs(tt.args...),
			e2e.PostRun(func(t *testing.T) {
				if t.Failed() || len(tt.listLines) == 0 {
					return
				}
				c.env.RunSingularity(
					t,
					e2e.AsSubtest("VerifyList"),
					e2e.WithProfile(e2e.UserProfile),
					e2e.WithArgs("--config", config.Name(), "list"),
					e2e.WithCommand("keyserver"),
					e2e.ExpectExit(
						0,
						e2e.ExpectOutput(
							e2e.ContainMatch,
							strings.Join(tt.listLines, "\n"),
						),
					),
				)
			}),
			e2e.ExpectExit(tt.expectExit),
		)
	}

	c.env.RunSingularity(
		t,
		e2e.AsSubtest("RemoteNameInvalid"),
		e2e.WithProfile(e2e.UserProfile),
		e2e.WithArgs("--config", config.Name(), "list", "InvalidCloud"),
		e2e.WithCommand("keyserver"),
		e2e.ExpectExit(255),
	)

	c.env.RunSingularity(
		t,
		e2e.AsSubtest("RemoteNameValid"),
		e2e.WithProfile(e2e.UserProfile),
		e2e.WithArgs("--config", config.Name(), "list", "OtherCloud"),
		e2e.WithCommand("keyserver"),
		e2e.ExpectExit(0, e2e.ExpectOutput(e2e.UnwantedContainMatch, testKeyserver)),
	)
}

// E2ETests is the main func to trigger the test suite
func E2ETests(env e2e.TestEnv) testhelper.Tests {
	c := ctx{
		env: env,
	}

	np := testhelper.NoParallel

	return testhelper.Tests{
		"keyserver": np(c.keyserver),
	}
}
