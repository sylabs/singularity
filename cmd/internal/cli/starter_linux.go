// Copyright (c) 2019-2022, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package cli

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

	"github.com/spf13/cobra"
	sifuser "github.com/sylabs/sif/v2/pkg/user"
	"github.com/sylabs/singularity/internal/pkg/buildcfg"
	"github.com/sylabs/singularity/internal/pkg/cgroups"
	"github.com/sylabs/singularity/internal/pkg/image/unpacker"
	"github.com/sylabs/singularity/internal/pkg/instance"
	"github.com/sylabs/singularity/internal/pkg/plugin"
	"github.com/sylabs/singularity/internal/pkg/runtime/engine/config/oci"
	"github.com/sylabs/singularity/internal/pkg/runtime/engine/config/oci/generate"
	"github.com/sylabs/singularity/internal/pkg/security"
	"github.com/sylabs/singularity/internal/pkg/util/bin"
	"github.com/sylabs/singularity/internal/pkg/util/env"
	"github.com/sylabs/singularity/internal/pkg/util/fs"
	"github.com/sylabs/singularity/internal/pkg/util/gpu"
	"github.com/sylabs/singularity/internal/pkg/util/shell/interpreter"
	"github.com/sylabs/singularity/internal/pkg/util/starter"
	"github.com/sylabs/singularity/internal/pkg/util/user"
	"github.com/sylabs/singularity/pkg/image"
	imgutil "github.com/sylabs/singularity/pkg/image"
	clicallback "github.com/sylabs/singularity/pkg/plugin/callback/cli"
	singularitycallback "github.com/sylabs/singularity/pkg/plugin/callback/runtime/engine/singularity"
	"github.com/sylabs/singularity/pkg/runtime/engine/config"
	singularityConfig "github.com/sylabs/singularity/pkg/runtime/engine/singularity/config"
	"github.com/sylabs/singularity/pkg/sylog"
	"github.com/sylabs/singularity/pkg/util/capabilities"
	"github.com/sylabs/singularity/pkg/util/cryptkey"
	"github.com/sylabs/singularity/pkg/util/fs/proc"
	"github.com/sylabs/singularity/pkg/util/namespaces"
	"github.com/sylabs/singularity/pkg/util/rlimit"
	"github.com/sylabs/singularity/pkg/util/singularityconf"
	"golang.org/x/sys/unix"
)

