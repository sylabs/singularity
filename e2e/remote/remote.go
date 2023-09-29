// Copyright (c) 2019-2023, Sylabs Inc. All rights reserved.
// Copyright (c) 2020, Control Command Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package remote

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"testing"

	"github.com/sylabs/singularity/v4/e2e/internal/e2e"
	"github.com/sylabs/singularity/v4/e2e/internal/testhelper"
	"github.com/sylabs/singularity/v4/internal/pkg/util/fs"
)

type ctx struct {
	env e2e.TestEnv
}

// remoteAdd checks the functionality of "singularity remote add" command.
// It Verifies that adding valid endpoints results in success and invalid
// one's results in failure.
func (c ctx) remoteAdd(t *testing.T) {
	config, err := os.CreateTemp(c.env.TestDir, "testConfig-")
	if err != nil {
		log.Fatal(err)
	}
	t.Cleanup(func() {
		if !t.Failed() {
			os.Remove(config.Name()) // clean up
		}
	})

	testPass := []struct {
		name   string
		remote string
		uri    string
	}{
		{"AddCloud", "cloud", "cloud.sylabs.io"},
		{"AddOtherCloud", "other", "cloud.sylabs.io"},
	}

	for _, tt := range testPass {
		argv := []string{"--config", config.Name(), "add", "--no-login", tt.remote, tt.uri}
		c.env.RunSingularity(
			t,
			e2e.AsSubtest(tt.name),
			e2e.WithProfile(e2e.UserProfile),
			e2e.WithCommand("remote"),
			e2e.WithArgs(argv...),
			e2e.ExpectExit(0),
		)
	}

	testFail := []struct {
		name   string
		remote string
		uri    string
	}{
		{"AddExistingRemote", "cloud", "cloud.sylabs.io"},
		{"AddExistingRemoteInvalidURI", "other", "anythingcangohere"},
	}

	for _, tt := range testFail {
		argv := []string{"--config", config.Name(), "add", "--no-login", tt.remote, tt.uri}
		c.env.RunSingularity(
			t,
			e2e.AsSubtest(tt.name),
			e2e.WithProfile(e2e.UserProfile),
			e2e.WithCommand("remote"),
			e2e.WithArgs(argv...),
			e2e.ExpectExit(255),
		)
	}
}

// remoteDefaultOrNot checks that the `--no-default` flag, or its absence
// (meaning the newly-added remote should be made default), are respected when a
// new remote endpoint is added.
func (c ctx) remoteDefaultOrNot(t *testing.T) {
	config, err := os.CreateTemp(c.env.TestDir, "testConfig-")
	if err != nil {
		log.Fatal(err)
	}
	t.Cleanup(func() {
		if !t.Failed() {
			os.Remove(config.Name())
		}
	})

	tests := []struct {
		testName        string
		remoteName      string
		remoteURI       string
		addFlags        []string
		shouldBeDefault bool
	}{
		{
			testName:        "AddNoDefault",
			remoteName:      "SomeCloudND",
			remoteURI:       "somecloud.example.com",
			addFlags:        []string{"--no-default"},
			shouldBeDefault: false,
		},
		{
			testName:        "AddNoDefaultShort",
			remoteName:      "SomeCloudND2",
			remoteURI:       "somecloud2.example.com",
			addFlags:        []string{"-n"},
			shouldBeDefault: false,
		},
		{
			testName:        "AddDefault",
			remoteName:      "SomeCloudD",
			remoteURI:       "somecloud3.example.com",
			addFlags:        []string{},
			shouldBeDefault: true,
		},
	}

	for _, tt := range tests {
		args := append(append([]string{"--config", config.Name(), "add", "--no-login"}, tt.addFlags...), []string{tt.remoteName, tt.remoteURI}...)
		c.env.RunSingularity(
			t,
			e2e.AsSubtest(tt.testName),
			e2e.WithProfile(e2e.UserProfile),
			e2e.WithCommand("remote"),
			e2e.WithArgs(args...),
			e2e.ExpectExit(0),
		)

		args = []string{"--config", config.Name(), "list"}
		expectedDefaultIndicator := "✓"
		if !tt.shouldBeDefault {
			expectedDefaultIndicator = " "
		}

		// The latter portion of the following regular expression includes
		// enough spaces so that it guarantees a that the last %s, corresponding
		// to expectedDefaultIndicator, will fall right below the "DEFAULT?"
		// heading in the output table of `remote list`. The test that has
		// tt.shouldBeDefault set to true ensures that this is so, otherwise
		// that one will fail to match this regexp.
		expectedOutput := fmt.Sprintf("%s[ ]+%s[ ]+%s                              ✓", tt.remoteName, tt.remoteURI, expectedDefaultIndicator)
		c.env.RunSingularity(
			t,
			e2e.AsSubtest(tt.testName),
			e2e.WithProfile(e2e.UserProfile),
			e2e.WithCommand("remote"),
			e2e.WithArgs(args...),
			e2e.ExpectExit(
				0,
				e2e.ExpectOutput(
					e2e.RegexMatch,
					expectedOutput,
				),
			),
		)
	}
}

