// Copyright (c) 2019-2023, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

// Package native implements a Launcher that will configure and launch a
// container with Singularity's own (native) runtime.
package native

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	lccgroups "github.com/opencontainers/runc/libcontainer/cgroups"
	"github.com/opencontainers/runtime-spec/specs-go"
	sifuser "github.com/sylabs/sif/v2/pkg/user"
	"github.com/sylabs/singularity/internal/pkg/buildcfg"
	"github.com/sylabs/singularity/internal/pkg/cgroups"
	"github.com/sylabs/singularity/internal/pkg/image/unpacker"
	"github.com/sylabs/singularity/internal/pkg/instance"
	"github.com/sylabs/singularity/internal/pkg/plugin"
	"github.com/sylabs/singularity/internal/pkg/runtime/engine/config/oci"
	"github.com/sylabs/singularity/internal/pkg/runtime/engine/config/oci/generate"
	"github.com/sylabs/singularity/internal/pkg/runtime/launcher"
	"github.com/sylabs/singularity/internal/pkg/security"
	"github.com/sylabs/singularity/internal/pkg/util/bin"
	"github.com/sylabs/singularity/internal/pkg/util/env"
	"github.com/sylabs/singularity/internal/pkg/util/fs"
	"github.com/sylabs/singularity/internal/pkg/util/gpu"
	"github.com/sylabs/singularity/internal/pkg/util/shell/interpreter"
	"github.com/sylabs/singularity/internal/pkg/util/starter"
	"github.com/sylabs/singularity/internal/pkg/util/user"
	imgutil "github.com/sylabs/singularity/pkg/image"
	clicallback "github.com/sylabs/singularity/pkg/plugin/callback/cli"
	singularitycallback "github.com/sylabs/singularity/pkg/plugin/callback/runtime/engine/singularity"
	"github.com/sylabs/singularity/pkg/runtime/engine/config"
	singularityConfig "github.com/sylabs/singularity/pkg/runtime/engine/singularity/config"
	"github.com/sylabs/singularity/pkg/sylog"
	"github.com/sylabs/singularity/pkg/util/bind"
	"github.com/sylabs/singularity/pkg/util/capabilities"
	"github.com/sylabs/singularity/pkg/util/cryptkey"
	"github.com/sylabs/singularity/pkg/util/namespaces"
	"github.com/sylabs/singularity/pkg/util/rlimit"
	"github.com/sylabs/singularity/pkg/util/singularityconf"
	"golang.org/x/sys/unix"
)

// Launcher will holds configuration for, and will launch a container using
// Singularity's own (native) runtime.
type Launcher struct {
	uid          uint32
	gid          uint32
	cfg          launcher.Options
	engineConfig *singularityConfig.EngineConfig
	generator    *generate.Generator
}

// NewLauncher returns a native.Launcher with an initial configuration set by opts.
func NewLauncher(opts ...launcher.Option) (*Launcher, error) {
	lo := launcher.Options{}
	for _, opt := range opts {
		if err := opt(&lo); err != nil {
			return nil, fmt.Errorf("%w", err)
		}
	}

	// Initialize empty default Singularity Engine and OCI configuration
	engineConfig := singularityConfig.NewConfig()
	engineConfig.File = singularityconf.GetCurrentConfig()
	if engineConfig.File == nil {
		return nil, fmt.Errorf("unable to get singularity configuration")
	}
	ociConfig := &oci.Config{}
	generator := generate.New(&ociConfig.Spec)
	engineConfig.OciConfig = ociConfig

	l := Launcher{
		uid:          uint32(os.Getuid()),
		gid:          uint32(os.Getgid()),
		cfg:          lo,
		engineConfig: engineConfig,
		generator:    generator,
	}

	return &l, nil
}

