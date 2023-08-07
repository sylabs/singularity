// Copyright (c) 2022-2023, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package oci

import (
	"context"
	"fmt"
	"os"
	"strings"
	"syscall"

	imgspecv1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/opencontainers/runtime-spec/specs-go"
	"github.com/samber/lo"
	"github.com/sylabs/singularity/v4/internal/pkg/fakeroot"
	"github.com/sylabs/singularity/v4/internal/pkg/runtime/engine/config/oci"
	"github.com/sylabs/singularity/v4/internal/pkg/runtime/engine/config/oci/generate"
	"github.com/sylabs/singularity/v4/internal/pkg/runtime/launcher"
	"github.com/sylabs/singularity/v4/internal/pkg/util/env"
	"github.com/sylabs/singularity/v4/internal/pkg/util/shell/interpreter"
	"github.com/sylabs/singularity/v4/pkg/sylog"
	"github.com/sylabs/singularity/v4/pkg/util/capabilities"
	"golang.org/x/term"
)

const singularityLibs = "/.singularity.d/libs"

// Script that can be run by /bin/sh to emulate native mode shell behavior.
// Set Singularity> prompt, try bash --norc, fall back to sh.
var ociShellScript = "export PS1='Singularity> '; test -x /bin/bash && exec /bin/bash --norc || exec /bin/sh"

// getProcess creates and returns an specs.Process struct defining the execution behavior of the container.
// The userEnv map returned also holds all user-requested environment variables (i.e. not those from the image).
func (l *Launcher) getProcess(ctx context.Context, imgSpec imgspecv1.Image, bundle string, ep launcher.ExecParams, u specs.User) (process *specs.Process, userEnv map[string]string, err error) {
	// Assemble the runtime & user-requested environment, which will be merged
	// with the image ENV and set in the container at runtime.
	rtEnv := defaultEnv(ep.Image, bundle)

	// Propagate TERM from host. Doing this here means it can be overridden by SINGULARITYENV_TERM.
	hostTerm, isHostTermSet := os.LookupEnv("TERM")
	if isHostTermSet {
		rtEnv["TERM"] = hostTerm
	}

	// SINGULARITYENV_ has lowest priority
	rtEnv = mergeMap(rtEnv, singularityEnvMap())
	// --env-file can override SINGULARITYENV_
	if l.cfg.EnvFile != "" {
		e, err := envFileMap(ctx, l.cfg.EnvFile)
		if err != nil {
			return nil, nil, err
		}
		rtEnv = mergeMap(rtEnv, e)
	}
	// --env flag can override --env-file and SINGULARITYENV_
	rtEnv = mergeMap(rtEnv, l.cfg.Env)

	// Ensure HOME points to the required home directory, even if it is a custom one, unless the container explicitly specifies its USER, in which case we don't want to touch HOME.
	if imgSpec.Config.User == "" {
		rtEnv["HOME"] = l.homeDest
	}

	cwd, err := l.getProcessCwd()
	if err != nil {
		return nil, nil, err
	}

	// OCI default is NoNewPrivileges = false
	noNewPrivs := false
	// --no-privs sets NoNewPrivileges
	if l.cfg.NoPrivs {
		noNewPrivs = true
	}

	caps, err := l.getProcessCapabilities(u.UID)
	if err != nil {
		return nil, nil, err
	}

	var args []string
	switch {
	case l.nativeSIF:
		// Native SIF image must run via in-container action script
		args, err = ep.ActionScriptArgs()
		if err != nil {
			return nil, nil, fmt.Errorf("while getting ProcessArgs: %w", err)
		}
		sylog.Debugf("Native SIF container process/args: %v", args)
	case ep.Action == "shell":
		// OCI-SIF shell handling to emulate native runtime shell
		args = []string{"/bin/sh", "-c", ociShellScript}
	default:
		// OCI-SIF, inheriting from image config
		args = getProcessArgs(imgSpec, ep)
	}

	p := specs.Process{
		Args:            args,
		Capabilities:    caps,
		Cwd:             cwd,
		Env:             getProcessEnv(imgSpec, rtEnv, l.nativeSIF),
		NoNewPrivileges: noNewPrivs,
		User:            u,
		Terminal:        getProcessTerminal(),
	}

	return &p, rtEnv, nil
}

// getProcessTerminal determines whether the container process should run with a terminal.
func getProcessTerminal() bool {
	// Sets the default Process.Terminal to false if our stdin is not a terminal.
	return term.IsTerminal(syscall.Stdin)
}

