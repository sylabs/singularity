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
	"strings"

	"github.com/sylabs/singularity/internal/pkg/buildcfg"
	fakerootConfig "github.com/sylabs/singularity/internal/pkg/runtime/engine/fakeroot/config"
	"github.com/sylabs/singularity/internal/pkg/util/starter"
	"github.com/sylabs/singularity/pkg/ocibundle/tools"
	"github.com/sylabs/singularity/pkg/runtime/engine/config"
	"github.com/sylabs/singularity/pkg/sylog"
	"github.com/sylabs/singularity/pkg/util/singularityconf"
)

// Delete deletes container resources
func Delete(ctx context.Context, containerID string, systemdCgroups bool) error {
	runtimeBin, err := runtime()
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

	cmd := exec.Command(runtimeBin, runtimeArgs...)
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
	runtimeBin, err := runtime()
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
	cmd.Stdin = os.Stdout
	sylog.Debugf("Calling %s with args %v", runtimeBin, runtimeArgs)
	return cmd.Run()
}

// Kill kills container process
func Kill(containerID string, killSignal string, systemdCgroups bool) error {
	runtimeBin, err := runtime()
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
	cmd.Stdin = os.Stdout
	sylog.Debugf("Calling %s with args %v", runtimeBin, runtimeArgs)
	return cmd.Run()
}

// Pause pauses processes in a container
func Pause(containerID string, systemdCgroups bool) error {
	runtimeBin, err := runtime()
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
	cmd.Stdin = os.Stdout
	sylog.Debugf("Calling %s with args %v", runtimeBin, runtimeArgs)
	return cmd.Run()
}

// Resume pauses processes in a container
func Resume(containerID string, systemdCgroups bool) error {
	runtimeBin, err := runtime()
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
	cmd.Stdin = os.Stdout
	sylog.Debugf("Calling %s with args %v", runtimeBin, runtimeArgs)
	return cmd.Run()
}

// Run runs a container (equivalent to create/start/delete)
func Run(ctx context.Context, containerID, bundlePath, pidFile string, systemdCgroups bool) error {
	runtimeBin, err := runtime()
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

	runtimeArgs = append(runtimeArgs, containerID)
	cmd := exec.Command(runtimeBin, runtimeArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdout
	sylog.Debugf("Calling %s with args %v", runtimeBin, runtimeArgs)
	return cmd.Run()
}

// WrapWithWriteableTmpFs runs a function wrapped with prep / cleanup steps for a writeable tmpfs.
func WrapWithWriteableTmpFs(f func() error, bundlePath string) error {
	// TODO: --oci mode always emulating --compat, which uses --writable-tmpfs.
	//       Provide a way of disabling this, for a read only rootfs.
	if err := prepareWriteableTmpfs(bundlePath); err != nil {
		return err
	}

	err := f()

	// Cleanup actions log errors, but don't return - so we get as much cleanup done as possible.
	if err := cleanupWritableTmpfs(bundlePath); err != nil {
		sylog.Errorf("While cleaning up writable tmpfs: %v", err)
	}

	// Return any error from the actual container payload - preserve exit code.
	return err
}

// WrapWithOverlays runs a function wrapped with prep / cleanup steps for overlays.
func WrapWithOverlays(f func() error, bundlePath string, overlayPaths []string) error {
	for _, p := range overlayPaths {
		if err := tools.CreateOverlayByPath(bundlePath, p); err != nil {
			return err
		}
	}

	err := f()

	// Return any error from the actual container payload - preserve exit code.
	return err
}

// RunWrappedNS reexecs singularity in a user namespace, with supplied uid/gid mapping, calling oci run.
func RunWrappedNS(ctx context.Context, containerID, bundlePath string, overlayPaths []string) error {
	absBundle, err := filepath.Abs(bundlePath)
	if err != nil {
		return fmt.Errorf("failed to determine bundle absolute path: %s", err)
	}

	args := []string{
		filepath.Join(buildcfg.BINDIR, "singularity"),
		"oci",
		"run-wrapped",
		"-b", absBundle,
	}
	for _, p := range overlayPaths {
		absPath, err := filepath.Abs(p)
		if err != nil {
			return fmt.Errorf("%w: could not convert %#v to absolute path", err, p)
		}

		args = append(args, "--overlay", absPath)
	}
	args = append(args, containerID)

	if err := os.Chdir(absBundle); err != nil {
		return fmt.Errorf("failed to change directory to %s: %s", absBundle, err)
	}

	sylog.Debugf("Calling fakeroot engine to execute %q", strings.Join(args, " "))

	cfg := &config.Common{
		EngineName:  fakerootConfig.Name,
		ContainerID: "fakeroot",
		EngineConfig: &fakerootConfig.EngineConfig{
			Envs:    os.Environ(),
			Args:    args,
			NoPIDNS: true,
		},
	}

	return starter.Run(
		"Singularity oci userns",
		cfg,
		starter.WithStdin(os.Stdin),
		starter.WithStdout(os.Stdout),
		starter.WithStderr(os.Stderr),
	)
}

// Start starts a previously created container
func Start(containerID string, systemdCgroups bool) error {
	runtimeBin, err := runtime()
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
	cmd.Stdin = os.Stdout
	sylog.Debugf("Calling %s with args %v", runtimeBin, runtimeArgs)
	return cmd.Run()
}

// State queries container state
func State(containerID string, systemdCgroups bool) error {
	runtimeBin, err := runtime()
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
	cmd.Stdin = os.Stdout
	sylog.Debugf("Calling %s with args %v", runtimeBin, runtimeArgs)
	return cmd.Run()
}

// Update updates container cgroups resources
func Update(containerID, cgFile string, systemdCgroups bool) error {
	runtimeBin, err := runtime()
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
	cmd.Stdin = os.Stdout
	sylog.Debugf("Calling %s with args %v", runtimeBin, runtimeArgs)
	return cmd.Run()
}

func prepareWriteableTmpfs(bundleDir string) error {
	sylog.Debugf("Configuring writable tmpfs overlay for %s", bundleDir)
	c := singularityconf.GetCurrentConfig()
	if c == nil {
		return fmt.Errorf("singularity configuration is not initialized")
	}
	return tools.CreateOverlayTmpfs(bundleDir, int(c.SessiondirMaxSize))
}

func cleanupWritableTmpfs(bundleDir string) error {
	sylog.Debugf("Cleaning up writable tmpfs overlay for %s", bundleDir)
	return tools.DeleteOverlay(bundleDir)
}
