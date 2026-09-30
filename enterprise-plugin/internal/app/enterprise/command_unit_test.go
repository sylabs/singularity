// Copyright (c) 2020-2026, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package enterprise

import (
	"bytes"
	"testing"

	"github.com/sebdah/goldie/v2"
	"github.com/spf13/cobra"
	"github.com/sylabs/singularity/v4/pkg/sylog"
)

type commandTester struct {
	cmd    *cobra.Command
	stdout bytes.Buffer
	stderr bytes.Buffer
}

// newCommandTester returns a commandTester that tests cmd.
func newCommandTester(cmd *cobra.Command) *commandTester {
	ct := commandTester{
		cmd:    cmd,
		stdout: bytes.Buffer{},
		stderr: bytes.Buffer{},
	}

	cmd.SetIn(bytes.NewReader(nil))
	cmd.SetOut(&ct.stdout)
	cmd.SetErr(&ct.stderr)
	sylog.SetWriter(&ct.stderr)

	return &ct
}

// execute runs the command under test with args.
func (ct *commandTester) execute(args ...string) error {
	if args == nil {
		args = []string{} // otherwise os.Args[1:] is used
	}
	ct.cmd.SetArgs(args)

	return ct.cmd.Execute()
}

// assertStdout compares the content written to stdout with the expected data in the golden file
// after executing it as a template with data parameter.
func (ct *commandTester) assertStdout(t *testing.T, data any) {
	t.Helper()

	g := goldie.New(t,
		goldie.WithTestNameForDir(true),
		goldie.WithSubTestNameForDir(true),
	)
	g.AssertWithTemplate(t, "stdout", data, ct.stdout.Bytes())
}

// assertStderr compares the content written to stderr with the expected data in the golden file
// after executing it as a template with data parameter.
func (ct *commandTester) assertStderr(t *testing.T, data any) {
	t.Helper()

	g := goldie.New(t,
		goldie.WithTestNameForDir(true),
		goldie.WithSubTestNameForDir(true),
	)
	g.AssertWithTemplate(t, "stderr", data, ct.stderr.Bytes())
}