// Exec prepares an EngineConfig defining how a container should be launched, then calls the starter binary to execute it.
// This includes interactive containers, instances, and joining an existing instance.
//
//nolint:maintidx
func (l *Launcher) Exec(ctx context.Context, image string, process string, args []string, instanceName string) error {
	var err error

	// Native runtime expects command to execute as arg[0]
	args = append([]string{process}, args...)

	// Set arguments to pass to contained process.
	l.generator.SetProcessArgs(args)

	// NoEval means we will not shell evaluate args / env in action scripts and environment processing.
	// This replicates OCI behavior and differes from historic Singularity behavior.
	if l.cfg.NoEval {
		l.engineConfig.SetNoEval(true)
		l.generator.AddProcessEnv("SINGULARITY_NO_EVAL", "1")
	}

	// Set container Umask w.r.t. our own, before any umask manipulation happens.
	l.setUmask()

	// Get our effective uid and gid for container execution.
	// If root user requests a target uid, gid via --security options, handle them now.
	l.uid, l.gid, err = l.setTargetIDs()
	if err != nil {
		sylog.Fatalf("Could not configure target UID/GID: %s", err)
	}

	// Set image to run, or instance to join, and SINGULARITY_CONTAINER/SINGULARITY_NAME env vars.
	if err := l.setImageOrInstance(image, instanceName); err != nil {
		sylog.Errorf("While setting image/instance: %s", err)
	}

	// Overlay or writable image requested?
	l.engineConfig.SetOverlayImage(l.cfg.OverlayPaths)
	l.engineConfig.SetWritableImage(l.cfg.Writable)

	// Check key is available for encrypted image, if applicable.
	// If we are joining an instance, then any encrypted image is already mounted.
	if !l.engineConfig.GetInstanceJoin() {
		err = l.checkEncryptionKey()
		if err != nil {
			sylog.Fatalf("While checking container encryption: %s", err)
		}
	}

	// Will we use the suid starter? If not we need to force the user namespace.
	useSuid, forceUserNs := l.useSuid()
	if forceUserNs {
		l.cfg.Namespaces.User = true
	}

	// In the setuid workflow, set RLIMIT_STACK to its default value, keeping the
	// original value to restore it before executing the container process.
	if useSuid {
		soft, hard, err := rlimit.Get("RLIMIT_STACK")
		if err != nil {
			sylog.Warningf("can't retrieve stack size limit: %s", err)
		}
		l.generator.AddProcessRlimits("RLIMIT_STACK", hard, soft)
	}

	// Handle requested binds, fuse mounts.
	if err := l.setBinds(); err != nil {
		sylog.Fatalf("While setting bind mount configuration: %s", err)
	}
	if err := l.setFuseMounts(); err != nil {
		sylog.Fatalf("While setting FUSE mount configuration: %s", err)
	}

	// Set the home directory that should be effective in the container.
	if err := l.setHome(); err != nil {
		sylog.Fatalf("While setting home directory: %s", err)
	}
	// Allow user to disable the home mount via --no-home.
	l.engineConfig.SetNoHome(l.cfg.NoHome)
	// Allow user to disable binds via --no-mount.
	l.setNoMountFlags()

	// GPU configuration may add library bind to /.singularity.d/libs.
	// Note: --nvccli may implicitly add --writable-tmpfs, so handle that *after* GPUs.
	if err := l.SetGPUConfig(); err != nil {
		// We must fatal on error, as we are checking for correct ownership of nvidia-container-cli,
		// which is important to maintain security.
		sylog.Fatalf("While setting GPU configuration: %s", err)
	}

	// --writable-tmpfs is for an ephemeral overlay, doesn't make sense if also asking to write to image itself.
	if l.cfg.Writable && l.cfg.WritableTmpfs {
		sylog.Warningf("Disabling --writable-tmpfs flag, mutually exclusive with --writable")
		l.engineConfig.SetWritableTmpfs(false)
	} else {
		l.engineConfig.SetWritableTmpfs(l.cfg.WritableTmpfs)
	}

	// If proot is requested (we are running an unprivileged build, without userns) we must bind it
	// into the container /.singularity.d/libs.
	if l.cfg.Proot != "" && l.uid != 0 {
		sylog.Debugf("Binding proot from %s", l.cfg.Proot)
		l.engineConfig.AppendLibrariesPath(l.cfg.Proot)
	}

	// Additional user requested library binds into /.singularity.d/libs.
	l.engineConfig.AppendLibrariesPath(l.cfg.ContainLibs...)

	// Additional directory overrides.
	l.engineConfig.SetScratchDir(l.cfg.ScratchDirs)
	l.engineConfig.SetWorkdir(l.cfg.WorkDir)

	// Container networking configuration.
	l.engineConfig.SetNetwork(l.cfg.Network)
	l.engineConfig.SetDNS(l.cfg.DNS)
	l.engineConfig.SetNetworkArgs(l.cfg.NetworkArgs)

	// If user wants to set a hostname, it requires the UTS namespace.
	if l.cfg.Hostname != "" {
		// This is a sanity-check; actionPreRun in actions.go should have prevented this scenario from arising.
		if !l.cfg.Namespaces.UTS {
			return fmt.Errorf("internal error: trying to set hostname without UTS namespace")
		}

		l.engineConfig.SetHostname(l.cfg.Hostname)
	}

	// Set requested capabilities (effective for root, or if sysadmin has permitted to another user).
	l.engineConfig.SetAddCaps(l.cfg.AddCaps)
	l.engineConfig.SetDropCaps(l.cfg.DropCaps)

	// Custom --config file (only effective in non-setuid or as root).
	l.engineConfig.SetConfigurationFile(l.cfg.ConfigFile)

	// When running as root, the user can optionally allow setuid with container.
	err = launcher.WithPrivilege(l.cfg.AllowSUID, "--allow-setuid", func() error {
		l.engineConfig.SetAllowSUID(l.cfg.AllowSUID)
		return nil
	})
	if err != nil {
		sylog.Fatalf("Could not configure --allow-setuid: %s", err)
	}

	// When running as root, the user can optionally keep all privs in the container.
	err = launcher.WithPrivilege(l.cfg.KeepPrivs, "--keep-privs", func() error {
		l.engineConfig.SetKeepPrivs(l.cfg.KeepPrivs)
		return nil
	})
	if err != nil {
		sylog.Fatalf("Could not configure --keep-privs: %s", err)
	}

	// User can optionally force dropping all privs from root in the container.
	l.engineConfig.SetNoPrivs(l.cfg.NoPrivs)

	// Set engine --security options (selinux, apparmor, seccomp functionality).
	l.engineConfig.SetSecurity(l.cfg.SecurityOpts)

	// User can override shell used when entering container.
	l.engineConfig.SetShell(l.cfg.ShellPath)
	if l.cfg.ShellPath != "" {
		l.generator.AddProcessEnv("SINGULARITY_SHELL", l.cfg.ShellPath)
	}

	// Are we running with userns and subuid / subgid fakeroot functionality?
	l.engineConfig.SetFakeroot(l.cfg.Fakeroot)
	if l.cfg.Fakeroot {
		l.cfg.Namespaces.User = true
	}
	// Allow optional skipping of setgroups in --fakeroot mode.
	if l.cfg.NoSetgroups {
		if l.cfg.Fakeroot {
			l.engineConfig.SetNoSetgroups(l.cfg.NoSetgroups)
		} else {
			sylog.Warningf("--no-setgroups only applies to --fakeroot mode")
		}
	}

	l.setCgroups(instanceName)

	// --boot flag requires privilege, so check for this.
	err = launcher.WithPrivilege(l.cfg.Boot, "--boot", func() error { return nil })
	if err != nil {
		sylog.Fatalf("Could not configure --boot: %s", err)
	}

	// --containall or --boot infer --contain.
	if l.cfg.Contain || l.cfg.ContainAll || l.cfg.Boot {
		l.engineConfig.SetContain(true)
		// --containall infers PID/IPC isolation and a clean environment.
		if l.cfg.ContainAll {
			l.cfg.Namespaces.PID = true
			l.cfg.Namespaces.IPC = true
			l.cfg.CleanEnv = true
		}
	}

	// Setup instance specific configuration if required.
	if instanceName != "" {
		l.generator.AddProcessEnv("SINGULARITY_INSTANCE", instanceName)
		l.cfg.Namespaces.PID = true
		l.engineConfig.SetInstance(true)
		l.engineConfig.SetBootInstance(l.cfg.Boot)

		if useSuid && !l.cfg.Namespaces.User && launcher.HidepidProc() {
			return fmt.Errorf("hidepid option set on /proc mount, require 'hidepid=0' to start instance with setuid workflow")
		}

		_, err := instance.Get(instanceName, instance.SingSubDir)
		if err == nil {
			return fmt.Errorf("instance %s already exists", instanceName)
		}

		if l.cfg.Boot {
			l.cfg.Namespaces.UTS = true
			l.cfg.Namespaces.Net = true
			if len(l.cfg.Hostname) < 1 {
				l.engineConfig.SetHostname(instanceName)
			}
			if !l.cfg.KeepPrivs {
				l.engineConfig.SetDropCaps("CAP_SYS_BOOT,CAP_SYS_RAWIO")
			}
			l.generator.SetProcessArgs([]string{"/sbin/init"})
		}
	}

	// Set the required namespaces in the engine config.
	l.setNamespaces()
	// Set the container environment.
	if err := l.setEnv(ctx, args); err != nil {
		return fmt.Errorf("while setting environment: %s", err)
	}
	// Set the container process work directory.
	l.setProcessCwd()

	l.generator.AddProcessEnv("SINGULARITY_APPNAME", l.cfg.AppName)

	// Get image ready to run, if needed, via FUSE mount / extraction / image driver handling.
	if err := l.prepareImage(ctx, image); err != nil {
		return fmt.Errorf("while preparing image: %s", err)
	}

	// Call the starter binary using our prepared config.
	if l.engineConfig.GetInstance() {
		err = l.starterInstance(instanceName, useSuid)
	} else {
		err = l.starterInteractive(useSuid)
	}

	// Execution is finished.
	if err != nil {
		return fmt.Errorf("while executing starter: %s", err)
	}
	return nil
}

