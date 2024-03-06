// Copyright (c) 2022-2023, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package oci

import (
	"reflect"
	"testing"

	imgspecv1 "github.com/opencontainers/image-spec/specs-go/v1"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/opencontainers/runtime-spec/specs-go"
	"github.com/samber/lo"
	"github.com/sylabs/singularity/v4/internal/pkg/runtime/engine/config/oci"
	"github.com/sylabs/singularity/v4/internal/pkg/runtime/launcher"
	"github.com/sylabs/singularity/v4/internal/pkg/util/env"
	"github.com/sylabs/singularity/v4/pkg/util/capabilities"
	"gotest.tools/v3/assert"
)

func TestGetProcessArgs(t *testing.T) {
	tests := []struct {
		name              string
		nativeSIF         bool
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
			ep := launcher.ExecParams{
				Process: tt.bundleProcess,
				Args:    tt.bundleArgs,
			}
			args := getProcessArgs(i, ep)
			if !reflect.DeepEqual(args, tt.expectProcessArgs) {
				t.Errorf("Expected: %v, Got: %v", tt.expectProcessArgs, args)
			}
		})
	}
}

func TestGetProcessEnvOCI(t *testing.T) {
	defaultPathEnv := "PATH=" + env.DefaultPath

	tests := []struct {
		name     string
		noCompat bool
		imageEnv []string
		hostEnv  []string
		userEnv  map[string]string
		wantEnv  []string
	}{
		{
			name:     "Default",
			noCompat: false,
			imageEnv: []string{},
			hostEnv:  []string{},
			userEnv:  map[string]string{},
			wantEnv: []string{
				defaultPathEnv,
				"LD_LIBRARY_PATH=/.singularity.d/libs",
			},
		},
		{
			name:     "PassTERM",
			noCompat: false,
			imageEnv: []string{},
			hostEnv:  []string{"TERM=xterm-256color"},
			userEnv:  map[string]string{},
			wantEnv: []string{
				"TERM=xterm-256color",
				defaultPathEnv,
				"LD_LIBRARY_PATH=/.singularity.d/libs",
			},
		},
		{
			name:     "PassHTTP_PROXY",
			noCompat: false,
			imageEnv: []string{},
			hostEnv:  []string{"HTTP_PROXY=proxy.example.com:3128"},
			userEnv:  map[string]string{},
			wantEnv: []string{
				"HTTP_PROXY=proxy.example.com:3128",
				defaultPathEnv,
				"LD_LIBRARY_PATH=/.singularity.d/libs",
			},
		},
		{
			name:     "BlockHostVar",
			noCompat: false,
			imageEnv: []string{},
			hostEnv:  []string{"NOT_FOR_CONTAINER=true"},
			userEnv:  map[string]string{},
			wantEnv: []string{
				defaultPathEnv,
				"LD_LIBRARY_PATH=/.singularity.d/libs",
			},
		},
		{
			name:     "ImagePath",
			noCompat: false,
			imageEnv: []string{"PATH=/foo"},
			hostEnv:  []string{},
			userEnv:  map[string]string{},
			wantEnv: []string{
				"PATH=/foo",
				"LD_LIBRARY_PATH=/.singularity.d/libs",
			},
		},
		{
			name:     "OverridePath",
			noCompat: false,
			imageEnv: []string{"PATH=/foo"},
			hostEnv:  []string{},
			userEnv:  map[string]string{"PATH": "/bar"},
			wantEnv: []string{
				"PATH=/bar",
				"LD_LIBRARY_PATH=/.singularity.d/libs",
			},
		},
		{
			name:     "AppendPath",
			noCompat: false,
			imageEnv: []string{"PATH=/foo"},
			hostEnv:  []string{},
			userEnv:  map[string]string{"APPEND_PATH": "/bar"},
			wantEnv: []string{
				"PATH=/foo:/bar",
				"LD_LIBRARY_PATH=/.singularity.d/libs",
			},
		},
		{
			name:     "PrependPath",
			noCompat: false,
			imageEnv: []string{"PATH=/foo"},
			hostEnv:  []string{},
			userEnv:  map[string]string{"PREPEND_PATH": "/bar"},
			wantEnv: []string{
				"PATH=/bar:/foo",
				"LD_LIBRARY_PATH=/.singularity.d/libs",
			},
		},
		{
			name:     "ImageLdLibraryPath",
			noCompat: false,
			imageEnv: []string{"LD_LIBRARY_PATH=/foo"},
			hostEnv:  []string{},
			userEnv:  map[string]string{},
			wantEnv: []string{
				"LD_LIBRARY_PATH=/foo:/.singularity.d/libs",
				defaultPathEnv,
			},
		},
		{
			name:     "BundleLdLibraryPath",
			noCompat: false,
			imageEnv: []string{},
			hostEnv:  []string{},
			userEnv:  map[string]string{"LD_LIBRARY_PATH": "/foo"},
			wantEnv: []string{
				defaultPathEnv,
				"LD_LIBRARY_PATH=/foo:/.singularity.d/libs",
			},
		},
		{
			name:     "OverrideLdLibraryPath",
			noCompat: false,
			imageEnv: []string{"LD_LIBRARY_PATH=/foo"},
			hostEnv:  []string{},
			userEnv:  map[string]string{"LD_LIBRARY_PATH": "/bar"},
			wantEnv: []string{
				"LD_LIBRARY_PATH=/bar:/.singularity.d/libs",
				defaultPathEnv,
			},
		},
		{
			name:     "ImageVar",
			noCompat: false,
			imageEnv: []string{"FOO=bar"},
			hostEnv:  []string{},
			userEnv:  map[string]string{},
			wantEnv: []string{
				"FOO=bar",
				defaultPathEnv,
				"LD_LIBRARY_PATH=/.singularity.d/libs",
			},
		},
		{
			name:     "ImageOverride",
			noCompat: false,
			imageEnv: []string{"FOO=bar"},
			hostEnv:  []string{},
			userEnv:  map[string]string{"FOO": "baz"},
			wantEnv: []string{
				"FOO=baz",
				defaultPathEnv,
				"LD_LIBRARY_PATH=/.singularity.d/libs",
			},
		},
		{
			name:     "ImageAdditional",
			noCompat: false,
			imageEnv: []string{"FOO=bar"},
			hostEnv:  []string{},
			userEnv:  map[string]string{"ABC": "123"},
			wantEnv: []string{
				"FOO=bar",
				"ABC=123",
				defaultPathEnv,
				"LD_LIBRARY_PATH=/.singularity.d/libs",
			},
		},
		{
			name:     "NoCompatHost",
			noCompat: true,
			imageEnv: []string{},
			hostEnv:  []string{"FOO=bar"},
			userEnv:  map[string]string{},
			wantEnv: []string{
				"FOO=bar",
				defaultPathEnv,
				"LD_LIBRARY_PATH=/.singularity.d/libs",
			},
		},
		{
			name:     "NoCompatImageOverrideHost",
			noCompat: true,
			imageEnv: []string{"FOO=baz"},
			hostEnv:  []string{"FOO=bar"},
			userEnv:  map[string]string{},
			wantEnv: []string{
				"FOO=baz",
				defaultPathEnv,
				"LD_LIBRARY_PATH=/.singularity.d/libs",
			},
		},
		{
			name:     "NoCompatUsetOverrideHost",
			noCompat: true,
			imageEnv: []string{},
			hostEnv:  []string{"FOO=bar"},
			userEnv:  map[string]string{"FOO": "baz"},
			wantEnv: []string{
				"FOO=baz",
				defaultPathEnv,
				"LD_LIBRARY_PATH=/.singularity.d/libs",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := &Launcher{
				cfg: launcher.Options{
					NoCompat: tt.noCompat,
				},
			}

			imgSpec := imgspecv1.Image{
				Config: imgspecv1.ImageConfig{Env: tt.imageEnv},
			}

			env := l.getProcessEnv(imgSpec, tt.hostEnv, tt.userEnv)
			assert.DeepEqual(t, tt.wantEnv, env)
		})
	}
}

