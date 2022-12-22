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
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/containers/image/v5/types"
	"github.com/google/uuid"
	"github.com/opencontainers/runtime-spec/specs-go"
	"github.com/sylabs/singularity/internal/pkg/buildcfg"
	"github.com/sylabs/singularity/internal/pkg/cache"
	"github.com/sylabs/singularity/internal/pkg/cgroups"
	"github.com/sylabs/singularity/internal/pkg/runtime/launcher"
	"github.com/sylabs/singularity/internal/pkg/util/fs/files"
	"github.com/sylabs/singularity/internal/pkg/util/user"
	"github.com/sylabs/singularity/pkg/ocibundle/native"
	"github.com/sylabs/singularity/pkg/ocibundle/tools"
	"github.com/sylabs/singularity/pkg/syfs"
	"github.com/sylabs/singularity/pkg/sylog"
	"github.com/sylabs/singularity/pkg/util/singularityconf"
	useragent "github.com/sylabs/singularity/pkg/util/user-agent"
	"golang.org/x/term"
)

var (
	ErrUnsupportedOption = errors.New("not supported by OCI launcher")
	ErrNotImplemented    = errors.New("not implemented by OCI launcher")
)

// Launcher will holds configuration for, and will launch a container using an
// OCI runtime.
type Launcher struct {
	cfg             launcher.Options
	singularityConf *singularityconf.File
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

	c := singularityconf.GetCurrentConfig()
	if c == nil {
		return nil, fmt.Errorf("singularity configuration is not initialized")
	}

	return &Launcher{cfg: lo, singularityConf: c}, nil
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

	if len(lo.FuseMount) > 0 {
		badOpt = append(badOpt, "FuseMount")
	}

	if len(lo.NoMount) > 0 {
		badOpt = append(badOpt, "NoMount")
	}

	if lo.NvCCLI {
		badOpt = append(badOpt, "NvCCLI")
	}

	if len(lo.ContainLibs) > 0 {
		badOpt = append(badOpt, "ContainLibs")
	}
	if lo.Proot != "" {
		badOpt = append(badOpt, "Proot")
	}

	if lo.CleanEnv {
		badOpt = append(badOpt, "CleanEnv")
	}
	if lo.NoEval {
		badOpt = append(badOpt, "NoEval")
	}

	// Network always set in CLI layer even if network namespace not requested.
	// We only support isolation at present
	if lo.Namespaces.Net && lo.Network != "none" {
		badOpt = append(badOpt, "Network (except none)")
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

// createSpec produces an OCI runtime specification, suitable to launch a
// container. This spec excludes ProcessArgs and Env, as these have to be
// computed where the image config is available, to account for the image's CMD
// / ENTRYPOINT / ENV.
func (l *Launcher) createSpec() (*specs.Spec, error) {
	spec := minimalSpec()

	// Override the default Process.Terminal to false if our stdin is not a terminal.
	if !term.IsTerminal(syscall.Stdin) {
		spec.Process.Terminal = false
	}

	spec.Process.User = l.getProcessUser()

	// If we are *not* requesting fakeroot, then we need to map the container
	// uid back to host uid, through the initial fakeroot userns.
	if !l.cfg.Fakeroot && os.Getuid() != 0 {
		uidMap, gidMap, err := l.getReverseUserMaps()
		if err != nil {
			return nil, err
		}
		spec.Linux.UIDMappings = uidMap
		spec.Linux.GIDMappings = gidMap
	}

	spec = addNamespaces(spec, l.cfg.Namespaces)

	cwd, err := l.getProcessCwd()
	if err != nil {
		return nil, err
	}
	spec.Process.Cwd = cwd

	mounts, err := l.getMounts()
	if err != nil {
		return nil, err
	}
	spec.Mounts = mounts

	cgPath, resources, err := l.getCgroup()
	if err != nil {
		return nil, err
	}
	if cgPath != "" {
		spec.Linux.CgroupsPath = cgPath
		spec.Linux.Resources = resources
	}

	return &spec, nil
}

func (l *Launcher) updatePasswdGroup(rootfs string) error {
	uid := os.Getuid()
	gid := os.Getgid()

	if os.Getuid() == 0 || l.cfg.Fakeroot {
		return nil
	}

	containerPasswd := filepath.Join(rootfs, "etc", "passwd")
	containerGroup := filepath.Join(rootfs, "etc", "group")

	pw, err := user.CurrentOriginal()
	if err != nil {
		return err
	}

	sylog.Debugf("Updating passwd file: %s", containerPasswd)
	content, err := files.Passwd(containerPasswd, pw.Dir, uid)
	if err != nil {
		return fmt.Errorf("while creating passwd file: %w", err)
	}
	if err := os.WriteFile(containerPasswd, content, 0o755); err != nil {
		return fmt.Errorf("while writing passwd file: %w", err)
	}

	sylog.Debugf("Updating group file: %s", containerGroup)
	content, err = files.Group(containerGroup, uid, []int{gid})
	if err != nil {
		return fmt.Errorf("while creating group file: %w", err)
	}
	if err := os.WriteFile(containerGroup, content, 0o755); err != nil {
		return fmt.Errorf("while writing passwd file: %w", err)
	}

	return nil
}

// Exec will interactively execute a container via the runc low-level runtime.
// image is a reference to an OCI image, e.g. docker://ubuntu or oci:/tmp/mycontainer
func (l *Launcher) Exec(ctx context.Context, image string, process string, args []string, instanceName string) error {
	if instanceName != "" {
		return fmt.Errorf("%w: instanceName", ErrNotImplemented)
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

	spec, err := l.createSpec()
	if err != nil {
		return fmt.Errorf("while creating OCI spec: %w", err)
	}

	// Assemble the runtime & user-requested environment, which will be merged
	// with the image ENV and set in the container at runtime.
	rtEnv := defaultEnv(image, bundleDir)
	// SINGULARITYENV_ has lowest priority
	rtEnv = mergeMap(rtEnv, singularityEnvMap())
	// --env-file can override SINGULARITYENV_
	if l.cfg.EnvFile != "" {
		e, err := envFileMap(ctx, l.cfg.EnvFile)
		if err != nil {
			return err
		}
		rtEnv = mergeMap(rtEnv, e)
	}
	// --env flag can override --env-file and SINGULARITYENV_
	rtEnv = mergeMap(rtEnv, l.cfg.Env)

	b, err := native.New(
		native.OptBundlePath(bundleDir),
		native.OptImageRef(image),
		native.OptSysCtx(sysCtx),
		native.OptImgCache(imgCache),
		native.OptProcessArgs(process, args),
		native.OptProcessEnv(rtEnv),
	)
	if err != nil {
		return err
	}

	if err := b.Create(ctx, spec); err != nil {
		return err
	}

	if err := l.updatePasswdGroup(tools.RootFs(b.Path()).Path()); err != nil {
		return err
	}

	id, err := uuid.NewRandom()
	if err != nil {
		return fmt.Errorf("while generating container id: %w", err)
	}

	if os.Getuid() == 0 {
		// Direct execution of runc/crun run.
		err = Run(ctx, id.String(), b.Path(), "", l.singularityConf.SystemdCgroups)
	} else {
		// Reexec singularity oci run in a userns with mappings.
		// Note - the oci run command will pull out the SystemdCgroups setting from config.
		err = RunNS(ctx, id.String(), b.Path(), "")
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		os.Exit(exitErr.ExitCode())
	}
	return err
}

// getCgroup will return a cgroup path and resources for the runtime to create.
func (l *Launcher) getCgroup() (path string, resources *specs.LinuxResources, err error) {
	if l.cfg.CGroupsJSON == "" {
		return "", nil, nil
	}
	path = cgroups.DefaultPathForPid(l.singularityConf.SystemdCgroups, -1)
	resources, err = cgroups.UnmarshalJSONResources(l.cfg.CGroupsJSON)
	if err != nil {
		return "", nil, err
	}
	return path, resources, nil
}

func mergeMap(a map[string]string, b map[string]string) map[string]string {
	for k, v := range b {
		a[k] = v
	}
	return a
}
