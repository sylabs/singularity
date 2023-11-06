// Copyright (c) 2018-2022, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

// Includes code from https://github.com/containers/podman
// Released under the Apache License Version 2.0

package oci

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/sylabs/singularity/v4/internal/pkg/buildcfg"
	"github.com/sylabs/singularity/v4/internal/pkg/util/bin"
	"github.com/sylabs/singularity/v4/pkg/sylog"
	"golang.org/x/sys/unix"
)

type ociError struct {
	Level string `json:"level,omitempty"`
	Time  string `json:"time,omitempty"`
	Msg   string `json:"msg,omitempty"`
}

// Create creates a container from an OCI bundle
func Create(containerID, bundlePath string, systemdCgroups bool) error {
	conmon, err := bin.FindBin("conmon")
	if err != nil {
		return err
	}
	runtimeBin, err := Runtime()
	if err != nil {
		return err
	}
	// chdir to bundle and lock it, so another oci create cannot use the same bundle
	absBundle, err := filepath.Abs(bundlePath)
	if err != nil {
		return fmt.Errorf("failed to determine bundle absolute path: %w", err)
	}
	if err := os.Chdir(absBundle); err != nil {
		return fmt.Errorf("failed to change directory to %s: %w", absBundle, err)
	}
	if err := lockBundle(absBundle); err != nil {
		return fmt.Errorf("while locking bundle: %w", err)
	}

	// Create our own state location for conmon and singularity related files
	sd, err := stateDir(containerID)
	if err != nil {
		return fmt.Errorf("while computing state directory: %w", err)
	}
	err = os.MkdirAll(sd, 0o700)
	if err != nil {
		return fmt.Errorf("while creating state directory: %w", err)
	}
	containerUUID, err := uuid.NewRandom()
	if err != nil {
		return err
	}

	// Pipes for sync and start communication with conmon
	syncFds, err := unix.Socketpair(unix.AF_LOCAL, unix.SOCK_SEQPACKET|unix.SOCK_CLOEXEC, 0)
	if err != nil {
		return fmt.Errorf("could not create sync socket pair: %w", err)
	}
	syncChild := os.NewFile(uintptr(syncFds[0]), "sync_child")
	syncParent := os.NewFile(uintptr(syncFds[1]), "sync_parent")
	defer syncParent.Close()

	startFds, err := unix.Socketpair(unix.AF_LOCAL, unix.SOCK_SEQPACKET|unix.SOCK_CLOEXEC, 0)
	if err != nil {
		return fmt.Errorf("could not create sync socket pair: %w", err)
	}
	startChild := os.NewFile(uintptr(startFds[0]), "start_child")
	startParent := os.NewFile(uintptr(startFds[1]), "start_parent")
	defer startParent.Close()

	singularityBin := filepath.Join(buildcfg.BINDIR, "singularity")

	rsd, err := runtimeStateDir()
	if err != nil {
		return err
	}

	cmdArgs := []string{
		"--api-version", "1",
		"--cid", containerID,
		"--name", containerID,
		"--cuuid", containerUUID.String(),
		"--runtime", runtimeBin,
		"--conmon-pidfile", path.Join(sd, conmonPidFile),
		"--container-pidfile", path.Join(sd, containerPidFile),
		"--log-path", path.Join(sd, containerLogFile),
		"--runtime-arg", "--root",
		"--runtime-arg", rsd,
		"--runtime-arg", "--log",
		"--runtime-arg", path.Join(sd, runcLogFile),
		"--full-attach",
		"--terminal",
		"--bundle", absBundle,
		"--exit-command", singularityBin,
		"--exit-command-arg", "--debug",
		"--exit-command-arg", "oci",
		"--exit-command-arg", "cleanup",
		"--exit-command-arg", containerID,
	}

	if systemdCgroups {
		cmdArgs = append(cmdArgs, "--systemd-cgroup")
	}

	cmd := exec.Command(conmon, cmdArgs...)
	cmd.Dir = absBundle
	cmd.Env = append(cmd.Env, fmt.Sprintf("_OCI_SYNCPIPE=%d", 3), fmt.Sprintf("_OCI_STARTPIPE=%d", 4))
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}
	cmd.ExtraFiles = append(cmd.ExtraFiles, syncChild, startChild)

	// Run conmon and close it's end of the pipes in our parent process
	sylog.Debugf("Starting conmon with args %v", cmdArgs)
	if err := cmd.Start(); err != nil {
		if err2 := releaseBundle(absBundle); err2 != nil {
			sylog.Errorf("while releasing bundle: %v", err)
		}
		return fmt.Errorf("while starting conmon: %w", err)
	}
	syncChild.Close()
	startChild.Close()

	// No other setup at present... just signal conmon to start work
	writeConmonPipeData(startParent)
	// After conmon receives from start pipe it should start container and exit
	// without error.
	err = cmd.Wait()
	if err != nil {
		if err2 := releaseBundle(absBundle); err2 != nil {
			sylog.Errorf("while releasing bundle: %v", err)
		}
		return fmt.Errorf("while starting conmon: %w", err)
	}

	// We check for errors from runc (which conmon invokes) via the sync pipe
	pid, err := readConmonPipeData(syncParent, path.Join(sd, runcLogFile))
	if err != nil {
		if err2 := Delete(context.TODO(), containerID, systemdCgroups); err2 != nil {
			sylog.Errorf("Removing container %s from runtime after creation failed", containerID)
		}
		return err
	}

	// Create a symlink from the state dir to the bundle, so it's easy to find later on.
	bundleLink := path.Join(sd, "bundle")
	if err := os.Symlink(absBundle, bundleLink); err != nil {
		return fmt.Errorf("could not link attach socket: %w", err)
	}

	sylog.Infof("Container %s created with PID %d", containerID, pid)
	return nil
}