// setUmask saves the current umask, to be set for the process run in the container,
// unless the --no-umask option was specified.
// https://github.com/hpcng/singularity/issues/5214
func (l *Launcher) setUmask() {
	currMask := syscall.Umask(0o022)
	if !l.cfg.NoUmask {
		sylog.Debugf("Saving umask %04o for propagation into container", currMask)
		l.engineConfig.SetUmask(currMask)
		l.engineConfig.SetRestoreUmask(true)
	}
}

// setTargetIDs sets engine configuration for any requested target UID and GID (when run as root).
// The effective uid and gid we will run under are returned as uid and gid.
func (l *Launcher) setTargetIDs() (uid, gid uint32, err error) {
	// Start with our actual uid / gid as invoked
	uid = uint32(os.Getuid())
	gid = uint32(os.Getgid())

	// Identify requested uid/gif (if any) from --security options
	uidParam := security.GetParam(l.cfg.SecurityOpts, "uid")
	gidParam := security.GetParam(l.cfg.SecurityOpts, "gid")

	targetUID := 0
	targetGID := make([]int, 0)

	// If a target uid was requested, and we are root, handle that.
	err = launcher.WithPrivilege(uidParam != "", "uid security feature", func() error {
		u, err := strconv.ParseUint(uidParam, 10, 32)
		if err != nil {
			return fmt.Errorf("failed to parse provided UID: %w", err)
		}
		targetUID = int(u)
		uid = uint32(targetUID)

		l.engineConfig.SetTargetUID(targetUID)
		return nil
	})
	if err != nil {
		return 0, 0, err
	}

	// If any target gids were requested, and we are root, handle that.
	err = launcher.WithPrivilege(gidParam != "", "gid security feature", func() error {
		gids := strings.Split(gidParam, ":")
		for _, id := range gids {
			g, err := strconv.ParseUint(id, 10, 32)
			if err != nil {
				return fmt.Errorf("failed to parse provided GID: %w", err)
			}
			targetGID = append(targetGID, int(g))
		}
		if len(gids) > 0 {
			gid = uint32(targetGID[0])
		}

		l.engineConfig.SetTargetGID(targetGID)
		return nil
	})
	if err != nil {
		return 0, 0, err
	}

	// Return the effective uid, gid the container will run with
	return uid, gid, nil
}

// setImageOrInstance sets the image to start, or instance and it's image to be joined.
func (l *Launcher) setImageOrInstance(image string, name string) error {
	if strings.HasPrefix(image, "instance://") {
		if name != "" {
			return fmt.Errorf("starting an instance from another is not allowed")
		}
		instanceName := instance.ExtractName(image)
		file, err := instance.Get(instanceName, instance.SingSubDir)
		if err != nil {
			return err
		}
		l.cfg.Namespaces.User = file.UserNs
		l.generator.AddProcessEnv("SINGULARITY_CONTAINER", file.Image)
		l.generator.AddProcessEnv("SINGULARITY_NAME", filepath.Base(file.Image))
		l.generator.AddProcessEnv("SINGULARITY_INSTANCE", instanceName)
		l.engineConfig.SetImage(image)
		l.engineConfig.SetInstanceJoin(true)

		// If we are running non-root, join the instance cgroup now, as we
		// can't manipulate the ppid cgroup in the engine prepareInstanceJoinConfig().
		// This flow is only applicable with the systemd cgroups manager.
		if file.Cgroup && l.uid != 0 {
			if !l.engineConfig.File.SystemdCgroups {
				return fmt.Errorf("joining non-root instance with cgroups requires systemd as cgroups manager")
			}

			pid := os.Getpid()

			// First, we create a new systemd managed cgroup for ourselves. This is so that we will be
			// under a common user-owned ancestor, allowing us to move into the instance cgroup next.
			// See: https://www.kernel.org/doc/html/v4.18/admin-guide/cgroup-v2.html#delegation-containment
			sylog.Debugf("Adding process %d to sibling cgroup", pid)
			manager, err := cgroups.NewManagerWithSpec(&specs.LinuxResources{}, pid, "", true)
			if err != nil {
				return fmt.Errorf("couldn't create cgroup manager: %w", err)
			}
			cgPath, _ := manager.GetCgroupRelPath()
			sylog.Debugf("In sibling cgroup: %s", cgPath)

			// Now we should be under the user-owned service directory in the cgroupfs,
			// so we can move into the actual instance cgroup that we want.
			sylog.Debugf("Moving process %d to instance cgroup", pid)
			manager, err = cgroups.GetManagerForPid(file.Pid)
			if err != nil {
				return fmt.Errorf("couldn't create cgroup manager: %w", err)
			}
			if err := manager.AddProc(pid); err != nil {
				return fmt.Errorf("couldn't add process to instance cgroup: %w", err)
			}
			cgPath, _ = manager.GetCgroupRelPath()
			sylog.Debugf("In instance cgroup: %s", cgPath)
		}
	} else {
		abspath, err := filepath.Abs(image)
		l.generator.AddProcessEnv("SINGULARITY_CONTAINER", abspath)
		l.generator.AddProcessEnv("SINGULARITY_NAME", filepath.Base(abspath))
		if err != nil {
			return fmt.Errorf("failed to determine image absolute path for %s: %w", image, err)
		}
		l.engineConfig.SetImage(abspath)
	}
	return nil
}

