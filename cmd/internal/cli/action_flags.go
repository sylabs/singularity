// Copyright (c) 2018-2024, Sylabs Inc. All rights reserved.
// Copyright (c) Contributors to the Apptainer project, established as
//   Apptainer a Series of LF Projects LLC.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package cli

import (
	"os"

	"github.com/sylabs/singularity/v4/pkg/cmdline"
)

// actionflags.go contains flag variables for action-like commands to draw from
var (
	appName             string
	dataPaths           []string
	bindPaths           []string
	mounts              []string
	homePath            string
	overlayPath         []string
	scratchPath         []string
	workdirPath         string
	cwdPath             string
	shellPath           string
	hostname            string
	network             string
	networkArgs         []string
	dns                 string
	security            []string
	cgroupsTOMLFile     string
	containLibsPath     []string
	fuseMount           []string
	singularityEnv      map[string]string
	singularityEnvFiles []string
	noMount             []string
	proot               string
	device              []string
	cdiDirs             []string

	isBoot          bool
	isFakeroot      bool
	noSetgroups     bool
	isCleanEnv      bool
	isCompat        bool
	noCompat        bool
	isContained     bool
	isContainAll    bool
	isWritable      bool
	isWritableTmpfs bool
	sifFUSE         bool
	nvidia          bool
	nvCCLI          bool
	rocm            bool
	noEval          bool
	noHome          bool
	noInit          bool
	noNvidia        bool
	noRocm          bool
	noUmask         bool
	disableCache    bool

	netNamespace   bool
	netnsPath      string
	utsNamespace   bool
	userNamespace  bool
	pidNamespace   bool
	noPidNamespace bool
	ipcNamespace   bool

	allowSUID bool
	keepPrivs bool
	noPrivs   bool
	addCaps   string
	dropCaps  string

	blkioWeight       int
	blkioWeightDevice []string
	cpuShares         int
	cpus              string // decimal
	cpuSetCPUs        string
	cpuSetMems        string
	memory            string // bytes
	memoryReservation string // bytes
	memorySwap        string // bytes
	oomKillDisable    bool
	pidsLimit         int
)

// --app
var actionAppFlag = cmdline.Flag{
	ID:           "actionAppFlag",
	Value:        &appName,
	DefaultValue: "",
	Name:         "app",
	Usage:        "set an application to run inside a container",
	EnvKeys:      []string{"APP", "APPNAME"},
}

// -B|--bind
var actionBindFlag = cmdline.Flag{
	ID:           "actionBindFlag",
	Value:        &bindPaths,
	DefaultValue: []string{},
	Name:         "bind",
	ShortHand:    "B",
	Usage:        "a user-bind path specification. spec has the format src[:dest[:opts]], where src and dest are outside and inside paths. If dest is not given, it is set equal to src. Mount options ('opts') may be specified as 'ro' (read-only) or 'rw' (read/write, which is the default). Multiple bind paths can be given by a comma separated list.",
	EnvKeys:      []string{"BIND", "BINDPATH"},
	Tag:          "<spec>",
	EnvHandler:   cmdline.EnvAppendValue,
}

// --data
var actionDataFlag = cmdline.Flag{
	ID:           "actionDataFlag",
	Value:        &dataPaths,
	DefaultValue: []string{},
	Name:         "data",
	Usage:        "a data-container bind specification src:dest, where src is the path to the data container, and dest is the destination path in the container. Multiple data container binds can be given as a comma separated list.",
	Tag:          "<spec>",
}

// --mount
var actionMountFlag = cmdline.Flag{
	ID:           "actionMountFlag",
	Value:        &mounts,
	DefaultValue: []string{},
	Name:         "mount",
	Usage:        "a mount specification e.g. 'type=bind,source=/opt,destination=/hostopt'.",
	EnvKeys:      []string{"MOUNT"},
	Tag:          "<spec>",
	EnvHandler:   cmdline.EnvAppendValue,
	StringArray:  true,
}

// -H|--home
var actionHomeFlag = cmdline.Flag{
	ID:           "actionHomeFlag",
	Value:        &homePath,
	DefaultValue: CurrentUser.HomeDir,
	Name:         "home",
	ShortHand:    "H",
	Usage:        "a home directory specification. spec can either be a src path or src:dest pair. src is the source path of the home directory outside the container and dest overrides the home directory within the container.",
	EnvKeys:      []string{"HOME"},
	Tag:          "<spec>",
}

// -o|--overlay
var actionOverlayFlag = cmdline.Flag{
	ID:           "actionOverlayFlag",
	Value:        &overlayPath,
	DefaultValue: []string{},
	Name:         "overlay",
	ShortHand:    "o",
	Usage:        "use an overlayFS image for persistent data storage or as read-only layer of container",
	EnvKeys:      []string{"OVERLAY", "OVERLAYIMAGE"},
	Tag:          "<path>",
}

