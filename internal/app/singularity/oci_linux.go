// Copyright (c) 2018-2022, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

// Includes code from https://github.com/containers/podman
// Released under the Apache License Version 2.0

package singularity

import (
	"context"
	"fmt"

	lccgroups "github.com/opencontainers/runc/libcontainer/cgroups"
	"github.com/sylabs/singularity/v4/internal/pkg/buildcfg"
	"github.com/sylabs/singularity/v4/internal/pkg/runtime/launcher/oci"
	ocibundle "github.com/sylabs/singularity/v4/pkg/ocibundle/sif"
	"github.com/sylabs/singularity/v4/pkg/util/namespaces"
	"github.com/sylabs/singularity/v4/pkg/util/singularityconf"
)

// OciArgs contains CLI arguments
type OciArgs struct {
	BundlePath   string
	OverlayPaths []string
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
	systemdCgroups, err := systemdCgroups()
	if err != nil {
		return err
	}
	return oci.Run(ctx, containerID, args.BundlePath, args.PidFile, systemdCgroups)
}

// OciCreate creates a container from an OCI bundle
func OciCreate(containerID string, args *OciArgs) error {
	systemdCgroups, err := systemdCgroups()
	if err != nil {
		return err
	}
	return oci.Create(containerID, args.BundlePath, systemdCgroups)
}

// OciStart starts a previously create container
func OciStart(containerID string) error {
	systemdCgroups, err := systemdCgroups()
	if err != nil {
		return err
	}
	return oci.Start(containerID, systemdCgroups)
}

// OciDelete deletes container resources
func OciDelete(ctx context.Context, containerID string) error {
	systemdCgroups, err := systemdCgroups()
	if err != nil {
		return err
	}
	return oci.Delete(ctx, containerID, systemdCgroups)
}

// OciExec executes a command in a container
func OciExec(containerID string, cmdArgs []string) error {
	systemdCgroups, err := systemdCgroups()
	if err != nil {
		return err
	}
	return oci.Exec(containerID, cmdArgs, systemdCgroups)
}

// OciKill kills container process
func OciKill(containerID string, killSignal string) error {
	return oci.Kill(containerID, killSignal)
}

// OciPause pauses processes in a container
func OciPause(containerID string) error {
	systemdCgroups, err := systemdCgroups()
	if err != nil {
		return err
	}
	return oci.Pause(containerID, systemdCgroups)
}

// OciResume pauses processes in a container
func OciResume(containerID string) error {
	systemdCgroups, err := systemdCgroups()
	if err != nil {
		return err
	}
	return oci.Resume(containerID, systemdCgroups)
}

// OciState queries container state
func OciState(containerID string, _ *OciArgs) error {
	systemdCgroups, err := systemdCgroups()
	if err != nil {
		return err
	}
	return oci.State(containerID, systemdCgroups)
}

// OciUpdate updates container cgroups resources
func OciUpdate(containerID string, args *OciArgs) error {
	systemdCgroups, err := systemdCgroups()
	if err != nil {
		return err
	}
	return oci.Update(containerID, args.FromFile, systemdCgroups)
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
func OciUmount(ctx context.Context, bundle string) error {
	d, err := ocibundle.FromSif("", bundle, true)
	if err != nil {
		return err
	}
	return d.Delete(ctx)
}

func systemdCgroups() (use bool, err error) {
	cfg := singularityconf.GetCurrentConfig()
	if cfg == nil {
		cfg, err = singularityconf.Parse(buildcfg.SINGULARITY_CONF_FILE)
		if err != nil {
			return false, fmt.Errorf("unable to parse singularity configuration file: %w", err)
		}
	}

	useSystemd := cfg.SystemdCgroups

	// As non-root, we need cgroups v2 unified mode for systemd support.
	// Fall back to cgroupfs if this is not available.
	hostUID, err := namespaces.HostUID()
	if err != nil {
		return false, fmt.Errorf("while finding host uid: %w", err)
	}
	if hostUID != 0 && !lccgroups.IsCgroup2UnifiedMode() {
		useSystemd = false
	}

	return useSystemd, nil
}
