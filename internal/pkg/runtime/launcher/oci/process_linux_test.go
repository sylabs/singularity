// Copyright (c) 2022, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package oci

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestSingularityEnvMap(t *testing.T) {
	tests := []struct {
		name   string
		setEnv map[string]string
		want   map[string]string
	}{
		{
			name:   "None",
			setEnv: map[string]string{},
			want:   map[string]string{},
		},
		{
			name:   "NonPrefixed",
			setEnv: map[string]string{"FOO": "bar"},
			want:   map[string]string{},
		},
		{
			name:   "PrefixedSingle",
			setEnv: map[string]string{"SINGULARITYENV_FOO": "bar"},
			want:   map[string]string{"FOO": "bar"},
		},
		{
			name: "PrefixedMultiple",
			setEnv: map[string]string{
				"SINGULARITYENV_FOO": "bar",
				"SINGULARITYENV_ABC": "123",
			},
			want: map[string]string{
				"FOO": "bar",
				"ABC": "123",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for k, v := range tt.setEnv {
				os.Setenv(k, v)
				t.Cleanup(func() {
					os.Unsetenv(k)
				})
			}
			if got := singularityEnvMap(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("singularityEnvMap() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEnvFileMap(t *testing.T) {
	tests := []struct {
		name    string
		envFile string
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
	}

	tmpDir := t.TempDir()
	envFile := filepath.Join(tmpDir, "env-file")

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := os.WriteFile(envFile, []byte(tt.envFile), 0o755); err != nil {
				t.Fatalf("Could not write test env-file: %v", err)
			}

			got, err := envFileMap(context.Background(), envFile)
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
