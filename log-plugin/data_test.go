// Copyright (c) 2020-2026, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package main

import (
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/sylabs/singularity/v4/pkg/cmdline"
)

const (
	// Prefix of expected name of the executable these tests are run as
	expectedExe = "log-plugin"
	// Prefix for expected version of Singularity
	expectedVersion = "4."
)

func TestPopulateAccount(t *testing.T) {
	var p pluginImplementation
	p.populateAccount()

	u, err := user.Current()
	if err != nil {
		t.Fatalf("Failed to retrieve user information: %v", err)
	}

	if strconv.Itoa(p.data.UID) != u.Uid {
		t.Errorf("Populated UID %v does not match expected %v", p.data.UID, u.Uid)
	}
	if strconv.Itoa(p.data.GID) != u.Gid {
		t.Errorf("Populated GID %v does not match expected %v", p.data.GID, u.Gid)
	}
	if p.data.User != u.Username {
		t.Errorf("Populated User %v does not match expected %v", p.data.User, u.Username)
	}
}

func TestPopulateSingularity(t *testing.T) {
	var p pluginImplementation

	// Singularity's commands are in an inaccessible internal package, so we have to fake a
	// `singularity run` nested command here for our tests.
	testRootCmd := cobra.Command{
		Use: "singularity",
	}
	testCmd := cobra.Command{
		Use:  "run",
		Args: cobra.ExactArgs(1),
	}
	testVal := true
	testFlag := cmdline.Flag{
		ID:           "testFlag",
		Value:        &testVal,
		DefaultValue: false,
		Name:         "test",
		ShortHand:    "t",
		Usage:        "test",
	}
	testCmdMan := cmdline.NewCommandManager(&testRootCmd)
	testCmdMan.RegisterCmd(&testCmd)
	testCmdMan.RegisterFlagForCmd(&testFlag, &testCmd)
	testCmdMan.SetCmdGroup("actions_instance", &testCmd)

	// TODO: We need to invoke cobra's flag parsing to test we get a list of flags
	// set on a command. However this is not currently working with the standard
	// pattern below. Need to investigate why - which will involve time diving into
	// Singularity's command and flag managers that are wrapping around cobra stuff
	//
	//
	// Faking `singularity run -t test.sif`
	// testCmd.SetArgs([]string{
	//	"--test",
	//	"test.sif",
	// })
	// buf := new(bytes.Buffer)
	// cmd.SetOutput(buf)
	// b := bytes.Buffer()
	// testRootCmd.DebugFlags()
	// err := testRootCmd.Execute()
	// if err != nil {
	//	t.Fatalf("Failed to invoke Cobra Command: %v", err)
	// }

	p.command = &testCmd
	p.commandManager = testCmdMan
	p.args = []string{"test.sif"}

	// Setup a known working directory
	tmpDir := t.TempDir()
	t.Chdir(tmpDir)

	p.populateSingularity()

	if !strings.HasPrefix(filepath.Base(p.data.Executable), expectedExe) {
		t.Errorf("Populated Executable %q does not match expected %q", p.data.Executable, expectedExe)
	}

	if !strings.HasPrefix(p.data.Version, expectedVersion) {
		t.Errorf("Populated Version %q does not begin with expected %q", p.data.Version, expectedVersion)
	}

	if p.data.Command != testCmd.Use {
		t.Errorf("Populated Command %q does not match expected %q", p.data.Command, testCmd.Use)
	}

	// TODO - enable once we can fake a Cobra command invocation / option parsing
	// if !reflect.DeepEqual(p.data.Flags, flagVals) {
	//	t.Errorf("Populated Flags %q does not match expected %q", p.data.Flags, flagVals)
	// }

	if p.data.CWD != tmpDir {
		t.Errorf("Populated CWD %q does not match expected %q", p.data.CWD, tmpDir)
	}
}

func TestPopulateEnvironment(t *testing.T) {
	var p pluginImplementation

	invalidVar := "NOT_AN_ENVVAR"
	testVar := "TestPopulateEnvironment"
	testVal := "test"
	t.Setenv(testVar, testVal)

	p.populateEnvironment()

	_, ok := p.data.Env[invalidVar]
	if ok == true {
		t.Errorf("Got ok %v for env var that should not be set", ok)
	}

	val, ok := p.data.Env[testVar]
	if ok == false {
		t.Errorf("Got ok %v for env var that should be set", ok)
	}
	if val != testVal {
		t.Errorf("Got %q, expected %q for env var %q", val, testVal, testVar)
	}
}