// execStarter prepares an EngineConfig defining how a container should be executed, then calls the starter binary to execute it.
// This includes interactive containers, instances, and joining an existing instance.
//
//nolint:maintidx
func execStarter(cobraCmd *cobra.Command, image string, args []string, instanceName string) {
	var err error

	// Initialize a new configuration for the engine.
	engineConfig := singularityConfig.NewConfig()
	engineConfig.File = singularityconf.GetCurrentConfig()
	if engineConfig.File == nil {
		sylog.Fatalf("Unable to get singularity configuration")
	}
	ociConfig := &oci.Config{}
	generator := generate.New(&ociConfig.Spec)
	engineConfig.OciConfig = ociConfig

	// Set arguments to pass to contained process.
	generator.SetProcessArgs(args)

	// NoEval means we will not shell evaluate args / env in action scripts and environment processing.
	// This replicates OCI behavior and differes from historic Singularity behavior.
	if NoEval {
		engineConfig.SetNoEval(true)
		generator.AddProcessEnv("SINGULARITY_NO_EVAL", "1")
	}

	// Set container Umask w.r.t. our own, before any umask manipulation happens.
	setUmask(engineConfig)

	// Get our effective uid and gid for container execution.
	// If root user requests a target uid, gid via --security options, handle them now.
	uid, gid, err := setTargetIDs(engineConfig)
	if err != nil {
		sylog.Fatalf("Could not configure target UID/GID: %s", err)
	}

	// Set image to run, or instance to join, and SINGULARITY_CONTAINER/SINGULARITY_NAME env vars.
	if err := setImageOrInstance(image, instanceName, uid, engineConfig, generator); err != nil {
		sylog.Errorf("While setting image/instance: %s", err)
	}

	// Overlay or writable image requested?
	engineConfig.SetOverlayImage(OverlayPath)
	engineConfig.SetWritableImage(IsWritable)
	// --writable-tmpfs is for an ephemeral overlay, doesn't make sense if also asking to write to image itself.
	if IsWritable && IsWritableTmpfs {
		sylog.Warningf("Disabling --writable-tmpfs flag, mutually exclusive with --writable")
		engineConfig.SetWritableTmpfs(false)
	} else {
		engineConfig.SetWritableTmpfs(IsWritableTmpfs)
	}

	// Check key is available for encrypted image, if applicable.
	err = checkEncryptionKey(cobraCmd, engineConfig)
	if err != nil {
		sylog.Fatalf("While checking container encryption: %s", err)
	}

	// Will we use the suid starter? If not we need to force the user namespace.
	useSuid, forceUserNs := useSuid(uid, engineConfig)
	if forceUserNs {
		UserNamespace = true
	}

	// In the setuid workflow, set RLIMIT_STACK to its default value, keeping the
	// original value to restore it before executing the container process.
	if useSuid {
		soft, hard, err := rlimit.Get("RLIMIT_STACK")
		if err != nil {
			sylog.Warningf("can't retrieve stack size limit: %s", err)
		}
		generator.AddProcessRlimits("RLIMIT_STACK", hard, soft)
	}

	// Handle requested binds, fuse mounts.
	if err := setBinds(engineConfig, generator); err != nil {
		sylog.Fatalf("While setting bind mount configuration: %s", err)
	}
	if err := setFuseMounts(engineConfig); err != nil {
		sylog.Fatalf("While setting FUSE mount configuration: %s", err)
	}

	// Set the home directory that should be effective in the container.
	customHome := cobraCmd.Flag("home").Changed
	if err := setHome(customHome, engineConfig); err != nil {
		sylog.Fatalf("While setting home directory: %s", err)
	}
	// Allow user to disable the home mount via --no-home.
	engineConfig.SetNoHome(NoHome)
	// Allow user to disable binds via --no-mount.
	setNoMountFlags(engineConfig)

	// GPU configuration may add library bind to /.singularity.d/libs.
	if err := SetGPUConfig(engineConfig); err != nil {
		// We must fatal on error, as we are checking for correct ownership of nvidia-container-cli,
		// which is important to maintain security.
		sylog.Fatalf("While setting GPU configuration: %s", err)
	}

	// If proot is requested (we are running an unprivileged build, without userns) we must bind it
	// into the container /.singularity.d/libs.
	if Proot != "" && uid != 0 {
		sylog.Debugf("Binding proot from %s", Proot)
		engineConfig.AppendLibrariesPath(Proot)
	}

	// Additional user requested library binds into /.singularity.d/libs.
	engineConfig.AppendLibrariesPath(ContainLibsPath...)

	// Additional directory overrides.
	engineConfig.SetScratchDir(ScratchPath)
	engineConfig.SetWorkdir(WorkdirPath)

	// Container networking configuration.
	engineConfig.SetNetwork(Network)
	engineConfig.SetDNS(DNS)
	engineConfig.SetNetworkArgs(NetworkArgs)

	// If user wants to set a hostname, it requires the UTS namespace.
	if Hostname != "" {
		UtsNamespace = true
		engineConfig.SetHostname(Hostname)
	}

	// Set requested capabilities (effective for root, or if sysadmin has permitted to another user).
	engineConfig.SetAddCaps(AddCaps)
	engineConfig.SetDropCaps(DropCaps)

	// Custom --config file (only effective in non-setuid or as root).
	engineConfig.SetConfigurationFile(configurationFile)

	// When running as root, the user can optionally allow setuid with container.
	err = withPrivilege(AllowSUID, "--allow-setuid", func() error {
		engineConfig.SetAllowSUID(AllowSUID)
		return nil
	})
	if err != nil {
		sylog.Fatalf("Could not configure --allow-setuid: %s", err)
	}

	// When running as root, the user can optionally keep all privs in the container.
	err = withPrivilege(KeepPrivs, "--keep-privs", func() error {
		engineConfig.SetKeepPrivs(KeepPrivs)
		return nil
	})
	if err != nil {
		sylog.Fatalf("Could not configure --keep-privs: %s", err)
	}

	// User can optionally force dropping all privs from root in the container.
	engineConfig.SetNoPrivs(NoPrivs)

	// Set engine --security options (selinux, apparmor, seccomp functionality).
	engineConfig.SetSecurity(Security)

	// User can override shell used when entering container.
	engineConfig.SetShell(ShellPath)
	if ShellPath != "" {
		generator.AddProcessEnv("SINGULARITY_SHELL", ShellPath)
	}

	// Are we running with userns and subuid / subgid fakeroot functionality?
	engineConfig.SetFakeroot(IsFakeroot)
	if IsFakeroot {
		UserNamespace = true
	}

	// If we are not root, we need to pass in XDG / DBUS environment so we can communicate
	// with systemd for any cgroups (v2) operations.
	if uid != 0 {
		sylog.Debugf("Recording rootless XDG_RUNTIME_DIR / DBUS_SESSION_BUS_ADDRESS")
		engineConfig.SetXdgRuntimeDir(os.Getenv("XDG_RUNTIME_DIR"))
		engineConfig.SetDbusSessionBusAddress(os.Getenv("DBUS_SESSION_BUS_ADDRESS"))
	}

	// Handle cgroups configuration (from limit flags, or provided conf file).
	cgJSON, err := getCgroupsJSON()
	if err != nil {
		sylog.Fatalf("While parsing cgroups configuration: %s", err)
	}
	engineConfig.SetCgroupsJSON(cgJSON)

	// --boot flag requires privilege, so check for this.
	err = withPrivilege(IsBoot, "--boot", func() error { return nil })
	if err != nil {
		sylog.Fatalf("Could not configure --boot: %s", err)
	}

	// --containall or --boot infer --contain.
	if IsContained || IsContainAll || IsBoot {
		engineConfig.SetContain(true)
		// --containall infers PID/IPC isolation and a clean environment.
		if IsContainAll {
			PidNamespace = true
			IpcNamespace = true
			IsCleanEnv = true
		}
	}

	// Setup instance specific configuration if required.
	if instanceName != "" {
		PidNamespace = true
		engineConfig.SetInstance(true)
		engineConfig.SetBootInstance(IsBoot)

		if useSuid && !UserNamespace && hidepidProc() {
			sylog.Fatalf("hidepid option set on /proc mount, require 'hidepid=0' to start instance with setuid workflow")
		}

		_, err := instance.Get(instanceName, instance.SingSubDir)
		if err == nil {
			sylog.Fatalf("instance %s already exists", instanceName)
		}

		if IsBoot {
			UtsNamespace = true
			NetNamespace = true
			if Hostname == "" {
				engineConfig.SetHostname(instanceName)
			}
			if !KeepPrivs {
				engineConfig.SetDropCaps("CAP_SYS_BOOT,CAP_SYS_RAWIO")
			}
			generator.SetProcessArgs([]string{"/sbin/init"})
		}
	}

	// Set the required namespaces in the engine config.
	setNamespaces(uid, gid, engineConfig, generator)
	// Set the container environment.
	if err := setEnv(args, engineConfig, generator); err != nil {
		sylog.Fatalf("While setting environment: %s", err)
	}
	// Set the container process work directory.
	setProcessCwd(engineConfig, generator)

	generator.AddProcessEnv("SINGULARITY_APPNAME", AppName)

	// Get image ready to run, if needed, via FUSE mount / extraction / image driver handling.
	if err := prepareImage(image, cobraCmd, engineConfig, generator); err != nil {
		sylog.Fatalf("While preparing image: %s", err)
	}

	// Call the starter binary using our prepared config.
	if engineConfig.GetInstance() {
		err = starterInstance(instanceName, uid, useSuid, engineConfig)
	} else {
		err = starterInteractive(useSuid, engineConfig)
	}

	// Execution is finished.
	if err != nil {
		sylog.Fatalf("While executing starter: %s", err)
	}
}