// -S|--scratch
var actionScratchFlag = cmdline.Flag{
	ID:           "actionScratchFlag",
	Value:        &scratchPath,
	DefaultValue: []string{},
	Name:         "scratch",
	ShortHand:    "S",
	Usage:        "include a scratch directory within the container that is linked to a temporary dir (use -W to force location)",
	EnvKeys:      []string{"SCRATCH", "SCRATCHDIR"},
	Tag:          "<path>",
}

// -W|--workdir
var actionWorkdirFlag = cmdline.Flag{
	ID:           "actionWorkdirFlag",
	Value:        &workdirPath,
	DefaultValue: "",
	Name:         "workdir",
	ShortHand:    "W",
	Usage:        "working directory to be used for /tmp and /var/tmp (if -c/--contain was also used)",
	EnvKeys:      []string{"WORKDIR"},
	Tag:          "<path>",
}

// --disable-cache
var actionDisableCacheFlag = cmdline.Flag{
	ID:           "actionDisableCacheFlag",
	Value:        &disableCache,
	DefaultValue: false,
	Name:         "disable-cache",
	Usage:        "dont use cache, and dont create cache",
	EnvKeys:      []string{"DISABLE_CACHE"},
}

// -s|--shell
var actionShellFlag = cmdline.Flag{
	ID:           "actionShellFlag",
	Value:        &shellPath,
	DefaultValue: "",
	Name:         "shell",
	ShortHand:    "s",
	Usage:        "path to program to use for interactive shell",
	EnvKeys:      []string{"SHELL"},
	Tag:          "<path>",
}

// --cwd
var actionCwdFlag = cmdline.Flag{
	ID:           "actionCwdFlag",
	Value:        &cwdPath,
	DefaultValue: "",
	Name:         "cwd",
	Usage:        "initial working directory for payload process inside the container (synonym for --pwd)",
	EnvKeys:      []string{"CWD", "TARGET_CWD"},
	Tag:          "<path>",
}

// --pwd
var actionPwdFlag = cmdline.Flag{
	ID:           "actionPwdFlag",
	Value:        &cwdPath,
	DefaultValue: "",
	Name:         "pwd",
	Usage:        "initial working directory for payload process inside the container (synonym for --cwd)",
	Hidden:       true,
	EnvKeys:      []string{"PWD", "TARGET_PWD"},
	Tag:          "<path>",
}

// --hostname
var actionHostnameFlag = cmdline.Flag{
	ID:           "actionHostnameFlag",
	Value:        &hostname,
	DefaultValue: "",
	Name:         "hostname",
	Usage:        "set container hostname. Infers --uts.",
	EnvKeys:      []string{"HOSTNAME"},
	Tag:          "<name>",
}

// --network
var actionNetworkFlag = cmdline.Flag{
	ID:           "actionNetworkFlag",
	Value:        &network,
	DefaultValue: "bridge",
	Name:         "network",
	Usage:        "specify desired network type separated by commas, each network will bring up a dedicated interface inside container",
	EnvKeys:      []string{"NETWORK"},
	Tag:          "<name>",
}

// --network-args
var actionNetworkArgsFlag = cmdline.Flag{
	ID:           "actionNetworkArgsFlag",
	Value:        &networkArgs,
	DefaultValue: []string{},
	Name:         "network-args",
	Usage:        "specify network arguments to pass to CNI plugins",
	EnvKeys:      []string{"NETWORK_ARGS"},
	Tag:          "<args>",
}

// --dns
var actionDNSFlag = cmdline.Flag{
	ID:           "actionDnsFlag",
	Value:        &dns,
	DefaultValue: "",
	Name:         "dns",
	Usage:        "list of DNS server separated by commas to add in resolv.conf",
	EnvKeys:      []string{"DNS"},
}

// --security
var actionSecurityFlag = cmdline.Flag{
	ID:           "actionSecurityFlag",
	Value:        &security,
	DefaultValue: []string{},
	Name:         "security",
	Usage:        "enable security features (SELinux, Apparmor, Seccomp)",
	EnvKeys:      []string{"SECURITY"},
}

// --apply-cgroups
var actionApplyCgroupsFlag = cmdline.Flag{
	ID:           "actionApplyCgroupsFlag",
	Value:        &cgroupsTOMLFile,
	DefaultValue: "",
	Name:         "apply-cgroups",
	Usage:        "apply cgroups from file for container processes (root only)",
	EnvKeys:      []string{"APPLY_CGROUPS"},
}

// hidden flag to handle SINGULARITY_CONTAINLIBS environment variable
var actionContainLibsFlag = cmdline.Flag{
	ID:           "actionContainLibsFlag",
	Value:        &containLibsPath,
	DefaultValue: []string{},
	Name:         "containlibs",
	Hidden:       true,
	EnvKeys:      []string{"CONTAINLIBS"},
}

