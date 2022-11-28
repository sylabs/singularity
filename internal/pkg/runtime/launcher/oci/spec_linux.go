// Copyright (c) 2022, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package oci

import (
	"github.com/opencontainers/runtime-spec/specs-go"
)

// MinimalSpec returns an OCI runtime spec with a minimal OCI configuration that
// is a starting point for compatibility with Singularity's native launcher in
// `--compat` mode.
func MinimalSpec() (*specs.Spec, error) {
	config := specs.Spec{
		Version: specs.Version,
	}
	config.Root = &specs.Root{
		Path: "rootfs",
		// TODO - support writable-tmpfs / writable
		Readonly: true,
	}
	config.Process = &specs.Process{
		Terminal: true,
		// Default fallback to a shell at / - will generally be overwritten by
		// the launcher.
		Args: []string{"sh"},
		Cwd:  "/",
	}
	config.Process.User = specs.User{}
	config.Process.Env = []string{
		"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
	}

	// TODO - these are appropriate minimum for rootless. We need to tie into
	// Singularity's cap-add / cap-drop mechanism.
	config.Process.Capabilities = &specs.LinuxCapabilities{
		Bounding: []string{
			"CAP_CHOWN",
			"CAP_DAC_OVERRIDE",
			"CAP_FOWNER",
			"CAP_FSETID",
			"CAP_KILL",
			"CAP_NET_BIND_SERVICE",
			"CAP_SETFCAP",
			"CAP_SETGID",
			"CAP_SETPCAP",
			"CAP_SETUID",
			"CAP_SYS_CHROOT",
		},
	}

	// All mounts are added by the launcher, as it must handle flags.
	config.Mounts = []specs.Mount{}

	config.Linux = &specs.Linux{
		// Minimum namespaces matching native runtime with --compat / --containall.
		// TODO: Additional namespaces to be added by launcher.
		Namespaces: []specs.LinuxNamespace{
			{
				Type: specs.IPCNamespace,
			},
			{
				Type: specs.PIDNamespace,
			},
			{
				Type: specs.MountNamespace,
			},
		},
	}
	return &config, nil
}