// checkEncryptionKey verifies key material is available if the image is encrypted.
// Allows us to fail fast if required key material is not available / usable.
func (l *Launcher) checkEncryptionKey() error {
	sylog.Debugf("Checking for encrypted system partition")
	img, err := imgutil.Init(l.engineConfig.GetImage(), false)
	if err != nil {
		return fmt.Errorf("could not open image %s: %w", l.engineConfig.GetImage(), err)
	}

	part, err := img.GetRootFsPartition()
	if err != nil {
		return fmt.Errorf("while getting root filesystem in %s: %w", l.engineConfig.GetImage(), err)
	}

	if part.Type == imgutil.ENCRYPTSQUASHFS {
		sylog.Debugf("Encrypted container filesystem detected")

		if l.cfg.KeyInfo == nil {
			return fmt.Errorf("no key was provided, cannot access encrypted container")
		}

		plaintextKey, err := cryptkey.PlaintextKey(*l.cfg.KeyInfo, l.engineConfig.GetImage())
		if err != nil {
			sylog.Errorf("Please check you are providing the correct key for decryption")
			return fmt.Errorf("cannot decrypt %s: %w", l.engineConfig.GetImage(), err)
		}

		l.engineConfig.SetEncryptionKey(plaintextKey)
	}
	// don't defer this call as in all cases it won't be
	// called before execing starter, so it would leak the
	// image file descriptor to the container process
	img.File.Close()
	return nil
}

// useSuid checks whether to use the setuid starter binary, and if we need to force the user namespace.
func (l *Launcher) useSuid() (useSuid, forceUserNs bool) {
	// privileged installation by default
	useSuid = true
	// Are we already in a user namespace?
	insideUserNs, _ := namespaces.IsInsideUserNamespace(os.Getpid())
	// singularity was compiled with '--without-suid' option
	if buildcfg.SINGULARITY_SUID_INSTALL == 0 {
		useSuid = false

		if !l.cfg.Namespaces.User && l.uid != 0 {
			sylog.Verbosef("Unprivileged installation: using user namespace")
			l.cfg.Namespaces.User = true
		}
	}

	// use non privileged starter binary:
	// - if running as root
	// - if already running inside a user namespace
	// - if user namespace is requested
	// - if running as user and 'allow setuid = no' is set in singularity.conf
	if l.uid == 0 || insideUserNs || l.cfg.Namespaces.User || !l.engineConfig.File.AllowSetuid {
		useSuid = false

		// fallback to user namespace:
		// - for non root user with setuid installation and 'allow setuid = no'
		// - for root user without effective capability CAP_SYS_ADMIN
		if l.uid != 0 && buildcfg.SINGULARITY_SUID_INSTALL == 1 && !l.engineConfig.File.AllowSetuid {
			sylog.Verbosef("'allow setuid' set to 'no' by configuration, fallback to user namespace")
			l.cfg.Namespaces.User = true
		} else if l.uid == 0 && !l.cfg.Namespaces.User {
			caps, err := capabilities.GetProcessEffective()
			if err != nil {
				sylog.Fatalf("Could not get process effective capabilities: %s", err)
			}
			if caps&uint64(1<<unix.CAP_SYS_ADMIN) == 0 {
				sylog.Verbosef("Effective capability CAP_SYS_ADMIN is missing, fallback to user namespace")
				l.cfg.Namespaces.User = true
			}
		}
	}
	return useSuid, forceUserNs
}

// setBinds sets engine configuration for requested bind mounts.
func (l *Launcher) setBinds() error {
	// First get binds from -B/--bind and env var
	binds, err := bind.ParseBindPath(strings.Join(l.cfg.BindPaths, ","))
	if err != nil {
		return fmt.Errorf("while parsing bind path: %w", err)
	}
	// Now add binds from one or more --mount and env var.
	for _, m := range l.cfg.Mounts {
		bps, err := bind.ParseMountString(m)
		if err != nil {
			return fmt.Errorf("while parsing mount %q: %w", m, err)
		}
		binds = append(binds, bps...)
	}

	l.engineConfig.SetBindPath(binds)
	l.generator.AddProcessEnv("SINGULARITY_BIND", strings.Join(l.cfg.BindPaths, ","))
	return nil
}

// setFuseMounts sets engine configuration for requested FUSE mounts.
func (l *Launcher) setFuseMounts() error {
	if len(l.cfg.FuseMount) > 0 {
		/* If --fusemount is given, imply --pid */
		l.cfg.Namespaces.PID = true
		if err := l.engineConfig.SetFuseMount(l.cfg.FuseMount); err != nil {
			return fmt.Errorf("while setting fuse mount: %w", err)
		}
	}
	return nil
}

// Set engine flags to disable mounts, to allow overriding them if they are set true
// in the singularity.conf.
func (l *Launcher) setNoMountFlags() {
	skipBinds := []string{}
	for _, v := range l.cfg.NoMount {
		switch v {
		case "proc":
			l.engineConfig.SetNoProc(true)
		case "sys":
			l.engineConfig.SetNoSys(true)
		case "dev":
			l.engineConfig.SetNoDev(true)
		case "devpts":
			l.engineConfig.SetNoDevPts(true)
		case "home":
			l.engineConfig.SetNoHome(true)
		case "tmp":
			l.engineConfig.SetNoTmp(true)
		case "hostfs":
			l.engineConfig.SetNoHostfs(true)
		case "cwd":
			l.engineConfig.SetNoCwd(true)
		// All bind path singularity.conf entries
		case "bind-paths":
			skipBinds = append(skipBinds, "*")
		default:
			// Single bind path singularity.conf entry by abs path
			if filepath.IsAbs(v) {
				skipBinds = append(skipBinds, v)
				continue
			}
			sylog.Warningf("Ignoring unknown mount type '%s'", v)
		}
	}
	l.engineConfig.SetSkipBinds(skipBinds)
}

