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
	"os"
	"strings"

	"github.com/containers/image/v5/types"
	"github.com/google/uuid"
	"github.com/sylabs/singularity/internal/pkg/buildcfg"
	"github.com/sylabs/singularity/internal/pkg/cache"
	"github.com/sylabs/singularity/internal/pkg/runtime/launcher"
	"github.com/sylabs/singularity/pkg/ocibundle/native"
	"github.com/sylabs/singularity/pkg/syfs"
	"github.com/sylabs/singularity/pkg/sylog"
	useragent "github.com/sylabs/singularity/pkg/util/user-agent"
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

// Exec will interactively execute a container via the runc low-level runtime.
// image is a reference to an OCI image, e.g. docker://ubuntu or oci:/tmp/mycontainer
func (l *Launcher) Exec(ctx context.Context, image string, cmd string, args []string, instanceName string) error {
	if instanceName != "" {
		return fmt.Errorf("%w: instanceName", ErrNotImplemented)
	}

	if cmd != "" {
		return fmt.Errorf("%w: cmd %v", ErrNotImplemented, cmd)
	}

	if len(args) > 0 {
		return fmt.Errorf("%w: args %v", ErrNotImplemented, args)
	}

	bundleDir, err := os.MkdirTemp("", "oci-bundle")
	if err != nil {
		return nil
	}
	defer func() {
		sylog.Debugf("Removing OCI bundle at: %s", bundleDir)
		if err := os.RemoveAll(bundleDir); err != nil {
			sylog.Errorf("Couldn't remove OCI bundle %s: %v", bundleDir, err)
		}
	}()

	sylog.Debugf("Creating OCI bundle at: %s", bundleDir)

	// TODO - propagate auth config
	sysCtx := &types.SystemContext{
		// OCIInsecureSkipTLSVerify: cp.b.Opts.NoHTTPS,
		// DockerAuthConfig:         cp.b.Opts.DockerAuthConfig,
		// DockerDaemonHost:         cp.b.Opts.DockerDaemonHost,
		OSChoice:                "linux",
		AuthFilePath:            syfs.DockerConf(),
		DockerRegistryUserAgent: useragent.Value(),
	}
	// if cp.b.Opts.NoHTTPS {
	//      cp.sysCtx.DockerInsecureSkipTLSVerify = types.NewOptionalBool(true)
	// }

	var imgCache *cache.Handle
	if !l.cfg.CacheDisabled {
		imgCache, err = cache.New(cache.Config{
			ParentDir: os.Getenv(cache.DirEnv),
		})
		if err != nil {
			return err
		}
	}

	b, err := native.New(
		native.OptBundlePath(bundleDir),
		native.OptImageRef(image),
		native.OptSysCtx(sysCtx),
		native.OptImgCache(imgCache),
	)
	if err != nil {
		return err
	}

	if err := b.Create(ctx, nil); err != nil {
		return err
	}

	id, err := uuid.NewRandom()
	if err != nil {
		return fmt.Errorf("while generating container id: %w", err)
	}
	return Run(ctx, id.String(), b.Path(), "")
}