// setUmask saves the current umask, to be set for the process run in the container,
// unless the --no-umask option was specified.
// https://github.com/hpcng/singularity/issues/5214
func setUmask(engineConfig *singularityConfig.EngineConfig) {
	currMask := syscall.Umask(0o022)
	if !NoUmask {
		sylog.Debugf("Saving umask %04o for propagation into container", currMask)
		engineConfig.SetUmask(currMask)
		engineConfig.SetRestoreUmask(true)
	}
}

// setTargetIDs sets engine configuration for any requested target UID and GID (when run as root).
// The effective uid and gid we will run under are returned as uid and gid.
func setTargetIDs(engineConfig *singularityConfig.EngineConfig) (uid, gid uint32, err error) {
	// Start with our actual uid / gid as invoked
	uid = uint32(os.Getuid())
	gid = uint32(os.Getgid())

	// Identify requested uid/gif (if any) from --security options
	uidParam := security.GetParam(Security, "uid")
	gidParam := security.GetParam(Security, "gid")

	targetUID := 0
	targetGID := make([]int, 0)

	// If a target uid was requested, and we are root, handle that.
	err = withPrivilege(uidParam != "", "uid security feature", func() error {
		u, err := strconv.ParseUint(uidParam, 10, 32)
		if err != nil {
			return fmt.Errorf("failed to parse provided UID: %w", err)
		}
		targetUID = int(u)
		uid = uint32(targetUID)

		engineConfig.SetTargetUID(targetUID)
		return nil
	})
	if err != nil {
		return 0, 0, err
	}

	// If any target gids were requested, and we are root, handle that.
	err = withPrivilege(gidParam != "", "gid security feature", func() error {
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

		engineConfig.SetTargetGID(targetGID)
		return nil
	})
	if err != nil {
		return 0, 0, err
	}

	// Return the effective uid, gid the container will run with
	return uid, gid, nil
}

