// Copyright (c) 2019-2021, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package singularityconf

// currentConfig corresponds to the current configuration, may
// be useful for packages requiring to share the same configuration.
var currentConfig *File

// SetCurrentConfig sets the provided configuration as the current
// configuration.
func SetCurrentConfig(config *File) {
	currentConfig = config
}

// GetCurrentConfig returns the current configuration if any.
func GetCurrentConfig() *File {
	return currentConfig
}

// File describes the singularity.conf file options
type File struct {
	AllowSetuid             bool     `default:"yes" authorized:"yes,no" directive:"allow setuid"`
	AllowIpcNs              bool     `default:"yes" authorized:"yes,no" directive:"allow ipc ns"`
	AllowPidNs              bool     `default:"yes" authorized:"yes,no" directive:"allow pid ns"`
	AllowUserNs             bool     `default:"yes" authorized:"yes,no" directive:"allow user ns"`
	AllowUtsNs              bool     `default:"yes" authorized:"yes,no" directive:"allow uts ns"`
	ConfigPasswd            bool     `default:"yes" authorized:"yes,no" directive:"config passwd"`
	ConfigGroup             bool     `default:"yes" authorized:"yes,no" directive:"config group"`
	ConfigResolvConf        bool     `default:"yes" authorized:"yes,no" directive:"config resolv_conf"`
	MountProc               bool     `default:"yes" authorized:"yes,no" directive:"mount proc"`
	MountSys                bool     `default:"yes" authorized:"yes,no" directive:"mount sys"`
	MountDevPts             bool     `default:"yes" authorized:"yes,no" directive:"mount devpts"`
	MountHome               bool     `default:"yes" authorized:"yes,no" directive:"mount home"`
	MountTmp                bool     `default:"yes" authorized:"yes,no" directive:"mount tmp"`
	MountHostfs             bool     `default:"no" authorized:"yes,no" directive:"mount hostfs"`
	UserBindControl         bool     `default:"yes" authorized:"yes,no" directive:"user bind control"`
	EnableFusemount         bool     `default:"yes" authorized:"yes,no" directive:"enable fusemount"`
	EnableUnderlay          bool     `default:"yes" authorized:"yes,no" directive:"enable underlay"`
	MountSlave              bool     `default:"yes" authorized:"yes,no" directive:"mount slave"`
	AllowContainerSIF       bool     `default:"yes" authorized:"yes,no" directive:"allow container sif"`
	AllowContainerEncrypted bool     `default:"yes" authorized:"yes,no" directive:"allow container encrypted"`
	AllowContainerSquashfs  bool     `default:"yes" authorized:"yes,no" directive:"allow container squashfs"`
	AllowContainerExtfs     bool     `default:"yes" authorized:"yes,no" directive:"allow container extfs"`
	AllowContainerDir       bool     `default:"yes" authorized:"yes,no" directive:"allow container dir"`
	AllowKernelSquashfs     bool     `default:"yes" authorized:"yes,no" directive:"allow kernel squashfs"`
	AllowKernelExtfs        bool     `default:"yes" authorized:"yes,no" directive:"allow kernel extfs"`
	AlwaysUseNv             bool     `default:"no" authorized:"yes,no" directive:"always use nv"`
	UseNvCCLI               bool     `default:"no" authorized:"yes,no" directive:"use nvidia-container-cli"`
	AlwaysUseRocm           bool     `default:"no" authorized:"yes,no" directive:"always use rocm"`
	SharedLoopDevices       bool     `default:"no" authorized:"yes,no" directive:"shared loop devices"`
	MaxLoopDevices          uint     `default:"256" directive:"max loop devices"`
	SessiondirMaxSize       uint     `default:"64" directive:"sessiondir max size"`
	MountDev                string   `default:"yes" authorized:"yes,no,minimal" directive:"mount dev"`
	EnableOverlay           string   `default:"try" authorized:"yes,no,try" directive:"enable overlay"`
	BindPath                []string `default:"/etc/localtime,/etc/hosts" directive:"bind path"`
	LimitContainerOwners    []string `directive:"limit container owners"`
	LimitContainerGroups    []string `directive:"limit container groups"`
	LimitContainerPaths     []string `directive:"limit container paths"`
	AllowNetUsers           []string `directive:"allow net users"`
	AllowNetGroups          []string `directive:"allow net groups"`
	AllowNetNetworks        []string `directive:"allow net networks"`
	AllowNetnsPaths         []string `directive:"allow netns paths"`
	RootDefaultCapabilities string   `default:"full" authorized:"full,file,no" directive:"root default capabilities"`
	MemoryFSType            string   `default:"tmpfs" authorized:"tmpfs,ramfs" directive:"memory fs type"`
	CniConfPath             string   `directive:"cni configuration path"`
	CniPluginPath           string   `directive:"cni plugin path"`
	CryptsetupPath          string   `directive:"cryptsetup path"`
	GoPath                  string   `directive:"go path"`
	LdconfigPath            string   `directive:"ldconfig path"`
	MksquashfsPath          string   `directive:"mksquashfs path"`
	MksquashfsProcs         uint     `default:"0" directive:"mksquashfs procs"`
	MksquashfsMem           string   `directive:"mksquashfs mem"`
	NvidiaContainerCliPath  string   `directive:"nvidia-container-cli path"`
	UnsquashfsPath          string   `directive:"unsquashfs path"`
	DownloadConcurrency     uint     `default:"3" directive:"download concurrency"`
	DownloadPartSize        uint     `default:"5242880" directive:"download part size"`
	DownloadBufferSize      uint     `default:"32768" directive:"download buffer size"`
	SystemdCgroups          bool     `default:"yes" authorized:"yes,no" directive:"systemd cgroups"`
	SIFFUSE                 bool     `default:"no" authorized:"yes,no" directive:"sif fuse"`
	OCIMode                 bool     `default:"no" authorized:"yes,no" directive:"oci mode"`
	TmpSandboxAllowed       bool     `default:"yes" authorized:"yes,no" directive:"tmp sandbox"`
}

