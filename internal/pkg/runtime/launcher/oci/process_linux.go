// Copyright (c) 2022, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package oci

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/opencontainers/runtime-spec/specs-go"
	"github.com/sylabs/singularity/internal/pkg/fakeroot"
	"github.com/sylabs/singularity/internal/pkg/util/env"
	"github.com/sylabs/singularity/internal/pkg/util/shell/interpreter"
	"github.com/sylabs/singularity/internal/pkg/util/user"
)

// getProcessUser computes the uid/gid(s) to be set on process execution.
// Currently this only supports the same uid / primary gid as on the host.
// TODO - expand for fakeroot, and arbitrary mapped user.
func (l *Launcher) getProcessUser() specs.User {
	if l.cfg.Fakeroot {
		return specs.User{
			UID: 0,
			GID: 0,
		}
	}
	return specs.User{
		UID: uint32(os.Getuid()),
		GID: uint32(os.Getgid()),
	}
}

// getProcessCwd computes the Cwd that the container process should start in.
// Currently this is the user's tmpfs home directory (see --containall).
func (l *Launcher) getProcessCwd() (dir string, err error) {
	if l.cfg.Fakeroot {
		return "/root", nil
	}

	pw, err := user.CurrentOriginal()
	if err != nil {
		return "", err
	}
	return pw.Dir, nil
}

// getReverseUserMaps returns uid and gid mappings that re-map container uid to host
// uid. This 'reverses' the host user to container root mapping in the initial
// userns from which the OCI runtime is launched.
//
//	host 1001 -> fakeroot userns 0 -> container 1001
func (l *Launcher) getReverseUserMaps() (uidMap, gidMap []specs.LinuxIDMapping, err error) {
	uid := uint32(os.Getuid())
	gid := uint32(os.Getgid())
	// Get user's configured subuid & subgid ranges
	subuidRange, err := fakeroot.GetIDRange(fakeroot.SubUIDFile, uid)
	if err != nil {
		return nil, nil, err
	}
	// We must always be able to map at least 0->65535 inside the container, so we cover 'nobody'.
	if subuidRange.Size < 65536 {
		return nil, nil, fmt.Errorf("subuid range size (%d) must be at least 65536", subuidRange.Size)
	}
	subgidRange, err := fakeroot.GetIDRange(fakeroot.SubGIDFile, uid)
	if err != nil {
		return nil, nil, err
	}
	if subgidRange.Size < 65536 {
		return nil, nil, fmt.Errorf("subuid range size (%d) must be at least 65536", subgidRange.Size)
	}

	if uid < subuidRange.Size {
		uidMap = []specs.LinuxIDMapping{
			{
				ContainerID: 0,
				HostID:      1,
				Size:        uid,
			},
			{
				ContainerID: uid,
				HostID:      0,
				Size:        1,
			},
			{
				ContainerID: uid + 1,
				HostID:      uid + 1,
				Size:        subuidRange.Size - uid,
			},
		}
	} else {
		uidMap = []specs.LinuxIDMapping{
			{
				ContainerID: 0,
				HostID:      1,
				Size:        subuidRange.Size,
			},
			{
				ContainerID: uid,
				HostID:      0,
				Size:        1,
			},
		}
	}

	if gid < subgidRange.Size {
		gidMap = []specs.LinuxIDMapping{
			{
				ContainerID: 0,
				HostID:      1,
				Size:        gid,
			},
			{
				ContainerID: gid,
				HostID:      0,
				Size:        1,
			},
			{
				ContainerID: gid + 1,
				HostID:      gid + 1,
				Size:        subgidRange.Size - gid,
			},
		}
	} else {
		gidMap = []specs.LinuxIDMapping{
			{
				ContainerID: 0,
				HostID:      1,
				Size:        subgidRange.Size,
			},
			{
				ContainerID: gid,
				HostID:      0,
				Size:        1,
			},
		}
	}

	return uidMap, gidMap, nil
}

// defaultEnv returns default environment variables set in the container.
func defaultEnv(image, bundle string) map[string]string {
	return map[string]string{
		env.SingularityPrefix + "CONTAINER": bundle,
		env.SingularityPrefix + "NAME":      image,
	}
}

// singularityEnvMap returns a map of SINGULARITYENV_ prefixed env vars to their values.
func singularityEnvMap() map[string]string {
	singularityEnv := map[string]string{}

	for _, envVar := range os.Environ() {
		if !strings.HasPrefix(envVar, env.SingularityEnvPrefix) {
			continue
		}
		parts := strings.SplitN(envVar, "=", 2)
		if len(parts) < 2 {
			continue
		}
		key := strings.TrimPrefix(parts[0], env.SingularityEnvPrefix)
		singularityEnv[key] = parts[1]
	}

	return singularityEnv
}

// envFileMap returns a map of KEY=VAL env vars from an environment file
func envFileMap(ctx context.Context, f string) (map[string]string, error) {
	envMap := map[string]string{}

	content, err := os.ReadFile(f)
	if err != nil {
		return envMap, fmt.Errorf("could not read environment file %q: %w", f, err)
	}

	// Use the embedded shell interpreter to evaluate the env file, with an empty starting environment.
	// Shell takes care of comments, quoting etc. for us and keeps compatibility with native runtime.
	env, err := interpreter.EvaluateEnv(ctx, content, []string{}, []string{})
	if err != nil {
		return envMap, fmt.Errorf("while processing %s: %w", f, err)
	}

	for _, envVar := range env {
		parts := strings.SplitN(envVar, "=", 2)
		if len(parts) < 2 {
			continue
		}
		// Strip out the runtime env vars set by the shell interpreter
		if parts[0] == "GID" ||
			parts[0] == "HOME" ||
			parts[0] == "IFS" ||
			parts[0] == "OPTIND" ||
			parts[0] == "PWD" ||
			parts[0] == "UID" {
			continue
		}
		envMap[parts[0]] = parts[1]
	}

	return envMap, nil
}