// --fusemount
var actionFuseMountFlag = cmdline.Flag{
	ID:           "actionFuseMountFlag",
	Value:        &fuseMount,
	DefaultValue: []string{},
	Name:         "fusemount",
	Usage:        "A FUSE filesystem mount specification of the form '<type>:<fuse command> <mountpoint>' - where <type> is 'container' or 'host', specifying where the mount will be performed ('container-daemon' or 'host-daemon' will run the FUSE process detached). <fuse command> is the path to the FUSE executable, plus options for the mount. <mountpoint> is the location in the container to which the FUSE mount will be attached. E.g. 'container:sshfs 10.0.0.1:/ /sshfs'. Implies --pid.",
	EnvKeys:      []string{"FUSESPEC"},
}

// hidden flag to handle SINGULARITY_TMPDIR environment variable
var actionTmpDirFlag = cmdline.Flag{
	ID:           "actionTmpDirFlag",
	Value:        &tmpDir,
	DefaultValue: os.TempDir(),
	Name:         "tmpdir",
	Usage:        "specify a temporary directory to use for build",
	Hidden:       true,
	EnvKeys:      []string{"TMPDIR"},
}

// --boot
var actionBootFlag = cmdline.Flag{
	ID:           "actionBootFlag",
	Value:        &isBoot,
	DefaultValue: false,
	Name:         "boot",
	Usage:        "execute /sbin/init to boot container (root only)",
	EnvKeys:      []string{"BOOT"},
}

// -f|--fakeroot
var actionFakerootFlag = cmdline.Flag{
	ID:           "actionFakerootFlag",
	Value:        &isFakeroot,
	DefaultValue: false,
	Name:         "fakeroot",
	ShortHand:    "f",
	Usage:        "run container in new user namespace as uid 0",
	EnvKeys:      []string{"FAKEROOT"},
}

// --no-setgroups
var actionNoSetgroupsFlag = cmdline.Flag{
	ID:           "actionNoSetgroupsFlag",
	Value:        &noSetgroups,
	DefaultValue: false,
	Name:         "no-setgroups",
	Usage:        "disable setgroups when entering --fakeroot user namespace",
	EnvKeys:      []string{"NO_SETGROUPS"},
}

// -e|--cleanenv
var actionCleanEnvFlag = cmdline.Flag{
	ID:           "actionCleanEnvFlag",
	Value:        &isCleanEnv,
	DefaultValue: false,
	Name:         "cleanenv",
	ShortHand:    "e",
	Usage:        "clean environment before running container",
	EnvKeys:      []string{"CLEANENV"},
}

// --compat
var actionCompatFlag = cmdline.Flag{
	ID:           "actionCompatFlag",
	Value:        &isCompat,
	DefaultValue: false,
	Name:         "compat",
	Usage:        "apply settings for increased OCI/Docker compatibility. Infers --containall, --no-init, --no-umask, --no-eval, --writable-tmpfs.",
	EnvKeys:      []string{"COMPAT"},
}

// --no-compat
var actionNoCompatFlag = cmdline.Flag{
	ID:           "actionNoCompatFlag",
	Value:        &noCompat,
	DefaultValue: false,
	Name:         "no-compat",
	Usage:        "(--oci mode) do not apply settings for increased OCI/Docker compatibility. Emulate native runtime defaults without --contain etc.",
	EnvKeys:      []string{"NO_COMPAT"},
}

// -c|--contain
var actionContainFlag = cmdline.Flag{
	ID:           "actionContainFlag",
	Value:        &isContained,
	DefaultValue: false,
	Name:         "contain",
	ShortHand:    "c",
	Usage:        "use minimal /dev and empty other directories (e.g. /tmp and $HOME) instead of sharing filesystems from your host",
	EnvKeys:      []string{"CONTAIN"},
}

// -C|--containall
var actionContainAllFlag = cmdline.Flag{
	ID:           "actionContainAllFlag",
	Value:        &isContainAll,
	DefaultValue: false,
	Name:         "containall",
	ShortHand:    "C",
	Usage:        "contain not only file systems, but also PID, IPC, and environment",
	EnvKeys:      []string{"CONTAINALL"},
}

// --nv
var actionNvidiaFlag = cmdline.Flag{
	ID:           "actionNvidiaFlag",
	Value:        &nvidia,
	DefaultValue: false,
	Name:         "nv",
	Usage:        "enable Nvidia support",
	EnvKeys:      []string{"NV"},
}

// --nvccli
var actionNvCCLIFlag = cmdline.Flag{
	ID:           "actionNvCCLIFlag",
	Value:        &nvCCLI,
	DefaultValue: false,
	Name:         "nvccli",
	Usage:        "use nvidia-container-cli for GPU setup (experimental)",
	EnvKeys:      []string{"NVCCLI"},
}