// setImageOrInstance sets the image to start, or instance and it's image to be joined.
func setImageOrInstance(image string, name string, uid uint32, engineConfig *singularityConfig.EngineConfig, generator *generate.Generator) error {
	if strings.HasPrefix(image, "instance://") {
		if name != "" {
			return fmt.Errorf("Starting an instance from another is not allowed")
		}
		instanceName := instance.ExtractName(image)
		file, err := instance.Get(instanceName, instance.SingSubDir)
		if err != nil {
			return err
		}
		UserNamespace = file.UserNs
		generator.AddProcessEnv("SINGULARITY_CONTAINER", file.Image)
		generator.AddProcessEnv("SINGULARITY_NAME", filepath.Base(file.Image))
		engineConfig.SetImage(image)
		engineConfig.SetInstanceJoin(true)

		// If we are running non-root, without a user ns, join the instance cgroup now, as we
		// can't manipulate the ppid cgroup in the engine
		// prepareInstanceJoinConfig().
		//
		// TODO - consider where /proc/sys/fs/cgroup is mounted in the engine
		// flow, to move this further down.
		if file.Cgroup && uid != 0 && !UserNamespace {
			pid := os.Getpid()
			sylog.Debugf("Adding process %d to instance cgroup", pid)
			manager, err := cgroups.GetManagerForPid(file.Pid)
			if err != nil {
				return fmt.Errorf("couldn't create cgroup manager: %w", err)
			}
			if err := manager.AddProc(pid); err != nil {
				return fmt.Errorf("couldn't add process to instance cgroup: %w", err)
			}
		}
	} else {
		abspath, err := filepath.Abs(image)
		generator.AddProcessEnv("SINGULARITY_CONTAINER", abspath)
		generator.AddProcessEnv("SINGULARITY_NAME", filepath.Base(abspath))
		if err != nil {
			return fmt.Errorf("Failed to determine image absolute path for %s: %w", image, err)
		}
		engineConfig.SetImage(abspath)
	}
	return nil
}

// checkEncryptionKey verifies key material is available if the image is encrypted.
// Allows us to fail fast if required key material is not available / usable.
func checkEncryptionKey(cobraCmd *cobra.Command, engineConfig *singularityConfig.EngineConfig) error {
	if !engineConfig.GetInstanceJoin() {
		sylog.Debugf("Checking for encrypted system partition")
		img, err := imgutil.Init(engineConfig.GetImage(), false)
		if err != nil {
			return fmt.Errorf("could not open image %s: %w", engineConfig.GetImage(), err)
		}

		part, err := img.GetRootFsPartition()
		if err != nil {
			return fmt.Errorf("while getting root filesystem in %s: %w", engineConfig.GetImage(), err)
		}

		if part.Type == imgutil.ENCRYPTSQUASHFS {
			sylog.Debugf("Encrypted container filesystem detected")

			keyInfo, err := getEncryptionMaterial(cobraCmd)
			if err != nil {
				return fmt.Errorf("Cannot load key for decryption: %w", err)
			}

			plaintextKey, err := cryptkey.PlaintextKey(keyInfo, engineConfig.GetImage())
			if err != nil {
				sylog.Errorf("Please check you are providing the correct key for decryption")
				return fmt.Errorf("Cannot decrypt %s: %w", engineConfig.GetImage(), err)
			}

			engineConfig.SetEncryptionKey(plaintextKey)
		}
		// don't defer this call as in all cases it won't be
		// called before execing starter, so it would leak the
		// image file descriptor to the container process
		img.File.Close()
	}
	return nil
}

// useSuid checks whether to use the setuid starter binary, and if we need to force the user namespace.
func useSuid(uid uint32, engineConfig *singularityConfig.EngineConfig) (useSuid, forceUserNs bool) {
	// privileged installation by default
	useSuid = true
	// Are we already in a user namespace?
	insideUserNs, _ := namespaces.IsInsideUserNamespace(os.Getpid())
	// singularity was compiled with '--without-suid' option
	if buildcfg.SINGULARITY_SUID_INSTALL == 0 {
		useSuid = false

		if !UserNamespace && uid != 0 {
			sylog.Verbosef("Unprivileged installation: using user namespace")
			UserNamespace = true
		}
	}

	// use non privileged starter binary:
	// - if running as root
	// - if already running inside a user namespace
	// - if user namespace is requested
	// - if running as user and 'allow setuid = no' is set in singularity.conf
	if uid == 0 || insideUserNs || UserNamespace || !engineConfig.File.AllowSetuid {
		useSuid = false

		// fallback to user namespace:
		// - for non root user with setuid installation and 'allow setuid = no'
		// - for root user without effective capability CAP_SYS_ADMIN
		if uid != 0 && buildcfg.SINGULARITY_SUID_INSTALL == 1 && !engineConfig.File.AllowSetuid {
			sylog.Verbosef("'allow setuid' set to 'no' by configuration, fallback to user namespace")
			UserNamespace = true
		} else if uid == 0 && !UserNamespace {
			caps, err := capabilities.GetProcessEffective()
			if err != nil {
				sylog.Fatalf("Could not get process effective capabilities: %s", err)
			}
			if caps&uint64(1<<unix.CAP_SYS_ADMIN) == 0 {
				sylog.Verbosef("Effective capability CAP_SYS_ADMIN is missing, fallback to user namespace")
				UserNamespace = true
			}
		}
	}
	return useSuid, forceUserNs
}