// setHome sets the correct home directory configuration for our circumstance.
// If it is not possible to mount a home directory then the mount will be disabled.
func (l *Launcher) setHome() error {
	l.engineConfig.SetCustomHome(l.cfg.CustomHome)
	// If we have fakeroot & the home flag has not been used then we have the standard
	// /root location for the root user $HOME in the container.
	// This doesn't count as a SetCustomHome(true), as we are mounting from the real
	// user's standard $HOME -> /root and we want to respect --contain not mounting
	// the $HOME in this case.
	// See https://github.com/sylabs/singularity/pull/5227
	if !l.cfg.CustomHome && l.cfg.Fakeroot {
		l.cfg.HomeDir = fmt.Sprintf("%s:/root", l.cfg.HomeDir)
	}
	// If we are running as sungularity as root, but requesting a target UID in the container,
	// handle set the home directory appropriately.
	targetUID := l.engineConfig.GetTargetUID()
	if l.cfg.CustomHome && targetUID != 0 {
		if targetUID > 500 {
			if pwd, err := user.GetPwUID(uint32(targetUID)); err == nil {
				sylog.Debugf("Target UID requested, set home directory to %s", pwd.Dir)
				l.cfg.HomeDir = pwd.Dir
				l.engineConfig.SetCustomHome(true)
			} else {
				sylog.Verbosef("Home directory for UID %d not found, home won't be mounted", targetUID)
				l.engineConfig.SetNoHome(true)
				l.cfg.HomeDir = "/"
			}
		} else {
			sylog.Verbosef("System UID %d requested, home won't be mounted", targetUID)
			l.engineConfig.SetNoHome(true)
			l.cfg.HomeDir = "/"
		}
	}

	// Handle any user request to override the home directory source/dest
	homeSlice := strings.Split(l.cfg.HomeDir, ":")
	if len(homeSlice) < 1 || len(homeSlice) > 2 {
		return fmt.Errorf("home argument has incorrect number of elements: %v", homeSlice)
	}
	l.engineConfig.SetHomeSource(homeSlice[0])
	if len(homeSlice) == 1 {
		l.engineConfig.SetHomeDest(homeSlice[0])
	} else {
		l.engineConfig.SetHomeDest(homeSlice[1])
	}
	return nil
}

// SetGPUConfig sets up EngineConfig entries for NV / ROCm usage, if requested.
func (l *Launcher) SetGPUConfig() error {
	if l.engineConfig.File.AlwaysUseNv && !l.cfg.NoNvidia {
		l.cfg.Nvidia = true
		sylog.Verbosef("'always use nv = yes' found in singularity.conf")
	}
	if l.engineConfig.File.AlwaysUseRocm && !l.cfg.NoRocm {
		l.cfg.Rocm = true
		sylog.Verbosef("'always use rocm = yes' found in singularity.conf")
	}

	if l.cfg.Nvidia && l.cfg.Rocm {
		sylog.Warningf("--nv and --rocm cannot be used together. Only --nv will be applied.")
	}

	if l.cfg.Nvidia {
		// If nvccli was not enabled by flag or config, drop down to legacy binds immediately
		if !l.engineConfig.File.UseNvCCLI && !l.cfg.NvCCLI {
			return l.setNVLegacyConfig()
		}

		// TODO: In privileged fakeroot mode we don't have the correct namespace context to run nvidia-container-cli
		// from  starter, so fall back to legacy NV handling until that workflow is refactored heavily.
		fakeRootPriv := l.cfg.Fakeroot && l.engineConfig.File.AllowSetuid && (buildcfg.SINGULARITY_SUID_INSTALL == 1)
		if !fakeRootPriv {
			return l.setNvCCLIConfig()
		}
		return fmt.Errorf("--fakeroot does not support --nvccli in set-uid installations")
	}

	if l.cfg.Rocm {
		return l.setRocmConfig()
	}
	return nil
}

// setNvCCLIConfig sets up EngineConfig entries for NVIDIA GPU configuration via nvidia-container-cli.
func (l *Launcher) setNvCCLIConfig() (err error) {
	sylog.Debugf("Using nvidia-container-cli for GPU setup")
	l.engineConfig.SetNvCCLI(true)

	if os.Getenv("NVIDIA_VISIBLE_DEVICES") == "" {
		if l.cfg.Contain || l.cfg.ContainAll {
			// When we use --contain we don't mount the NV devices by default in the nvidia-container-cli flow,
			// they must be mounted via specifying with`NVIDIA_VISIBLE_DEVICES`. This differs from the legacy
			// flow which mounts all GPU devices, always... so warn the user.
			sylog.Warningf("When using nvidia-container-cli with --contain NVIDIA_VISIBLE_DEVICES must be set or no GPUs will be available in container.")
		} else {
			// In non-contained mode set NVIDIA_VISIBLE_DEVICES="all" by default, so MIGs are available.
			// Otherwise there is a difference vs legacy GPU binding. See Issue #471.
			sylog.Infof("Setting 'NVIDIA_VISIBLE_DEVICES=all' to emulate legacy GPU binding.")
			os.Setenv("NVIDIA_VISIBLE_DEVICES", "all")
		}
	}

	// Pass NVIDIA_ env vars that will be converted to nvidia-container-cli options
	nvCCLIEnv := []string{}
	for _, e := range os.Environ() {
		if strings.HasPrefix(e, "NVIDIA_") {
			nvCCLIEnv = append(nvCCLIEnv, e)
		}
	}
	l.engineConfig.SetNvCCLIEnv(nvCCLIEnv)

	if l.cfg.Namespaces.User && !l.cfg.Writable {
		return fmt.Errorf("nvidia-container-cli requires --writable with user namespace/fakeroot")
	}
	if !l.cfg.Writable && !l.cfg.WritableTmpfs {
		sylog.Infof("Setting --writable-tmpfs (required by nvidia-container-cli)")
		l.cfg.WritableTmpfs = true
	}

	return nil
}

// setNvLegacyConfig sets up EngineConfig entries for NVIDIA GPU configuration via direct binds of configured bins/libs.
func (l *Launcher) setNVLegacyConfig() error {
	sylog.Debugf("Using legacy binds for nv GPU setup")
	l.engineConfig.SetNvLegacy(true)
	gpuConfFile := filepath.Join(buildcfg.SINGULARITY_CONFDIR, "nvliblist.conf")
	// bind persistenced socket if found
	ipcs, err := gpu.NvidiaIpcsPath()
	if err != nil {
		sylog.Warningf("While finding nv ipcs: %v", err)
	}
	libs, bins, err := gpu.NvidiaPaths(gpuConfFile)
	if err != nil {
		sylog.Warningf("While finding nv bind points: %v", err)
	}
	l.setGPUBinds(libs, bins, ipcs, "nv")
	return nil
}

