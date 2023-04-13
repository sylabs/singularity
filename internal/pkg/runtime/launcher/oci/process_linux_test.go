// Copyright (c) 2022-2023, Sylabs Inc. All rights reserved.
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

	imgspecv1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/opencontainers/runtime-spec/specs-go"
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

func TestGetProcessArgs(t *testing.T) {
	tests := []struct {
		name              string
		imgEntrypoint     []string
		imgCmd            []string
		bundleProcess     string
		bundleArgs        []string
		expectProcessArgs []string
	}{
		{
			name:              "imageEntrypointOnly",
			imgEntrypoint:     []string{"ENTRYPOINT"},
			imgCmd:            []string{},
			bundleProcess:     "",
			bundleArgs:        []string{},
			expectProcessArgs: []string{"ENTRYPOINT"},
		},
		{
			name:              "imageCmdOnly",
			imgEntrypoint:     []string{},
			imgCmd:            []string{"CMD"},
			bundleProcess:     "",
			bundleArgs:        []string{},
			expectProcessArgs: []string{"CMD"},
		},
		{
			name:              "imageEntrypointCMD",
			imgEntrypoint:     []string{"ENTRYPOINT"},
			imgCmd:            []string{"CMD"},
			bundleProcess:     "",
			bundleArgs:        []string{},
			expectProcessArgs: []string{"ENTRYPOINT", "CMD"},
		},
		{
			name:              "ProcessOnly",
			imgEntrypoint:     []string{},
			imgCmd:            []string{},
			bundleProcess:     "PROCESS",
			bundleArgs:        []string{},
			expectProcessArgs: []string{"PROCESS"},
		},
		{
			name:              "ArgsOnly",
			imgEntrypoint:     []string{},
			imgCmd:            []string{},
			bundleProcess:     "",
			bundleArgs:        []string{"ARGS"},
			expectProcessArgs: []string{"ARGS"},
		},
		{
			name:              "ProcessArgs",
			imgEntrypoint:     []string{},
			imgCmd:            []string{},
			bundleProcess:     "PROCESS",
			bundleArgs:        []string{"ARGS"},
			expectProcessArgs: []string{"PROCESS", "ARGS"},
		},
		{
			name:              "overrideEntrypointOnlyProcess",
			imgEntrypoint:     []string{"ENTRYPOINT"},
			imgCmd:            []string{},
			bundleProcess:     "PROCESS",
			bundleArgs:        []string{},
			expectProcessArgs: []string{"PROCESS"},
		},
		{
			name:              "overrideCmdOnlyArgs",
			imgEntrypoint:     []string{},
			imgCmd:            []string{"CMD"},
			bundleProcess:     "",
			bundleArgs:        []string{"ARGS"},
			expectProcessArgs: []string{"ARGS"},
		},
		{
			name:              "overrideBothProcess",
			imgEntrypoint:     []string{"ENTRYPOINT"},
			imgCmd:            []string{"CMD"},
			bundleProcess:     "PROCESS",
			bundleArgs:        []string{},
			expectProcessArgs: []string{"PROCESS"},
		},
		{
			name:              "overrideBothArgs",
			imgEntrypoint:     []string{"ENTRYPOINT"},
			imgCmd:            []string{"CMD"},
			bundleProcess:     "",
			bundleArgs:        []string{"ARGS"},
			expectProcessArgs: []string{"ENTRYPOINT", "ARGS"},
		},
		{
			name:              "overrideBothProcessArgs",
			imgEntrypoint:     []string{"ENTRYPOINT"},
			imgCmd:            []string{"CMD"},
			bundleProcess:     "PROCESS",
			bundleArgs:        []string{"ARGS"},
			expectProcessArgs: []string{"PROCESS", "ARGS"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			i := imgspecv1.Image{
				Config: imgspecv1.ImageConfig{
					Entrypoint: tt.imgEntrypoint,
					Cmd:        tt.imgCmd,
				},
			}
			args := getProcessArgs(i, tt.bundleProcess, tt.bundleArgs)
			if !reflect.DeepEqual(args, tt.expectProcessArgs) {
				t.Errorf("Expected: %v, Got: %v", tt.expectProcessArgs, args)
			}
		})
	}
}

