// Copyright (c) 2022, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package oci

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	imgspecv1 "github.com/opencontainers/image-spec/specs-go/v1"
	runtimespec "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/sylabs/singularity/v4/internal/pkg/runtime/launcher"
	"github.com/sylabs/singularity/v4/internal/pkg/util/rootless"
	"github.com/sylabs/singularity/v4/pkg/sylog"
)

// defaultNamespaces matching native runtime with --compat / --containall, except PID which can be disabled.
var defaultNamespaces = []runtimespec.LinuxNamespace{
	{
		Type: runtimespec.IPCNamespace,
	},
	{
		Type: runtimespec.MountNamespace,
	},
}

// defaultResources - an empty set, no resource limits
var defaultResources = runtimespec.LinuxResources{}

// minimalSpec returns an OCI runtime spec with a minimal OCI configuration that
// is a starting point for compatibility with Singularity's native launcher in
// `--compat` mode.
func minimalSpec() runtimespec.Spec {
	config := runtimespec.Spec{
		Version: runtimespec.Version,
	}
	config.Root = &runtimespec.Root{
		Path:     "rootfs",
		Readonly: false,
	}
	config.Process = &runtimespec.Process{
		Terminal: true,
		// Default fallback to a shell at / - will generally be overwritten by
		// the launcher.
		Args: []string{"sh"},
		Cwd:  "/",
	}
	config.Process.User = runtimespec.User{}
	config.Process.Env = []string{
		"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
	}

	// All mounts are added by the launcher, as it must handle flags.
	config.Mounts = []runtimespec.Mount{}

	config.Linux = &runtimespec.Linux{
		Namespaces: defaultNamespaces,
		// Ensure this is not nil to work around crun bug.
		// https://github.com/containers/crun/issues/1402
		Resources: &defaultResources,
	}
	return config
}

// addNamespaces adds requested namespace, if appropriate, to an existing spec.
// It is assumed that spec contains at least the defaultNamespaces.
func addNamespaces(spec *runtimespec.Spec, ns launcher.Namespaces) error {
	if ns.IPC {
		sylog.Infof("--oci runtime always uses an IPC namespace, ipc flag is redundant.")
	}

	// Currently supports only `--network none`, i.e. isolated loopback only.
	// Launcher.checkopts enforces this.
	if ns.Net {
		spec.Linux.Namespaces = append(
			spec.Linux.Namespaces,
			runtimespec.LinuxNamespace{Type: runtimespec.NetworkNamespace},
		)
	}

	if ns.PID {
		sylog.Infof("--oci runtime uses a PID namespace by default, pid flag is redundant.")
	}

	if !ns.NoPID {
		spec.Linux.Namespaces = append(
			spec.Linux.Namespaces,
			runtimespec.LinuxNamespace{Type: runtimespec.PIDNamespace},
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
				runtimespec.LinuxNamespace{Type: runtimespec.UserNamespace},
			)
		} else {
			sylog.Infof("The --oci runtime always creates a user namespace when run as non-root, --userns / -u flag is redundant.")
		}
	}

	if ns.UTS {
		spec.Linux.Namespaces = append(
			spec.Linux.Namespaces,
			runtimespec.LinuxNamespace{Type: runtimespec.UTSNamespace},
		)
	}

	return nil
}

// addAnnotations adds the required annotations to the runtime-spec, based on an image-spec.
// See https://github.com/opencontainers/image-spec/blob/main/conversion.md
func addAnnotations(rSpec *runtimespec.Spec, iSpec *imgspecv1.Image) error {
	if rSpec == nil {
		return fmt.Errorf("runtime spec is required")
	}
	if iSpec == nil {
		return nil
	}

	if rSpec.Annotations == nil {
		rSpec.Annotations = map[string]string{}
	}

	if iSpec.OS != "" {
		rSpec.Annotations["org.opencontainers.image.os"] = iSpec.OS
	}
	if iSpec.Architecture != "" {
		rSpec.Annotations["org.opencontainers.image.architecture"] = iSpec.Architecture
	}
	if iSpec.Variant != "" {
		rSpec.Annotations["org.opencontainers.image.variant"] = iSpec.Variant
	}
	if iSpec.OSVersion != "" {
		rSpec.Annotations["org.opencontainers.image.os.version"] = iSpec.OSVersion
	}
	if iSpec.OSFeatures != nil {
		rSpec.Annotations["org.opencontainers.image.os.features"] = strings.Join(iSpec.OSFeatures, ",")
	}
	if iSpec.Author != "" {
		rSpec.Annotations["org.opencontainers.image.author"] = iSpec.Author
	}
	if iSpec.Created != nil {
		rSpec.Annotations["org.opencontainers.image.created"] = iSpec.Created.Format(time.RFC3339)
	}
	if iSpec.Config.StopSignal != "" {
		rSpec.Annotations["org.opencontainers.image.stopSignal"] = iSpec.Config.StopSignal
	}

	// Note: Explicit Config.Labels take precedence over the above.
	for k, v := range iSpec.Config.Labels {
		rSpec.Annotations[k] = v
	}

	return nil
}

// noSetgroupsAnnotation will set the `run.oci.keep_original_groups=1` annotation
// to disable the setgroups call when entering the container. Supported by crun, but not runc.
func noSetgroupsAnnotation(rSpec *runtimespec.Spec) error {
	runtime, err := Runtime()
	if err != nil {
		return err
	}
	if filepath.Base(runtime) != "crun" {
		return fmt.Errorf("runtime '%q' does not support --no-setgroups", runtime)
	}

	if rSpec.Annotations == nil {
		rSpec.Annotations = map[string]string{}
	}
	rSpec.Annotations["run.oci.keep_original_groups"] = "1"
	return nil
}
