// Copyright (c) 2019-2025, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

//go:build singularity_engine

package bin

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/sylabs/singularity/v4/internal/pkg/buildcfg"
	"github.com/sylabs/singularity/v4/internal/pkg/util/env"
	"github.com/sylabs/singularity/v4/pkg/util/singularityconf"
)

func TestFindOnPath(t *testing.T) {
	// findOnPath should give same as exec.LookPath, but additionally work
	// in the case where $PATH doesn't include default sensible directories
	// as these are added to $PATH before the lookup.

	// Find the true path of 'cp' under a sensible PATH=env.DefaultPath
	// Forcing this avoid issues with PATH across sudo calls for the tests,
	// differing orders, /usr/bin -> /bin symlinks etc.
	t.Setenv("PATH", env.DefaultPath)
	truePath, err := exec.LookPath("cp")
	if err != nil {
		t.Fatalf("exec.LookPath failed to find cp: %v", err)
	}

	t.Run("sensible path", func(t *testing.T) {
		gotPath, err := findOnPath("cp")
		if err != nil {
			t.Errorf("unexpected error from findOnPath: %v", err)
		}
		if gotPath != truePath {
			t.Errorf("Got %q, expected %q", gotPath, truePath)
		}
	})

	t.Run("bad path", func(t *testing.T) {
		// Force a PATH that doesn't contain cp
		t.Setenv("PATH", "/invalid/dir:/another/invalid/dir")

		gotPath, err := findOnPath("cp")
		if err != nil {
			t.Errorf("unexpected error from findOnPath: %v", err)
		}
		if gotPath != truePath {
			t.Errorf("Got %q, expected %q", gotPath, truePath)
		}
	})
}

func TestFindFromConfigOrPath(t *testing.T) {
	//nolint:dupl
	cases := []struct {
		name          string
		bin           string
		buildcfg      string
		expectSuccess bool
		configKey     string
		configVal     string
		expectPath    string
	}{
		{
			name:          "go valid",
			bin:           "go",
			buildcfg:      buildcfg.GO_PATH,
			configKey:     "go path",
			configVal:     buildcfg.GO_PATH,
			expectPath:    buildcfg.GO_PATH,
			expectSuccess: true,
		},
		{
			name:          "go invalid",
			bin:           "go",
			buildcfg:      buildcfg.GO_PATH,
			configKey:     "go path",
			configVal:     "/invalid/dir/go",
			expectSuccess: false,
		},
		{
			name:          "go empty",
			bin:           "go",
			buildcfg:      buildcfg.GO_PATH,
			configKey:     "go path",
			configVal:     "",
			expectPath:    "_LOOKPATH_",
			expectSuccess: true,
		},
		{
			name:          "mksquashfs valid",
			bin:           "mksquashfs",
			buildcfg:      buildcfg.MKSQUASHFS_PATH,
			configKey:     "mksquashfs path",
			configVal:     buildcfg.MKSQUASHFS_PATH,
			expectPath:    buildcfg.MKSQUASHFS_PATH,
			expectSuccess: true,
		},
		{
			name:          "mksquashfs invalid",
			bin:           "mksquashfs",
			buildcfg:      buildcfg.MKSQUASHFS_PATH,
			configKey:     "mksquashfs path",
			configVal:     "/invalid/dir/go",
			expectSuccess: false,
		},
		{
			name:          "mksquashfs empty",
			bin:           "mksquashfs",
			buildcfg:      buildcfg.MKSQUASHFS_PATH,
			configKey:     "mksquashfs path",
			configVal:     "",
			expectPath:    "_LOOKPATH_",
			expectSuccess: true,
		},
		{
			name:          "unsquashfs valid",
			bin:           "unsquashfs",
			buildcfg:      buildcfg.UNSQUASHFS_PATH,
			configKey:     "unsquashfs path",
			configVal:     buildcfg.UNSQUASHFS_PATH,
			expectPath:    buildcfg.UNSQUASHFS_PATH,
			expectSuccess: true,
		},
		{
			name:          "unsquashfs invalid",
			bin:           "unsquashfs",
			buildcfg:      buildcfg.UNSQUASHFS_PATH,
			configKey:     "unsquashfs path",
			configVal:     "/invalid/dir/go",
			expectSuccess: false,
		},
		{
			name:          "unsquashfs empty",
			bin:           "unsquashfs",
			buildcfg:      buildcfg.UNSQUASHFS_PATH,
			configKey:     "unsquashfs path",
			configVal:     "",
			expectPath:    "_LOOKPATH_",
			expectSuccess: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.buildcfg == "" {
				t.Skip("skipping - no buildcfg path known")
			}
			lookPath, err := exec.LookPath(tc.bin)
			if err != nil {
				t.Skipf("Error from exec.LookPath for %q: %v", tc.bin, err)
			}

			if tc.expectPath == "_LOOKPATH_" {
				tc.expectPath = lookPath
			}

			testConf := filepath.Join(t.TempDir(), "test.conf")
			cfg := fmt.Sprintf("%s = %s\n", tc.configKey, tc.configVal)
			os.WriteFile(testConf, []byte(cfg), 0o644)

			conf, err := singularityconf.Parse(testConf)
			if err != nil {
				t.Errorf("Error parsing test singularityconf: %v", err)
			}
			singularityconf.SetCurrentConfig(conf)

			path, err := findFromConfigOrPath(tc.bin)

			if tc.expectSuccess && err == nil {
				// expect success, no error, check path
				if path != tc.expectPath {
					t.Errorf("Expecting %q, got %q", tc.expectPath, path)
				}
			}

			if tc.expectSuccess && err != nil {
				// expect success, got error
				t.Errorf("unexpected error: %v", err)
			}

			if !tc.expectSuccess && err == nil {
				// expect failure, got no error
				t.Errorf("expected error, got %q", path)
			}
		})
	}
}