// setRocmConfig sets up EngineConfig entries for ROCm GPU configuration via direct binds of configured bins/libs.
func (l *Launcher) setRocmConfig() error {
	sylog.Debugf("Using rocm GPU setup")
	l.engineConfig.SetRocm(true)
	gpuConfFile := filepath.Join(buildcfg.SINGULARITY_CONFDIR, "rocmliblist.conf")
	libs, bins, err := gpu.RocmPaths(gpuConfFile)
	if err != nil {
		sylog.Warningf("While finding ROCm bind points: %v", err)
	}
	l.setGPUBinds(libs, bins, []string{}, "nv")
	return nil
}

// setGPUBinds sets EngineConfig entries to bind the provided list of libs, bins, ipc files.
func (l *Launcher) setGPUBinds(libs, bins, ipcs []string, gpuPlatform string) {
	files := make([]string, len(bins)+len(ipcs))
	if len(files) == 0 {
		sylog.Warningf("Could not find any %s files on this host!", gpuPlatform)
	} else {
		if l.cfg.Writable {
			sylog.Warningf("%s files may not be bound with --writable", gpuPlatform)
		}
		for i, binary := range bins {
			usrBinBinary := filepath.Join("/usr/bin", filepath.Base(binary))
			files[i] = strings.Join([]string{binary, usrBinBinary}, ":")
		}
		for i, ipc := range ipcs {
			files[i+len(bins)] = ipc
		}
		l.engineConfig.SetFilesPath(files)
	}
	if len(libs) == 0 {
		sylog.Warningf("Could not find any %s libraries on this host!", gpuPlatform)
	} else {
		l.engineConfig.SetLibrariesPath(libs)
	}
}

// setNamespaces sets namespace configuration for the engine.
func (l *Launcher) setNamespaces() {
	// unprivileged installation could not use fakeroot
	// network because it requires a setuid installation
	// so we fallback to none
	if l.cfg.Namespaces.Net {
		if l.cfg.Fakeroot && l.cfg.Network != "none" {
			if buildcfg.SINGULARITY_SUID_INSTALL == 0 || !l.engineConfig.File.AllowSetuid {
				sylog.Warningf(
					"fakeroot with unprivileged installation or 'allow setuid = no' " +
						"could not use 'fakeroot' network, fallback to 'none' network",
				)
				l.engineConfig.SetNetwork("none")
			}
		}
		l.generator.AddOrReplaceLinuxNamespace("network", "")
	}
	if l.cfg.Namespaces.UTS {
		l.generator.AddOrReplaceLinuxNamespace("uts", "")
	}
	if l.cfg.Namespaces.PID {
		l.generator.AddOrReplaceLinuxNamespace("pid", "")
		l.engineConfig.SetNoInit(l.cfg.NoInit)
	}
	if l.cfg.Namespaces.IPC {
		l.generator.AddOrReplaceLinuxNamespace("ipc", "")
	}
	if l.cfg.Namespaces.User {
		l.generator.AddOrReplaceLinuxNamespace("user", "")
		if !l.cfg.Fakeroot {
			l.generator.AddLinuxUIDMapping(l.uid, l.uid, 1)
			l.generator.AddLinuxGIDMapping(l.gid, l.gid, 1)
		}
	}
}

// setEnv sets the environment for the container, from the host environment, glads, env-file.
func (l *Launcher) setEnv(ctx context.Context, args []string) error {
	if l.cfg.EnvFile != "" {
		currentEnv := append(
			os.Environ(),
			"SINGULARITY_IMAGE="+l.engineConfig.GetImage(),
		)

		content, err := os.ReadFile(l.cfg.EnvFile)
		if err != nil {
			return fmt.Errorf("could not read environment file %q: %w", l.cfg.EnvFile, err)
		}

		env, err := interpreter.EvaluateEnv(ctx, content, args, currentEnv)
		if err != nil {
			return fmt.Errorf("while processing %s: %w", l.cfg.EnvFile, err)
		}
		// --env variables will take precedence over variables
		// defined by the environment file
		sylog.Debugf("Setting environment variables from file %s", l.cfg.EnvFile)

		// Update Env with those from file
		for _, envar := range env {
			e := strings.SplitN(envar, "=", 2)
			if len(e) != 2 {
				sylog.Warningf("Ignored environment variable %q: '=' is missing", envar)
				continue
			}
			// Don't attempt to overwrite bash builtin readonly vars
			// https://github.com/sylabs/singularity/issues/1263
			if e[0] == "UID" || e[0] == "GID" {
				continue
			}
			// Ensure we don't overwrite --env variables with environment file
			if _, ok := l.cfg.Env[e[0]]; ok {
				sylog.Warningf("Ignored environment variable %s from %s: override from --env", e[0], l.cfg.EnvFile)
			} else {
				l.cfg.Env[e[0]] = e[1]
			}
		}
	}
	// process --env and --env-file variables for injection
	// into the environment by prefixing them with SINGULARITYENV_
	for envName, envValue := range l.cfg.Env {
		// We can allow envValue to be empty (explicit set to empty) but not name!
		if envName == "" {
			sylog.Warningf("Ignore environment variable %s=%s: variable name missing", envName, envValue)
			continue
		}
		os.Setenv("SINGULARITYENV_"+envName, envValue)
	}
	// Copy and cache environment
	environment := os.Environ()
	// Clean environment
	singularityEnv := env.SetContainerEnv(l.generator, environment, l.cfg.CleanEnv, l.engineConfig.GetHomeDest())
	l.engineConfig.SetSingularityEnv(singularityEnv)
	return nil
}

// setProcessCwd sets the container process working directory
func (l *Launcher) setProcessCwd() {
	if pwd, err := os.Getwd(); err == nil {
		l.engineConfig.SetCwd(pwd)
		if l.cfg.PwdPath != "" {
			l.generator.SetProcessCwd(l.cfg.PwdPath)
		} else {
			if l.engineConfig.GetContain() {
				l.generator.SetProcessCwd(l.engineConfig.GetHomeDest())
			} else {
				l.generator.SetProcessCwd(pwd)
			}
		}
	} else {
		sylog.Warningf("can't determine current working directory: %s", err)
	}
}