const TemplateAsset = `# SINGULARITY.CONF
# This is the global configuration file for Singularity. This file controls
# what the container is allowed to do on a particular host, and as a result
# this file must be owned by root.

# ALLOW SETUID: [BOOL]
# DEFAULT: yes
# Should we allow users to utilize the setuid program flow within Singularity?
# note1: This is the default mode, and to utilize all features, this option
# must be enabled.  For example, without this option loop mounts of image 
# files will not work; only sandbox image directories, which do not need loop
# mounts, will work (subject to note 2).
# note2: If this option is disabled, it will rely on unprivileged user
# namespaces which have not been integrated equally between different Linux
# distributions.
allow setuid = {{ if eq .AllowSetuid true }}yes{{ else }}no{{ end }}

# OCI MODE: [BOOL]
# DEFAULT: no
# Should we use the OCI runtime, and push/pull OCI-SIF images by default?
# Mimics always specifying --oci on the command line.
# Can be reversed by specifying --no-oci on the command line.
# Note that OCI mode requires unprivileged user namespace creation and
# subuid / subgid mappings.
oci mode = {{ if eq .OCIMode true }}yes{{ else }}no{{ end }}

# MAX LOOP DEVICES: [INT]
# DEFAULT: 256
# Set the maximum number of loop devices that Singularity should ever attempt
# to utilize.
max loop devices = {{ .MaxLoopDevices }}

# ALLOW IPC NS: [BOOL]
# DEFAULT: yes
# Should we allow users to request the IPC namespace?
allow ipc ns = {{ if eq .AllowIpcNs true }}yes{{ else }}no{{ end }}

# ALLOW PID NS: [BOOL]
# DEFAULT: yes
# Should we allow users to request the PID namespace? Note that for some HPC
# resources, the PID namespace may confuse the resource manager and break how
# some MPI implementations utilize shared memory. (note, on some older
# systems, the PID namespace is always used)
allow pid ns = {{ if eq .AllowPidNs true }}yes{{ else }}no{{ end }}

# ALLOW USER NS: [BOOL]
# DEFAULT: yes
# Should we allow users to request the USER namespace?
allow user ns = {{ if eq .AllowUserNs true }}yes{{ else }}no{{ end }}


# ALLOW UTS NS: [BOOL]
# DEFAULT: yes
# Should we allow users to request the UTS namespace?
allow uts ns = {{ if eq .AllowUtsNs true }}yes{{ else }}no{{ end }}

# CONFIG PASSWD: [BOOL]
# DEFAULT: yes
# If /etc/passwd exists within the container, this will automatically append
# an entry for the calling user.
config passwd = {{ if eq .ConfigPasswd true }}yes{{ else }}no{{ end }}

# CONFIG GROUP: [BOOL]
# DEFAULT: yes
# If /etc/group exists within the container, this will automatically append
# group entries for the calling user.
config group = {{ if eq .ConfigGroup true }}yes{{ else }}no{{ end }}

# CONFIG RESOLV_CONF: [BOOL]
# DEFAULT: yes
# If there is a bind point within the container, use the host's
# /etc/resolv.conf.
config resolv_conf = {{ if eq .ConfigResolvConf true }}yes{{ else }}no{{ end }}

# MOUNT PROC: [BOOL]
# DEFAULT: yes
# Should we automatically bind mount /proc within the container?
mount proc = {{ if eq .MountProc true }}yes{{ else }}no{{ end }}

# MOUNT SYS: [BOOL]
# DEFAULT: yes
# Should we automatically bind mount /sys within the container?
mount sys = {{ if eq .MountSys true }}yes{{ else }}no{{ end }}

# MOUNT DEV: [yes/no/minimal]
# DEFAULT: yes
# Should we automatically bind mount /dev within the container? If 'minimal'
# is chosen, then only 'null', 'zero', 'random', 'urandom', and 'shm' will
# be included (the same effect as the --contain options)
#
# Must be set to 'yes' or 'minimal' to use --oci mode.
mount dev = {{ .MountDev }}

# MOUNT DEVPTS: [BOOL]
# DEFAULT: yes
# Should we mount a new instance of devpts if there is a 'minimal'
# /dev, or -C is passed?  Note, this requires that your kernel was
# configured with CONFIG_DEVPTS_MULTIPLE_INSTANCES=y, or that you're
# running kernel 4.7 or newer.
#
# Must be set to 'yes' to use --oci mode.
mount devpts = {{ if eq .MountDevPts true }}yes{{ else }}no{{ end }}

# MOUNT HOME: [BOOL]
# DEFAULT: yes
# Should we automatically determine the calling user's home directory and
# attempt to mount it's base path into the container? If the --contain option
# is used, the home directory will be created within the session directory or
# can be overridden with the SINGULARITY_HOME or SINGULARITY_WORKDIR
# environment variables (or their corresponding command line options).
mount home = {{ if eq .MountHome true }}yes{{ else }}no{{ end }}

# MOUNT TMP: [BOOL]
# DEFAULT: yes
# Should we automatically bind mount /tmp and /var/tmp into the container? If
# the --contain option is used, both tmp locations will be created in the
# session directory or can be specified via the  SINGULARITY_WORKDIR
# environment variable (or the --workingdir command line option).
mount tmp = {{ if eq .MountTmp true }}yes{{ else }}no{{ end }}

# MOUNT HOSTFS: [BOOL]
# DEFAULT: no
# Probe for all mounted file systems that are mounted on the host, and bind
# those into the container?
mount hostfs = {{ if eq .MountHostfs true }}yes{{ else }}no{{ end }}

# BIND PATH: [STRING]
# DEFAULT: Undefined
# Define a list of files/directories that should be made available from within
# the container. The file or directory must exist within the container on
# which to attach to. you can specify a different source and destination
# path (respectively) with a colon; otherwise source and dest are the same.
#
# In native mode, these are ignored if singularity is invoked with --contain except
# for /etc/hosts and /etc/localtime. When invoked with --contain and --net,
# /etc/hosts would contain a default generated content for localhost resolution.
#
# In OCI mode these are only mounted when --no-compat is specified.
#bind path = /etc/singularity/default-nsswitch.conf:/etc/nsswitch.conf
#bind path = /opt
#bind path = /scratch
{{ range $path := .BindPath }}
{{- if ne $path "" -}}
bind path = {{$path}}
{{ end -}}
{{ end }}
# USER BIND CONTROL: [BOOL]
# DEFAULT: yes
# Allow users to influence and/or define bind points at runtime? This will allow
# users to specify bind points, scratch and tmp locations. (note: User bind
# control is only allowed if the host also supports PR_SET_NO_NEW_PRIVS)
user bind control = {{ if eq .UserBindControl true }}yes{{ else }}no{{ end }}

# ENABLE FUSEMOUNT: [BOOL]
# DEFAULT: yes
# Allow users to mount fuse filesystems inside containers with the --fusemount
# command line option.
enable fusemount = {{ if eq .EnableFusemount true }}yes{{ else }}no{{ end }}

# ENABLE OVERLAY: [yes/no/try]
# DEFAULT: try
# Enabling this option will make it possible to specify bind paths to locations
# that do not currently exist within the container.  If 'try' is chosen,
# overlayfs will be tried but if it is unavailable it will be silently ignored.
enable overlay = {{ .EnableOverlay }}

# ENABLE UNDERLAY: [yes/no]
# DEFAULT: yes
# Enabling this option will make it possible to specify bind paths to locations
# that do not currently exist within the container even if overlay is not
# working.  If overlay is available, it will be tried first.
enable underlay = {{ if eq .EnableUnderlay true }}yes{{ else }}no{{ end }}

# TMP SANDBOX: [yes/no]
# DEFAULT: yes
# If set to yes, container images will automatically be extracted to a
# temporary sandbox directory when mounting the image is not supported.
# If set to no, images will not be extracted to a temporary sandbox
# in action/instance flows. An explicit build to a sandbox will be required.
tmp sandbox = {{ if eq .TmpSandboxAllowed true }}yes{{ else }}no{{ end }}

# MOUNT SLAVE: [BOOL]
# DEFAULT: yes
# Should we automatically propagate file-system changes from the host?
# This should be set to 'yes' when autofs mounts in the system should
# show up in the container.
mount slave = {{ if eq .MountSlave true }}yes{{ else }}no{{ end }}

# SESSIONDIR MAXSIZE: [STRING]
# DEFAULT: 64
# This specifies how large the default tmpfs sessiondir should be (in MB).
# The sessiondir is used to hold data written to isolated directories when
# running with --contain, ephemeral changes when running with --writable-tmpfs.
# In --oci mode, each tmpfs mount in the container can be up to this size.
sessiondir max size = {{ .SessiondirMaxSize }}

# *****************************************************************************
# WARNING
#
# The 'limit container' and 'allow container' directives are not effective if
# unprivileged user namespaces are enabled. They are only effectively applied
# when Singularity is running using the native runtime in setuid mode, and
# unprivileged container execution is not possible on the host.
#
# You must disable unprivileged user namespace creation on the host if you rely
# on the these directives to limit container execution. This will disable OCI
# mode, which is unprivileged and cannot enforce these limits.
#
# See the 'Security' and 'Configuration Files' sections of the Admin Guide for
# more information.
# *****************************************************************************

# LIMIT CONTAINER OWNERS: [STRING]
# DEFAULT: NULL
# Only allow containers to be used that are owned by a given user. If this
# configuration is undefined (commented or set to NULL), all containers are
# allowed to be used. 
#
# Only effective in setuid mode, with unprivileged user namespace creation disabled.
# Ignored for the root user.
#limit container owners = gmk, singularity, nobody
{{ range $index, $owner := .LimitContainerOwners }}
{{- if eq $index 0 }}limit container owners = {{ else }}, {{ end }}{{$owner}}
{{- end }}

# LIMIT CONTAINER GROUPS: [STRING]
# DEFAULT: NULL
# Only allow containers to be used that are owned by a given group. If this
# configuration is undefined (commented or set to NULL), all containers are
# allowed to be used.
#
# Only effective in setuid mode, with unprivileged user namespace creation disabled.
# Ignored for the root user.
#limit container groups = group1, singularity, nobody
{{ range $index, $group := .LimitContainerGroups }}
{{- if eq $index 0 }}limit container groups = {{ else }}, {{ end }}{{$group}}
{{- end }}

# LIMIT CONTAINER PATHS: [STRING]
# DEFAULT: NULL
# Only allow containers to be used that are located within an allowed path
# prefix. If this configuration is undefined (commented or set to NULL),
# containers will be allowed to run from anywhere on the file system.
#
# Only effective in setuid mode, with unprivileged user namespace creation disabled.
# Ignored for the root user.
#limit container paths = /scratch, /tmp, /global
{{ range $index, $path := .LimitContainerPaths }}
{{- if eq $index 0 }}limit container paths = {{ else }}, {{ end }}{{$path}}
{{- end }}

# ALLOW CONTAINER ${TYPE}: [BOOL]
# DEFAULT: yes
# This feature limits what kind of containers that Singularity will allow
# users to use.
#
# Only effective in setuid mode, with unprivileged user namespace creation disabled.
# Ignored for the root user.
#
# Allow use of unencrypted SIF containers
allow container sif = {{ if eq .AllowContainerSIF true}}yes{{ else }}no{{ end }}
#
# Allow use of encrypted SIF containers
allow container encrypted = {{ if eq .AllowContainerEncrypted true }}yes{{ else }}no{{ end }}
#
# Allow use of non-SIF image formats
allow container squashfs = {{ if eq .AllowContainerSquashfs true }}yes{{ else }}no{{ end }}
allow container extfs = {{ if eq .AllowContainerExtfs true }}yes{{ else }}no{{ end }}
allow container dir = {{ if eq .AllowContainerDir true }}yes{{ else }}no{{ end }}

# ALLOW KERNEL SQUASHFS: [BOOL]
# DEFAULT: yes
# If set to no, Singularity will not perform any kernel mounts of squashfs filesystems.
# Instead, for SIF / SquashFS containers, a squashfuse mount will be attempted, with
# extraction to a temporary sandbox directory if this fails.
# Applicable to setuid mode only.
allow kernel squashfs = {{ if eq .AllowKernelSquashfs true }}yes{{ else }}no{{ end }}

# ALLOW KERNEL EXTFS: [BOOL]
# DEFAULT: yes
# If set to no, Singularity will not perform any kernel mounts of extfs filesystems.
# This affects both stand-alone image files and filesystems embedded in a SIF file.
# Applicable to setuid mode only.
allow kernel extfs = {{ if eq .AllowKernelExtfs true }}yes{{ else }}no{{ end }}

# ALLOW NET USERS: [STRING]
# DEFAULT: NULL
# A list of non-root users that are permitted to use the CNI configurations
# specified in the 'allow net networks' directive, and can join existing
# network namespaces listed in the 'allow netns paths' directive.
# By default only root may use CNI configurations, or join existing network
# namespaces, except in the case of a fakeroot execution where only the 
# 40_fakeroot.conflist CNI configuration is used. The restriction only applies
# when Singularity is running in SUID mode and the user is non-root.
#allow net users = gmk, singularity
{{ range $index, $owner := .AllowNetUsers }}
{{- if eq $index 0 }}allow net users = {{ else }}, {{ end }}{{$owner}}
{{- end }}

# ALLOW NET GROUPS: [STRING]
# DEFAULT: NULL
# A list of non-root groups that are permitted to use the CNI configurations
# specified in the 'allow net networks' directive, and can join existing
# network namespaces listed in the 'allow netns paths' directive.
# By default only root may use CNI configurations, or join existing network
# namespaces, except in the case of a fakeroot execution where only the 
# 40_fakeroot.conflist CNI configuration is used. The restriction only applies
# when Singularity is running in SUID mode and the user is non-root.
#allow net groups = group1, singularity
{{ range $index, $group := .AllowNetGroups }}
{{- if eq $index 0 }}allow net groups = {{ else }}, {{ end }}{{$group}}
{{- end }}

# ALLOW NET NETWORKS: [STRING]
# DEFAULT: NULL
# Specify the names of CNI network configurations that may be used by users and
# groups listed in the allow net users / allow net groups directives. This restriction
# only applies when Singularity is running in SUID mode and the user is non-root.
#allow net networks = bridge
{{ range $index, $group := .AllowNetNetworks }}
{{- if eq $index 0 }}allow net networks = {{ else }}, {{ end }}{{$group}}
{{- end }}

# ALLOW NETNS PATHS: [STRING]
# DEFAULT: NULL
# Specify the paths to network namespaces that may be joined by users and groups
# listed in the allow net users / allow net groups directives. This restriction
# only applies when Singularity is running in SUID mode and the user is non-root.
#allow netns paths = /var/run/netns/my_network
{{ range $index, $path := .AllowNetnsPaths }}
{{- if eq $index 0 }}allow netns paths = {{ else }}, {{ end }}{{$path}}
{{- end }}

# ALWAYS USE NV ${TYPE}: [BOOL]
# DEFAULT: no
# This feature allows an administrator to determine that every action command
# should be executed implicitly with the --nv option (useful for GPU only 
# environments). 
always use nv = {{ if eq .AlwaysUseNv true }}yes{{ else }}no{{ end }}

# USE NVIDIA-NVIDIA-CONTAINER-CLI ${TYPE}: [BOOL]
# DEFAULT: no
# EXPERIMENTAL
# If set to yes, Singularity will attempt to use nvidia-container-cli to setup
# GPUs within a container when the --nv flag is enabled.
# If no (default), the legacy binding of entries in nvbliblist.conf will be performed.
use nvidia-container-cli = {{ if eq .UseNvCCLI true }}yes{{ else }}no{{ end }}

# ALWAYS USE ROCM ${TYPE}: [BOOL]
# DEFAULT: no
# This feature allows an administrator to determine that every action command
# should be executed implicitly with the --rocm option (useful for GPU only
# environments).
always use rocm = {{ if eq .AlwaysUseRocm true }}yes{{ else }}no{{ end }}

# ROOT DEFAULT CAPABILITIES: [full/file/no]
# DEFAULT: full
# Define default root capability set kept during runtime.
# Applies to the singularity runtime only, not --oci mode.
# - full: keep all capabilities (same as --keep-privs)
# - file: keep capabilities configured for root in
#         ${prefix}/etc/singularity/capability.json
# - no: no capabilities (same as --no-privs)
root default capabilities = {{ .RootDefaultCapabilities }}

# MEMORY FS TYPE: [tmpfs/ramfs]
# DEFAULT: tmpfs
# This feature allow to choose temporary filesystem type used by Singularity.
# Cray CLE 5 and 6 up to CLE 6.0.UP05 there is an issue (kernel panic) when Singularity
# use tmpfs, so on affected version it's recommended to set this value to ramfs to avoid
# kernel panic
memory fs type = {{ .MemoryFSType }}

# CNI CONFIGURATION PATH: [STRING]
# DEFAULT: Undefined
# Defines path from where CNI configuration files are stored
#cni configuration path =
{{ if ne .CniConfPath "" }}cni configuration path = {{ .CniConfPath }}{{ end }}
# CNI PLUGIN PATH: [STRING]
# DEFAULT: Undefined
# Defines path from where CNI executable plugins are stored
#cni plugin path =
{{ if ne .CniPluginPath "" }}cni plugin path = {{ .CniPluginPath }}{{ end }}

# CRYPTSETUP PATH: [STRING]
# DEFAULT: Undefined
# Path to the cryptsetup executable, used to work with encrypted containers.
# Must be set to build or run encrypted containers.
# Executable must be owned by root for security reasons.
# cryptsetup path =
{{ if ne .CryptsetupPath "" }}cryptsetup path = {{ .CryptsetupPath }}{{ end }}

# GO PATH: [STRING]
# DEFAULT: Undefined
# Path to the go executable, used to compile plugins.
# If not set, SingularityCE will search $PATH, /usr/local/sbin, /usr/local/bin,
# /usr/sbin, /usr/bin, /sbin, /bin.
# go path =
{{ if ne .GoPath "" }}go path = {{ .GoPath }}{{ end }}

# LDCONFIG PATH: [STRING]
# DEFAULT: Undefined
# Path to the ldconfig executable, used to find GPU libraries.
# Must be set to use --nv / --nvccli.
# Executable must be owned by root for security reasons.
# ldconfig path =
{{ if ne .LdconfigPath "" }}ldconfig path = {{ .LdconfigPath }}{{ end }}

# MKSQUASHFS PATH: [STRING]
# DEFAULT: Undefined
# Path to the mksquashfs executable, used to create SIF and SquashFS containers.
# If not set, SingularityCE will search $PATH, /usr/local/sbin, /usr/local/bin,
# /usr/sbin, /usr/bin, /sbin, /bin.
# mksquashfs path =
{{ if ne .MksquashfsPath "" }}mksquashfs path = {{ .MksquashfsPath }}{{ end }}

# MKSQUASHFS PROCS: [UINT]
# DEFAULT: 0 (All CPUs)
# This allows the administrator to specify the number of CPUs for mksquashfs 
# to use when building an image.  The fewer processors the longer it takes.
# To enable it to use all available CPU's set this to 0.
# mksquashfs procs = 0
mksquashfs procs = {{ .MksquashfsProcs }}

# MKSQUASHFS MEM: [STRING]
# DEFAULT: Unlimited
# This allows the administrator to set the maximum amount of memory for mkswapfs
# to use when building an image.  e.g. 1G for 1gb or 500M for 500mb. Restricting memory
# can have a major impact on the time it takes mksquashfs to create the image.
# NOTE: This fuctionality did not exist in squashfs-tools prior to version 4.3
# If using an earlier version you should not set this.
# mksquashfs mem = 1G
{{ if ne .MksquashfsMem "" }}mksquashfs mem = {{ .MksquashfsMem }}{{ end }}

# NVIDIA-CONTAINER-CLI PATH: [STRING]
# DEFAULT: Undefined
# Path to the nvidia-container-cli executable, used to find GPU libraries.
# Must be set to use --nvccli.
# Executable must be owned by root for security reasons.
# nvidia-container-cli path =
{{ if ne .NvidiaContainerCliPath "" }}nvidia-container-cli path = {{ .NvidiaContainerCliPath }}{{ end }}

# UNSQUASHFS PATH: [STRING]
# DEFAULT: Undefined
# Path to the unsquashfs executable, used to extract SIF and SquashFS containers
# If not set, SingularityCE will search $PATH, /usr/local/sbin, /usr/local/bin,
# /usr/sbin, /usr/bin, /sbin, /bin.
# unsquashfs path =
{{ if ne .UnsquashfsPath "" }}unsquashfs path = {{ .UnsquashfsPath }}{{ end }}

# SHARED LOOP DEVICES: [BOOL]
# DEFAULT: no
# Allow to share same images associated with loop devices to minimize loop
# usage and optimize kernel cache (useful for MPI)
shared loop devices = {{ if eq .SharedLoopDevices true }}yes{{ else }}no{{ end }}

# DOWNLOAD CONCURRENCY: [UINT]
# DEFAULT: 3
# This option specifies how many concurrent streams when downloading (pulling)
# an image from cloud library.
download concurrency = {{ .DownloadConcurrency }}

# DOWNLOAD PART SIZE: [UINT]
# DEFAULT: 5242880
# This option specifies the size of each part when concurrent downloads are
# enabled.
download part size = {{ .DownloadPartSize }}

# DOWNLOAD BUFFER SIZE: [UINT]
# DEFAULT: 32768
# This option specifies the transfer buffer size when concurrent downloads
# are enabled.
download buffer size = {{ .DownloadBufferSize }}

# SYSTEMD CGROUPS: [BOOL]
# DEFAULT: yes
# Whether to use systemd to manage container cgroups. Required for rootless cgroups
# functionality. 'no' will manage cgroups directly via cgroupfs.
systemd cgroups = {{ if eq .SystemdCgroups true }}yes{{ else }}no{{ end }}

# SIF FUSE: [BOOL]
# DEFAULT: no
# DEPRECATED - FUSE mounts are now used automatically when kernel mounts are disabled / unavailable.
# Whether to try mounting SIF images with Squashfuse by default.
# Applies only to unprivileged / user namespace flows. Requires squashfuse and
# fusermount on PATH. Will fall back to extracting the SIF on failure.
sif fuse = {{ if eq .SIFFUSE true }}yes{{ else }}no{{ end }}
`
