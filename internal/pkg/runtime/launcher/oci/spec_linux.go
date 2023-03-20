// Copyright (c) 2022, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package oci

import (
	"fmt"
	"os"

	"github.com/container-orchestrated-devices/container-device-interface/pkg/cdi"
	"github.com/opencontainers/runtime-spec/specs-go"

	"github.com/sylabs/singularity/internal/pkg/runtime/launcher"
	"github.com/sylabs/singularity/pkg/sylog"
)

// defaultNamespaces matching native runtime with --compat / --containall.
var defaultNamespaces = []specs.LinuxNamespace{
	{
		Type: specs.IPCNamespace,
	},
	{
		Type: specs.PIDNamespace,
	},
	{
		Type: specs.MountNamespace,
	},
}

// minimalSpec returns an OCI runtime spec with a minimal OCI configuration that
// is a starting point for compatibility with Singularity's native launcher in
// `--compat` mode.
func minimalSpec() specs.Spec {
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

	config.Linux = &specs.Linux{Namespaces: defaultNamespaces}
	return config
}

// addNamespaces adds requested namespace, if appropriate, to an existing spec.
// It is assumed that spec contains at least the defaultNamespaces.
func addNamespaces(spec specs.Spec, ns launcher.Namespaces) specs.Spec {
	if ns.IPC {
		sylog.Infof("--oci runtime always uses an IPC namespace, ipc flag is redundant.")
	}

	// Currently supports only `--network none`, i.e. isolated loopback only.
	// Launcher.checkopts enforces this.
	if ns.Net {
		spec.Linux.Namespaces = append(
			spec.Linux.Namespaces,
			specs.LinuxNamespace{Type: specs.NetworkNamespace},
		)
	}

	if ns.PID {
		sylog.Infof("--oci runtime always uses a PID namespace, pid flag is redundant.")
	}

	if ns.User {
		if os.Getuid() == 0 {
			spec.Linux.Namespaces = append(
				spec.Linux.Namespaces,
				specs.LinuxNamespace{Type: specs.UserNamespace},
			)
		} else {
			sylog.Infof("--oci runtime always uses a user namespace when run as a non-root userns, user flag is redundant.")
		}
	}

	if ns.UTS {
		spec.Linux.Namespaces = append(
			spec.Linux.Namespaces,
			specs.LinuxNamespace{Type: specs.UTSNamespace},
		)
	}

	return spec
}

// addCDIDevice adds the requested device to an existing spec.
func addCDIDevice(spec specs.Spec, cdiDevice string) (specs.Spec, error) {
	if !launcher.IsCDIDevice(cdiDevice) {
		return spec, fmt.Errorf("string %#v does not represent a valid CDI device", cdiDevice)
	}

	// Get the CDI registry, passing a cdi.WithAutoRefresh(false) option so that CDI registry files are not scanned asynchronously. (We are about to call a manual refresh, below.)
	registry := cdi.GetRegistry(cdi.WithAutoRefresh(false))

	// Refresh the CDI registry.
	if err := registry.Refresh(); err != nil {
		return spec, fmt.Errorf("Error encountered refreshing the CDI registry: %v", err)
	}

	_, err := registry.InjectDevices(&spec, cdiDevice)
	if err != nil {
		return spec, fmt.Errorf("Error encountered setting up CDI devices: %w", err)
	}

	return spec, nil
}