// remoteRemove tests the functionality of "singularity remote remove" command.
// 1. Adds remote endpoints
// 2. Deletes the already added entries
// 3. Verfies that removing an invalid entry results in a failure
func (c ctx) remoteRemove(t *testing.T) {
	config, err := os.CreateTemp(c.env.TestDir, "testConfig-")
	if err != nil {
		log.Fatal(err)
	}

	t.Cleanup(func() {
		if !t.Failed() {
			os.Remove(config.Name()) // clean up
		}
	})

	// Prep config by adding multiple remotes
	add := []struct {
		name   string
		remote string
		uri    string
	}{
		{"addCloud", "cloud", "cloud.sylabs.io"},
		{"addOther", "other", "cloud.sylabs.io"},
	}

	for _, tt := range add {
		argv := []string{"--config", config.Name(), "add", "--no-login", tt.remote, tt.uri}
		c.env.RunSingularity(
			t,
			e2e.AsSubtest(tt.name),
			e2e.WithProfile(e2e.UserProfile),
			e2e.WithCommand("remote"),
			e2e.WithArgs(argv...),
			e2e.ExpectExit(0),
		)
	}

	testPass := []struct {
		name   string
		remote string
	}{
		{"RemoveCloud", "cloud"},
		{"RemoveOther", "other"},
	}

	for _, tt := range testPass {
		argv := []string{"--config", config.Name(), "remove", tt.remote}
		c.env.RunSingularity(
			t,
			e2e.AsSubtest(tt.name),
			e2e.WithProfile(e2e.UserProfile),
			e2e.WithCommand("remote"),
			e2e.WithArgs(argv...),
			e2e.ExpectExit(0),
		)
	}

	testFail := []struct {
		name   string
		remote string
	}{
		{"RemoveNonExistingRemote", "cloud"},
	}

	for _, tt := range testFail {
		argv := []string{"--config", config.Name(), "remove", tt.remote}
		c.env.RunSingularity(
			t,
			e2e.AsSubtest(tt.name),
			e2e.WithProfile(e2e.UserProfile),
			e2e.WithCommand("remote"),
			e2e.WithArgs(argv...),
			e2e.ExpectExit(255),
		)
	}
}

// remoteUse tests the functionality of "singularity remote use" command.
// 1. Tries to use non-existing remote entry
// 2. Adds remote entries and tries to use those
func (c ctx) remoteUse(t *testing.T) {
	config, err := os.CreateTemp(c.env.TestDir, "testConfig-")
	if err != nil {
		log.Fatal(err)
	}

	t.Cleanup(func() {
		if !t.Failed() {
			os.Remove(config.Name()) // clean up
		}
	})

	testFail := []struct {
		name   string
		remote string
	}{
		{"UseNonExistingRemote", "cloud"},
	}

	for _, tt := range testFail {
		argv := []string{"--config", config.Name(), "use", tt.remote}
		c.env.RunSingularity(
			t,
			e2e.AsSubtest(tt.name),
			e2e.WithProfile(e2e.UserProfile),
			e2e.WithCommand("remote"),
			e2e.WithArgs(argv...),
			e2e.ExpectExit(255),
		)
	}

	// Prep config by adding multiple remotes
	add := []struct {
		name   string
		remote string
		uri    string
	}{
		{"addCloud", "cloud", "cloud.sylabs.io"},
		{"addOther", "other", "cloud.sylabs.io"},
	}

	for _, tt := range add {
		argv := []string{"--config", config.Name(), "add", "--no-login", tt.remote, tt.uri}
		c.env.RunSingularity(
			t,
			e2e.AsSubtest(tt.name),
			e2e.WithProfile(e2e.UserProfile),
			e2e.WithCommand("remote"),
			e2e.WithArgs(argv...),
			e2e.ExpectExit(0),
		)
	}

	testPass := []struct {
		name   string
		remote string
	}{
		{"UseFromNothingToRemote", "cloud"},
		{"UseFromRemoteToRemote", "other"},
	}

	for _, tt := range testPass {
		argv := []string{"--config", config.Name(), "use", tt.remote}
		c.env.RunSingularity(
			t,
			e2e.AsSubtest(tt.name),
			e2e.WithProfile(e2e.UserProfile),
			e2e.WithCommand("remote"),
			e2e.WithArgs(argv...),
			e2e.ExpectExit(0),
		)
	}
}