// setCgroups sets cgroup related configuration
func (l *Launcher) setCgroups(instanceName string) error {
	// If we are not root, we need to pass in XDG / DBUS environment so we can communicate
	// with systemd for any cgroups (v2) operations.
	if l.uid != 0 {
		sylog.Debugf("Recording rootless XDG_RUNTIME_DIR / DBUS_SESSION_BUS_ADDRESS")
		l.engineConfig.SetXdgRuntimeDir(os.Getenv("XDG_RUNTIME_DIR"))
		l.engineConfig.SetDbusSessionBusAddress(os.Getenv("DBUS_SESSION_BUS_ADDRESS"))
	}

	if l.cfg.CGroupsJSON != "" {
		// Handle cgroups configuration (parsed from file or flags in CLI).
		l.engineConfig.SetCgroupsJSON(l.cfg.CGroupsJSON)
		return nil
	}

	if instanceName == "" {
		return nil
	}

	// If we are an instance, always use a cgroup if possible, to enable stats.
	// root can always create a cgroup.
	useCG := l.uid == 0
	// non-root needs cgroups v2 unified mode + systemd as cgroups manager.
	if l.uid != 0 && lccgroups.IsCgroup2UnifiedMode() && l.engineConfig.File.SystemdCgroups {
		useCG = true
	}

	if useCG {
		cg := cgroups.Config{}
		cgJSON, err := cg.MarshalJSON()
		if err != nil {
			return err
		}
		l.engineConfig.SetCgroupsJSON(cgJSON)
		return nil
	}

	sylog.Infof("Instance stats will not be available - requires cgroups v2 with systemd as manager.")
	return nil
}

// PrepareImage perfoms any image preparation required before execution.
// This is currently limited to extraction or FUSE mount when using the user namespace,
// and activating any image driver plugins that might handle the image mount.
func (l *Launcher) prepareImage(c context.Context, image string) error {
	insideUserNs, _ := namespaces.IsInsideUserNamespace(os.Getpid())

	if l.cfg.SIFFUSE && !(l.cfg.Namespaces.User || insideUserNs) {
		sylog.Warningf("--sif-fuse is not supported without user namespace, ignoring.")
	}

	// convert image file to sandbox if we are using user
	// namespace or if we are currently running inside a
	// user namespace
	if (l.cfg.Namespaces.User || insideUserNs) && fs.IsFile(image) {
		convert := true
		// load image driver plugins
		if l.engineConfig.File.ImageDriver != "" { // nolint:staticcheck
			callbackType := (singularitycallback.RegisterImageDriver)(nil) // nolint:staticcheck
			callbacks, err := plugin.LoadCallbacks(callbackType)
			if err != nil {
				sylog.Debugf("Loading plugins callbacks '%T' failed: %s", callbackType, err)
			} else {
				for _, callback := range callbacks {
					if err := callback.(singularitycallback.RegisterImageDriver)(true); err != nil { // nolint:staticcheck
						sylog.Debugf("While registering image driver: %s", err)
					}
				}
			}
			driver := imgutil.GetDriver(l.engineConfig.File.ImageDriver) // nolint:staticcheck
			if driver != nil && driver.Features()&imgutil.ImageFeature != 0 {
				// the image driver indicates support for image so let's
				// proceed with the image driver without conversion
				convert = false
			}
		}

		if convert {
			tryFUSE := l.cfg.SIFFUSE || l.engineConfig.File.SIFFUSE
			if tryFUSE && l.cfg.Fakeroot {
				return fmt.Errorf("fakeroot is not currently supported with FUSE SIF mounts")
			}

			fuse, tempDir, imageDir, err := handleImage(c, image, tryFUSE)
			if err != nil {
				return fmt.Errorf("while handling %s: %w", image, err)
			}
			l.engineConfig.SetImage(imageDir)
			l.engineConfig.SetImageFuse(fuse)
			l.engineConfig.SetDeleteTempDir(tempDir)
			l.generator.AddProcessEnv("SINGULARITY_CONTAINER", imageDir)
			// if '--disable-cache' flag, then remove original SIF after converting to sandbox
			if l.cfg.CacheDisabled {
				sylog.Debugf("Removing tmp image: %s", image)
				err := os.Remove(image)
				if err != nil {
					return fmt.Errorf("unable to remove tmp image: %s: %w", image, err)
				}
			}
		}
	}
	return nil
}

// handleImage makes the image at filename available at directory dir within a
// temporary directory tempDir, by extraction or squashfuse mount. It is the
// caller's responsibility to remove tempDir when no longer needed. If isFUSE is
// returned true, then the imageDir is a FUSE mount, and must be unmounted
// during cleanup.
func handleImage(ctx context.Context, filename string, tryFUSE bool) (isFUSE bool, tempDir, imageDir string, err error) {
	img, err := imgutil.Init(filename, false)
	if err != nil {
		return false, "", "", fmt.Errorf("could not open image %s: %s", filename, err)
	}
	defer img.File.Close()

	part, err := img.GetRootFsPartition()
	if err != nil {
		return false, "", "", fmt.Errorf("while getting root filesystem in %s: %s", filename, err)
	}

	// Nice message if we have been given an older ext3 image, which cannot be extracted due to lack of privilege
	// to loopback mount.
	if part.Type == imgutil.EXT3 {
		sylog.Errorf("File %q is an ext3 format container image.", filename)
		sylog.Errorf("Only SIF and squashfs images can be extracted in unprivileged mode.")
		sylog.Errorf("Use `singularity build` to convert this image to a SIF file using a setuid install of Singularity.")
	}

	// Only squashfs can be extracted
	if part.Type != imgutil.SQUASHFS {
		return false, "", "", fmt.Errorf("not a squashfs root filesystem")
	}

	tempDir, imageDir, err = mkContainerDirs()
	if err != nil {
		return false, "", "", err
	}

	// Attempt squashfuse mount
	if tryFUSE {
		err := squashfuseMount(ctx, img, imageDir)
		if err == nil {
			return true, tempDir, imageDir, nil
		}
		sylog.Warningf("SIF squashfuse mount failed, falling back to extraction: %v", err)
	}

	// Fall back to extraction to directory
	err = extractImage(img, imageDir)
	if err == nil {
		return false, tempDir, imageDir, nil
	}

	if err2 := os.RemoveAll(tempDir); err2 != nil {
		sylog.Errorf("Couldn't remove temporary directory %s: %s", tempDir, err2)
	}
	return false, "", "", fmt.Errorf("while extracting image: %v", err)
}