func TestGetProcessEnv(t *testing.T) {
	tests := []struct {
		name      string
		imageEnv  []string
		bundleEnv map[string]string
		wantEnv   []string
	}{
		{
			name:      "Default",
			imageEnv:  []string{},
			bundleEnv: map[string]string{},
			wantEnv:   []string{"LD_LIBRARY_PATH=/.singularity.d/libs"},
		},
		{
			name:      "ImagePath",
			imageEnv:  []string{"PATH=/foo"},
			bundleEnv: map[string]string{},
			wantEnv: []string{
				"PATH=/foo",
				"LD_LIBRARY_PATH=/.singularity.d/libs",
			},
		},
		{
			name:      "OverridePath",
			imageEnv:  []string{"PATH=/foo"},
			bundleEnv: map[string]string{"PATH": "/bar"},
			wantEnv: []string{
				"PATH=/bar",
				"LD_LIBRARY_PATH=/.singularity.d/libs",
			},
		},
		{
			name:      "AppendPath",
			imageEnv:  []string{"PATH=/foo"},
			bundleEnv: map[string]string{"APPEND_PATH": "/bar"},
			wantEnv: []string{
				"PATH=/foo:/bar",
				"LD_LIBRARY_PATH=/.singularity.d/libs",
			},
		},
		{
			name:      "PrependPath",
			imageEnv:  []string{"PATH=/foo"},
			bundleEnv: map[string]string{"PREPEND_PATH": "/bar"},
			wantEnv: []string{
				"PATH=/bar:/foo",
				"LD_LIBRARY_PATH=/.singularity.d/libs",
			},
		},
		{
			name:      "ImageLdLibraryPath",
			imageEnv:  []string{"LD_LIBRARY_PATH=/foo"},
			bundleEnv: map[string]string{},
			wantEnv: []string{
				"LD_LIBRARY_PATH=/foo:/.singularity.d/libs",
			},
		},
		{
			name:      "BundleLdLibraryPath",
			imageEnv:  []string{},
			bundleEnv: map[string]string{"LD_LIBRARY_PATH": "/foo"},
			wantEnv: []string{
				"LD_LIBRARY_PATH=/foo:/.singularity.d/libs",
			},
		},
		{
			name:      "OverrideLdLibraryPath",
			imageEnv:  []string{"LD_LIBRARY_PATH=/foo"},
			bundleEnv: map[string]string{"LD_LIBRARY_PATH": "/bar"},
			wantEnv: []string{
				"LD_LIBRARY_PATH=/bar:/.singularity.d/libs",
			},
		},
		{
			name:      "ImageVar",
			imageEnv:  []string{"FOO=bar"},
			bundleEnv: map[string]string{},
			wantEnv: []string{
				"FOO=bar",
				"LD_LIBRARY_PATH=/.singularity.d/libs",
			},
		},
		{
			name:      "ImageOverride",
			imageEnv:  []string{"FOO=bar"},
			bundleEnv: map[string]string{"FOO": "baz"},
			wantEnv: []string{
				"FOO=baz",
				"LD_LIBRARY_PATH=/.singularity.d/libs",
			},
		},
		{
			name:      "ImageAdditional",
			imageEnv:  []string{"FOO=bar"},
			bundleEnv: map[string]string{"ABC": "123"},
			wantEnv: []string{
				"FOO=bar",
				"ABC=123",
				"LD_LIBRARY_PATH=/.singularity.d/libs",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			imgSpec := imgspecv1.Image{
				Config: imgspecv1.ImageConfig{Env: tt.imageEnv},
			}

			env := getProcessEnv(imgSpec, tt.bundleEnv)

			if !reflect.DeepEqual(env, tt.wantEnv) {
				t.Errorf("want: %v, got: %v", tt.wantEnv, env)
			}
		})
	}
}

func TestLauncher_reverseMapByRange(t *testing.T) {
	tests := []struct {
		name       string
		targetUID  uint32
		targetGID  uint32
		subUIDMap  specs.LinuxIDMapping
		subGIDMap  specs.LinuxIDMapping
		wantUIDMap []specs.LinuxIDMapping
		wantGIDMap []specs.LinuxIDMapping
		wantErr    bool
	}{
		{
			// TargetID is smaller than size of subuid/subgid map.
			name:      "LowTargetID",
			targetUID: 1000,
			targetGID: 2000,
			subUIDMap: specs.LinuxIDMapping{HostID: 1000, ContainerID: 100000, Size: 65536},
			subGIDMap: specs.LinuxIDMapping{HostID: 2000, ContainerID: 200000, Size: 65536},
			wantUIDMap: []specs.LinuxIDMapping{
				{ContainerID: 0, HostID: 1, Size: 1000},
				{ContainerID: 1000, HostID: 0, Size: 1},
				{ContainerID: 1001, HostID: 1001, Size: 64536},
			},
			wantGIDMap: []specs.LinuxIDMapping{
				{ContainerID: 0, HostID: 1, Size: 2000},
				{ContainerID: 2000, HostID: 0, Size: 1},
				{ContainerID: 2001, HostID: 2001, Size: 63536},
			},
		},
		{
			// TargetID is higher than size of subuid/subgid map.
			name:      "HighTargetID",
			targetUID: 70000,
			targetGID: 80000,
			subUIDMap: specs.LinuxIDMapping{HostID: 1000, ContainerID: 100000, Size: 65536},
			subGIDMap: specs.LinuxIDMapping{HostID: 2000, ContainerID: 200000, Size: 65536},
			wantUIDMap: []specs.LinuxIDMapping{
				{ContainerID: 0, HostID: 1, Size: 65536},
				{ContainerID: 70000, HostID: 0, Size: 1},
			},
			wantGIDMap: []specs.LinuxIDMapping{
				{ContainerID: 0, HostID: 1, Size: 65536},
				{ContainerID: 80000, HostID: 0, Size: 1},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotUIDMap, gotGIDMap := reverseMapByRange(tt.targetUID, tt.targetGID, tt.subUIDMap, tt.subGIDMap)
			if !reflect.DeepEqual(gotUIDMap, tt.wantUIDMap) {
				t.Errorf("Launcher.getReverseUserMaps() gotUidMap = %v, want %v", gotUIDMap, tt.wantUIDMap)
			}
			if !reflect.DeepEqual(gotGIDMap, tt.wantGIDMap) {
				t.Errorf("Launcher.getReverseUserMaps() gotGidMap = %v, want %v", gotGIDMap, tt.wantGIDMap)
			}
		})
	}
}