// remoteStatus tests the functionality of "singularity remote status" command.
// 1. Adds remote endpoints
// 2. Verifies that remote status command succeeds on existing endpoints
// 3. Verifies that remote status command fails on non-existing endpoints
func (c ctx) remoteStatus(t *testing.T) {
	config, err := os.CreateTemp(c.env.TestDir, "testConfig-")
	if err != nil {
		log.Fatal(err)
	}

	t.Cleanup(func() {
		if !t.Failed() {
			os.Remove(config.Name()) // clean up
		}
	})

	// Prep config by adding multiple remotes
	add := []struct {
		name   string
		remote string
		uri    string
	}{
		{"addCloud", "cloud", "cloud.sylabs.io"},
		{"addInvalidRemote", "invalid", "notarealendpoint"},
	}

	for _, tt := range add {
		argv := []string{"--config", config.Name(), "add", "--no-login", tt.remote, tt.uri}
		c.env.RunSingularity(
			t,
			e2e.AsSubtest(tt.name),
			e2e.WithProfile(e2e.UserProfile),
			e2e.WithCommand("remote"),
			e2e.WithArgs(argv...),
			e2e.ExpectExit(0),
		)
	}

	testPass := []struct {
		name   string
		remote string
	}{
		{"ValidRemote", "cloud"},
	}

	for _, tt := range testPass {
		argv := []string{"--config", config.Name(), "status", tt.remote}
		c.env.RunSingularity(
			t,
			e2e.AsSubtest(tt.name),
			e2e.WithProfile(e2e.UserProfile),
			e2e.WithCommand("remote"),
			e2e.WithArgs(argv...),
			e2e.ExpectExit(0),
		)
	}

	testFail := []struct {
		name   string
		remote string
	}{
		{"NonExistingRemote", "notaremote"},
		{"NonExistingEndpoint", "invalid"},
	}

	for _, tt := range testFail {
		argv := []string{"--config", config.Name(), "status", tt.remote}
		c.env.RunSingularity(
			t,
			e2e.AsSubtest(tt.name),
			e2e.WithProfile(e2e.UserProfile),
			e2e.WithCommand("remote"),
			e2e.WithArgs(argv...),
			e2e.ExpectExit(255),
		)
	}
}

