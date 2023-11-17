// Copyright (c) 2018-2023, Sylabs Inc. All rights reserved.
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
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"

	"github.com/sylabs/singularity/v4/pkg/sylog"
)

// Delete deletes container resources
func Delete(ctx context.Context, containerID string, systemdCgroups bool) error {
	runtimeBin, err := Runtime()
	if err != nil {
		return err
	}
	rsd, err := runtimeStateDir()
	if err != nil {
		return err
	}

	runtimeArgs := []string{
		"--root", rsd,
	}
	if systemdCgroups {
		runtimeArgs = append(runtimeArgs, "--systemd-cgroup")
	}
	runtimeArgs = append(runtimeArgs, "delete", containerID)

	cmd := exec.CommandContext(ctx, runtimeBin, runtimeArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdout
	sylog.Debugf("Calling %s with args %v", runtimeBin, runtimeArgs)
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
func Exec(containerID string, cmdArgs []string, systemdCgroups bool) error {
	runtimeBin, err := Runtime()
	if err != nil {
		return err
	}
	rsd, err := runtimeStateDir()
	if err != nil {
		return err
	}

	runtimeArgs := []string{
		"--root", rsd,
	}
	if systemdCgroups {
		runtimeArgs = append(runtimeArgs, "--systemd-cgroup")
	}
	runtimeArgs = append(runtimeArgs, "exec", containerID)
	runtimeArgs = append(runtimeArgs, cmdArgs...)
	cmd := exec.Command(runtimeBin, runtimeArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	sylog.Debugf("Calling %s with args %v", runtimeBin, runtimeArgs)
	return cmd.Run()
}

// Kill kills container process
func Kill(containerID string, killSignal string) error {
	runtimeBin, err := Runtime()
	if err != nil {
		return err
	}
	rsd, err := runtimeStateDir()
	if err != nil {
		return err
	}

	runtimeArgs := []string{
		"--root", rsd,
		"kill",
		containerID,
		killSignal,
	}

	cmd := exec.Command(runtimeBin, runtimeArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	sylog.Debugf("Calling %s with args %v", runtimeBin, runtimeArgs)
	return cmd.Run()
}

// Pause pauses processes in a container
func Pause(containerID string, systemdCgroups bool) error {
	runtimeBin, err := Runtime()
	if err != nil {
		return err
	}
	rsd, err := runtimeStateDir()
	if err != nil {
		return err
	}

	runtimeArgs := []string{
		"--root", rsd,
	}
	if systemdCgroups {
		runtimeArgs = append(runtimeArgs, "--systemd-cgroup")
	}
	runtimeArgs = append(runtimeArgs, "pause", containerID)

	cmd := exec.Command(runtimeBin, runtimeArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	sylog.Debugf("Calling %s with args %v", runtimeBin, runtimeArgs)
	return cmd.Run()
}

// Resume un-pauses processes in a container
func Resume(containerID string, systemdCgroups bool) error {
	runtimeBin, err := Runtime()
	if err != nil {
		return err
	}
	rsd, err := runtimeStateDir()
	if err != nil {
		return err
	}

	runtimeArgs := []string{
		"--root", rsd,
	}
	if systemdCgroups {
		runtimeArgs = append(runtimeArgs, "--systemd-cgroup")
	}
	runtimeArgs = append(runtimeArgs, "resume", containerID)

	cmd := exec.Command(runtimeBin, runtimeArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	sylog.Debugf("Calling %s with args %v", runtimeBin, runtimeArgs)
	return cmd.Run()
}

// Run runs a container (equivalent to create/start/delete)
func Run(ctx context.Context, containerID, bundlePath, pidFile string, systemdCgroups bool) error {
	runtimeBin, err := Runtime()
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

	rsd, err := runtimeStateDir()
	if err != nil {
		return err
	}

	runtimeArgs := []string{
		"--root", rsd,
	}
	if systemdCgroups {
		runtimeArgs = append(runtimeArgs, "--systemd-cgroup")
	}
	runtimeArgs = append(runtimeArgs, "run", "-b", absBundle)
	if pidFile != "" {
		runtimeArgs = append(runtimeArgs, "--pid-file="+pidFile)
	}

	signals := make(chan os.Signal, 2)
	signal.Notify(signals)
	sylog.Debugf("Starting signal proxy for container %s", containerID)
	go signalProxy(containerID, signals)

	runtimeArgs = append(runtimeArgs, containerID)
	cmd := exec.CommandContext(ctx, runtimeBin, runtimeArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	sylog.Debugf("Calling %s with args %v", runtimeBin, runtimeArgs)
	return cmd.Run()
}

func signalProxy(containerID string, signals chan os.Signal) {
	for {
		s := <-signals
		switch s {
		case syscall.SIGCHLD:
			break
		case syscall.SIGURG:
			// Ignore SIGURG, which is used for non-cooperative goroutine
			// preemption starting with Go 1.14. For more information, see
			// https://github.com/golang/go/issues/24543.
			break
		default:
			sylog.Debugf("Received signal %s", s.String())
			//nolint:forcetypeassert
			sysSig := s.(syscall.Signal)
			sylog.Debugf("Sending signal %s to container %s", sysSig.String(), containerID)
			if err := Kill(containerID, strconv.Itoa(int(sysSig))); err != nil {
				sylog.Errorf("Failed to send signal %s to container %s", sysSig.String(), containerID)
			}
		}
	}
}

// Start starts a previously created container
func Start(containerID string, systemdCgroups bool) error {
	runtimeBin, err := Runtime()
	if err != nil {
		return err
	}
	rsd, err := runtimeStateDir()
	if err != nil {
		return err
	}

	runtimeArgs := []string{
		"--root", rsd,
	}
	if systemdCgroups {
		runtimeArgs = append(runtimeArgs, "--systemd-cgroup")
	}
	runtimeArgs = append(runtimeArgs, "start", containerID)

	cmd := exec.Command(runtimeBin, runtimeArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	sylog.Debugf("Calling %s with args %v", runtimeBin, runtimeArgs)
	return cmd.Run()
}

// State queries container state
func State(containerID string, systemdCgroups bool) error {
	runtimeBin, err := Runtime()
	if err != nil {
		return err
	}
	rsd, err := runtimeStateDir()
	if err != nil {
		return err
	}

	runtimeArgs := []string{
		"--root", rsd,
	}
	if systemdCgroups {
		runtimeArgs = append(runtimeArgs, "--systemd-cgroup")
	}
	runtimeArgs = append(runtimeArgs, "state", containerID)

	cmd := exec.Command(runtimeBin, runtimeArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	sylog.Debugf("Calling %s with args %v", runtimeBin, runtimeArgs)
	return cmd.Run()
}

// Update updates container cgroups resources
func Update(containerID, cgFile string, systemdCgroups bool) error {
	runtimeBin, err := Runtime()
	if err != nil {
		return err
	}
	rsd, err := runtimeStateDir()
	if err != nil {
		return err
	}

	runtimeArgs := []string{
		"--root", rsd,
	}
	if systemdCgroups {
		runtimeArgs = append(runtimeArgs, "--systemd-cgroup")
	}
	runtimeArgs = append(runtimeArgs, "update", "-r", cgFile, containerID)

	cmd := exec.Command(runtimeBin, runtimeArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	sylog.Debugf("Calling %s with args %v", runtimeBin, runtimeArgs)
	return cmd.Run()
}