// getProcessArgs returns the process args for a container, with reference to the OCI Image Spec.
// The process and image parameters may override the image CMD and/or ENTRYPOINT.
func getProcessArgs(imageSpec imgspecv1.Image, ep launcher.ExecParams) []string {
	var processArgs []string

	if ep.Process != "" {
		processArgs = []string{ep.Process}
	} else {
		processArgs = imageSpec.Config.Entrypoint
	}

	if len(ep.Args) > 0 {
		processArgs = append(processArgs, ep.Args...)
	} else {
		if ep.Process == "" {
			processArgs = append(processArgs, imageSpec.Config.Cmd...)
		}
	}
	return processArgs
}

// getProcessCwd computes the Cwd that the container process should start in.
// Default in OCI mode, like native --compat, is $HOME.
// In native emulation (--no-compat), we use the CWD.
// Can be overridden with a custom value via --cwd/pwd.

// Because this is called after mounts have already been computed, we can count on homeDest containing the right value, incorporating any custom home dir overrides (i.e., --home).
func (l *Launcher) getProcessCwd() (dir string, err error) {
	if len(l.cfg.CwdPath) > 0 {
		return l.cfg.CwdPath, nil
	}

	if l.cfg.NoCompat {
		return os.Getwd()
	}

	return l.homeDest, nil
}

// getReverseUserMaps returns uid and gid mappings that re-map container uid to target
// uid. This 'reverses' the host user to container root mapping in the initial
// userns from which the OCI runtime is launched.
//
//	e.g. host 1001 -> fakeroot userns 0 -> container targetUID
func getReverseUserMaps(hostUID, targetUID, targetGID uint32) (uidMap, gidMap []specs.LinuxIDMapping, err error) {
	// Get user's configured subuid & subgid ranges
	subuidRange, err := fakeroot.GetIDRange(fakeroot.SubUIDFile, hostUID)
	if err != nil {
		return nil, nil, err
	}
	// We must always be able to map at least 0->65535 inside the container, so we cover 'nobody'.
	if subuidRange.Size < 65536 {
		return nil, nil, fmt.Errorf("subuid range size (%d) must be at least 65536", subuidRange.Size)
	}
	subgidRange, err := fakeroot.GetIDRange(fakeroot.SubGIDFile, hostUID)
	if err != nil {
		return nil, nil, err
	}
	if subgidRange.Size < 65536 {
		return nil, nil, fmt.Errorf("subuid range size (%d) must be at least 65536", subgidRange.Size)
	}

	uidMap, gidMap = reverseMapByRange(targetUID, targetGID, *subuidRange, *subgidRange)
	return uidMap, gidMap, nil
}

func reverseMapByRange(targetUID, targetGID uint32, subuidRange, subgidRange specs.LinuxIDMapping) (uidMap, gidMap []specs.LinuxIDMapping) {
	if targetUID < subuidRange.Size {
		uidMap = []specs.LinuxIDMapping{
			{
				ContainerID: 0,
				HostID:      1,
				Size:        targetUID,
			},
			{
				ContainerID: targetUID,
				HostID:      0,
				Size:        1,
			},
			{
				ContainerID: targetUID + 1,
				HostID:      targetUID + 1,
				Size:        subuidRange.Size - targetUID,
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
				ContainerID: targetUID,
				HostID:      0,
				Size:        1,
			},
		}
	}

	if targetGID < subgidRange.Size {
		gidMap = []specs.LinuxIDMapping{
			{
				ContainerID: 0,
				HostID:      1,
				Size:        targetGID,
			},
			{
				ContainerID: targetGID,
				HostID:      0,
				Size:        1,
			},
			{
				ContainerID: targetGID + 1,
				HostID:      targetGID + 1,
				Size:        subgidRange.Size - targetGID,
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
				ContainerID: targetGID,
				HostID:      0,
				Size:        1,
			},
		}
	}

	return uidMap, gidMap
}

