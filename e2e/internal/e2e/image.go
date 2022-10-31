// Copyright (c) 2019-2022, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package e2e

import (
	"os"
	"runtime"
	"sync"
	"testing"
)

var (
	ensureMutex sync.Mutex
	pullMutex   sync.Mutex
)

// EnsureImage checks if e2e test image is already built or built
// it otherwise.
func EnsureImage(t *testing.T, env TestEnv) {
	ensureMutex.Lock()
	defer ensureMutex.Unlock()

	switch _, err := os.Stat(env.ImagePath); {
	case err == nil:
		// OK: file exists, return
		return

	case os.IsNotExist(err):
		// OK: file does not exist, continue

	default:
		// FATAL: something else is wrong
		t.Fatalf("Failed when checking image %q: %+v\n",
			env.ImagePath,
			err)
	}

	env.RunSingularity(
		t,
		WithProfile(RootProfile),
		WithCommand("build"),
		WithArgs("--force", env.ImagePath, "testdata/Singularity"),
		ExpectExit(0),
	)
}

// PullImage will pull a test image.
func PullImage(t *testing.T, env TestEnv, imageURL string, arch string, path string) {
	pullMutex.Lock()
	defer pullMutex.Unlock()

	if arch == "" {
		arch = runtime.GOARCH
	}

	switch _, err := os.Stat(path); {
	case err == nil:
		// OK: file exists, return
		return

	case os.IsNotExist(err):
		// OK: file does not exist, continue

	default:
		// FATAL: something else is wrong
		t.Fatalf("Failed when checking image %q: %+v\n", path, err)
	}

	env.RunSingularity(
		t,
		WithProfile(UserProfile),
		WithCommand("pull"),
		WithArgs("--force", "--allow-unsigned", "--arch", arch, path, imageURL),
		ExpectExit(0),
	)
}

// BusyboxImage will provide the path to a local busybox SIF image for the current architecture
func BusyboxSIF(t *testing.T) string {
	busyboxSIF := "testdata/busybox_" + runtime.GOARCH + ".sif"
	_, err := os.Stat(busyboxSIF)
	if os.IsNotExist(err) {
		t.Fatalf("busybox image not found for %s", runtime.GOARCH)
	}
	if err != nil {
		t.Error(err)
	}
	return busyboxSIF
}
