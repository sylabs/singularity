// Copyright (c) 2022-2023, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package launcher

import (
	"fmt"

	"github.com/sylabs/singularity/v4/internal/pkg/ociimage"
	"github.com/sylabs/singularity/v4/internal/pkg/util/fs/overlay"
	"github.com/sylabs/singularity/v4/pkg/util/cryptkey"
)

// Namespaces holds flags for the optional (non-mount) namespaces that can be
// requested for a container launch.
type Namespaces struct {
	User bool
	UTS  bool
	PID  bool
	IPC  bool
	Net  bool
	// NoPID will force the PID namespace not to be used, even if set by default / other flags.
	NoPID bool
}

// Options accumulates launch configuration from passed functional options. Note
// that the Options is modified heavily by logic during the Exec function call.
type Options struct {
	// Writable marks the container image itself as writable.
	Writable bool
	// WritableTmpfs applies an ephemeral writable overlay to the container.
	WritableTmpfs bool
	// OverlayPaths holds paths to image or directory overlays to be applied.
	OverlayPaths []string
	// Scratchdir lists paths into the container to be mounted from a temporary location on the host.
	ScratchDirs []string
	// WorkDir is the parent path for scratch directories, and contained home/tmp on the host.
	WorkDir string

	// HomeDir is the home directory to mount into the container, or a src:dst pair.
	HomeDir string
	// CustomHome is a marker that HomeDir is user-supplied, and should not be
	// modified by the logic used for fakeroot execution.
	CustomHome bool
	// NoHome disables automatic mounting of the home directory into the container.
	NoHome bool

	// BindPaths lists paths to bind from host to container, which may be <src>:<dest> pairs.
	BindPaths []string
	// DataBinds lists data container binds, as <src sif>:<dest> pairs.
	DataBinds []string
	// FuseMount lists paths to be mounted into the container using a FUSE binary, and their options.
	FuseMount []string
	// Mounts lists paths to bind from host to container, from the docker compatible `--mount` flag (CSV format).
	Mounts []string
	// NoMount is a list of automatic / configured mounts to disable.
	NoMount []string

	// Nvidia enables NVIDIA GPU support.
	Nvidia bool
	// NcCCLI sets NVIDIA GPU support to use the nvidia-container-cli.
	NvCCLI bool
	// NoNvidia disables NVIDIA GPU support when set default in singularity.conf.
	NoNvidia bool
	// Rocm enables Rocm GPU support.
	Rocm bool
	// NoRocm disable Rocm GPU support when set default in singularity.conf.
	NoRocm bool

	// ContainLibs lists paths of libraries to bind mount into the container .singularity.d/libs dir.
	ContainLibs []string
	// Proot is the path to a proot binary to bind mount into the container .singularity.d/libs dir.
	Proot string

	// Env is a map of name=value env vars to set in the container.
	Env map[string]string
	// EnvFiles contains filenames to read container env vars from.
	EnvFiles []string
	// CleanEnv starts the container with a clean environment, excluding host env vars.
	CleanEnv bool
	// NoEval instructs Singularity not to shell evaluate args and env vars.
	NoEval bool

	// Namespaces is the list of optional Namespaces requested for the container.
	Namespaces Namespaces

	// NetnsPath is the path to a network namespace to join, rather than
	// creating one / applying a CNI config.
	NetnsPath string

	// Network is the name of an optional CNI networking configuration to apply.
	Network string
	// NetworkArgs are argument to pass to the CNI plugin that will configure networking when Network is set.
	NetworkArgs []string
	// Hostname is the hostname to set in the container (infers/requires UTS namespace).
	Hostname string
	// DNS is the comma separated list of DNS servers to be set in the container's resolv.conf.
	DNS string

	// AddCaps is the list of capabilities to Add to the container process.
	AddCaps string
	// DropCaps is the list of capabilities to drop from the container process.
	DropCaps string
	// AllowSUID permits setuid executables inside a container started by the root user.
	AllowSUID bool
	// KeepPrivs keeps all privileges inside a container started by the root user.
	KeepPrivs bool
	// NoPrivs drops all privileges inside a container.
	NoPrivs bool
	// SecurityOpts is the list of security options (selinux, apparmor, seccomp) to apply.
	SecurityOpts []string
	// NoUmask disables propagation of the host umask into the container, using a default 0022.
	NoUmask bool

	// CGroupsJSON is a JSON format cgroups resource limit specification to apply.
	CGroupsJSON string

	// ConfigFile is an alternate singularity.conf that will be used by unprivileged installations only.
	ConfigFile string

	// ShellPath is a custom shell executable to be launched in the container.
	ShellPath string
	// CwdPath is the initial working directory in the container.
	CwdPath string

	// Fakeroot enables the fake root mode, using user namespaces and subuid / subgid mapping.
	Fakeroot bool
	// NoSetgroups disables calling setgroups for the fakeroot user namespace.
	NoSetgroups bool
	// Boot enables execution of /sbin/init on startup of an instance container.
	Boot bool
	// NoInit disables shim process when PID namespace is used.
	NoInit bool
	// Contain starts the container with minimal /dev and empty home/tmp mounts.
	Contain bool
	// ContainAll infers Contain, and adds PID, IPC namespaces, and CleanEnv.
	ContainAll bool

	// AppName sets a SCIF application name to run.
	AppName string

	// KeyInfo holds encryption key information for accessing encrypted containers.
	KeyInfo *cryptkey.KeyInfo

	// SIFFUSE enables mounting SIF container images using FUSE.
	SIFFUSE bool
	// CacheDisabled indicates caching of images was disabled in the CLI, as in
	// userns flows we will need to delete the redundant temporary pulled image after
	// conversion to sandbox.
	CacheDisabled bool

	// TransportOptions holds Docker/OCI image transport configuration (auth etc.)
	// This will be used by a launcher handling OCI images directly.
	TransportOptions *ociimage.TransportOptions

	// TmpSandbox forces unpacking of images into temporary sandbox dirs when a
	// kernel or FUSE mount would otherwise be used.
	TmpSandbox bool

	// NoTmpSandbox prohibits unpacking of images into temporary sandbox dirs.
	NoTmpSandbox bool

	// Devices contains the list of device mappings (if any), e.g. CDI mappings.
	Devices []string

	// CdiDirs contains the list of directories in which CDI should look for device definition JSON files
	CdiDirs []string

	// NoCompat indicates the container should be run in non-OCI compatible
	// mode, i.e. with default mounts etc. as native mode. Effective for the OCI
	// launcher only.
	NoCompat bool
}

