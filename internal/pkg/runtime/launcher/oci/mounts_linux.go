// Copyright (c) 2022, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

// Package oci implements a Launcher that will configure and launch a container
// with an OCI runtime. It also provides implementations of OCI state
// transitions that can be called directly, Create/Start/Kill etc.
package oci

import (
	"fmt"
	"os"
	"strconv"

	"github.com/opencontainers/runtime-spec/specs-go"
	"github.com/sylabs/singularity/internal/pkg/util/user"
)

// getMounts returns a mount list for the container's OCI runtime spec.
func (l *Launcher) getMounts() ([]specs.Mount, error) {
	mounts := &[]specs.Mount{}
	l.addProcMount(mounts)
	l.addSysMount(mounts)
	err := addDevMounts(mounts)
	if err != nil {
		return nil, fmt.Errorf("while configuring devpts mount: %w", err)
	}
	l.addTmpMounts(mounts)
	err = l.addHomeMount(mounts)
	if err != nil {
		return nil, fmt.Errorf("while configuring home mount: %w", err)
	}
	return *mounts, nil
}

// addTmpMounts adds tmpfs mounts for /tmp and /var/tmp in the container.
func (l *Launcher) addTmpMounts(mounts *[]specs.Mount) {
	*mounts = append(*mounts,
		specs.Mount{
			Destination: "/tmp",
			Type:        "tmpfs",
			Source:      "tmpfs",
			Options:     []string{"nosuid", "relatime", "mode=777", "size=65536k"},
		},
		specs.Mount{
			Destination: "/tmp",
			Type:        "tmpfs",
			Source:      "tmpfs",
			Options:     []string{"nosuid", "relatime", "mode=777", "size=65536k"},
		})
}

// addDevMounts adds mounts to assemble a minimal /dev in the container.
func addDevMounts(mounts *[]specs.Mount) error {
	ptsMount := specs.Mount{
		Destination: "/dev/pts",
		Type:        "devpts",
		Source:      "devpts",
		Options:     []string{"nosuid", "noexec", "newinstance", "ptmxmode=0666", "mode=0620"},
	}

	if os.Getuid() == 0 {
		group, err := user.GetGrNam("tty")
		if err != nil {
			return fmt.Errorf("while identifying tty gid: %w", err)
		}
		ptsMount.Options = append(ptsMount.Options, fmt.Sprintf("gid=%d", group.GID))
	}

	*mounts = append(*mounts,
		specs.Mount{
			Destination: "/dev",
			Type:        "tmpfs",
			Source:      "tmpfs",
			Options:     []string{"nosuid", "strictatime", "mode=755", "size=65536k"},
		},
		ptsMount,
		specs.Mount{
			Destination: "/dev/shm",
			Type:        "tmpfs",
			Source:      "shm",
			Options:     []string{"nosuid", "noexec", "nodev", "mode=1777", "size=65536k"},
		},
		specs.Mount{
			Destination: "/dev/mqueue",
			Type:        "mqueue",
			Source:      "mqueue",
			Options:     []string{"nosuid", "noexec", "nodev"},
		},
	)

	return nil
}

// addProcMount adds the /proc tree in the container.
func (l *Launcher) addProcMount(mounts *[]specs.Mount) {
	*mounts = append(*mounts,
		specs.Mount{
			Source:      "proc",
			Destination: "/proc",
			Type:        "proc",
		})
}

// addSysMount adds the /sys tree in the container.
func (l *Launcher) addSysMount(mounts *[]specs.Mount) {
	if os.Getuid() == 0 {
		*mounts = append(*mounts,
			specs.Mount{
				Source:      "sysfs",
				Destination: "/sys",
				Type:        "sysfs",
				Options:     []string{"nosuid", "noexec", "nodev", "ro"},
			})
	} else {
		*mounts = append(*mounts,
			specs.Mount{
				Source:      "/sys",
				Destination: "/sys",
				Type:        "none",
				Options:     []string{"rbind", "nosuid", "noexec", "nodev", "ro"},
			})
	}
}

// addHomeMount adds a user home directory as a tmpfs mount. We are currently
// emulating `--compat` / `--containall`, so the user must specifically bind in
// their home directory from the host for it to be available.
func (l *Launcher) addHomeMount(mounts *[]specs.Mount) error {
	if l.cfg.Fakeroot {
		*mounts = append(*mounts,
			specs.Mount{
				Destination: "/root",
				Type:        "tmpfs",
				Source:      "tmpfs",
				Options:     []string{"nosuid", "relatime", "mode=755", "size=65536k"},
			})
		return nil
	}

	pw, err := user.CurrentOriginal()
	if err != nil {
		return err
	}
	*mounts = append(*mounts,
		specs.Mount{
			Destination: pw.Dir,
			Type:        "tmpfs",
			Source:      "tmpfs",
			Options:     []string{"nosuid", "relatime", "mode=755", "size=65536k", "uid=" + strconv.Itoa(int(pw.UID)), "gid=" + strconv.Itoa(int(pw.GID))},
		})
	return nil
}