// The following utility functions are taken from https://github.com/containers/podman
// Released under the Apache License Version 2.0

func readConmonPipeData(pipe *os.File, ociLog string) (int, error) {
	// syncInfo is used to return data from monitor process to daemon
	type syncInfo struct {
		Data    int    `json:"data"`
		Message string `json:"message,omitempty"`
	}

	// Wait to get container pid from conmon
	type syncStruct struct {
		si  *syncInfo
		err error
	}
	ch := make(chan syncStruct)
	go func() {
		var si *syncInfo
		rdr := bufio.NewReader(pipe)
		b, err := rdr.ReadBytes('\n')
		if err != nil {
			ch <- syncStruct{err: err}
		}
		if err := json.Unmarshal(b, &si); err != nil {
			ch <- syncStruct{err: err}
			return
		}
		ch <- syncStruct{si: si}
	}()

	data := -1
	select {
	case ss := <-ch:
		if ss.err != nil {
			if ociLog != "" {
				ociLogData, err := os.ReadFile(ociLog)
				if err == nil {
					var ociErr ociError
					if err := json.Unmarshal(ociLogData, &ociErr); err == nil {
						return -1, fmt.Errorf("runc error: %s", ociErr.Msg)
					}
				}
			}
			return -1, fmt.Errorf("container create failed (no logs from conmon): %w", ss.err)
		}
		sylog.Debugf("Received: %d", ss.si.Data)
		if ss.si.Data < 0 {
			if ociLog != "" {
				ociLogData, err := os.ReadFile(ociLog)
				if err == nil {
					var ociErr ociError
					if err := json.Unmarshal(ociLogData, &ociErr); err == nil {
						return ss.si.Data, fmt.Errorf("runc error: %s", ociErr.Msg)
					}
				}
			}
			// If we failed to parse the JSON errors, then print the output as it is
			if ss.si.Message != "" {
				return ss.si.Data, fmt.Errorf("runc error: %s", ss.si.Message)
			}
			return ss.si.Data, fmt.Errorf("container creation failed")
		}
		data = ss.si.Data
	case <-time.After(createTimeout):
		return -1, fmt.Errorf("container creation timeout")
	}
	return data, nil
}

// writeConmonPipeData writes nonce data to a pipe
func writeConmonPipeData(pipe *os.File) error {
	someData := []byte{0}
	_, err := pipe.Write(someData)
	return err
}