type Option func(co *Options) error

// OptWritable sets the container image to be writable.
func OptWritable(b bool) Option {
	return func(lo *Options) error {
		lo.Writable = b
		return nil
	}
}

// OptWritableTmpFs applies an ephemeral writable overlay to the container.
func OptWritableTmpfs(b bool) Option {
	return func(lo *Options) error {
		lo.WritableTmpfs = b
		return nil
	}
}

// OptOverlayPaths sets overlay images and directories to apply to the container.
// Relative paths are resolved to absolute paths at this point.
func OptOverlayPaths(op []string) Option {
	return func(lo *Options) error {
		var err error
		for i, p := range op {
			op[i], err = overlay.AbsOverlay(p)
			if err != nil {
				return fmt.Errorf("could not convert %q to absolute path: %w", p, err)
			}
		}
		lo.OverlayPaths = op
		return nil
	}
}

// OptScratchDirs sets temporary host directories to create and bind into the container.
func OptScratchDirs(sd []string) Option {
	return func(lo *Options) error {
		lo.ScratchDirs = sd
		return nil
	}
}

// OptWorkDir sets the parent path for scratch directories, and contained home/tmp on the host.
func OptWorkDir(wd string) Option {
	return func(lo *Options) error {
		lo.WorkDir = wd
		return nil
	}
}

// OptHome sets the home directory configuration for the container.
//
// homeDir is the path or src:dst to bind mount.
// custom is a marker that this is user supplied, and must not be overridden.
// disable will disable the home mount entirely, ignoring other options.
func OptHome(homeDir string, custom bool, disable bool) Option {
	return func(lo *Options) error {
		lo.HomeDir = homeDir
		lo.CustomHome = custom
		lo.NoHome = disable
		return nil
	}
}

// MountSpecs holds the various kinds of mount specifications that can be a
// applied to a container.
type MountSpecs struct {
	// Binds holds <src>[:<dst>[:<opts>]] bind mount specifications from the CLI
	// --bind flag
	Binds []string
	// DataBinds holds <src sif>:<dst> data container bind specifications from
	// the CLI --data flag.
	DataBinds []string
	// Mounts holds Docker csv style mount specifications from the CLI --mount
	// flag.
	Mounts []string
	// FuseMounts holds <type>:<fuse command> <mountpoint> FUSE mount
	// specifications from the CLI --fusemount flag.
	FuseMounts []string
}