// --rocm flag to automatically bind
var actionRocmFlag = cmdline.Flag{
	ID:           "actionRocmFlag",
	Value:        &rocm,
	DefaultValue: false,
	Name:         "rocm",
	Usage:        "enable experimental Rocm support",
	EnvKeys:      []string{"ROCM"},
}

// -w|--writable
var actionWritableFlag = cmdline.Flag{
	ID:           "actionWritableFlag",
	Value:        &isWritable,
	DefaultValue: false,
	Name:         "writable",
	ShortHand:    "w",
	Usage:        "by default all Singularity containers are available as read only. This option makes the file system accessible as read/write.",
	EnvKeys:      []string{"WRITABLE"},
}

// --writable-tmpfs
var actionWritableTmpfsFlag = cmdline.Flag{
	ID:           "actionWritableTmpfsFlag",
	Value:        &isWritableTmpfs,
	DefaultValue: false,
	Name:         "writable-tmpfs",
	Usage:        "makes the file system accessible as read-write with non persistent data (with overlay support only)",
	EnvKeys:      []string{"WRITABLE_TMPFS"},
}

// --no-home
var actionNoHomeFlag = cmdline.Flag{
	ID:           "actionNoHomeFlag",
	Value:        &noHome,
	DefaultValue: false,
	Name:         "no-home",
	Usage:        "do NOT mount users home directory if /home is not the current working directory",
	EnvKeys:      []string{"NO_HOME"},
}

// --no-mount
var actionNoMountFlag = cmdline.Flag{
	ID:           "actionNoMountFlag",
	Value:        &noMount,
	DefaultValue: []string{},
	Name:         "no-mount",
	Usage:        "disable one or more 'mount xxx' options set in singularity.conf, specify absolute destination path to disable a bind path entry, or 'bind-paths' to disable all bind path entries.",
	EnvKeys:      []string{"NO_MOUNT"},
}

// --no-init
var actionNoInitFlag = cmdline.Flag{
	ID:           "actionNoInitFlag",
	Value:        &noInit,
	DefaultValue: false,
	Name:         "no-init",
	Usage:        "do NOT start shim process with --pid",
	EnvKeys:      []string{"NOSHIMINIT"},
}

// hidden flag to disable nvidia bindings when 'always use nv = yes'
var actionNoNvidiaFlag = cmdline.Flag{
	ID:           "actionNoNvidiaFlag",
	Value:        &noNvidia,
	DefaultValue: false,
	Name:         "no-nv",
	Hidden:       true,
	EnvKeys:      []string{"NV_OFF", "NO_NV"},
}

// hidden flag to disable rocm bindings when 'always use rocm = yes'
var actionNoRocmFlag = cmdline.Flag{
	ID:           "actionNoRocmFlag",
	Value:        &noRocm,
	DefaultValue: false,
	Name:         "no-rocm",
	Hidden:       true,
	EnvKeys:      []string{"ROCM_OFF", "NO_ROCM"},
}

// -p|--pid
var actionPidNamespaceFlag = cmdline.Flag{
	ID:           "actionPidNamespaceFlag",
	Value:        &pidNamespace,
	DefaultValue: false,
	Name:         "pid",
	ShortHand:    "p",
	Usage:        "run container in a new PID namespace",
	EnvKeys:      []string{"PID", "UNSHARE_PID"},
}

// --no-pid
var actionNoPidNamespaceFlag = cmdline.Flag{
	ID:           "actionNoPidNamespaceFlag",
	Value:        &noPidNamespace,
	DefaultValue: false,
	Name:         "no-pid",
	Usage:        "do not run container in a new PID namespace",
	EnvKeys:      []string{"NO_PID"},
}

// -i|--ipc
var actionIpcNamespaceFlag = cmdline.Flag{
	ID:           "actionIpcNamespaceFlag",
	Value:        &ipcNamespace,
	DefaultValue: false,
	Name:         "ipc",
	ShortHand:    "i",
	Usage:        "run container in a new IPC namespace",
	EnvKeys:      []string{"IPC", "UNSHARE_IPC"},
}

// -n|--net
var actionNetNamespaceFlag = cmdline.Flag{
	ID:           "actionNetNamespaceFlag",
	Value:        &netNamespace,
	DefaultValue: false,
	Name:         "net",
	ShortHand:    "n",
	Usage:        "run container in a new network namespace (sets up a bridge network interface by default)",
	EnvKeys:      []string{"NET", "UNSHARE_NET"},
}

// --netns-path
var actionNetnsPathFlag = cmdline.Flag{
	ID:           "actionNetnsPathFlag",
	Value:        &netnsPath,
	DefaultValue: "",
	Name:         "netns-path",
	Usage:        "join the network namespace at the specified path (as root, or if permitted in singularity.conf)",
	EnvKeys:      []string{"NETNS_PATH"},
}

