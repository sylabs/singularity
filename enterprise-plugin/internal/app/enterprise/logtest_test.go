// Copyright (c) 2021-2026, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package enterprise

import (
	"fmt"
	"os"
	"testing"
)

// Verify sylog logging displays as expected with flags set on the root command.
func TestExecutor_logtest(t *testing.T) {
	tests := []struct {
		name  string
		flags []string
	}{
		{
			name:  "sylog-debug",
			flags: []string{"--debug", "logtest"},
		},
		{
			name:  "sylog-verbose",
			flags: []string{"--verbose", "logtest"},
		},
		{
			name:  "sylog-quiet",
			flags: []string{"--quiet", "logtest"},
		},
		{
			name:  "sylog-silent",
			flags: []string{"--silent", "logtest"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e, err := NewExecutor()
			if err != nil {
				t.Fatal(err)
			}
			ct := newCommandTester(e.rootCmd)
			if err := ct.execute(tt.flags...); err != nil {
				t.Fatalf("root command execution failed: %v", err)
			}
			// sylog messages have a [U=<uid>,P=<pid>] prefix that we need to template
			uidStr := fmt.Sprintf("[U=%d,P=%d]", os.Getuid(), os.Getpid())
			data := struct {
				UIDPid string
			}{
				UIDPid: fmt.Sprintf("%-19s", uidStr),
			}
			ct.assertStdout(t, nil)
			ct.assertStderr(t, data)
		})
	}
}