// OptMounts sets user-requested mounts to propagate into the container.
func OptMounts(ms MountSpecs) Option {
	return func(lo *Options) error {
		lo.BindPaths = ms.Binds
		lo.DataBinds = ms.DataBinds
		lo.Mounts = ms.Mounts
		lo.FuseMount = ms.FuseMounts
		return nil
	}
}

// OptNoMount disables the specified bind mounts.
func OptNoMount(nm []string) Option {
	return func(lo *Options) error {
		lo.NoMount = nm
		return nil
	}
}

// OptNvidia enables NVIDIA GPU support.
//
// nvccli sets whether to use the nvidia-container-runtime (true), or legacy bind mounts (false).
func OptNvidia(nv bool, nvccli bool) Option {
	return func(lo *Options) error {
		lo.Nvidia = nv || nvccli
		lo.NvCCLI = nvccli
		return nil
	}
}

// OptNoNvidia disables NVIDIA GPU support, even if enabled via singularity.conf.
func OptNoNvidia(b bool) Option {
	return func(lo *Options) error {
		lo.NoNvidia = b
		return nil
	}
}

// OptRocm enable Rocm GPU support.
func OptRocm(b bool) Option {
	return func(lo *Options) error {
		lo.Rocm = b
		return nil
	}
}

// OptNoRocm disables Rocm GPU support, even if enabled via singularity.conf.
func OptNoRocm(b bool) Option {
	return func(lo *Options) error {
		lo.NoRocm = b
		return nil
	}
}

// OptContainLibs mounts specified libraries into the container .singularity.d/libs dir.
func OptContainLibs(cl []string) Option {
	return func(lo *Options) error {
		lo.ContainLibs = cl
		return nil
	}
}

// OptProot mounts specified proot executable into the container .singularity.d/libs dir.
func OptProot(p string) Option {
	return func(lo *Options) error {
		lo.Proot = p
		return nil
	}
}

// OptEnv sets container environment
//
// envFiles is a slice of paths to files container environment variables to set.
// env is a map of name=value env vars to set.
// clean removes host variables from the container environment.
func OptEnv(env map[string]string, envFiles []string, clean bool) Option {
	return func(lo *Options) error {
		lo.Env = env
		lo.EnvFiles = envFiles
		lo.CleanEnv = clean
		return nil
	}
}

// OptNoEval disables shell evaluation of args and env vars.
func OptNoEval(b bool) Option {
	return func(lo *Options) error {
		lo.NoEval = b
		return nil
	}
}

// OptNamespaces enable the individual kernel-support namespaces for the container.
func OptNamespaces(n Namespaces) Option {
	return func(lo *Options) error {
		lo.Namespaces = n
		return nil
	}
}

// OptJoinNetNamespace sets the network namespace to join, if permitted.
func OptNetnsPath(n string) Option {
	return func(lo *Options) error {
		lo.NetnsPath = n
		return nil
	}
}

// OptNetwork enables CNI networking.
//
// network is the name of the CNI configuration to enable.
// args are arguments to pass to the CNI plugin.
func OptNetwork(network string, args []string) Option {
	return func(lo *Options) error {
		lo.Network = network
		lo.NetworkArgs = args
		return nil
	}
}

// OptHostname sets a hostname for the container (infers/requires UTS namespace).
func OptHostname(h string) Option {
	return func(lo *Options) error {
		lo.Hostname = h
		return nil
	}
}

// OptDNS sets a DNS entry for the container resolv.conf.
func OptDNS(d string) Option {
	return func(lo *Options) error {
		lo.DNS = d
		return nil
	}
}

// OptCaps sets capabilities to add and drop.
func OptCaps(add, drop string) Option {
	return func(lo *Options) error {
		lo.AddCaps = add
		lo.DropCaps = drop
		return nil
	}
}

// OptAllowSUID permits setuid executables inside a container started by the root user.
func OptAllowSUID(b bool) Option {
	return func(lo *Options) error {
		lo.AllowSUID = b
		return nil
	}
}

// OptKeepPrivs keeps all privileges inside a container started by the root user.
func OptKeepPrivs(b bool) Option {
	return func(lo *Options) error {
		lo.KeepPrivs = b
		return nil
	}
}

// OptNoPrivs drops all privileges inside a container.
func OptNoPrivs(b bool) Option {
	return func(lo *Options) error {
		lo.NoPrivs = b
		return nil
	}
}

// OptSecurity supplies a list of security options (selinux, apparmor, seccomp) to apply.
func OptSecurity(s []string) Option {
	return func(lo *Options) error {
		lo.SecurityOpts = s
		return nil
	}
}

