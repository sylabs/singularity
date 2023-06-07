// Copyright (c) 2022-2023, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package launcher

import (
	"reflect"
	"testing"
)

func TestExecParams_ActionArgs(t *testing.T) {
	tests := []struct {
		name     string
		Image    string
		Action   string
		Process  string
		Args     []string
		Instance string
		wantArgs []string
		wantErr  bool
	}{
		// exec
		{
			name:     "exec",
			Image:    "image.sif",
			Action:   "exec",
			Process:  "process",
			wantArgs: []string{"/.singularity.d/actions/exec", "process"},
			wantErr:  false,
		},
		{
			name:     "execArgs",
			Image:    "image.sif",
			Action:   "exec",
			Process:  "process",
			Args:     []string{"a", "b", "c"},
			wantArgs: []string{"/.singularity.d/actions/exec", "process", "a", "b", "c"},
			wantErr:  false,
		},
		{
			name:     "execNoProcess",
			Image:    "image.sif",
			Action:   "exec",
			Args:     []string{"a", "b", "c"},
			wantArgs: []string{},
			wantErr:  true,
		},
		{
			name:     "execWithInstance",
			Image:    "image.sif",
			Action:   "exec",
			Process:  "process",
			Instance: "myinstance",
			wantArgs: []string{},
			wantErr:  true,
		},
		// run
		{
			name:     "run",
			Image:    "image.sif",
			Action:   "run",
			wantArgs: []string{"/.singularity.d/actions/run"},
			wantErr:  false,
		},
		{
			name:     "runArgs",
			Image:    "image.sif",
			Action:   "run",
			Args:     []string{"a", "b", "c"},
			wantArgs: []string{"/.singularity.d/actions/run", "a", "b", "c"},
			wantErr:  false,
		},
		{
			name:     "runNoImage",
			Action:   "run",
			wantArgs: []string{},
			wantErr:  true,
		},
		{
			name:     "runWithProcess",
			Image:    "image.sif",
			Action:   "run",
			Process:  "process",
			wantArgs: []string{},
			wantErr:  true,
		},
		{
			name:     "runWithInstance",
			Image:    "image.sif",
			Action:   "run",
			Instance: "myinstance",
			wantArgs: []string{"/.singularity.d/actions/run"},
			wantErr:  false,
		},
		// shell
		{
			name:     "shell",
			Image:    "image.sif",
			Action:   "shell",
			wantArgs: []string{"/.singularity.d/actions/shell"},
			wantErr:  false,
		},
		{
			name:     "shellArgs",
			Image:    "image.sif",
			Action:   "shell",
			Args:     []string{"a", "b", "c"},
			wantArgs: []string{"/.singularity.d/actions/shell", "a", "b", "c"},
			wantErr:  false,
		},
		{
			name:     "shellWithProcess",
			Image:    "image.sif",
			Action:   "shell",
			Process:  "process",
			wantArgs: []string{},
			wantErr:  true,
		},
		{
			name:     "shellWithInstance",
			Image:    "image.sif",
			Action:   "shell",
			Instance: "myinstance",
			wantArgs: []string{},
			wantErr:  true,
		},
		// start
		{
			name:     "start",
			Image:    "image.sif",
			Action:   "start",
			Instance: "myinstance",
			wantArgs: []string{"/.singularity.d/actions/start"},
			wantErr:  false,
		},
		{
			name:     "startArgs",
			Image:    "image.sif",
			Action:   "start",
			Instance: "myinstance",
			Args:     []string{"a", "b", "c"},
			wantArgs: []string{"/.singularity.d/actions/start", "a", "b", "c"},
			wantErr:  false,
		},
		{
			name:     "startWithProcess",
			Image:    "image.sif",
			Action:   "start",
			Instance: "myinstance",
			Process:  "process",
			wantArgs: []string{},
			wantErr:  true,
		},
		{
			name:     "startNoInstance",
			Image:    "image.sif",
			Action:   "start",
			wantArgs: []string{},
			wantErr:  true,
		},
		// test
		{
			name:     "test",
			Image:    "image.sif",
			Action:   "test",
			wantArgs: []string{"/.singularity.d/actions/test"},
			wantErr:  false,
		},
		{
			name:     "testArgs",
			Image:    "image.sif",
			Action:   "test",
			Args:     []string{"a", "b", "c"},
			wantArgs: []string{"/.singularity.d/actions/test", "a", "b", "c"},
			wantErr:  false,
		},
		{
			name:     "testWithProcess",
			Image:    "image.sif",
			Action:   "test",
			Process:  "process",
			wantArgs: []string{},
			wantErr:  true,
		},
		{
			name:     "testWithInstance",
			Image:    "image.sif",
			Action:   "test",
			Instance: "myinstance",
			wantArgs: []string{},
			wantErr:  true,
		},
		// Invalid
		{
			name:     "InvalidAction",
			Image:    "image.sif",
			Action:   "delete",
			wantArgs: []string{},
			wantErr:  true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ep := ExecParams{
				Image:    tt.Image,
				Action:   tt.Action,
				Process:  tt.Process,
				Args:     tt.Args,
				Instance: tt.Instance,
			}
			gotArgs, err := ep.ActionScriptArgs()
			if (err != nil) != tt.wantErr {
				t.Errorf("ExecParams.ActionArgs() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(gotArgs, tt.wantArgs) {
				t.Errorf("ExecParams.ActionArgs() = %v, want %v", gotArgs, tt.wantArgs)
			}
		})
	}
}