// remoteList tests the functionality of "singularity remote list" command
func (c ctx) remoteList(t *testing.T) {
	config, err := os.CreateTemp(c.env.TestDir, "testConfig-")
	if err != nil {
		log.Fatal(err)
	}

	t.Cleanup(func() {
		if !t.Failed() {
			os.Remove(config.Name()) // clean up
		}
	})

	testPass := []struct {
		name string
	}{
		{"EmptyConfig"},
	}

	for _, tt := range testPass {
		argv := []string{"--config", config.Name(), "list"}
		c.env.RunSingularity(
			t,
			e2e.AsSubtest(tt.name),
			e2e.WithProfile(e2e.UserProfile),
			e2e.WithCommand("remote"),
			e2e.WithArgs(argv...),
			e2e.ExpectExit(0),
		)
	}

	// Prep config by adding multiple remotes
	add := []struct {
		name   string
		remote string
		uri    string
	}{
		{"addCloud", "cloud", "cloud.sylabs.io"},
		{"addRemote", "remote", "cloud.sylabs.io"},
	}

	for _, tt := range add {
		argv := []string{"--config", config.Name(), "add", "--no-login", tt.remote, tt.uri}
		c.env.RunSingularity(
			t,
			e2e.AsSubtest(tt.name),
			e2e.WithProfile(e2e.UserProfile),
			e2e.WithCommand("remote"),
			e2e.WithArgs(argv...),
			e2e.ExpectExit(0),
		)
	}

	testPass = []struct {
		name string
	}{
		{"PopulatedConfig"},
	}

	for _, tt := range testPass {
		argv := []string{"--config", config.Name(), "list"}
		c.env.RunSingularity(
			t,
			e2e.AsSubtest(tt.name),
			e2e.WithProfile(e2e.UserProfile),
			e2e.WithCommand("remote"),
			e2e.WithArgs(argv...),
			e2e.ExpectExit(0),
		)
	}

	// Prep config by selecting a remote to default to
	use := []struct {
		name   string
		remote string
	}{
		{"useCloud", "cloud"},
	}

	for _, tt := range use {
		argv := []string{"--config", config.Name(), "use", tt.remote}
		c.env.RunSingularity(
			t,
			e2e.AsSubtest(tt.name),
			e2e.WithProfile(e2e.UserProfile),
			e2e.WithCommand("remote"),
			e2e.WithArgs(argv...),
			e2e.ExpectExit(0),
		)
	}

	testPass = []struct {
		name string
	}{
		{"PopulatedConfigWithDefault"},
	}

	for _, tt := range testPass {
		argv := []string{"--config", config.Name(), "list"}
		c.env.RunSingularity(
			t,
			e2e.AsSubtest(tt.name),
			e2e.WithProfile(e2e.UserProfile),
			e2e.WithCommand("remote"),
			e2e.WithArgs(argv...),
			e2e.ExpectExit(0),
		)
	}
}

func (c ctx) remoteTestHelp(t *testing.T) {
	tests := []struct {
		name           string
		cmdArgs        []string
		expectedOutput string
	}{
		{
			name:           "add help",
			cmdArgs:        []string{"add", "--help"},
			expectedOutput: "Add a new singularity remote endpoint",
		},
		{
			name:           "list help",
			cmdArgs:        []string{"list", "--help"},
			expectedOutput: "List all singularity remote endpoints that are configured",
		},
		{
			name:           "login help",
			cmdArgs:        []string{"login", "--help"},
			expectedOutput: "Login to a singularity remote endpoint",
		},
		{
			name:           "remove help",
			cmdArgs:        []string{"remove", "--help"},
			expectedOutput: "Remove an existing singularity remote endpoint",
		},
		{
			name:           "status help",
			cmdArgs:        []string{"status", "--help"},
			expectedOutput: "Check the status of the singularity services at an endpoint",
		},
		{
			name:           "use help",
			cmdArgs:        []string{"use", "--help"},
			expectedOutput: "Set a singularity remote endpoint to be actively used",
		},
	}

	for _, tt := range tests {
		c.env.RunSingularity(
			t,
			e2e.AsSubtest(tt.name),
			e2e.WithProfile(e2e.UserProfile),
			e2e.WithCommand("remote"),
			e2e.WithArgs(tt.cmdArgs...),
			e2e.ExpectExit(
				0,
				e2e.ExpectOutput(e2e.RegexMatch, `^`+tt.expectedOutput),
			),
		)
	}
}

