// Copyright (c) 2020-2026 Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package enterprise

import (
	"bytes"
	"io"
	"testing"
	"time"

	"github.com/blang/semver/v4"
)

const (
	TestSifImage      = "../../pkg/image/testdata/test-image.sif"
	TestRegistryImage = "docker://localhost:5000/test-image-sif"
)

func TestNewExecutor(t *testing.T) {
	//nolint:maligned // Filled in by the linker.
	tests := []struct {
		name          string
		opts          []ExecutorOpt
		wantVersion   semver.Version
		wantGitCommit string
		wantGitDirty  bool
		wantBuiltAt   time.Time
		wantBuiltBy   string
		wantErr       bool
	}{
		{
			name: "Defaults",
		},
		{
			name: "BadVersion",
			opts: []ExecutorOpt{
				OptExecutorVersion("bad"),
			},
			wantErr: true,
		},
		{
			name: "Version",
			opts: []ExecutorOpt{
				OptExecutorVersion("1.0.0-alpha.1+build"),
			},
			wantVersion: semver.MustParse("1.0.0-alpha.1+build"),
		},
		{
			name: "GitCommit",
			opts: []ExecutorOpt{
				OptExecutorGitCommit("01234567"),
			},
			wantGitCommit: "01234567",
		},
		{
			name: "BadGitState",
			opts: []ExecutorOpt{
				OptExecutorGitState("bad"),
			},
			wantErr: true,
		},
		{
			name: "GitStateClean",
			opts: []ExecutorOpt{
				OptExecutorGitState("clean"),
			},
			wantGitDirty: false,
		},
		{
			name: "GitStateDirty",
			opts: []ExecutorOpt{
				OptExecutorGitState("dirty"),
			},
			wantGitDirty: true,
		},
		{
			name: "BadBuiltAt",
			opts: []ExecutorOpt{
				OptExecutorBuiltAt("bad"),
			},
			wantErr: true,
		},
		{
			name: "BuiltAt",
			opts: []ExecutorOpt{
				OptExecutorBuiltAt("2006-01-02T15:04:05Z"),
			},
			wantBuiltAt: time.Unix(1136214245, 0),
		},
		{
			name: "BuiltBy",
			opts: []ExecutorOpt{
				OptExecutorBuiltBy("bob"),
			},
			wantBuiltBy: "bob",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e, err := NewExecutor(tt.opts...)

			if (err != nil) != tt.wantErr {
				t.Errorf("got error %v, want %v", err, tt.wantErr)
			}

			if e != nil {
				if got, want := e.v, tt.wantVersion; !got.Equals(want) {
					t.Errorf("got version %v, want %v", got, want)
				}
				if got, want := e.gitCommit, tt.wantGitCommit; got != want {
					t.Errorf("got gitCommit %v, want %v", got, want)
				}
				if got, want := e.gitDirty, tt.wantGitDirty; got != want {
					t.Errorf("got gitDirty %v, want %v", got, want)
				}
				if got, want := e.builtAt, tt.wantBuiltAt; !got.Equal(want) {
					t.Errorf("got builtAt %v, want %v", got, want)
				}
				if got, want := e.builtBy, tt.wantBuiltBy; got != want {
					t.Errorf("got builtBy %v, want %v", got, want)
				}
			}
		})
	}
}

func TestExecutor_Execute(t *testing.T) {
	e, err := NewExecutor()
	if err != nil {
		t.Fatal(err)
	}

	// Explicitly override args, so test is agnostic of os.Args[1:].
	e.rootCmd.SetArgs([]string{})

	// Disconnect stdin/stdout/stderr.
	e.rootCmd.SetIn(bytes.NewReader(nil))
	e.rootCmd.SetOut(io.Discard)
	e.rootCmd.SetErr(io.Discard)

	if err := e.Execute(t.Context()); err != nil {
		t.Error("unexepcted nil error")
	}
}