func TestGetProcessEnvNative(t *testing.T) {
	defaultPathEnv := "PATH=" + env.DefaultPath

	tests := []struct {
		name    string
		hostEnv []string
		userEnv map[string]string
		wantEnv []string
	}{
		{
			name:    "Default",
			hostEnv: []string{},
			userEnv: map[string]string{},
			wantEnv: []string{defaultPathEnv},
		},
		{
			name:    "BlockHostVar",
			hostEnv: []string{"NOT_FOR_CONTAINER=true"},
			userEnv: map[string]string{},
			wantEnv: []string{
				defaultPathEnv,
			},
		},
		{
			name:    "OverridePath",
			hostEnv: []string{},
			userEnv: map[string]string{"PATH": "/bar"},
			wantEnv: []string{
				defaultPathEnv,
				"SING_USER_DEFINED_PATH=/bar",
			},
		},
		{
			name:    "AppendPath",
			hostEnv: []string{},
			userEnv: map[string]string{"APPEND_PATH": "/bar"},
			wantEnv: []string{
				defaultPathEnv,
				"SING_USER_DEFINED_APPEND_PATH=/bar",
			},
		},
		{
			name:    "PrependPath",
			hostEnv: []string{},
			userEnv: map[string]string{"PREPEND_PATH": "/bar"},
			wantEnv: []string{
				defaultPathEnv,
				"SING_USER_DEFINED_PREPEND_PATH=/bar",
			},
		},
		{
			name:    "OverrideLDLibraryPath",
			hostEnv: []string{},
			userEnv: map[string]string{"LD_LIBRARY_PATH": "/foo"},
			wantEnv: []string{
				defaultPathEnv,
				"LD_LIBRARY_PATH=/foo",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := &Launcher{
				nativeSIF: true,
			}
			imgSpec := imgspecv1.Image{
				Config: v1.ImageConfig{
					Env: []string{"PATH=" + env.DefaultPath},
				},
			}
			env := l.getProcessEnv(imgSpec, tt.hostEnv, tt.userEnv)
			assert.DeepEqual(t, tt.wantEnv, env)
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

func TestLauncher_getBaseCapabilities(t *testing.T) {
	currCaps, err := capabilities.GetProcessEffective()
	if err != nil {
		t.Fatal(err)
	}
	currCapStrings := capabilities.ToStrings(currCaps)

	tests := []struct {
		name      string
		keepPrivs bool
		noPrivs   bool
		want      []string
		wantErr   bool
	}{
		{
			name:      "Default",
			keepPrivs: false,
			noPrivs:   false,
			want:      oci.DefaultCaps,
			wantErr:   false,
		},
		{
			name:      "NoPrivs",
			keepPrivs: false,
			noPrivs:   true,
			want:      []string{},
			wantErr:   false,
		},
		{
			name:      "NoPrivsPrecendence",
			keepPrivs: true,
			noPrivs:   true,
			want:      []string{},
			wantErr:   false,
		},
		{
			name:      "KeepPrivs",
			keepPrivs: true,
			noPrivs:   false,
			want:      currCapStrings,
			wantErr:   false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := &Launcher{
				cfg: launcher.Options{
					KeepPrivs: tt.keepPrivs,
					NoPrivs:   tt.noPrivs,
				},
			}
			got, err := l.getBaseCapabilities()
			if (err != nil) != tt.wantErr {
				t.Errorf("Launcher.getBaseCapabilities() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Launcher.getBaseCapabilities() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestLauncher_getProcessCapabilities(t *testing.T) {
	tests := []struct {
		name     string
		addCaps  string
		dropCaps string
		uid      uint32
		want     *specs.LinuxCapabilities
		wantErr  bool
	}{
		{
			name:     "DefaultRoot",
			addCaps:  "",
			dropCaps: "",
			uid:      0,
			want: &specs.LinuxCapabilities{
				Permitted:   oci.DefaultCaps,
				Effective:   oci.DefaultCaps,
				Bounding:    oci.DefaultCaps,
				Inheritable: []string{},
				Ambient:     []string{},
			},
			wantErr: false,
		},
		{
			name:     "DefaultUser",
			addCaps:  "",
			dropCaps: "",
			uid:      1000,
			want: &specs.LinuxCapabilities{
				Permitted:   []string{},
				Effective:   []string{},
				Bounding:    oci.DefaultCaps,
				Inheritable: []string{},
				Ambient:     []string{},
			},
			wantErr: false,
		},
		{
			name:     "AddRoot",
			addCaps:  "CAP_SYSLOG,CAP_WAKE_ALARM",
			dropCaps: "",
			uid:      0,
			want: &specs.LinuxCapabilities{
				Permitted:   append(oci.DefaultCaps, "CAP_SYSLOG", "CAP_WAKE_ALARM"),
				Effective:   append(oci.DefaultCaps, "CAP_SYSLOG", "CAP_WAKE_ALARM"),
				Bounding:    append(oci.DefaultCaps, "CAP_SYSLOG", "CAP_WAKE_ALARM"),
				Inheritable: []string{},
				Ambient:     []string{},
			},
			wantErr: false,
		},
		{
			name:     "DropRoot",
			addCaps:  "",
			dropCaps: "CAP_SETUID,CAP_SETGID",
			uid:      0,
			want: &specs.LinuxCapabilities{
				Permitted:   lo.Without(oci.DefaultCaps, "CAP_SETUID", "CAP_SETGID"),
				Effective:   lo.Without(oci.DefaultCaps, "CAP_SETUID", "CAP_SETGID"),
				Bounding:    lo.Without(oci.DefaultCaps, "CAP_SETUID", "CAP_SETGID"),
				Inheritable: []string{},
				Ambient:     []string{},
			},
			wantErr: false,
		},
		{
			name:     "AddUser",
			addCaps:  "CAP_SYSLOG,CAP_WAKE_ALARM",
			dropCaps: "",
			uid:      1000,
			want: &specs.LinuxCapabilities{
				Permitted:   []string{"CAP_SYSLOG", "CAP_WAKE_ALARM"},
				Effective:   []string{"CAP_SYSLOG", "CAP_WAKE_ALARM"},
				Bounding:    append(oci.DefaultCaps, "CAP_SYSLOG", "CAP_WAKE_ALARM"),
				Inheritable: []string{"CAP_SYSLOG", "CAP_WAKE_ALARM"},
				Ambient:     []string{"CAP_SYSLOG", "CAP_WAKE_ALARM"},
			},
			wantErr: false,
		},
		{
			name:     "DropUser",
			addCaps:  "",
			dropCaps: "CAP_SETUID,CAP_SETGID",
			uid:      1000,
			want: &specs.LinuxCapabilities{
				Permitted:   []string{},
				Effective:   []string{},
				Bounding:    lo.Without(oci.DefaultCaps, "CAP_SETUID", "CAP_SETGID"),
				Inheritable: []string{},
				Ambient:     []string{},
			},
			wantErr: false,
		},
		{
			name:     "AddDropRoot",
			addCaps:  "CAP_SYSLOG,CAP_WAKE_ALARM",
			dropCaps: "CAP_SETUID,CAP_SETGID",
			uid:      0,
			want: &specs.LinuxCapabilities{
				Permitted:   lo.Without(append(oci.DefaultCaps, "CAP_SYSLOG", "CAP_WAKE_ALARM"), "CAP_SETUID", "CAP_SETGID"),
				Effective:   lo.Without(append(oci.DefaultCaps, "CAP_SYSLOG", "CAP_WAKE_ALARM"), "CAP_SETUID", "CAP_SETGID"),
				Bounding:    lo.Without(append(oci.DefaultCaps, "CAP_SYSLOG", "CAP_WAKE_ALARM"), "CAP_SETUID", "CAP_SETGID"),
				Inheritable: []string{},
				Ambient:     []string{},
			},
			wantErr: false,
		},
		{
			name:     "AddDropUser",
			addCaps:  "CAP_SYSLOG,CAP_WAKE_ALARM",
			dropCaps: "CAP_SETUID,CAP_SETGID",
			uid:      1000,
			want: &specs.LinuxCapabilities{
				Permitted:   []string{"CAP_SYSLOG", "CAP_WAKE_ALARM"},
				Effective:   []string{"CAP_SYSLOG", "CAP_WAKE_ALARM"},
				Bounding:    lo.Without(append(oci.DefaultCaps, "CAP_SYSLOG", "CAP_WAKE_ALARM"), "CAP_SETUID", "CAP_SETGID"),
				Inheritable: []string{"CAP_SYSLOG", "CAP_WAKE_ALARM"},
				Ambient:     []string{"CAP_SYSLOG", "CAP_WAKE_ALARM"},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := &Launcher{
				cfg: launcher.Options{
					AddCaps:  tt.addCaps,
					DropCaps: tt.dropCaps,
				},
			}
			got, err := l.getProcessCapabilities(tt.uid)
			if (err != nil) != tt.wantErr {
				t.Errorf("Launcher.getProcessCapabilities() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Launcher.getProcessCapabilities() = %v, want %v", got, tt.want)
			}
		})
	}
}