func (c ctx) remoteUseExclusive(t *testing.T) {
	var (
		sylabsRemote = "SylabsCloud"
		testRemote   = "e2e"
	)

	// Move the user's and root's remote.yaml files aside for the purposes of this test
	origRelPath := filepath.Join(".singularity", "remote.yaml")
	asideRelPath := fmt.Sprintf("%s.aside-remoteUseExclusive", origRelPath)
	moveAsideRemoteYAML := func(t *testing.T) {
		homeDir := e2e.CurrentUser(t).Dir
		origRemoteYAML := filepath.Join(homeDir, origRelPath)
		asideRemoteYAML := filepath.Join(homeDir, asideRelPath)
		if !fs.IsReadable(origRemoteYAML) {
			return
		}
		if err := os.Rename(origRemoteYAML, asideRemoteYAML); err != nil {
			t.Fatalf("While trying to mv %q to %q: %v", origRemoteYAML, asideRemoteYAML, err)
		}
	}
	restoreRemoteYAML := func(t *testing.T) {
		homeDir := e2e.CurrentUser(t).Dir
		origRemoteYAML := filepath.Join(homeDir, origRelPath)
		asideRemoteYAML := filepath.Join(homeDir, asideRelPath)
		if !fs.IsReadable(asideRemoteYAML) {
			return
		}
		if err := os.Rename(asideRemoteYAML, origRemoteYAML); err != nil {
			t.Fatalf("While trying to mv %q to %q: %v", asideRemoteYAML, origRemoteYAML, err)
		}
	}

	moveAsideRemoteYAML(t)
	e2e.Privileged(moveAsideRemoteYAML)(t)

	t.Cleanup(func() {
		restoreRemoteYAML(t)
		e2e.Privileged(restoreRemoteYAML)(t)
	})

	tests := []struct {
		name       string
		command    string
		args       []string
		expectExit int
		profile    e2e.Profile
	}{
		{
			name:       "use exclusive as user",
			command:    "remote use",
			args:       []string{"--exclusive", "--global", testRemote},
			expectExit: 255,
			profile:    e2e.UserProfile,
		},
		{
			name:       "add remote",
			command:    "remote add",
			args:       []string{"--global", testRemote, "cloud.test.com"},
			expectExit: 0,
			profile:    e2e.RootProfile,
		},
		{
			name:       "use remote exclusive with global as root",
			command:    "remote use",
			args:       []string{"--exclusive", "--global", testRemote},
			expectExit: 0,
			profile:    e2e.RootProfile,
		},
		{
			name:       "use remote SylabsCloud as user KO",
			command:    "remote use",
			args:       []string{sylabsRemote},
			expectExit: 255,
			profile:    e2e.UserProfile,
		},
		{
			name:       "remove e2e remote",
			command:    "remote remove",
			args:       []string{"--global", testRemote},
			expectExit: 0,
			profile:    e2e.RootProfile,
		},
		{
			name:       "use remote SylabsCloud as user OK",
			command:    "remote use",
			args:       []string{sylabsRemote},
			expectExit: 0,
			profile:    e2e.UserProfile,
		},
		{
			name:       "add remote",
			command:    "remote add",
			args:       []string{"--global", testRemote, "cloud.test.com"},
			expectExit: 0,
			profile:    e2e.RootProfile,
		},
		{
			name:       "use remote exclusive without global as root",
			command:    "remote use",
			args:       []string{"--exclusive", testRemote},
			expectExit: 0,
			profile:    e2e.RootProfile,
		},
		{
			name:       "use remote SylabsCloud as exclusive",
			command:    "remote use",
			args:       []string{"--exclusive", sylabsRemote},
			expectExit: 0,
			profile:    e2e.RootProfile,
		},
		{
			name:       "use remote e2e as exclusive",
			command:    "remote use",
			args:       []string{"--exclusive", testRemote},
			expectExit: 0,
			profile:    e2e.RootProfile,
		},
		{
			name:       "use remote SylabsCloud as user KO",
			command:    "remote use",
			args:       []string{sylabsRemote},
			expectExit: 255,
			profile:    e2e.UserProfile,
		},
		{
			name:       "remove e2e remote",
			command:    "remote remove",
			args:       []string{"--global", testRemote},
			expectExit: 0,
			profile:    e2e.RootProfile,
		},
		{
			name:       "no default remote set",
			command:    "key search",
			args:       []string{"@"},
			expectExit: 255,
			profile:    e2e.RootProfile,
		},
		{
			name:       "use remote SylabsCloud global",
			command:    "remote use",
			args:       []string{"--global", sylabsRemote},
			expectExit: 0,
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
			e2e.ExpectExit(tt.expectExit),
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
		"add":            c.remoteAdd,
		"list":           c.remoteList,
		"default or not": c.remoteDefaultOrNot,
		"remove":         c.remoteRemove,
		"status":         c.remoteStatus,
		"test help":      c.remoteTestHelp,
		"use":            c.remoteUse,
		"use exclusive":  np(c.remoteUseExclusive),
	}
}