// getProcessEnv combines the image config ENV with the ENV requested at runtime.
// APPEND_PATH and PREPEND_PATH are honored as with the native singularity runtime.
// LD_LIBRARY_PATH is modified to always include the singularity lib bind directory.
func getProcessEnv(imageSpec imgspecv1.Image, runtimeEnv map[string]string, nativeSIF bool) []string {
	path := ""
	appendPath := ""
	prependPath := ""
	ldLibraryPath := ""

	// Start with the environment from the image config.
	g := generate.New(nil)
	g.Config.Process = &specs.Process{Env: imageSpec.Config.Env}

	// Obtain PATH, and LD_LIBRARY_PATH if set in the image config, for special handling.
	for _, env := range imageSpec.Config.Env {
		e := strings.SplitN(env, "=", 2)
		if len(e) < 2 {
			continue
		}
		// The image config PATH is not accurate for native SIF images - it is a
		// default, and a PATH may be declared in the image /.singularity.d/env
		// scripts. Ignore, so we can pick that up.
		if e[0] == "PATH" && !nativeSIF {
			path = e[1]
		}
		if e[0] == "LD_LIBRARY_PATH" {
			ldLibraryPath = e[1]
		}
	}

	// Apply env vars from runtime, except PATH and LD_LIBRARY_PATH releated.
	for k, v := range runtimeEnv {
		switch k {
		case "PATH":
			path = v
		case "APPEND_PATH":
			appendPath = v
		case "PREPEND_PATH":
			prependPath = v
		case "LD_LIBRARY_PATH":
			ldLibraryPath = v
		default:
			g.AddProcessEnv(k, v)
		}
	}

	// Handle PATH differently beteween OCI and native images
	if nativeSIF {
		setNativePath(g, prependPath, path, appendPath)
	} else {
		setOCIPath(g, prependPath, path, appendPath)
	}

	// Ensure LD_LIBRARY_PATH always contains singularity lib binding dir.
	// This is handled by environment scripts in native SIF images.
	if !nativeSIF && !strings.Contains(ldLibraryPath, singularityLibs) {
		ldLibraryPath = strings.TrimPrefix(ldLibraryPath+":"+singularityLibs, ":")
	}
	if ldLibraryPath != "" {
		g.AddProcessEnv("LD_LIBRARY_PATH", ldLibraryPath)
	}

	return g.Config.Process.Env
}

func setOCIPath(g *generate.Generator, prependPath, path, appendPath string) {
	// Compute and set optionally APPEND-ed / PREPEND-ed PATH.
	if appendPath != "" {
		path = strings.TrimSuffix(path, ":") + ":" + appendPath
	}
	if prependPath != "" {
		path = prependPath + ":" + strings.TrimPrefix(path, ":")
	}
	if path != "" {
		g.AddProcessEnv("PATH", path)
	}
}

func setNativePath(g *generate.Generator, prependPath, path, appendPath string) {
	// Set env vars used by Singularity env script to handle PATH.
	if prependPath != "" {
		g.AddProcessEnv("SING_USER_DEFINED_PREPEND_PATH", prependPath)
	}
	if path != "" {
		g.AddProcessEnv("SING_USER_DEFINED_PATH", path)
	}
	if appendPath != "" {
		g.AddProcessEnv("SING_USER_DEFINED_APPEND_PATH", appendPath)
	}
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

// getBaseCapabilities returns the capabilities that are enabled for the user
// prior to processing AddCaps / DropCaps.
func (l *Launcher) getBaseCapabilities() ([]string, error) {
	if l.cfg.NoPrivs {
		return []string{}, nil
	}

	if l.cfg.KeepPrivs {
		c, err := capabilities.GetProcessEffective()
		if err != nil {
			return nil, err
		}

		return capabilities.ToStrings(c), nil
	}

	return oci.DefaultCaps, nil
}

// getProcessCapabilities returns the capabilities that are enabled for the
// user, after applying all capabilities related options.
func (l *Launcher) getProcessCapabilities(targetUID uint32) (*specs.LinuxCapabilities, error) {
	caps, err := l.getBaseCapabilities()
	if err != nil {
		return nil, err
	}

	addCaps, ignoredCaps := capabilities.Split(l.cfg.AddCaps)
	if len(ignoredCaps) > 0 {
		sylog.Warningf("Ignoring unknown --add-caps: %s", strings.Join(ignoredCaps, ","))
	}

	dropCaps, ignoredCaps := capabilities.Split(l.cfg.DropCaps)
	if len(ignoredCaps) > 0 {
		sylog.Warningf("Ignoring unknown --drop-caps: %s", strings.Join(ignoredCaps, ","))
	}

	caps = append(caps, addCaps...)
	caps = capabilities.RemoveDuplicated(caps)
	caps = lo.Without(caps, dropCaps...)

	// If root inside the container, Permitted==Effective==Bounding.
	if targetUID == 0 {
		return &specs.LinuxCapabilities{
			Permitted:   caps,
			Effective:   caps,
			Bounding:    caps,
			Inheritable: []string{},
			Ambient:     []string{},
		}, nil
	}

	// If non-root inside the container, Permitted/Effective/Inheritable/Ambient
	// are only the explicitly requested capabilities.
	explicitCaps := lo.Without(addCaps, dropCaps...)
	return &specs.LinuxCapabilities{
		Permitted:   explicitCaps,
		Effective:   explicitCaps,
		Bounding:    caps,
		Inheritable: explicitCaps,
		Ambient:     explicitCaps,
	}, nil
}
