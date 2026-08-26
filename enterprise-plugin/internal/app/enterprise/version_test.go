// Copyright (c) 2020-2026, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package enterprise

import (
	"fmt"
	"runtime"
	"testing"
)

func TestExecutor_version(t *testing.T) {
	tests := []struct {
		name string
		opts []ExecutorOpt
	}{
		{
			name: "Defaults",
		},
		{
			name: "Version",
			opts: []ExecutorOpt{
				OptExecutorVersion("0.1.2-alpha.3+build"),
			},
		},
		{
			name: "BuiltAt",
			opts: []ExecutorOpt{
				OptExecutorBuiltAt("2006-01-02T15:04:05Z"),
			},
		},
		{
			name: "BuiltBy",
			opts: []ExecutorOpt{
				OptExecutorBuiltBy("test"),
			},
		},
		{
			name: "GitStateClean",
			opts: []ExecutorOpt{
				OptExecutorGitCommit("e68ef3dced07b0e28b5f9f06b3bb648e853a684a"),
				OptExecutorGitState("clean"),
			},
		},
		{
			name: "GitStateDirty",
			opts: []ExecutorOpt{
				OptExecutorGitCommit("e68ef3dced07b0e28b5f9f06b3bb648e853a684a"),
				OptExecutorGitState("dirty"),
			},
		},
		{
			name: "Full",
			opts: []ExecutorOpt{
				OptExecutorVersion("0.1.2-alpha.3+build"),
				OptExecutorBuiltAt("2006-01-02T15:04:05Z"),
				OptExecutorBuiltBy("test"),
				OptExecutorGitCommit("e68ef3dced07b0e28b5f9f06b3bb648e853a684a"),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e, err := NewExecutor(tt.opts...)
			if err != nil {
				t.Fatal(err)
			}

			ct := newCommandTester(e.initVersion())

			if err := ct.execute(); err != nil {
				t.Fatal(err)
			}

			// Output contains Go version and OS/arch, which are templated in stdout.golden.

			data := struct {
				Runtime string
			}{
				Runtime: fmt.Sprintf("%v (%v/%v)", runtime.Version(), runtime.GOOS, runtime.GOARCH),
			}
			ct.assertStdout(t, data)

			ct.assertStderr(t, nil)
		})
	}
}