// mkContainerDirs creates a tempDir, with a nested 'root' imageDir that an image can be placed into.
// The directory nesting is required so that extraction of an image doesn't apply permissions that
// cause the tempDir to be accessible to others.
func mkContainerDirs() (tempDir, imageDir string, err error) {
	// keep compatibility with v2
	tmpdir := os.Getenv("SINGULARITY_TMPDIR")
	if tmpdir == "" {
		tmpdir = os.Getenv("SINGULARITY_LOCALCACHEDIR")
		if tmpdir == "" {
			tmpdir = os.Getenv("SINGULARITY_CACHEDIR")
		}
	}

	// create temporary dir
	tempDir, err = os.MkdirTemp(tmpdir, "rootfs-")
	if err != nil {
		return "", "", fmt.Errorf("could not create temporary directory: %s", err)
	}
	defer func() {
		if err != nil {
			os.RemoveAll(tempDir)
		}
	}()

	// create an inner dir to extract to, so we don't clobber the secure permissions on the tmpDir.
	imageDir = filepath.Join(tempDir, "root")
	if err := os.Mkdir(imageDir, 0o755); err != nil {
		return "", "", fmt.Errorf("could not create root directory: %s", err)
	}

	return tempDir, imageDir, nil
}

// extractImage extracts img to directory dir within a temporary directory
// tempDir. It is the caller's responsibility to remove tempDir
// when no longer needed.
func extractImage(img *imgutil.Image, imageDir string) error {
	sylog.Infof("Converting SIF file to temporary sandbox...")
	unsquashfsPath, err := bin.FindBin("unsquashfs")
	if err != nil {
		return err
	}

	// create a reader for rootfs partition
	reader, err := imgutil.NewPartitionReader(img, "", 0)
	if err != nil {
		return fmt.Errorf("could not extract root filesystem: %s", err)
	}
	s := unpacker.NewSquashfs()
	if !s.HasUnsquashfs() && unsquashfsPath != "" {
		s.UnsquashfsPath = unsquashfsPath
	}

	// extract root filesystem
	if err := s.ExtractAll(reader, imageDir); err != nil {
		return fmt.Errorf("root filesystem extraction failed: %s", err)
	}

	return nil
}

// squashfuseMount mounts img using squashfuse to directory imageDir. It is the
// caller's responsibility to umount imageDir when no longer needed.
func squashfuseMount(ctx context.Context, img *imgutil.Image, imageDir string) (err error) {
	part, err := img.GetRootFsPartition()
	if err != nil {
		return fmt.Errorf("while getting root filesystem : %s", err)
	}
	if img.Type != imgutil.SIF && part.Type != imgutil.SQUASHFS {
		return fmt.Errorf("only SIF images are supported")
	}
	sylog.Infof("Mounting SIF with FUSE...")

	squashfusePath, err := bin.FindBin("squashfuse")
	if err != nil {
		return fmt.Errorf("squashfuse is required: %w", err)
	}
	if _, err := bin.FindBin("fusermount"); err != nil {
		return fmt.Errorf("fusermount is required: %w", err)
	}

	return sifuser.Mount(ctx, img.Path, imageDir,
		sifuser.OptMountStdout(os.Stdout),
		sifuser.OptMountStderr(os.Stderr),
		sifuser.OptMountSquashfusePath(squashfusePath))
}

// starterInteractive executes the starter binary to run an image interactively, given the supplied engineConfig
func (l *Launcher) starterInteractive(useSuid bool) error {
	loadOverlay := false
	if !l.cfg.Namespaces.User && buildcfg.SINGULARITY_SUID_INSTALL == 1 {
		loadOverlay = true
	}

	cfg := &config.Common{
		EngineName:   singularityConfig.Name,
		EngineConfig: l.engineConfig,
	}

	// Allow any plugins with callbacks to modify the assembled Config
	runPluginCallbacks(cfg)

	err := starter.Exec(
		"Singularity runtime parent",
		cfg,
		starter.UseSuid(useSuid),
		starter.LoadOverlayModule(loadOverlay),
		starter.CleanupHost(l.engineConfig.GetImageFuse()),
	)
	return err
}

// starterInstance executes the starter binary to run an instance given the supplied engineConfig
func (l *Launcher) starterInstance(name string, useSuid bool) error {
	cfg := &config.Common{
		EngineName:   singularityConfig.Name,
		ContainerID:  name,
		EngineConfig: l.engineConfig,
	}

	// Allow any plugins with callbacks to modify the assembled Config
	runPluginCallbacks(cfg)

	pwd, err := user.GetPwUID(uint32(os.Getuid()))
	if err != nil {
		return fmt.Errorf("failed to retrieve user information for UID %d: %w", os.Getuid(), err)
	}
	procname, err := instance.ProcName(name, pwd.Name)
	if err != nil {
		return err
	}

	stdout, stderr, err := instance.SetLogFile(name, int(l.uid), instance.LogSubDir)
	if err != nil {
		return fmt.Errorf("failed to create instance log files: %w", err)
	}

	start, err := stderr.Seek(0, io.SeekEnd)
	if err != nil {
		sylog.Warningf("failed to get standard error stream offset: %s", err)
	}

	loadOverlay := false
	if !l.cfg.Namespaces.User && buildcfg.SINGULARITY_SUID_INSTALL == 1 {
		loadOverlay = true
	}

	cmdErr := starter.Run(
		procname,
		cfg,
		starter.UseSuid(useSuid),
		starter.WithStdout(stdout),
		starter.WithStderr(stderr),
		starter.LoadOverlayModule(loadOverlay),
		starter.CleanupHost(l.engineConfig.GetImageFuse()),
	)

	if sylog.GetLevel() != 0 {
		// starter can exit a bit before all errors has been reported
		// by instance process, wait a bit to catch all errors
		time.Sleep(100 * time.Millisecond)

		end, err := stderr.Seek(0, io.SeekEnd)
		if err != nil {
			sylog.Warningf("failed to get standard error stream offset: %s", err)
		}
		if end-start > 0 {
			output := make([]byte, end-start)
			stderr.ReadAt(output, start)
			fmt.Println(string(output))
		}
	}

	if cmdErr != nil {
		return fmt.Errorf("failed to start instance: %w", cmdErr)
	}
	sylog.Verbosef("you will find instance output here: %s", stdout.Name())
	sylog.Verbosef("you will find instance error here: %s", stderr.Name())
	sylog.Infof("instance started successfully")

	return nil
}

// runPluginCallbacks executes any plugin callbacks to manipulate the engine config passed in
func runPluginCallbacks(cfg *config.Common) error {
	callbackType := (clicallback.SingularityEngineConfig)(nil)
	callbacks, err := plugin.LoadCallbacks(callbackType)
	if err != nil {
		return fmt.Errorf("while loading plugin callbacks '%T': %w", callbackType, err)
	}
	for _, c := range callbacks {
		c.(clicallback.SingularityEngineConfig)(cfg)
	}
	return nil
}
