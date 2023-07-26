// Copyright (c) 2019-2023, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

//go:build !singularity_engine

package bin

import (
	"fmt"
	"os/exec"
)

// findOnPath falls back to exec.LookPath when not built as part of Singularity.
func findOnPath(name string) (path string, err error) {
	return exec.LookPath(name)
}

// findFromConfigOrPath falls back to exec.LookPath when not built as part of Singularity.
func findFromConfigOrPath(name string) (path string, err error) {
	return exec.LookPath(path)
}

// findFromConfigOnly returns an error when not built as part of Singularity.
func findFromConfigOnly(name string) (path string, err error) {
	return "", fmt.Errorf("findFromConfigOnly is not implemented")
}

// findConmon falls back to exec.LookPath when not built as part of Singularity.
func findConmon(name string) (path string, err error) {
	return findOnPath(name)
}

// findSquashfuse looks for squashfuse_ll / squashfuse on PATH.
func findSquashfuse(name string) (path string, err error) {
	// squashfuse_ll if found on PATH
	llPath, err := findOnPath("squashfuse_ll")
	if err == nil {
		return llPath, nil
	}
	// squashfuse if found on PATH
	return findOnPath(name)
}
