// Copyright (c) 2024, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package hooks

import (
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"testing"

	cchooks "github.com/containers/common/pkg/hooks/1.0.0"
	"github.com/opencontainers/runtime-spec/specs-go"
	"github.com/sylabs/singularity/v4/internal/pkg/test"
	"github.com/sylabs/singularity/v4/internal/pkg/util/fs"
)

func TestNativeHook_Validate(t *testing.T) {
	trueBool := true

	tests := []struct {
		name    string
		nh      *NativeHook
		wantErr bool
	}{
		{
			name: "Valid",
			nh: &NativeHook{
				Version: "1.0.0",
				Hook:    specs.Hook{Path: "/bin/true"},
				When:    cchooks.When{Always: &trueBool},
				Stages:  []string{"poststart"},
			},
			wantErr: false,
		},
		{
			name: "BadVersion",
			nh: &NativeHook{
				Version: "99.0.0",
				Hook:    specs.Hook{Path: "/bin/true"},
				When:    cchooks.When{Always: &trueBool},
				Stages:  []string{"poststart"},
			},
			wantErr: true,
		},
		{
			name: "NoHookPath",
			nh: &NativeHook{
				Version: "1.0.0",
				Hook:    specs.Hook{},
				When:    cchooks.When{Always: &trueBool},
				Stages:  []string{"poststart"},
			},
			wantErr: true,
		},
		{
			name: "WhenNotAlways",
			nh: &NativeHook{
				Version: "1.0.0",
				Hook:    specs.Hook{Path: "/bin/true"},
				When:    cchooks.When{},
				Stages:  []string{"poststart"},
			},
			wantErr: true,
		},
		{
			name: "WhenAnnotations",
			nh: &NativeHook{
				Version: "1.0.0",
				Hook:    specs.Hook{Path: "/bin/true"},
				When:    cchooks.When{Always: &trueBool, Annotations: map[string]string{"key": "val"}},
				Stages:  []string{"poststart"},
			},
			wantErr: true,
		},
		{
			name: "WhenCommands",
			nh: &NativeHook{
				Version: "1.0.0",
				Hook:    specs.Hook{Path: "/bin/true"},
				When:    cchooks.When{Always: &trueBool, Commands: []string{"cmd"}},
				Stages:  []string{"poststart"},
			},
			wantErr: true,
		},
		{
			name: "WhenHasBindMounts",
			nh: &NativeHook{
				Version: "1.0.0",
				Hook:    specs.Hook{Path: "/bin/true"},
				When:    cchooks.When{Always: &trueBool, HasBindMounts: &trueBool},
				Stages:  []string{"poststart"},
			},
			wantErr: true,
		},
		{
			name: "NoStages",
			nh: &NativeHook{
				Version: "1.0.0",
				Hook:    specs.Hook{Path: "/bin/true"},
				When:    cchooks.When{Always: &trueBool},
			},
			wantErr: true,
		},
		{
			name: "BadStages",
			nh: &NativeHook{
				Version: "1.0.0",
				Hook:    specs.Hook{Path: "/bin/true"},
				When:    cchooks.When{Always: &trueBool},
				Stages:  []string{"prestart"},
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.nh.Validate(); (err != nil) != tt.wantErr {
				t.Errorf("NativeHook.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestNativeHook_run(t *testing.T) {
	unprivUID := -1
	test.WithoutPrivilege(func(_ *testing.T) {
		unprivUID = os.Getuid()
	})(t)

	state := specs.State{
		Version: "1.1.0",
		ID:      "container",
		Status:  "running",
		Pid:     123,
		Bundle:  "/my/container.sif",
	}
	stateJSON := `{"ociVersion":"1.1.0","id":"container","status":"running","pid":123,"bundle":"/my/container.sif"}`

	tests := []struct {
		name            string
		nativeHook      *NativeHook
		allowPrivileged bool
		withPrivilege   bool
		wantStdout      string
		wantErr         bool
	}{
		{
			name:            "nil",
			nativeHook:      nil,
			allowPrivileged: false,
			withPrivilege:   false,
			wantStdout:      "",
			wantErr:         true,
		},
		{
			name:            "noPath",
			nativeHook:      &NativeHook{Name: "test"},
			allowPrivileged: false,
			withPrivilege:   false,
			wantStdout:      "",
			wantErr:         true,
		},
		{
			name:            "true",
			nativeHook:      &NativeHook{Name: "test", Hook: specs.Hook{Path: "/bin/true"}},
			allowPrivileged: false,
			withPrivilege:   false,
			wantStdout:      "",
			wantErr:         false,
		},
		{
			name:            "false",
			nativeHook:      &NativeHook{Name: "test", Hook: specs.Hook{Path: "/bin/false"}},
			allowPrivileged: false,
			withPrivilege:   false,
			wantStdout:      "",
			wantErr:         true,
		},
		{
			name: "hookArgs",
			nativeHook: &NativeHook{
				Name: "test",
				Hook: specs.Hook{
					Path: "/bin/echo",
					Args: []string{"A", "B", "C"},
				},
			},
			allowPrivileged: false,
			withPrivilege:   false,
			wantStdout:      "A B C\n",
			wantErr:         false,
		},
		{
			name: "hookEnv",
			nativeHook: &NativeHook{
				Name: "test",
				Hook: specs.Hook{
					Path: "/bin/sh",
					Args: []string{"-c", "echo -n $FOO"},
					Env:  []string{"FOO=BAR"},
				},
			},
			allowPrivileged: false,
			withPrivilege:   false,
			wantStdout:      "BAR",
			wantErr:         false,
		},
		{
			name:            "stateStdIn",
			nativeHook:      &NativeHook{Name: "test", Hook: specs.Hook{Path: "/bin/cat"}},
			allowPrivileged: false,
			withPrivilege:   false,
			wantStdout:      stateJSON,
			wantErr:         false,
		},
		{
			name: "pidEnv",
			nativeHook: &NativeHook{
				Name: "test",
				Hook: specs.Hook{
					Path: "/bin/sh",
					Args: []string{"-c", "echo -n $SINGULARITY_CONTAINER_PID"},
				},
			},
			allowPrivileged: false,
			withPrivilege:   false,
			wantStdout:      strconv.Itoa(state.Pid),
			wantErr:         false,
		},
		{
			name: "containerEnv",
			nativeHook: &NativeHook{
				Name: "test",
				Hook: specs.Hook{
					Path: "/bin/sh",
					Args: []string{"-c", "echo -n $SINGULARITY_CONTAINER"},
				},
			},
			allowPrivileged: false,
			withPrivilege:   false,
			wantStdout:      state.Bundle,
			wantErr:         false,
		},
		{
			name: "privDisallowedUser",
			nativeHook: &NativeHook{
				Name:       "test",
				Hook:       specs.Hook{Path: "/bin/true"},
				Privileged: true,
			},
			allowPrivileged: false,
			withPrivilege:   false,
			wantStdout:      "",
			wantErr:         true,
		},
		{
			name: "privDisallowedEscalated",
			nativeHook: &NativeHook{
				Name:       "test",
				Hook:       specs.Hook{Path: "/bin/true"},
				Privileged: true,
			},
			allowPrivileged: false,
			withPrivilege:   true,
			wantStdout:      "",
			wantErr:         true,
		},
		{
			name: "privAllowed",
			nativeHook: &NativeHook{
				Name:       "test",
				Hook:       specs.Hook{Path: "/bin/true"},
				Privileged: true,
			},
			allowPrivileged: true,
			withPrivilege:   true,
			wantStdout:      "",
			wantErr:         false,
		},
		{
			name: "unprivUID",
			nativeHook: &NativeHook{
				Name:       "test",
				Hook:       specs.Hook{Path: "/bin/id", Args: []string{"-u"}},
				Privileged: false,
			},
			allowPrivileged: false,
			wantStdout:      strconv.Itoa(unprivUID) + "\n",
			wantErr:         false,
		},
		{
			name: "privUID",
			nativeHook: &NativeHook{
				Name:       "test",
				Hook:       specs.Hook{Path: "/bin/id", Args: []string{"-u"}},
				Privileged: true,
			},
			allowPrivileged: true,
			withPrivilege:   true,
			wantStdout:      "0\n",
			wantErr:         false,
		},
		{
			name: "privEnv",
			nativeHook: &NativeHook{
				Name: "test",
				Hook: specs.Hook{
					Path: "/bin/sh",
					Args: []string{"-c", "echo -n $SINGULARITY_HOOK_PRIVILEGED"},
				},
				Privileged: true,
			},
			allowPrivileged: true,
			withPrivilege:   true,
			wantStdout:      "1",
			wantErr:         false,
		},
	}
	for _, tt := range tests {
		subtest := func(t *testing.T) {
			got, err := tt.nativeHook.run(state, tt.allowPrivileged)
			if (err != nil) != tt.wantErr {
				t.Errorf("NativeHook.run() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if string(got) != tt.wantStdout {
				t.Errorf("NativeHook.run() = %v, want %v", string(got), tt.wantStdout)
			}
		}

		if tt.withPrivilege {
			t.Run(tt.name, test.WithPrivilege(subtest))
		} else {
			t.Run(tt.name, test.WithoutPrivilege(subtest))
		}
	}
}

func Test_loadNativeHooksUnpriv(t *testing.T) {
	test.DropPrivilege(t)
	defer test.ResetPrivilege(t)
	trueBool := true
	validHook := NativeHook{
		Name:    "valid.json",
		Version: "1.0.0",
		Hook: specs.Hook{
			Path: "/bin/true",
			Args: []string{"arg1", "arg2"},
			Env:  []string{"I_AM_A_HOOK=1"},
		},
		When: cchooks.When{
			Always: &trueBool,
		},
		Stages:     []string{"poststart"},
		Privileged: false,
	}

	// Symlink to the hooks dir.
	symlinkOuterDir := t.TempDir()
	targetDir := t.TempDir()
	symlinkDir := filepath.Join(symlinkOuterDir, "symlinkDir")
	if err := os.Symlink(targetDir, symlinkDir); err != nil {
		t.Fatal(err)
	}

	// Symlink for a hook file outside escapeDir.

	escapeDir := t.TempDir()
	outsideDir := t.TempDir()
	outsideHook := filepath.Join(outsideDir, "outside.json")
	if err := fs.CopyFile("testdata/valid/valid.json", outsideHook, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outsideHook, filepath.Join(escapeDir, "escape.json")); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name    string
		hookDir string
		want    []NativeHook
		wantErr bool
	}{
		{
			name:    "invalid",
			hookDir: "testdata/invalid",
			want:    nil,
			wantErr: true,
		},
		{
			name:    "valid",
			hookDir: "testdata/valid",
			want:    []NativeHook{validHook},
			wantErr: false,
		},
		{
			name:    "onlyUnprivileged",
			hookDir: "testdata/privilege",
			want:    []NativeHook{validHook},
			wantErr: false,
		},
		// We refuse to open the hooks directory if it is a symlink.
		{
			name:    "noSymlinkDir",
			hookDir: symlinkDir,
			want:    nil,
			wantErr: true,
		},
		// We always skip symlinks when reading hooks.
		{
			name:    "noSymlinkFile",
			hookDir: escapeDir,
			want:    []NativeHook{},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := loadNativeHooks(tt.hookDir, false)
			if (err != nil) != tt.wantErr {
				t.Errorf("loadNativeHooks() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("loadNativeHooks() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_loadNativeHooksPriv(t *testing.T) {
	test.EnsurePrivilege(t)
	trueBool := true
	privilegedHook := NativeHook{
		Name:    "privileged.json",
		Version: "1.0.0",
		Hook: specs.Hook{
			Path: "/bin/true",
			Args: []string{"arg1", "arg2"},
			Env:  []string{"I_AM_A_HOOK=1"},
		},
		When: cchooks.When{
			Always: &trueBool,
		},
		Stages:     []string{"poststart"},
		Privileged: true,
	}

	// Create a root owned structure in a tmpdir
	rootDir := t.TempDir()
	if err := fs.CopyFile("testdata/privilege/privileged.json", filepath.Join(rootDir, "privileged.json"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Symlink to the hooks dir.
	symlinkOuterDir := t.TempDir()
	symlinkDir := filepath.Join(symlinkOuterDir, "symlinkDir")
	if err := os.Symlink(rootDir, symlinkDir); err != nil {
		t.Fatal(err)
	}

	// Symlink for a hook file outside escapeDir.
	escapeDir := t.TempDir()
	outsideDir := t.TempDir()
	outsideHook := filepath.Join(outsideDir, "outside.json")
	if err := fs.CopyFile("testdata/privilege/privileged.json", outsideHook, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outsideHook, filepath.Join(escapeDir, "escape.json")); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name    string
		hookDir string
		want    []NativeHook
		wantErr bool
	}{
		{
			name:    "badOwnership",
			hookDir: "testdata/privilege",
			want:    nil,
			wantErr: true,
		},
		{
			name:    "rootOwnership",
			hookDir: rootDir,
			want:    []NativeHook{privilegedHook},
			wantErr: false,
		},
		// We refuse to open the hooks directory if it is a symlink.
		{
			name:    "noSymlinkDir",
			hookDir: symlinkDir,
			want:    nil,
			wantErr: true,
		},
		// We always skip symlinks when reading hooks.
		{
			name:    "noSymlinkFile",
			hookDir: escapeDir,
			want:    []NativeHook{},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := loadNativeHooks(tt.hookDir, true)
			if (err != nil) != tt.wantErr {
				t.Errorf("loadNativeHooks() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("loadNativeHooks() = %v, want %v", got, tt.want)
			}
		})
	}
}