func TestFindFromConfigOnly(t *testing.T) {
	//nolint:dupl
	cases := []struct {
		name          string
		bin           string
		buildcfg      string
		expectSuccess bool
		configKey     string
		configVal     string
		expectPath    string
	}{
		{
			name:          "nvidia-container-cli valid",
			bin:           "nvidia-container-cli",
			buildcfg:      buildcfg.NVIDIA_CONTAINER_CLI_PATH,
			configKey:     "nvidia-container-cli path",
			configVal:     buildcfg.NVIDIA_CONTAINER_CLI_PATH,
			expectPath:    buildcfg.NVIDIA_CONTAINER_CLI_PATH,
			expectSuccess: true,
		},
		{
			name:          "nvidia-container-cli invalid",
			bin:           "nvidia-container-cli",
			buildcfg:      buildcfg.NVIDIA_CONTAINER_CLI_PATH,
			configKey:     "nvidia-container-cli path",
			configVal:     "/invalid/dir/go",
			expectSuccess: false,
		},
		{
			name:          "nvidia-container-cli empty",
			bin:           "nvidia-container-cli",
			buildcfg:      buildcfg.NVIDIA_CONTAINER_CLI_PATH,
			configKey:     "nvidia-container-cli path",
			configVal:     "",
			expectPath:    "",
			expectSuccess: false,
		},
		{
			name:          "cryptsetup valid",
			bin:           "cryptsetup",
			buildcfg:      buildcfg.CRYPTSETUP_PATH,
			configKey:     "cryptsetup path",
			configVal:     buildcfg.CRYPTSETUP_PATH,
			expectPath:    buildcfg.CRYPTSETUP_PATH,
			expectSuccess: true,
		},
		{
			name:          "cryptsetup invalid",
			bin:           "cryptsetup",
			buildcfg:      buildcfg.CRYPTSETUP_PATH,
			configKey:     "cryptsetup path",
			configVal:     "/invalid/dir/cryptsetup",
			expectSuccess: false,
		},
		{
			name:          "cryptsetup empty",
			bin:           "cryptsetup",
			buildcfg:      buildcfg.CRYPTSETUP_PATH,
			configKey:     "cryptsetup path",
			configVal:     "",
			expectPath:    "",
			expectSuccess: false,
		},
		{
			name:          "ldconfig valid",
			bin:           "ldconfig",
			buildcfg:      buildcfg.LDCONFIG_PATH,
			configKey:     "ldconfig path",
			configVal:     buildcfg.LDCONFIG_PATH,
			expectPath:    buildcfg.LDCONFIG_PATH,
			expectSuccess: true,
		},
		{
			name:          "ldconfig invalid",
			bin:           "ldconfig",
			buildcfg:      buildcfg.LDCONFIG_PATH,
			configKey:     "ldconfig path",
			configVal:     "/invalid/dir/go",
			expectSuccess: false,
		},
		{
			name:          "ldconfig empty",
			bin:           "ldconfig",
			buildcfg:      buildcfg.LDCONFIG_PATH,
			configKey:     "ldconfig path",
			configVal:     "",
			expectPath:    "",
			expectSuccess: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.buildcfg == "" {
				t.Skip("skipping - no buildcfg path known")
			}

			testConf := filepath.Join(t.TempDir(), "test.conf")
			cfg := fmt.Sprintf("%s = %s\n", tc.configKey, tc.configVal)
			os.WriteFile(testConf, []byte(cfg), 0o644)

			conf, err := singularityconf.Parse(testConf)
			if err != nil {
				t.Errorf("Error parsing test singularityconf: %v", err)
			}
			singularityconf.SetCurrentConfig(conf)

			path, err := findFromConfigOnly(tc.bin)

			if tc.expectSuccess && err == nil {
				// expect success, no error, check path
				if path != tc.expectPath {
					t.Errorf("Expecting %q, got %q", tc.expectPath, path)
				}
			}

			if tc.expectSuccess && err != nil {
				// expect success, got error
				t.Errorf("unexpected error: %v", err)
			}

			if !tc.expectSuccess && err == nil {
				// expect failure, got no error
				t.Errorf("expected error, got %q", path)
			}
		})
	}
}