// --uts
var actionUtsNamespaceFlag = cmdline.Flag{
	ID:           "actionUtsNamespaceFlag",
	Value:        &utsNamespace,
	DefaultValue: false,
	Name:         "uts",
	Usage:        "run container in a new UTS namespace",
	EnvKeys:      []string{"UTS", "UNSHARE_UTS"},
}

// -u|--userns
var actionUserNamespaceFlag = cmdline.Flag{
	ID:           "actionUserNamespaceFlag",
	Value:        &userNamespace,
	DefaultValue: false,
	Name:         "userns",
	ShortHand:    "u",
	Usage:        "run container in a new user namespace, allowing Singularity to run completely unprivileged on recent kernels. This disables some features of Singularity, for example it only works with sandbox images.",
	EnvKeys:      []string{"USERNS", "UNSHARE_USERNS"},
}

// --keep-privs
var actionKeepPrivsFlag = cmdline.Flag{
	ID:           "actionKeepPrivsFlag",
	Value:        &keepPrivs,
	DefaultValue: false,
	Name:         "keep-privs",
	Usage:        "let root user keep privileges in container (root only)",
	EnvKeys:      []string{"KEEP_PRIVS"},
}

// --no-privs
var actionNoPrivsFlag = cmdline.Flag{
	ID:           "actionNoPrivsFlag",
	Value:        &noPrivs,
	DefaultValue: false,
	Name:         "no-privs",
	Usage:        "drop all privileges in container (root only in non-OCI mode)",
	EnvKeys:      []string{"NO_PRIVS"},
}

// --add-caps
var actionAddCapsFlag = cmdline.Flag{
	ID:           "actionAddCapsFlag",
	Value:        &addCaps,
	DefaultValue: "",
	Name:         "add-caps",
	Usage:        "a comma separated capability list to add",
	EnvKeys:      []string{"ADD_CAPS"},
}

// --drop-caps
var actionDropCapsFlag = cmdline.Flag{
	ID:           "actionDropCapsFlag",
	Value:        &dropCaps,
	DefaultValue: "",
	Name:         "drop-caps",
	Usage:        "a comma separated capability list to drop",
	EnvKeys:      []string{"DROP_CAPS"},
}

// --allow-setuid
var actionAllowSetuidFlag = cmdline.Flag{
	ID:           "actionAllowSetuidFlag",
	Value:        &allowSUID,
	DefaultValue: false,
	Name:         "allow-setuid",
	Usage:        "allow setuid binaries in container (root only)",
	EnvKeys:      []string{"ALLOW_SETUID"},
}

// --env
var actionEnvFlag = cmdline.Flag{
	ID:           "actionEnvFlag",
	Value:        &singularityEnv,
	DefaultValue: map[string]string{},
	Name:         "env",
	Usage:        "pass environment variable to contained process",
}

// --env-file
var actionEnvFileFlag = cmdline.Flag{
	ID:           "actionEnvFileFlag",
	Value:        &singularityEnvFiles,
	DefaultValue: []string{},
	Name:         "env-file",
	Usage:        "pass environment variables from file to contained process",
	EnvKeys:      []string{"ENV_FILE"},
}

// --no-umask
var actionNoUmaskFlag = cmdline.Flag{
	ID:           "actionNoUmask",
	Value:        &noUmask,
	DefaultValue: false,
	Name:         "no-umask",
	Usage:        "do not propagate umask to the container, set default 0022 umask",
	EnvKeys:      []string{"NO_UMASK"},
}

// --no-eval
var actionNoEvalFlag = cmdline.Flag{
	ID:           "actionNoEval",
	Value:        &noEval,
	DefaultValue: false,
	Name:         "no-eval",
	Usage:        "do not shell evaluate env vars or OCI container CMD/ENTRYPOINT/ARGS",
	EnvKeys:      []string{"NO_EVAL"},
}

// --blkio-weight
var actionBlkioWeightFlag = cmdline.Flag{
	ID:           "actionBlkioWeight",
	Value:        &blkioWeight,
	DefaultValue: 0,
	Name:         "blkio-weight",
	Usage:        "Block IO relative weight in range 10-1000, 0 to disable",
	EnvKeys:      []string{"BLKIO_WEIGHT"},
}

// --blkio-weight-device
var actionBlkioWeightDeviceFlag = cmdline.Flag{
	ID:           "actionBlkioWeightDevice",
	Value:        &blkioWeightDevice,
	DefaultValue: []string{},
	Name:         "blkio-weight-device",
	Usage:        "Device specific block IO relative weight",
	EnvKeys:      []string{"BLKIO_WEIGHT_DEVICE"},
}

// --cpu-shares
var actionCPUSharesFlag = cmdline.Flag{
	ID:           "actionCPUShares",
	Value:        &cpuShares,
	DefaultValue: -1,
	Name:         "cpu-shares",
	Usage:        "CPU shares for container",
	EnvKeys:      []string{"CPU_SHARES"},
}

