// Copyright (c) 2018-2022, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

// Includes code from https://github.com/containers/podman
// Released under the Apache License Version 2.0

package singularity

import (
	"context"

	"github.com/sylabs/singularity/internal/pkg/runtime/launcher/oci"
	ocibundle "github.com/sylabs/singularity/pkg/ocibundle/sif"
)

// OciArgs contains CLI arguments
type OciArgs struct {
	BundlePath   string
	LogPath      string
	LogFormat    string
	PidFile      string
	FromFile     string
	KillSignal   string
	KillTimeout  uint32
	EmptyProcess bool
	ForceKill    bool
}

// OciRun runs a container (equivalent to create/start/delete)
func OciRun(ctx context.Context, containerID string, args *OciArgs) error {
	return oci.Run(ctx, containerID, args.BundlePath, args.PidFile)
}

// OciCreate creates a container from an OCI bundle
func OciCreate(containerID string, args *OciArgs) error {
	return oci.Create(containerID, args.BundlePath)
}

// OciStart starts a previously create container
func OciStart(containerID string) error {
	return oci.Start(containerID)
}

// OciDelete deletes container resources
func OciDelete(ctx context.Context, containerID string) error {
	return oci.Delete(ctx, containerID)
}

// OciExec executes a command in a container
func OciExec(containerID string, cmdArgs []string) error {
	return oci.Exec(containerID, cmdArgs)
}

// OciKill kills container process
func OciKill(containerID string, killSignal string) error {
	return oci.Kill(containerID, killSignal)
}

// OciPause pauses processes in a container
func OciPause(containerID string) error {
	return oci.Pause(containerID)
}

// OciResume pauses processes in a container
func OciResume(containerID string) error {
	return oci.Resume(containerID)
}

// OciState queries container state
func OciState(containerID string, args *OciArgs) error {
	return oci.State(containerID)
}

// OciUpdate updates container cgroups resources
func OciUpdate(containerID string, args *OciArgs) error {
	return oci.Update(containerID, args.FromFile)
}

// OciMount mount a SIF image to create an OCI bundle
func OciMount(ctx context.Context, image string, bundle string) error {
	d, err := ocibundle.FromSif(image, bundle, true)
	if err != nil {
		return err
	}
	return d.Create(ctx, nil)
}

// OciUmount umount SIF and delete OCI bundle
func OciUmount(bundle string) error {
	d, err := ocibundle.FromSif("", bundle, true)
	if err != nil {
		return err
	}
	return d.Delete()
}
