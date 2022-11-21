// Copyright (c) 2022, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package oci

import (
	"fmt"
	"os"

	"github.com/opencontainers/runtime-spec/specs-go"
	"github.com/sylabs/singularity/internal/pkg/fakeroot"
	"github.com/sylabs/singularity/internal/pkg/util/user"
)

// getProcessUser computes the uid/gid(s) to be set on process execution.
// Currently this only supports the same uid / primary gid as on the host.
// TODO - expand for fakeroot, and arbitrary mapped user.
func (l *Launcher) getProcessUser() specs.User {
	return specs.User{
		UID: uint32(os.Getuid()),
		GID: uint32(os.Getgid()),
	}
}

// getProcessCwd computes the Cwd that the container process should start in.
// Currently this is the user's tmpfs home directory (see --containall).
func (l *Launcher) getProcessCwd() (dir string, err error) {
	pw, err := user.CurrentOriginal()
	if err != nil {
		return "", err
	}
	return pw.Dir, nil
}

// getIDMaps returns uid and gid mappings appropriate for a non-root user, if required.
func (l *Launcher) getIDMaps() (uidMap, gidMap []specs.LinuxIDMapping, err error) {
	uid := uint32(os.Getuid())
	// Root user gets pass-through mapping
	if uid == 0 {
		uidMap = []specs.LinuxIDMapping{
			{
				ContainerID: 0,
				HostID:      0,
				Size:        65536,
			},
		}
		gidMap = []specs.LinuxIDMapping{
			{
				ContainerID: 0,
				HostID:      0,
				Size:        65536,
			},
		}
		return uidMap, gidMap, nil
	}
	// Set non-root uid/gid per Singularity defaults
	gid := uint32(os.Getgid())
	// Get user's configured subuid & subgid ranges
	subuidRange, err := fakeroot.GetIDRange(fakeroot.SubUIDFile, uid)
	if err != nil {
		return nil, nil, err
	}
	// We must be able to map at least 0->65535 inside the container
	if subuidRange.Size < 65536 {
		return nil, nil, fmt.Errorf("subuid range size (%d) must be at least 65536", subuidRange.Size)
	}
	subgidRange, err := fakeroot.GetIDRange(fakeroot.SubGIDFile, uid)
	if err != nil {
		return nil, nil, err
	}
	if subgidRange.Size <= gid {
		return nil, nil, fmt.Errorf("subuid range size (%d) must be at least 65536", subgidRange.Size)
	}

	// Preserve own uid container->host, map everything else to subuid range.
	if uid < 65536 {
		uidMap = []specs.LinuxIDMapping{
			{
				ContainerID: 0,
				HostID:      subuidRange.HostID,
				Size:        uid,
			},
			{
				ContainerID: uid,
				HostID:      uid,
				Size:        1,
			},
			{
				ContainerID: uid + 1,
				HostID:      subuidRange.HostID + uid,
				Size:        subuidRange.Size - uid,
			},
		}
	} else {
		uidMap = []specs.LinuxIDMapping{
			{
				ContainerID: 0,
				HostID:      subuidRange.HostID,
				Size:        65536,
			},
			{
				ContainerID: uid,
				HostID:      uid,
				Size:        1,
			},
		}
	}

	// Preserve own gid container->host, map everything else to subgid range.
	if gid < 65536 {
		gidMap = []specs.LinuxIDMapping{
			{
				ContainerID: 0,
				HostID:      subgidRange.HostID,
				Size:        gid,
			},
			{
				ContainerID: gid,
				HostID:      gid,
				Size:        1,
			},
			{
				ContainerID: gid + 1,
				HostID:      subgidRange.HostID + gid,
				Size:        subgidRange.Size - gid,
			},
		}
	} else {
		gidMap = []specs.LinuxIDMapping{
			{
				ContainerID: 0,
				HostID:      subgidRange.HostID,
				Size:        65536,
			},
			{
				ContainerID: gid,
				HostID:      gid,
				Size:        1,
			},
		}
	}

	return uidMap, gidMap, nil
}