// --cpus
var actionCPUsFlag = cmdline.Flag{
	ID:           "actionCPUs",
	Value:        &cpus,
	DefaultValue: "",
	Name:         "cpus",
	Usage:        "Number of CPUs available to container",
	EnvKeys:      []string{"CPU_SHARES"},
}

// --cpuset-cpus
var actionCPUsetCPUsFlag = cmdline.Flag{
	ID:           "actionCPUsetCPUs",
	Value:        &cpuSetCPUs,
	DefaultValue: "",
	Name:         "cpuset-cpus",
	Usage:        "List of host CPUs available to container",
	EnvKeys:      []string{"CPUSET_CPUS"},
}

// --cpuset-mems
var actionCPUsetMemsFlag = cmdline.Flag{
	ID:           "actionCPUsetMems",
	Value:        &cpuSetMems,
	DefaultValue: "",
	Name:         "cpuset-mems",
	Usage:        "List of host memory nodes available to container",
	EnvKeys:      []string{"CPUSET_MEMS"},
}

// --memory
var actionMemoryFlag = cmdline.Flag{
	ID:           "actionMemory",
	Value:        &memory,
	DefaultValue: "",
	Name:         "memory",
	Usage:        "Memory limit in bytes",
	EnvKeys:      []string{"MEMORY"},
}

// --memory-reservation
var actionMemoryReservationFlag = cmdline.Flag{
	ID:           "actionMemoryReservation",
	Value:        &memoryReservation,
	DefaultValue: "",
	Name:         "memory-reservation",
	Usage:        "Memory soft limit in bytes",
	EnvKeys:      []string{"MEMORY_RESERVATION"},
}

// --memory-swap
var actionMemorySwapFlag = cmdline.Flag{
	ID:           "actionMemorySwap",
	Value:        &memorySwap,
	DefaultValue: "",
	Name:         "memory-swap",
	Usage:        "Swap limit, use -1 for unlimited swap",
	EnvKeys:      []string{"MEMORY_SWAP"},
}

// --oom-kill-disable
var actionOomKillDisableFlag = cmdline.Flag{
	ID:           "oomKillDisable",
	Value:        &oomKillDisable,
	DefaultValue: false,
	Name:         "oom-kill-disable",
	Usage:        "Disable OOM killer",
	EnvKeys:      []string{"OOM_KILL_DISABLE"},
}

// --pids-limit
var actionPidsLimitFlag = cmdline.Flag{
	ID:           "actionPidsLimit",
	Value:        &pidsLimit,
	DefaultValue: 0,
	Name:         "pids-limit",
	Usage:        "Limit number of container PIDs, use -1 for unlimited",
	EnvKeys:      []string{"PIDS_LIMIT"},
}

// --sif-fuse
var actionSIFFUSEFlag = cmdline.Flag{
	ID:           "actionSIFFUSE",
	Value:        &sifFUSE,
	DefaultValue: false,
	Name:         "sif-fuse",
	Usage:        "attempt FUSE mount of SIF",
	EnvKeys:      []string{"SIF_FUSE"},
	Deprecated:   "FUSE mounts are now used automatically when kernel mounts are disabled / unavailable.",
}

// --proot (hidden)
var actionProotFlag = cmdline.Flag{
	ID:           "actionProot",
	Value:        &proot,
	DefaultValue: "",
	Name:         "proot",
	Usage:        "Bind proot from the host into /.singularity.d/libs",
	EnvKeys:      []string{"PROOT"},
	Hidden:       true,
}

// --device
var actionDevice = cmdline.Flag{
	ID:           "actionDevice",
	Value:        &device,
	DefaultValue: []string{},
	Name:         "device",
	Usage:        "fully-qualified CDI device name(s). A fully-qualified CDI device name consists of a VENDOR, CLASS, and NAME, which are combined as follows: <VENDOR>/<CLASS>=<NAME> (e.g. vendor.com/device=mydevice). Multiple fully-qualified CDI device names can be given as a comma separated list.",
}

// --cdi-dirs
var actionCdiDirs = cmdline.Flag{
	ID:           "actionCdiDirs",
	Value:        &cdiDirs,
	DefaultValue: []string{},
	Name:         "cdi-dirs",
	Usage:        "comma-separated list of directories in which CDI should look for device definition JSON files. If omitted, default will be: /etc/cdi,/var/run/cdi",
}

