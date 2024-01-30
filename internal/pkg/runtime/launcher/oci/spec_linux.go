// Copyright (c) 2022, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package oci

import (
	"fmt"
	"path/filepath"

	"github.com/opencontainers/runtime-spec/specs-go"

	"github.com/sylabs/singularity/v4/internal/pkg/runtime/launcher"
	"github.com/sylabs/singularity/v4/internal/pkg/util/rootless"
	"github.com/sylabs/singularity/v4/pkg/sylog"
)

// defaultNamespaces matching native runtime with --compat / --containall, except PID which can be disabled.
var defaultNamespaces = []specs.LinuxNamespace{
	{
		Type: specs.IPCNamespace,
	},
	{
		Type: specs.MountNamespace,
	},
}

// defaultResources - an empty set, no resource limits
var defaultResources = specs.LinuxResources{}

// minimalSpec returns an OCI runtime spec with a minimal OCI configuration that
// is a starting point for compatibility with Singularity's native launcher in
// `--compat` mode.
func minimalSpec() specs.Spec {
	config := specs.Spec{
		Version: specs.Version,
	}
	config.Root = &specs.Root{
		Path:     "rootfs",
		Readonly: false,
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

	// All mounts are added by the launcher, as it must handle flags.
	config.Mounts = []specs.Mount{}

	config.Linux = &specs.Linux{
		Namespaces: defaultNamespaces,
		// Ensure this is not nil to work around crun bug.
		// https://github.com/containers/crun/issues/1402
		Resources: &defaultResources,
	}
	return config
}

// addNamespaces adds requested namespace, if appropriate, to an existing spec.
// It is assumed that spec contains at least the defaultNamespaces.
func addNamespaces(spec *specs.Spec, ns launcher.Namespaces) error {
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
		sylog.Infof("--oci runtime uses a PID namespace by default, pid flag is redundant.")
	}

	if !ns.NoPID {
		spec.Linux.Namespaces = append(
			spec.Linux.Namespaces,
			specs.LinuxNamespace{Type: specs.PIDNamespace},
		)
	}

	if ns.User {
		uid, err := rootless.Getuid()
		if err != nil {
			return err
		}

		if uid == 0 {
			spec.Linux.Namespaces = append(
				spec.Linux.Namespaces,
				specs.LinuxNamespace{Type: specs.UserNamespace},
			)
		} else {
			sylog.Infof("The --oci runtime always creates a user namespace when run as non-root, --userns / -u flag is redundant.")
		}
	}

	if ns.UTS {
		spec.Linux.Namespaces = append(
			spec.Linux.Namespaces,
			specs.LinuxNamespace{Type: specs.UTSNamespace},
		)
	}

	return nil
}

// noSetgroupsAnnotation will set the `run.oci.keep_original_groups=1` annotation
// to disable the setgroups call when entering the container. Supported by crun, but not runc.
func noSetgroupsAnnotation(spec *specs.Spec) error {
	runtime, err := Runtime()
	if err != nil {
		return err
	}
	if filepath.Base(runtime) != "crun" {
		return fmt.Errorf("runtime '%q' does not support --no-setgroups", runtime)
	}

	if spec.Annotations == nil {
		spec.Annotations = map[string]string{}
	}
	spec.Annotations["run.oci.keep_original_groups"] = "1"
	return nil
}