// OptNoUmask disables propagation of the host umask into the container, using a default 0022.
func OptNoUmask(b bool) Option {
	return func(lo *Options) error {
		lo.NoUmask = b
		return nil
	}
}

// OptCgroupsJSON sets a Cgroups resource limit configuration to apply to the container.
func OptCgroupsJSON(cj string) Option {
	return func(lo *Options) error {
		lo.CGroupsJSON = cj
		return nil
	}
}

// OptConfigFile specifies an alternate singularity.conf that will be used by unprivileged installations only.
func OptConfigFile(c string) Option {
	return func(lo *Options) error {
		lo.ConfigFile = c
		return nil
	}
}

// OptShellPath specifies a custom shell executable to be launched in the container.
func OptShellPath(s string) Option {
	return func(lo *Options) error {
		lo.ShellPath = s
		return nil
	}
}

// OptCwdPath specifies the initial working directory in the container.
func OptCwdPath(p string) Option {
	return func(lo *Options) error {
		lo.CwdPath = p
		return nil
	}
}

// OptFakeroot enables the fake root mode, using user namespaces and subuid / subgid mapping.
func OptFakeroot(b bool) Option {
	return func(lo *Options) error {
		lo.Fakeroot = b
		return nil
	}
}

// OptNoSetgroups disables calling setgroups for the fakeroot user namespace.
func OptNoSetgroups(b bool) Option {
	return func(lo *Options) error {
		lo.NoSetgroups = b
		return nil
	}
}

// OptBoot enables execution of /sbin/init on startup of an instance container.
func OptBoot(b bool) Option {
	return func(lo *Options) error {
		lo.Boot = b
		return nil
	}
}

// OptNoInit disables shim process when PID namespace is used.
func OptNoInit(b bool) Option {
	return func(lo *Options) error {
		lo.NoInit = b
		return nil
	}
}

// OptContain starts the container with minimal /dev and empty home/tmp mounts.
func OptContain(b bool) Option {
	return func(lo *Options) error {
		lo.Contain = b
		return nil
	}
}

// OptContainAll infers Contain, and adds PID, IPC namespaces, and CleanEnv.
func OptContainAll(b bool) Option {
	return func(lo *Options) error {
		lo.ContainAll = b
		return nil
	}
}

// OptAppName sets a SCIF application name to run.
func OptAppName(a string) Option {
	return func(lo *Options) error {
		lo.AppName = a
		return nil
	}
}

// OptKeyInfo sets encryption key material to use when accessing an encrypted container image.
func OptKeyInfo(ki *cryptkey.KeyInfo) Option {
	return func(lo *Options) error {
		lo.KeyInfo = ki
		return nil
	}
}

// OptSIFFuse enables FUSE mounting of a SIF image, if possible.
func OptSIFFuse(b bool) Option {
	return func(lo *Options) error {
		lo.SIFFUSE = b
		return nil
	}
}

// TmpSandbox forces unpacking of images into temporary sandbox dirs when a
// kernel or FUSE mount would otherwise be used.
func OptTmpSandbox(b bool) Option {
	return func(lo *Options) error {
		lo.TmpSandbox = b
		return nil
	}
}

// OptNoTmpSandbox prohibits unpacking of images into temporary sandbox dirs.
func OptNoTmpSandbox(b bool) Option {
	return func(lo *Options) error {
		lo.NoTmpSandbox = b
		return nil
	}
}

// OptCacheDisabled indicates caching of images was disabled in the CLI.
func OptCacheDisabled(b bool) Option {
	return func(lo *Options) error {
		lo.CacheDisabled = b
		return nil
	}
}

// OptTransportOptions sets Docker/OCI image transport options (auth etc.)
func OptTransportOptions(tOpts *ociimage.TransportOptions) Option {
	return func(lo *Options) error {
		lo.TransportOptions = tOpts
		return nil
	}
}

// OptDevice sets CDI device mappings to apply.
func OptDevice(op []string) Option {
	return func(lo *Options) error {
		lo.Devices = op
		return nil
	}
}

// OptCdiDirs sets CDI spec search-directories to apply.
func OptCdiDirs(op []string) Option {
	return func(lo *Options) error {
		lo.CdiDirs = op
		return nil
	}
}

// OptNoCompat disable OCI compatible mode, for singularity native mode default behaviors.
func OptNoCompat(b bool) Option {
	return func(lo *Options) error {
		lo.NoCompat = b
		return nil
	}
}
