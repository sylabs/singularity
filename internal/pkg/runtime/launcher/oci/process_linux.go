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
	"github.com/sylabs/singularity/internal/pkg/fakeroot"
	"github.com/sylabs/singularity/internal/pkg/runtime/engine/config/oci/generate"
	"github.com/sylabs/singularity/internal/pkg/util/env"
	"github.com/sylabs/singularity/internal/pkg/util/shell/interpreter"
	"golang.org/x/term"
)

const singularityLibs = "/.singularity.d/libs"

func (l *Launcher) getProcess(ctx context.Context, imgSpec imgspecv1.Image, image, bundle, process string, args []string, u specs.User) (*specs.Process, error) {
	// Assemble the runtime & user-requested environment, which will be merged
	// with the image ENV and set in the container at runtime.
	rtEnv := defaultEnv(image, bundle)

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
			return nil, err
		}
		rtEnv = mergeMap(rtEnv, e)
	}
	// --env flag can override --env-file and SINGULARITYENV_
	rtEnv = mergeMap(rtEnv, l.cfg.Env)

	// Ensure HOME points to the required home directory, even if it is a custom one, unless the container explicitly specifies its USER, in which case we don't want to touch HOME.
	if imgSpec.Config.User == "" {
		rtEnv["HOME"] = l.cfg.HomeDir
	}

	cwd, err := l.getProcessCwd()
	if err != nil {
		return nil, err
	}

	p := specs.Process{
		Args:     getProcessArgs(imgSpec, process, args),
		Cwd:      cwd,
		Env:      getProcessEnv(imgSpec, rtEnv),
		User:     u,
		Terminal: getProcessTerminal(),
	}

	return &p, nil
}

// getProcessTerminal determines whether the container process should run with a terminal.
func getProcessTerminal() bool {
	// Sets the default Process.Terminal to false if our stdin is not a terminal.
	return term.IsTerminal(syscall.Stdin)
}

// getProcessArgs returns the process args for a container, with reference to the OCI Image Spec.
// The process and image parameters may override the image CMD and/or ENTRYPOINT.
func getProcessArgs(imageSpec imgspecv1.Image, process string, args []string) []string {
	var processArgs []string

	if process != "" {
		processArgs = []string{process}
	} else {
		processArgs = imageSpec.Config.Entrypoint
	}

	if len(args) > 0 {
		processArgs = append(processArgs, args...)
	} else {
		if process == "" {
			processArgs = append(processArgs, imageSpec.Config.Cmd...)
		}
	}

	return processArgs
}

// getProcessCwd computes the Cwd that the container process should start in.
// Currently this is the user's tmpfs home directory (see --containall).
// Because this is called after mounts have already been computed, we can count on l.cfg.HomeDir containing the right value, incorporating any custom home dir overrides (i.e., --home).
func (l *Launcher) getProcessCwd() (dir string, err error) {
	if len(l.cfg.CwdPath) > 0 {
		return l.cfg.CwdPath, nil
	}

	return l.cfg.HomeDir, nil
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
func getProcessEnv(imageSpec imgspecv1.Image, runtimeEnv map[string]string) []string {
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
		if e[0] == "PATH" {
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

	// Compute and set optionally APPEND-ed / PREPEND-ed PATH.
	if appendPath != "" {
		path = path + ":" + appendPath
	}
	if prependPath != "" {
		path = prependPath + ":" + path
	}
	if path != "" {
		g.AddProcessEnv("PATH", path)
	}

	// Ensure LD_LIBRARY_PATH always contains singularity lib binding dir.
	if !strings.Contains(ldLibraryPath, singularityLibs) {
		ldLibraryPath = strings.TrimPrefix(ldLibraryPath+":"+singularityLibs, ":")
	}
	g.AddProcessEnv("LD_LIBRARY_PATH", ldLibraryPath)

	return g.Config.Process.Env
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