// setBinds sets engine configuration for requested bind mounts.
func setBinds(engineConfig *singularityConfig.EngineConfig, generator *generate.Generator) error {
	// First get binds from -B/--bind and env var
	binds, err := singularityConfig.ParseBindPath(strings.Join(BindPaths, ","))
	if err != nil {
		return fmt.Errorf("while parsing bind path: %w", err)
	}
	// Now add binds from one or more --mount and env var.
	for _, m := range Mounts {
		bps, err := singularityConfig.ParseMountString(m)
		if err != nil {
			return fmt.Errorf("while parsing mount %q: %w", m, err)
		}
		binds = append(binds, bps...)
	}

	engineConfig.SetBindPath(binds)
	generator.AddProcessEnv("SINGULARITY_BIND", strings.Join(BindPaths, ","))
	return nil
}

// setFuseMounts sets engine configuration for requested FUSE mounts.
func setFuseMounts(engineConfig *singularityConfig.EngineConfig) error {
	if len(FuseMount) > 0 {
		/* If --fusemount is given, imply --pid */
		PidNamespace = true
		if err := engineConfig.SetFuseMount(FuseMount); err != nil {
			return fmt.Errorf("while setting fuse mount: %w", err)
		}
	}
	return nil
}

// Set engine flags to disable mounts, to allow overriding them if they are set true
// in the singularity.conf.
func setNoMountFlags(c *singularityConfig.EngineConfig) {
	skipBinds := []string{}
	for _, v := range NoMount {
		switch v {
		case "proc":
			c.SetNoProc(true)
		case "sys":
			c.SetNoSys(true)
		case "dev":
			c.SetNoDev(true)
		case "devpts":
			c.SetNoDevPts(true)
		case "home":
			c.SetNoHome(true)
		case "tmp":
			c.SetNoTmp(true)
		case "hostfs":
			c.SetNoHostfs(true)
		case "cwd":
			c.SetNoCwd(true)
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
	c.SetSkipBinds(skipBinds)
}

// setHome sets the correct home directory configuration for our circumstance.
// If it is not possible to mount a home directory then the mount will be disabled.
func setHome(customHome bool, engineConfig *singularityConfig.EngineConfig) error {
	engineConfig.SetCustomHome(customHome)
	// If we have fakeroot & the home flag has not been used then we have the standard
	// /root location for the root user $HOME in the container.
	// This doesn't count as a SetCustomHome(true), as we are mounting from the real
	// user's standard $HOME -> /root and we want to respect --contain not mounting
	// the $HOME in this case.
	// See https://github.com/sylabs/singularity/pull/5227
	if !customHome && IsFakeroot {
		HomePath = fmt.Sprintf("%s:/root", HomePath)
	}
	// If we are running as sungularity as root, but requesting a target UID in the container,
	// handle set the home directory appropriately.
	targetUID := engineConfig.GetTargetUID()
	if customHome && targetUID != 0 {
		if targetUID > 500 {
			if pwd, err := user.GetPwUID(uint32(targetUID)); err == nil {
				sylog.Debugf("Target UID requested, set home directory to %s", pwd.Dir)
				HomePath = pwd.Dir
				engineConfig.SetCustomHome(true)
			} else {
				sylog.Verbosef("Home directory for UID %d not found, home won't be mounted", targetUID)
				engineConfig.SetNoHome(true)
				HomePath = "/"
			}
		} else {
			sylog.Verbosef("System UID %d requested, home won't be mounted", targetUID)
			engineConfig.SetNoHome(true)
			HomePath = "/"
		}
	}

	// Handle any user request to override the home directory source/dest
	homeSlice := strings.Split(HomePath, ":")
	if len(homeSlice) > 2 || len(homeSlice) == 0 {
		return fmt.Errorf("home argument has incorrect number of elements: %v", len(homeSlice))
	}
	engineConfig.SetHomeSource(homeSlice[0])
	if len(homeSlice) == 1 {
		engineConfig.SetHomeDest(homeSlice[0])
	} else {
		engineConfig.SetHomeDest(homeSlice[1])
	}
	return nil
}

// SetGPUConfig sets up EngineConfig entries for NV / ROCm usage, if requested.
func SetGPUConfig(engineConfig *singularityConfig.EngineConfig) error {
	if engineConfig.File.AlwaysUseNv && !NoNvidia {
		Nvidia = true
		sylog.Verbosef("'always use nv = yes' found in singularity.conf")
	}
	if engineConfig.File.AlwaysUseRocm && !NoRocm {
		Rocm = true
		sylog.Verbosef("'always use rocm = yes' found in singularity.conf")
	}

	if Nvidia && Rocm {
		sylog.Warningf("--nv and --rocm cannot be used together. Only --nv will be applied.")
	}

	if Nvidia {
		// If nvccli was not enabled by flag or config, drop down to legacy binds immediately
		if !engineConfig.File.UseNvCCLI && !NvCCLI {
			return setNVLegacyConfig(engineConfig)
		}

		// TODO: In privileged fakeroot mode we don't have the correct namespace context to run nvidia-container-cli
		// from  starter, so fall back to legacy NV handling until that workflow is refactored heavily.
		fakeRootPriv := IsFakeroot && engineConfig.File.AllowSetuid && (buildcfg.SINGULARITY_SUID_INSTALL == 1)
		if !fakeRootPriv {
			return setNvCCLIConfig(engineConfig)
		}
		return fmt.Errorf("--fakeroot does not support --nvccli in set-uid installations")
	}

	if Rocm {
		return setRocmConfig(engineConfig)
	}
	return nil
}

// setNvCCLIConfig sets up EngineConfig entries for NVIDIA GPU configuration via nvidia-container-cli.
func setNvCCLIConfig(engineConfig *singularityConfig.EngineConfig) (err error) {
	sylog.Debugf("Using nvidia-container-cli for GPU setup")
	engineConfig.SetNvCCLI(true)

	if os.Getenv("NVIDIA_VISIBLE_DEVICES") == "" {
		if IsContained || IsContainAll {
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
	engineConfig.SetNvCCLIEnv(nvCCLIEnv)

	if UserNamespace && !IsWritable {
		return fmt.Errorf("nvidia-container-cli requires --writable with user namespace/fakeroot")
	}
	if !IsWritable && !IsWritableTmpfs {
		sylog.Infof("Setting --writable-tmpfs (required by nvidia-container-cli)")
		IsWritableTmpfs = true
	}

	return nil
}

// setNvLegacyConfig sets up EngineConfig entries for NVIDIA GPU configuration via direct binds of configured bins/libs.
func setNVLegacyConfig(engineConfig *singularityConfig.EngineConfig) error {
	sylog.Debugf("Using legacy binds for nv GPU setup")
	engineConfig.SetNvLegacy(true)
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
	setGPUBinds(libs, bins, ipcs, "nv", engineConfig)
	return nil
}

// setRocmConfig sets up EngineConfig entries for ROCm GPU configuration via direct binds of configured bins/libs.
func setRocmConfig(engineConfig *singularityConfig.EngineConfig) error {
	sylog.Debugf("Using rocm GPU setup")
	engineConfig.SetRocm(true)
	gpuConfFile := filepath.Join(buildcfg.SINGULARITY_CONFDIR, "rocmliblist.conf")
	libs, bins, err := gpu.RocmPaths(gpuConfFile)
	if err != nil {
		sylog.Warningf("While finding ROCm bind points: %v", err)
	}
	setGPUBinds(libs, bins, []string{}, "nv", engineConfig)
	return nil
}

// setGPUBinds sets EngineConfig entries to bind the provided list of libs, bins, ipc files.
func setGPUBinds(libs, bins, ipcs []string, gpuPlatform string, engineConfig *singularityConfig.EngineConfig) {
	files := make([]string, len(bins)+len(ipcs))
	if len(files) == 0 {
		sylog.Warningf("Could not find any %s files on this host!", gpuPlatform)
	} else {
		if IsWritable {
			sylog.Warningf("%s files may not be bound with --writable", gpuPlatform)
		}
		for i, binary := range bins {
			usrBinBinary := filepath.Join("/usr/bin", filepath.Base(binary))
			files[i] = strings.Join([]string{binary, usrBinBinary}, ":")
		}
		for i, ipc := range ipcs {
			files[i+len(bins)] = ipc
		}
		engineConfig.SetFilesPath(files)
	}
	if len(libs) == 0 {
		sylog.Warningf("Could not find any %s libraries on this host!", gpuPlatform)
	} else {
		engineConfig.SetLibrariesPath(libs)
	}
}

// setNamespaces sets namespace configuration for the engine.
func setNamespaces(uid uint32, gid uint32, engineConfig *singularityConfig.EngineConfig, generator *generate.Generator) {
	// unprivileged installation could not use fakeroot
	// network because it requires a setuid installation
	// so we fallback to none
	if NetNamespace {
		if IsFakeroot && Network != "none" {
			engineConfig.SetNetwork("fakeroot")

			if buildcfg.SINGULARITY_SUID_INSTALL == 0 || !engineConfig.File.AllowSetuid {
				sylog.Warningf(
					"fakeroot with unprivileged installation or 'allow setuid = no' " +
						"could not use 'fakeroot' network, fallback to 'none' network",
				)
				engineConfig.SetNetwork("none")
			}
		}
		generator.AddOrReplaceLinuxNamespace("network", "")
	}
	if UtsNamespace {
		generator.AddOrReplaceLinuxNamespace("uts", "")
	}
	if PidNamespace {
		generator.AddOrReplaceLinuxNamespace("pid", "")
		engineConfig.SetNoInit(NoInit)
	}
	if IpcNamespace {
		generator.AddOrReplaceLinuxNamespace("ipc", "")
	}
	if UserNamespace {
		generator.AddOrReplaceLinuxNamespace("user", "")
		if !IsFakeroot {
			generator.AddLinuxUIDMapping(uid, uid, 1)
			generator.AddLinuxGIDMapping(gid, gid, 1)
		}
	}
}

// setEnv sets the environment for the container, from the host environment, glads, env-file.
func setEnv(args []string, engineConfig *singularityConfig.EngineConfig, generator *generate.Generator) error {
	if SingularityEnvFile != "" {
		currentEnv := append(
			os.Environ(),
			"SINGULARITY_IMAGE="+engineConfig.GetImage(),
		)

		content, err := os.ReadFile(SingularityEnvFile)
		if err != nil {
			return fmt.Errorf("Could not read %q environment file: %w", SingularityEnvFile, err)
		}

		env, err := interpreter.EvaluateEnv(content, args, currentEnv)
		if err != nil {
			return fmt.Errorf("While processing %s: %w", SingularityEnvFile, err)
		}
		// --env variables will take precedence over variables
		// defined by the environment file
		sylog.Debugf("Setting environment variables from file %s", SingularityEnvFile)

		// Update SingularityEnv with those from file
		for _, envar := range env {
			e := strings.SplitN(envar, "=", 2)
			if len(e) != 2 {
				sylog.Warningf("Ignore environment variable %q: '=' is missing", envar)
				continue
			}
			// Ensure we don't overwrite --env variables with environment file
			if _, ok := SingularityEnv[e[0]]; ok {
				sylog.Warningf("Ignore environment variable %s from %s: override from --env", e[0], SingularityEnvFile)
			} else {
				SingularityEnv[e[0]] = e[1]
			}
		}
	}
	// process --env and --env-file variables for injection
	// into the environment by prefixing them with SINGULARITYENV_
	for envName, envValue := range SingularityEnv {
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
	singularityEnv := env.SetContainerEnv(generator, environment, IsCleanEnv, engineConfig.GetHomeDest())
	engineConfig.SetSingularityEnv(singularityEnv)
	return nil
}

// setProcessCwd sets the container process working directory
func setProcessCwd(engineConfig *singularityConfig.EngineConfig, generator *generate.Generator) {
	if pwd, err := os.Getwd(); err == nil {
		engineConfig.SetCwd(pwd)
		if PwdPath != "" {
			generator.SetProcessCwd(PwdPath)
		} else {
			if engineConfig.GetContain() {
				generator.SetProcessCwd(engineConfig.GetHomeDest())
			} else {
				generator.SetProcessCwd(pwd)
			}
		}
	} else {
		sylog.Warningf("can't determine current working directory: %s", err)
	}
}

// PrepareImage perfoms any image preparation required before execution.
// This is currently limited to extraction or FUSE mount when using the user namespace,
// and activating any image driver plugins that might handle the image mount.
func prepareImage(image string, cobraCmd *cobra.Command, engineConfig *singularityConfig.EngineConfig, generator *generate.Generator) error {
	insideUserNs, _ := namespaces.IsInsideUserNamespace(os.Getpid())

	if SIFFUSE && !(UserNamespace || insideUserNs) {
		sylog.Warningf("--sif-fuse is not supported without user namespace, ignoring.")
	}

	// convert image file to sandbox if we are using user
	// namespace or if we are currently running inside a
	// user namespace
	if (UserNamespace || insideUserNs) && fs.IsFile(image) {
		convert := true
		// load image driver plugins
		if engineConfig.File.ImageDriver != "" {

			callbackType := (singularitycallback.RegisterImageDriver)(nil)
			callbacks, err := plugin.LoadCallbacks(callbackType)
			if err != nil {
				sylog.Debugf("Loading plugins callbacks '%T' failed: %s", callbackType, err)
			} else {
				for _, callback := range callbacks {
					if err := callback.(singularitycallback.RegisterImageDriver)(true); err != nil {
						sylog.Debugf("While registering image driver: %s", err)
					}
				}
			}
			driver := imgutil.GetDriver(engineConfig.File.ImageDriver)
			if driver != nil && driver.Features()&imgutil.ImageFeature != 0 {
				// the image driver indicates support for image so let's
				// proceed with the image driver without conversion
				convert = false
			}
		}

		if convert {
			tryFUSE := SIFFUSE || engineConfig.File.SIFFUSE
			fuse, tempDir, imageDir, err := handleImage(cobraCmd.Context(), image, tryFUSE)
			if err != nil {
				return fmt.Errorf("while handling %s: %w", image, err)
			}
			engineConfig.SetImage(imageDir)
			engineConfig.SetImageFuse(fuse)
			engineConfig.SetDeleteTempDir(tempDir)
			generator.AddProcessEnv("SINGULARITY_CONTAINER", imageDir)
			// if '--disable-cache' flag, then remove original SIF after converting to sandbox
			if disableCache {
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
	if img.Type != image.SIF && part.Type != image.SQUASHFS {
		return fmt.Errorf("only SIF images are supported")
	}
	if IsFakeroot {
		return fmt.Errorf("fakeroot is not currently supported")
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
func starterInteractive(useSuid bool, engineConfig *singularityConfig.EngineConfig) error {
	loadOverlay := false
	if !UserNamespace && buildcfg.SINGULARITY_SUID_INSTALL == 1 {
		loadOverlay = true
	}

	cfg := &config.Common{
		EngineName:   singularityConfig.Name,
		EngineConfig: engineConfig,
	}

	// Allow any plugins with callbacks to modify the assembled Config
	runPluginCallbacks(cfg)

	err := starter.Exec(
		"Singularity runtime parent",
		cfg,
		starter.UseSuid(useSuid),
		starter.LoadOverlayModule(loadOverlay),
		starter.CleanupHost(engineConfig.GetImageFuse()),
	)
	return err
}

// starterInstance executes the starter binary to run an instance given the supplied engineConfig
func starterInstance(name string, uid uint32, useSuid bool, engineConfig *singularityConfig.EngineConfig) error {
	cfg := &config.Common{
		EngineName:   singularityConfig.Name,
		ContainerID:  name,
		EngineConfig: engineConfig,
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

	stdout, stderr, err := instance.SetLogFile(name, int(uid), instance.LogSubDir)
	if err != nil {
		return fmt.Errorf("failed to create instance log files: %w", err)
	}

	start, err := stderr.Seek(0, io.SeekEnd)
	if err != nil {
		sylog.Warningf("failed to get standard error stream offset: %s", err)
	}

	loadOverlay := false
	if !UserNamespace && buildcfg.SINGULARITY_SUID_INSTALL == 1 {
		loadOverlay = true
	}

	cmdErr := starter.Run(
		procname,
		cfg,
		starter.UseSuid(useSuid),
		starter.WithStdout(stdout),
		starter.WithStderr(stderr),
		starter.LoadOverlayModule(loadOverlay),
		starter.CleanupHost(engineConfig.GetImageFuse()),
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
		return fmt.Errorf("While loading plugins callbacks '%T': %w", callbackType, err)
	}
	for _, c := range callbacks {
		c.(clicallback.SingularityEngineConfig)(cfg)
	}
	return nil
}

// withPrivilege calls fn if cond is satisfied, and we are uid 0
func withPrivilege(cond bool, desc string, fn func() error) error {
	if !cond {
		return nil
	}
	if os.Getuid() != 0 {
		return fmt.Errorf("%s requires root privileges", desc)
	}
	return fn()
}

// hidepidProc checks if hidepid is set on /proc mount point, when this
// option is an instance started with setuid workflow could not even be
// joined later or stopped correctly.
func hidepidProc() bool {
	entries, err := proc.GetMountInfoEntry("/proc/self/mountinfo")
	if err != nil {
		sylog.Warningf("while reading /proc/self/mountinfo: %s", err)
		return false
	}
	for _, e := range entries {
		if e.Point == "/proc" {
			for _, o := range e.SuperOptions {
				if strings.HasPrefix(o, "hidepid=") {
					return true
				}
			}
		}
	}
	return false
}
