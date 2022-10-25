// Copyright (c) 2022, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

// Package oci implements a Launcher that will configure and launch a container
// with an OCI runtime. It also provides implementations of OCI state
// transitions that can be called directly, Create/Start/Kill etc.
package oci

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/sylabs/singularity/internal/pkg/buildcfg"
	"github.com/sylabs/singularity/internal/pkg/runtime/launcher"
)

var (
	ErrUnsupportedOption = errors.New("not supported by OCI launcher")
	ErrNotImplemented    = errors.New("not implemented by OCI launcher")
)

// Launcher will holds configuration for, and will launch a container using an
// OCI runtime.
type Launcher struct {
	cfg launcher.Options
}

// NewLauncher returns a oci.Launcher with an initial configuration set by opts.
func NewLauncher(opts ...launcher.Option) (*Launcher, error) {
	lo := launcher.Options{}
	for _, opt := range opts {
		if err := opt(&lo); err != nil {
			return nil, fmt.Errorf("%w", err)
		}
	}

	if err := checkOpts(lo); err != nil {
		return nil, err
	}
	return &Launcher{lo}, nil
}

// checkOpts ensures that options set are supported by the oci.Launcher.
//
// nolint:maintidx
func checkOpts(lo launcher.Options) error {
	badOpt := []string{}

	if lo.Writable {
		badOpt = append(badOpt, "Writable")
	}
	if lo.WritableTmpfs {
		badOpt = append(badOpt, "WritableTmpfs")
	}
	if len(lo.OverlayPaths) > 0 {
		badOpt = append(badOpt, "OverlayPaths")
	}
	if len(lo.ScratchDirs) > 0 {
		badOpt = append(badOpt, "ScratchDirs")
	}
	if lo.WorkDir != "" {
		badOpt = append(badOpt, "WorkDir")
	}

	// Home is always sent from the CLI, and must be valid as an option, but
	// CustomHome signifies if it was a user specified value which we don't
	// support (yet).
	if lo.CustomHome {
		badOpt = append(badOpt, "CustomHome")
	}
	if lo.NoHome {
		badOpt = append(badOpt, "NoHome")
	}

	if len(lo.BindPaths) > 0 {
		badOpt = append(badOpt, "BindPaths")
	}
	if len(lo.FuseMount) > 0 {
		badOpt = append(badOpt, "FuseMount")
	}
	if len(lo.Mounts) > 0 {
		badOpt = append(badOpt, "Mounts")
	}
	if len(lo.NoMount) > 0 {
		badOpt = append(badOpt, "NoMount")
	}

	if lo.Nvidia {
		badOpt = append(badOpt, "Nvidia")
	}
	if lo.NvCCLI {
		badOpt = append(badOpt, "NvCCLI")
	}
	if lo.NoNvidia {
		badOpt = append(badOpt, "NoNvidia")
	}
	if lo.Rocm {
		badOpt = append(badOpt, "Rocm")
	}
	if lo.NoRocm {
		badOpt = append(badOpt, "NoRocm")
	}

	if len(lo.ContainLibs) > 0 {
		badOpt = append(badOpt, "ContainLibs")
	}
	if lo.Proot != "" {
		badOpt = append(badOpt, "Proot")
	}

	if len(lo.Env) > 0 {
		badOpt = append(badOpt, "Env")
	}
	if lo.EnvFile != "" {
		badOpt = append(badOpt, "EnvFile")
	}
	if lo.CleanEnv {
		badOpt = append(badOpt, "CleanEnv")
	}
	if lo.NoEval {
		badOpt = append(badOpt, "NoEval")
	}

	if lo.Namespaces.IPC {
		badOpt = append(badOpt, "Namespaces.IPC")
	}
	if lo.Namespaces.Net {
		badOpt = append(badOpt, "Namespaces.Net")
	}
	if lo.Namespaces.PID {
		badOpt = append(badOpt, "Namespaces.PID")
	}
	if lo.Namespaces.UTS {
		badOpt = append(badOpt, "Namespaces.UTS")
	}
	if lo.Namespaces.User {
		badOpt = append(badOpt, "Namespaces.User")
	}

	// Network always set in CLI layer even if network namespace not requested.
	if lo.Namespaces.Net && lo.Network != "" {
		badOpt = append(badOpt, "Network")
	}

	if len(lo.NetworkArgs) > 0 {
		badOpt = append(badOpt, "NetworkArgs")
	}
	if lo.Hostname != "" {
		badOpt = append(badOpt, "Hostname")
	}
	if lo.DNS != "" {
		badOpt = append(badOpt, "DNS")
	}

	if lo.AddCaps != "" {
		badOpt = append(badOpt, "AddCaps")
	}
	if lo.DropCaps != "" {
		badOpt = append(badOpt, "DropCaps")
	}
	if lo.AllowSUID {
		badOpt = append(badOpt, "AllowSUID")
	}
	if lo.KeepPrivs {
		badOpt = append(badOpt, "KeepPrivs")
	}
	if lo.NoPrivs {
		badOpt = append(badOpt, "NoPrivs")
	}
	if len(lo.SecurityOpts) > 0 {
		badOpt = append(badOpt, "SecurityOpts")
	}
	if lo.NoUmask {
		badOpt = append(badOpt, "NoUmask")
	}

	if lo.CGroupsJSON != "" {
		badOpt = append(badOpt, "CGroupsJSON")
	}

	// ConfigFile always set by CLI. We should support only the default from build time.
	if lo.ConfigFile != "" && lo.ConfigFile != buildcfg.SINGULARITY_CONF_FILE {
		badOpt = append(badOpt, "ConfigFile")
	}

	if lo.ShellPath != "" {
		badOpt = append(badOpt, "ShellPath")
	}
	if lo.PwdPath != "" {
		badOpt = append(badOpt, "PwdPath")
	}

	if lo.Fakeroot {
		badOpt = append(badOpt, "Fakeroot")
	}
	if lo.Boot {
		badOpt = append(badOpt, "Boot")
	}
	if lo.NoInit {
		badOpt = append(badOpt, "NoInit")
	}
	if lo.Contain {
		badOpt = append(badOpt, "Contain")
	}
	if lo.ContainAll {
		badOpt = append(badOpt, "ContainAll")
	}

	if lo.AppName != "" {
		badOpt = append(badOpt, "AppName")
	}

	if lo.KeyInfo != nil {
		badOpt = append(badOpt, "KeyInfo")
	}

	if lo.SIFFUSE {
		badOpt = append(badOpt, "SIFFUSE")
	}
	if lo.CacheDisabled {
		badOpt = append(badOpt, "CacheDisabled")
	}

	if len(badOpt) > 0 {
		return fmt.Errorf("%w: %s", ErrUnsupportedOption, strings.Join(badOpt, ","))
	}

	return nil
}

// Exec is not yet implemented.
func (l *Launcher) Exec(ctx context.Context, image string, args []string, instanceName string) error {
	return ErrNotImplemented
}