func init() {
	addCmdInit(func(cmdManager *cmdline.CommandManager) {
		cmdManager.RegisterCmd(ExecCmd)
		cmdManager.RegisterCmd(ShellCmd)
		cmdManager.RegisterCmd(RunCmd)
		cmdManager.RegisterCmd(TestCmd)

		cmdManager.SetCmdGroup("actions", ExecCmd, ShellCmd, RunCmd, TestCmd)
		actionsCmd := cmdManager.GetCmdGroup("actions")

		if instanceStartCmd != nil {
			cmdManager.SetCmdGroup("actions_instance", ExecCmd, ShellCmd, RunCmd, TestCmd, instanceStartCmd, instanceRunCmd)
			cmdManager.RegisterFlagForCmd(&actionBootFlag, instanceStartCmd, instanceRunCmd)
		} else {
			cmdManager.SetCmdGroup("actions_instance", actionsCmd...)
		}
		actionsInstanceCmd := cmdManager.GetCmdGroup("actions_instance")

		cmdManager.RegisterFlagForCmd(&actionAddCapsFlag, actionsInstanceCmd...)
		cmdManager.RegisterFlagForCmd(&actionAllowSetuidFlag, actionsInstanceCmd...)
		cmdManager.RegisterFlagForCmd(&actionAppFlag, actionsInstanceCmd...)
		cmdManager.RegisterFlagForCmd(&actionApplyCgroupsFlag, actionsInstanceCmd...)
		cmdManager.RegisterFlagForCmd(&actionDataFlag, actionsInstanceCmd...)
		cmdManager.RegisterFlagForCmd(&actionBindFlag, actionsInstanceCmd...)
		cmdManager.RegisterFlagForCmd(&actionCleanEnvFlag, actionsInstanceCmd...)
		cmdManager.RegisterFlagForCmd(&actionCompatFlag, actionsInstanceCmd...)
		cmdManager.RegisterFlagForCmd(&actionNoCompatFlag, actionsInstanceCmd...)
		cmdManager.RegisterFlagForCmd(&actionContainAllFlag, actionsInstanceCmd...)
		cmdManager.RegisterFlagForCmd(&actionContainFlag, actionsInstanceCmd...)
		cmdManager.RegisterFlagForCmd(&actionContainLibsFlag, actionsInstanceCmd...)
		cmdManager.RegisterFlagForCmd(&actionDisableCacheFlag, actionsInstanceCmd...)
		cmdManager.RegisterFlagForCmd(&actionDNSFlag, actionsInstanceCmd...)
		cmdManager.RegisterFlagForCmd(&actionDropCapsFlag, actionsInstanceCmd...)
		cmdManager.RegisterFlagForCmd(&actionFakerootFlag, actionsInstanceCmd...)
		cmdManager.RegisterFlagForCmd(&actionNoSetgroupsFlag, actionsInstanceCmd...)
		cmdManager.RegisterFlagForCmd(&actionFuseMountFlag, actionsInstanceCmd...)
		cmdManager.RegisterFlagForCmd(&actionHomeFlag, actionsInstanceCmd...)
		cmdManager.RegisterFlagForCmd(&actionHostnameFlag, actionsInstanceCmd...)
		cmdManager.RegisterFlagForCmd(&actionIpcNamespaceFlag, actionsInstanceCmd...)
		cmdManager.RegisterFlagForCmd(&actionKeepPrivsFlag, actionsInstanceCmd...)
		cmdManager.RegisterFlagForCmd(&actionMountFlag, actionsInstanceCmd...)
		cmdManager.RegisterFlagForCmd(&actionNetNamespaceFlag, actionsInstanceCmd...)
		cmdManager.RegisterFlagForCmd(&actionNetnsPathFlag, actionsInstanceCmd...)
		cmdManager.RegisterFlagForCmd(&actionNetworkArgsFlag, actionsInstanceCmd...)
		cmdManager.RegisterFlagForCmd(&actionNetworkFlag, actionsInstanceCmd...)
		cmdManager.RegisterFlagForCmd(&actionNoHomeFlag, actionsInstanceCmd...)
		cmdManager.RegisterFlagForCmd(&actionNoMountFlag, actionsInstanceCmd...)
		cmdManager.RegisterFlagForCmd(&actionNoInitFlag, actionsInstanceCmd...)
		cmdManager.RegisterFlagForCmd(&actionNoNvidiaFlag, actionsInstanceCmd...)
		cmdManager.RegisterFlagForCmd(&actionNoRocmFlag, actionsInstanceCmd...)
		cmdManager.RegisterFlagForCmd(&actionNoPrivsFlag, actionsInstanceCmd...)
		cmdManager.RegisterFlagForCmd(&actionNvidiaFlag, actionsInstanceCmd...)
		cmdManager.RegisterFlagForCmd(&actionNvCCLIFlag, actionsInstanceCmd...)
		cmdManager.RegisterFlagForCmd(&actionRocmFlag, actionsInstanceCmd...)
		cmdManager.RegisterFlagForCmd(&actionOverlayFlag, actionsInstanceCmd...)
		cmdManager.RegisterFlagForCmd(&commonPromptForPassphraseFlag, actionsInstanceCmd...)
		cmdManager.RegisterFlagForCmd(&commonPEMFlag, actionsInstanceCmd...)
		cmdManager.RegisterFlagForCmd(&actionPidNamespaceFlag, actionsCmd...)
		cmdManager.RegisterFlagForCmd(&actionNoPidNamespaceFlag, actionsCmd...)
		cmdManager.RegisterFlagForCmd(&actionCwdFlag, actionsCmd...)
		cmdManager.RegisterFlagForCmd(&actionPwdFlag, actionsCmd...)
		cmdManager.RegisterFlagForCmd(&actionScratchFlag, actionsInstanceCmd...)
		cmdManager.RegisterFlagForCmd(&actionSecurityFlag, actionsInstanceCmd...)
		cmdManager.RegisterFlagForCmd(&actionShellFlag, ShellCmd)
		cmdManager.RegisterFlagForCmd(&actionTmpDirFlag, actionsInstanceCmd...)
		cmdManager.RegisterFlagForCmd(&actionUserNamespaceFlag, actionsInstanceCmd...)
		cmdManager.RegisterFlagForCmd(&actionUtsNamespaceFlag, actionsInstanceCmd...)
		cmdManager.RegisterFlagForCmd(&actionWorkdirFlag, actionsInstanceCmd...)
		cmdManager.RegisterFlagForCmd(&actionWritableFlag, actionsInstanceCmd...)
		cmdManager.RegisterFlagForCmd(&actionWritableTmpfsFlag, actionsInstanceCmd...)
		cmdManager.RegisterFlagForCmd(&commonNoHTTPSFlag, actionsInstanceCmd...)
		cmdManager.RegisterFlagForCmd(&commonOldNoHTTPSFlag, actionsInstanceCmd...)
		cmdManager.RegisterFlagForCmd(&dockerLoginFlag, actionsInstanceCmd...)
		cmdManager.RegisterFlagForCmd(&dockerHostFlag, actionsInstanceCmd...)
		cmdManager.RegisterFlagForCmd(&dockerPasswordFlag, actionsInstanceCmd...)
		cmdManager.RegisterFlagForCmd(&dockerUsernameFlag, actionsInstanceCmd...)
		cmdManager.RegisterFlagForCmd(&actionEnvFlag, actionsInstanceCmd...)
		cmdManager.RegisterFlagForCmd(&actionEnvFileFlag, actionsInstanceCmd...)
		cmdManager.RegisterFlagForCmd(&actionNoUmaskFlag, actionsInstanceCmd...)
		cmdManager.RegisterFlagForCmd(&actionNoEvalFlag, actionsInstanceCmd...)
		cmdManager.RegisterFlagForCmd(&actionBlkioWeightFlag, actionsInstanceCmd...)
		cmdManager.RegisterFlagForCmd(&actionBlkioWeightDeviceFlag, actionsInstanceCmd...)
		cmdManager.RegisterFlagForCmd(&actionCPUSharesFlag, actionsInstanceCmd...)
		cmdManager.RegisterFlagForCmd(&actionCPUsFlag, actionsInstanceCmd...)
		cmdManager.RegisterFlagForCmd(&actionCPUsetCPUsFlag, actionsInstanceCmd...)
		cmdManager.RegisterFlagForCmd(&actionCPUsetMemsFlag, actionsInstanceCmd...)
		cmdManager.RegisterFlagForCmd(&actionMemoryFlag, actionsInstanceCmd...)
		cmdManager.RegisterFlagForCmd(&actionMemoryReservationFlag, actionsInstanceCmd...)
		cmdManager.RegisterFlagForCmd(&actionMemorySwapFlag, actionsInstanceCmd...)
		cmdManager.RegisterFlagForCmd(&actionOomKillDisableFlag, actionsInstanceCmd...)
		cmdManager.RegisterFlagForCmd(&actionPidsLimitFlag, actionsInstanceCmd...)
		cmdManager.RegisterFlagForCmd(&actionSIFFUSEFlag, actionsCmd...)
		cmdManager.RegisterFlagForCmd(&actionProotFlag, actionsCmd...)
		cmdManager.RegisterFlagForCmd(&commonOCIFlag, actionsInstanceCmd...)
		cmdManager.RegisterFlagForCmd(&commonNoOCIFlag, actionsInstanceCmd...)
		cmdManager.RegisterFlagForCmd(&commonKeepLayersFlag, actionsInstanceCmd...)
		cmdManager.RegisterFlagForCmd(&actionTmpSandbox, actionsInstanceCmd...)
		cmdManager.RegisterFlagForCmd(&actionNoTmpSandbox, actionsInstanceCmd...)
		cmdManager.RegisterFlagForCmd(&commonAuthFileFlag, actionsInstanceCmd...)
		cmdManager.RegisterFlagForCmd(&actionDevice, actionsCmd...)
		cmdManager.RegisterFlagForCmd(&actionCdiDirs, actionsCmd...)
	})
}
