// Copyright (c) 2024-2026, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package hooks

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	cchooks "github.com/containers/common/pkg/hooks/1.0.0"
	"github.com/opencontainers/runtime-spec/specs-go"
	"github.com/sylabs/singularity/v4/internal/pkg/buildcfg"
	"github.com/sylabs/singularity/v4/internal/pkg/util/fs"
	"github.com/sylabs/singularity/v4/pkg/sylog"
	"golang.org/x/sys/unix"
)

const nativeHooksDir = "native-hooks.d"

// NativeHook represents a hook that can be executed by the native runtime. It
// has the same basic structure as the containers/common current.Hook type, with the
// addition of a boolean 'Privileged' field that marks the hook for privileged
// escalation in the setuid flow, and a string 'Name' field.
type NativeHook struct {
	// Name for the hook, will be set to the hook filename if not specified.
	Name string `json:"name"`
	// Version of the hook structure - 1.0.0 is expected
	Version string `json:"version"`
	// Hook as an OCI runtime-spec Hook struct
	Hook specs.Hook `json:"hook"`
	// When the hook should run
	When cchooks.When `json:"when"`
	// Stages in the container lifecycle at which the hook should run
	Stages []string `json:"stages"`
	// Privileged indicates the hook should be executed only in setuid, and with escalated privilege.
	Privileged bool `json:"privileged"`
}

// Validate checks that a NativeHook is supported by the runtime. At present,
// only poststart hooks that run always are supported.
func (n *NativeHook) Validate() error {
	if n == nil {
		return fmt.Errorf("native hook is nil")
	}

	if n.Version != cchooks.Version {
		return fmt.Errorf("hook version %q is not supported (%q required)", n.Version, cchooks.Version)
	}

	if n.Hook.Path == "" {
		return fmt.Errorf("hook.path is required")
	}

	if n.When.Annotations != nil ||
		n.When.Commands != nil ||
		n.When.HasBindMounts != nil {
		return fmt.Errorf("when.annotations/commands/hasBindMounts conditions are not supported")
	}

	if n.When.Always == nil || !*n.When.Always {
		return fmt.Errorf("when.always must be true")
	}

	if len(n.Stages) != 1 || n.Stages[0] != "poststart" {
		return fmt.Errorf("only the poststart stage is supported, found %v", n.Stages)
	}

	return nil
}

// Run executes the NativeHook binary with configured args and env.
// If allowPrivileged is false, a hook with `Privileged=true` will refuse to run.
// If allowPrivileged is true, a hook with `Privileged=true` will be run as root.
// A JSON OCI runtime-spec state structure is passed over STDIN. A non-standard
// `SINGULARITY_CONTAINER_PID` env var is added for the convenience of hooks
// that would otherwise need to parse the PID from the state JSON. A
// `SINGULARITY_HOOK_PRIVILEGED` env var is set if the hook is being run with
// privilege.
func (n *NativeHook) Run(state specs.State, allowPrivileged bool) error {
	stdout, err := n.run(state, allowPrivileged)
	sylog.Debugf("Hook %q STDOUT: %s", n.Name, stdout)
	if exitError, ok := err.(*exec.ExitError); ok {
		sylog.Debugf("Hook %q STDERR: %s", n.Name, exitError.Stderr)
		return exitError
	}
	return err
}

func (n *NativeHook) run(state specs.State, allowPrivileged bool) ([]byte, error) {
	if n == nil {
		return nil, fmt.Errorf("native hook is nil")
	}
	if n.Privileged && !allowPrivileged {
		return nil, fmt.Errorf("attempted to run a privileged hook in non-privileged mode")
	}
	if n.Hook.Path == "" {
		return nil, fmt.Errorf("hook.path is required")
	}

	stateJSON, err := json.Marshal(state)
	if err != nil {
		return nil, fmt.Errorf("while encoding container state: %w", err)
	}
	stateReader := bytes.NewBuffer(stateJSON)

	cmd := exec.Command(n.Hook.Path, n.Hook.Args...)
	cmd.Env = n.Hook.Env
	cmd.Env = append(cmd.Env,
		"SINGULARITY_CONTAINER_PID="+strconv.Itoa(state.Pid),
		"SINGULARITY_CONTAINER="+state.Bundle)
	cmd.Stdin = stateReader
	cmd.Dir = "/"

	if n.Privileged {
		sylog.Debugf("Running %q with privilege", n.Name)
		cmd.Env = append(cmd.Env, "SINGULARITY_HOOK_PRIVILEGED=1")
		cmd.SysProcAttr = &syscall.SysProcAttr{
			Credential: &syscall.Credential{
				Uid: 0,
			},
		}
	}

	sylog.Debugf("Executing hook %q with path %q args %v", n.Name, cmd.Path, cmd.Args)
	return cmd.Output()
}

// LoadNativeHooks loads all hooks from JSON files in the configuration
// directory. If privileged is false, hooks that specify `Privileged: true` are
// skipped, and there are no ownership checks. If privileged is true, all hooks
// are loaded and must have `root:root` ownership.
func LoadNativeHooks(privileged bool) ([]NativeHook, error) {
	hookDir := filepath.Join(buildcfg.SINGULARITY_CONFDIR, nativeHooksDir)
	if _, err := os.Stat(hookDir); os.IsNotExist(err) {
		return nil, nil
	}
	return loadNativeHooks(hookDir, privileged)
}

func loadNativeHooks(hookDir string, privileged bool) ([]NativeHook, error) {
	loadedHooks := []NativeHook{}

	dir, err := os.OpenFile(hookDir, os.O_RDONLY|unix.O_NOFOLLOW|unix.O_DIRECTORY, 0)
	if err != nil {
		return nil, err
	}
	defer dir.Close()
	if privileged && (!fs.FileHasOwner(dir, 0) || !fs.FileHasGroup(dir, 0)) {
		return nil, fmt.Errorf("%q must be owned by root:root", hookDir)
	}

	des, err := dir.ReadDir(-1)
	if err != nil {
		return nil, err
	}

	for _, de := range des {
		if !de.Type().IsRegular() {
			continue
		}
		if !strings.HasSuffix(de.Name(), ".json") {
			continue
		}

		fd, err := unix.Openat(int(dir.Fd()), de.Name(), os.O_RDONLY|unix.O_NOFOLLOW, 0)
		if err != nil {
			return nil, err
		}
		f := os.NewFile(uintptr(fd), de.Name())
		defer f.Close()

		if privileged && (!fs.FileHasOwner(f, 0) || !fs.FileHasGroup(f, 0)) {
			return nil, fmt.Errorf("%q must be owned by root:root", f.Name())
		}

		nh, err := loadNativeHook(f)
		if err != nil {
			return nil, err
		}

		if nh.Privileged && !privileged {
			sylog.Debugf("Skipping native hook %q: privileged != %v", nh.Name, privileged)
			continue
		}

		loadedHooks = append(loadedHooks, *nh)
		sylog.Debugf("Loaded native hook %q (%s)", nh.Name, nh.Stages)
	}
	return loadedHooks, nil
}

func loadNativeHook(f *os.File) (*NativeHook, error) {
	hjson, err := io.ReadAll(f)
	if err != nil {
		return nil, fmt.Errorf("while reading %q: %w", f.Name(), err)
	}
	nh := NativeHook{}
	if err := json.Unmarshal(hjson, &nh); err != nil {
		return nil, err
	}
	if nh.Name == "" {
		nh.Name = filepath.Base(f.Name())
	}
	if err := nh.Validate(); err != nil {
		return nil, fmt.Errorf("while validating %q: %w", f.Name(), err)
	}
	return &nh, nil
}
