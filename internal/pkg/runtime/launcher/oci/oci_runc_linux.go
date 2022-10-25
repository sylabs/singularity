// Copyright (c) 2018-2022, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

// Includes code from https://github.com/containers/podman
// Released under the Apache License Version 2.0

package oci

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/sylabs/singularity/internal/pkg/util/bin"
	"github.com/sylabs/singularity/pkg/sylog"
)

// Delete deletes container resources
func Delete(ctx context.Context, containerID string) error {
	runc, err := bin.FindBin("runc")
	if err != nil {
		return err
	}
	runcArgs := []string{
		"--root", runcStateDir,
		"delete",
		containerID,
	}

	cmd := exec.Command(runc, runcArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdout
	sylog.Debugf("Calling runc with args %v", runcArgs)
	err = cmd.Run()
	if err != nil {
		return fmt.Errorf("while calling runc delete: %w", err)
	}

	sd, err := stateDir(containerID)
	if err != nil {
		return fmt.Errorf("while computing state directory: %w", err)
	}

	bLink := filepath.Join(sd, bundleLink)
	bundle, err := filepath.EvalSymlinks(bLink)
	if err != nil {
		return fmt.Errorf("while finding bundle directory: %w", err)
	}

	sylog.Debugf("Removing bundle symlink")
	if err := os.Remove(bLink); err != nil {
		return fmt.Errorf("while removing bundle symlink: %w", err)
	}

	sylog.Debugf("Releasing bundle lock")
	return releaseBundle(bundle)
}

// Exec executes a command in a container
func Exec(containerID string, cmdArgs []string) error {
	runc, err := bin.FindBin("runc")
	if err != nil {
		return err
	}
	runcArgs := []string{
		"--root", runcStateDir,
		"exec",
		containerID,
	}
	runcArgs = append(runcArgs, cmdArgs...)
	cmd := exec.Command(runc, runcArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdout
	sylog.Debugf("Calling runc with args %v", runcArgs)
	return cmd.Run()
}

// Kill kills container process
func Kill(containerID string, killSignal string) error {
	runc, err := bin.FindBin("runc")
	if err != nil {
		return err
	}
	runcArgs := []string{
		"--root", runcStateDir,
		"kill",
		containerID,
		killSignal,
	}

	cmd := exec.Command(runc, runcArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdout
	sylog.Debugf("Calling runc with args %v", runcArgs)
	return cmd.Run()
}

// Pause pauses processes in a container
func Pause(containerID string) error {
	runc, err := bin.FindBin("runc")
	if err != nil {
		return err
	}
	runcArgs := []string{
		"--root", runcStateDir,
		"pause",
		containerID,
	}

	cmd := exec.Command(runc, runcArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdout
	sylog.Debugf("Calling runc with args %v", runcArgs)
	return cmd.Run()
}

// Resume pauses processes in a container
func Resume(containerID string) error {
	runc, err := bin.FindBin("runc")
	if err != nil {
		return err
	}
	runcArgs := []string{
		"--root", runcStateDir,
		"resume",
		containerID,
	}

	cmd := exec.Command(runc, runcArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdout
	sylog.Debugf("Calling runc with args %v", runcArgs)
	return cmd.Run()
}

// Run runs a container (equivalent to create/start/delete)
func Run(ctx context.Context, containerID, bundlePath, pidFile string) error {
	runc, err := bin.FindBin("runc")
	if err != nil {
		return err
	}
	absBundle, err := filepath.Abs(bundlePath)
	if err != nil {
		return fmt.Errorf("failed to determine bundle absolute path: %s", err)
	}

	if err := os.Chdir(absBundle); err != nil {
		return fmt.Errorf("failed to change directory to %s: %s", absBundle, err)
	}

	runcArgs := []string{
		"--root", runcStateDir,
		"run",
		"-b", absBundle,
	}
	if pidFile != "" {
		runcArgs = append(runcArgs, "--pid-file="+pidFile)
	}
	runcArgs = append(runcArgs, containerID)
	cmd := exec.Command(runc, runcArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdout
	sylog.Debugf("Calling runc with args %v", runcArgs)
	return cmd.Run()
}

// Start starts a previously created container
func Start(containerID string) error {
	runc, err := bin.FindBin("runc")
	if err != nil {
		return err
	}
	runcArgs := []string{
		"--root", runcStateDir,
		"start",
		containerID,
	}

	cmd := exec.Command(runc, runcArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdout
	sylog.Debugf("Calling runc with args %v", runcArgs)
	return cmd.Run()
}

// State queries container state
func State(containerID string) error {
	runc, err := bin.FindBin("runc")
	if err != nil {
		return err
	}
	runcArgs := []string{
		"--root", runcStateDir,
		"state",
		containerID,
	}

	cmd := exec.Command(runc, runcArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdout
	sylog.Debugf("Calling runc with args %v", runcArgs)
	return cmd.Run()
}

// Update updates container cgroups resources
func Update(containerID, cgFile string) error {
	runc, err := bin.FindBin("runc")
	if err != nil {
		return err
	}
	runcArgs := []string{
		"--root", runcStateDir,
		"update",
		"-r", cgFile,
		containerID,
	}

	cmd := exec.Command(runc, runcArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdout
	sylog.Debugf("Calling runc with args %v", runcArgs)
	return cmd.Run()
}
