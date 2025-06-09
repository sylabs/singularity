// Copyright (c) 2018-2023, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package env

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/sylabs/singularity/v4/internal/pkg/test"
)

func TestSetFromList(t *testing.T) {
	test.DropPrivilege(t)
	defer test.ResetPrivilege(t)

	tt := []struct {
		name    string
		environ []string
		wantErr bool
	}{
		{
			name: "all ok",
			environ: []string{
				"LD_LIBRARY_PATH=/.singularity.d/libs",
				"HOME=/home/tester",
				"PS1=test",
				"TERM=xterm-256color",
				"PATH=/usr/games:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
				"LANG=C",
				"SINGULARITY_CONTAINER=/tmp/lolcow.sif",
				"PWD=/tmp",
				"LC_ALL=C",
				"SINGULARITY_NAME=lolcow.sif",
			},
			wantErr: false,
		},
		{
			name: "bad envs",
			environ: []string{
				"LD_LIBRARY_PATH=/.singularity.d/libs",
				"HOME=/home/tester",
				"PS1=test",
				"TERM=xterm-256color",
				"PATH=/usr/games:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
				"LANG=C",
				"SINGULARITY_CONTAINER=/tmp/lolcow.sif",
				"TEST",
				"LC_ALL=C",
				"SINGULARITY_NAME=lolcow.sif",
			},
			wantErr: true,
		},
	}
	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			err := SetFromList(tc.environ)
			if tc.wantErr && err == nil {
				t.Fatalf("Expected error, but got nil")
			}
			if !tc.wantErr && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}

func TestSingularityEnvMap(t *testing.T) {
	tests := []struct {
		name    string
		hostEnv []string
		want    map[string]string
	}{
		{
			name:    "None",
			hostEnv: []string{},
			want:    map[string]string{},
		},
		{
			name:    "NonPrefixed",
			hostEnv: []string{"FOO=bar"},
			want:    map[string]string{},
		},
		{
			name:    "PrefixedSingle",
			hostEnv: []string{"SINGULARITYENV_FOO=bar"},
			want:    map[string]string{"FOO": "bar"},
		},
		{
			name: "PrefixedMultiple",
			hostEnv: []string{
				"SINGULARITYENV_FOO=bar",
				"SINGULARITYENV_ABC=123",
			},
			want: map[string]string{
				"FOO": "bar",
				"ABC": "123",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SingularityEnvMap(tt.hostEnv); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("singularityEnvMap() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEnvFileMap(t *testing.T) {
	tests := []struct {
		name    string
		envFile string
		hostEnv []string
		want    map[string]string
		wantErr bool
	}{
		{
			name:    "EmptyFile",
			envFile: "",
			want:    map[string]string{},
			wantErr: false,
		},
		{
			name: "Simple",
			envFile: `FOO=BAR
			ABC=123`,
			want: map[string]string{
				"FOO": "BAR",
				"ABC": "123",
			},
			wantErr: false,
		},
		{
			name:    "DoubleQuote",
			envFile: `FOO="FOO BAR"`,
			want: map[string]string{
				"FOO": "FOO BAR",
			},
			wantErr: false,
		},
		{
			name:    "SingleQuote",
			envFile: `FOO='FOO BAR'`,
			want: map[string]string{
				"FOO": "FOO BAR",
			},
			wantErr: false,
		},
		{
			name:    "MultiLine",
			envFile: "FOO=\"FOO\nBAR\"",
			want: map[string]string{
				"FOO": "FOO\nBAR",
			},
			wantErr: false,
		},
		{
			name:    "Invalid",
			envFile: "!!!@@NOTAVAR",
			want:    map[string]string{},
			wantErr: true,
		},
		{
			name:    "HostEnvUnset",
			envFile: "HELLO=$YOU",
			want: map[string]string{
				"HELLO": "",
			},
			wantErr: false,
		},
		{
			name:    "HostEnvSet",
			envFile: "HELLO=$YOU",
			hostEnv: []string{"YOU=YOU"},
			want: map[string]string{
				"HELLO": "YOU",
			},
			wantErr: false,
		},
	}

	tmpDir := t.TempDir()
	envFile := filepath.Join(tmpDir, "env-file")

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := os.WriteFile(envFile, []byte(tt.envFile), 0o755); err != nil {
				t.Fatalf("Could not write test env-file: %v", err)
			}

			got, err := FileMap(context.Background(), envFile, []string{}, tt.hostEnv)
			if (err != nil) != tt.wantErr {
				t.Errorf("envFileMap() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("envFileMap() = %v, want %v", got, tt.want)
			}
		})
	}
}
