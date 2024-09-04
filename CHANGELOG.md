# SingularityCE Changelog

## Changes Since Last Release

## 4.2.0 \[2024-09-04\]

### New Features & Functionality

- It is now possible to use multiple environment variable files using the
  `--env-file` flag, files can be specified as a comma-separated list or
  by using the flag multiple times. Variables defined in later files take
  precedence.
- `singularity.conf` now accepts setting new options regarding namespaces:
  - `allow ipc ns` : disable the use of the `--ipc` flag.
  - `allow user ns` : disable creation of user namespaces. This will prevent
    execution of containers with the `--userns` or `--fakeroot` flags, and
    unprivileged installations of SingularityCE.
  - `allow uts ns` : invalidate the use of the `--uts` and `--hostname` flags.
- A new `singularity data package` command allows files and directories to
  be packaged into an OCI-SIF data container.
- A new `--layer-format` flag for `singularity push` allows layers in an OCI-SIF
  image to be pushed to `library://` and `docker://` registries in `squashfs`
  (default) or `tar` format. Images pushed with `--layer-format tar` can be
  pulled and run by other OCI runtimes.
- A writable overlay can be added to an OCI-SIF file with the `singularity
  overlay create` command. The overlay will be applied read-only, by default,
  when executing the OCI-SIF. To write changes to the container into the overlay,
  use the `--writable` flag.
- A writable overlay is added to an OCI-SIF file as an ext3 format layer,
  appended to the encapsulated OCI image. After the overlay has been modified,
  use the `singularity overlay sync` command to synchronize the OCI digests with
  the overlay content.
- A new `singularity overlay seal` command converts a writable overlay inside
  an OCI-SIF image into a read-only squashfs layer. This seals changes made to
  the image via the overlay, so that they are permanent.
- Added a new `instance run` command that will execute the runscript when an
  instance is initiated instead of executing the startscript.
- The new `--netns-path` flag takes a path to a network namespace to join when
  starting a container. The `root` user may join any network namespace. An
  unprivileged user can only join a network namespace specified in the new
  `allowed netns paths` directive in `singularity.conf`, if they are also listed
  in `allowed net users` / `allowed net groups`. Not currently supported with
  `--fakeroot`, or in `--oci` mode.

### Requirements

- Requires a minimum of Go 1.21.5 to build due to dependency updates.
- OCI-SIF embedded writable overlay functionality requires `fuse2fs` >= 1.46.6.

## 4.1.5 \[2024-08-14\]

### Bug Fixes

- Fix fall-back to temporary sandbox rootfs bundle in OCI-Mode for OCI URIs
  (`docker://`) etc.
- Fix confusing error messages / incorrect fall-back attempt when explicit
  execution of an OCI-SIF fails.
- Fix failing builds from local images that have symbolic links for paths that
  are part of the base container environment (e.g. /var/tmp -> /tmp).
- Fix issue where `--platform` / `--arch` did not apply when pulling an OCI
  image to native SIF via image manifest, rather than image index.

## 4.1.4 \[2024-06-28\]

### Bug Fixes

- Use ABI 3 for Apparmor profile on Ubuntu <23.10.
- Avoid unnecessary copying / extraction of OCI images and Docker tarballs into
  a layout directory when they are directly accessible as a local file /
  directory.
- Avoid unnecessary intermediate temporary image layout when building from
  Dockerfile to OCI-SIF.
- `%files from` in a definition file will now correctly copy symlinks that point
  to a target above the destination directory, but inside the destination stage
  rootfs.

## 4.1.3 \[2024-05-08\]

### Requirements

- Requires a minimum of Go 1.21 to build. Go 1.20 is end-of-life.

### Bug Fixes

- Set default `PATH` in container run in OCI-Mode when image does not set `PATH`.
- Fix storage of credentials for `docker.io` to behave the same as for
  `index.docker.io`.
- Improve documentation for `remote list` command.
- Don't fail with lack of descriptor capacity when writing OCI images with many
  layers to OCI-SIF.
- Ensure a fixed number of spare descriptors is present in the OCI-SIF when
  pulling an OCI image.

## 4.1.2 \[2024-03-05\]

### Bug Fixes

- Set OCI runtime-spec annotations that are required by the documented
  image-spec conversion process.
- In `--oci` mode always set inner ID map based on host user, not `USER` in OCI
  container. Fixes incorrect permissions for files owned by `USER` in the
  container.
- Provide warning / info message for OCI image-spec features (volumes, exposed
  ports) that are not supported by singularity.
- Honor `WORKDIR` by default for OCI images in `--oci` mode, as required by OCI
  image-spec.
- Restore previous `--writable` behaviour when running a container image from
  SIF/SquashFS in user namepace mode. The image will be extracted to a temporary
  sandbox, which is writable at runtime. Note that any changes are not made to
  the original image.
- Fix `target: no such file or directory` error in native mode when extracting
  layers from certain OCI images that manipulate hard links across layers.
- Fix extraction of OCI layers when run in a root mapped user namespace
  (e.g.. `unshare -r`).
- Use user namespace for wrapping of `unsquashfs` when singularity is run with
  `--userns / -u` flag. Fixes temporary sandbox extraction of images in non-root
  mapped user namespace (e.g. `unshare -c`).

## 4.1.1 \[2024-02-01\]

### Security Related Fixes

- Update `github.com/moby/buildkit` dependency, used for `--oci` Dockerfile
  builds, addressing the following upstream CVEs:
  - [CVE-2024-23650](https://github.com/moby/buildkit/security/advisories/GHSA-9p26-698r-w4hx)
    Possible panic when incorrect parameters sent from frontend
  - [CVE-2024-23651](https://github.com/moby/buildkit/security/advisories/GHSA-m3r6-h7wv-7xxv)
    Possible race condition with accessing subpaths from cache mounts.
  - [CVE-2024-23652](https://github.com/moby/buildkit/security/advisories/GHSA-4v98-7qmw-rqr8)
    Possible host system access from mount stub cleaner.
  - [CVE-2024-23653](https://github.com/moby/buildkit/security/advisories/GHSA-wr6v-9f75-vh2g)
    Interactive containers API does not validate entitlements check.

Note also that in OCI-Mode, SingularityCE may call out to `runc` versions
vulnerable to
[CVE-2024-21626](https://github.com/opencontainers/runc/security/advisories/GHSA-xr7r-f8xq-vfvv).
`runc` is not bundled with SingularityCE, and should be updated via your Linux
distribution's package manager, or manually.

### Bug Fixes

- Workaround segfault in `crun` v1.11+ when no resource limits are specified.
  <https://github.com/containers/crun/issues/1402>

## 4.1.0 \[2024-01-25\]

### Changed defaults / behaviours

- `--oci` mode containers and native mode instances can now be successfully
  started as a non-root user on cgroups v2 systems when both:
  - The system configuration / environment does not provide the correct
    information necessary to communicate with systemd via dbus.
  - Resource limits (e.g. `--cpus`) have not been requested.

  The container / instance will be started in the current cgroup, and information
  about the configuration issue displayed to the user as warnings.
- In native mode, SIF/SquashFS container images will now be mounted with
  squashfuse when kernel mounts are disabled in `singularity.conf`, or cannot be
  used (non-setuid / user namespace workflow). If the FUSE mount fails,
  Singularity will fall back to extracting the container to a temporary sandbox
  in order to run it.
- In native mode, bare extfs container images will now be mounted with
  fuse2fs when kernel mounts are disabled in `singularity.conf`, or cannot be
  used (non-setuid / user namespace workflow).

### New Features & Functionality

- The `registry login` and `registry logout` commands now support a `--authfile
  <path>` flag, which causes the OCI credentials to be written to / removed from
  a custom file located at `<path>` instead of the default location
  (`$HOME/.singularity/docker-config.json`). The commands `pull`, `push`, `run`,
  `exec`, `shell`, and `instance start` can now also be passed a `--authfile
  <path>` option, to read OCI registry credentials from this custom file.
- A new `--keep-layers` flag, for the `pull` and `run/shell/exec/instance start`
  commands, allows individual layers to be preserved when an OCI-SIF image is
  created from an OCI source. Multi layer OCI-SIF images can be run with
  SingularityCE 4.1 and later.
- Singularity will now build OCI-SIF images from Dockerfiles, if the `--oci`
  flag is used with the `build` command. Provide a Dockerfile as the final
  argument to `build`, instead of a Singularity definition (.def) file. Supports
  `--build-arg` / `--build-arg-file` options, `--arch` for cross-architecture
  builds, `--authfile` and other authentication options, and more. See the [user
  guide](https://docs.sylabs.io/guides/latest/user-guide/build_a_container.html#dockerfile)
  for more information.
- Docker-style SCIF containers
  ([https://sci-f.github.io/tutorial-preview-install](https://sci-f.github.io/tutorial-preview-install))
  are now supported. If the entrypoint of an OCI container is the `scif`
  executable, then the `run` / `exec` / `shell` commands in `--oci` mode can be
  given the `--app <appname>` flag, and will automatically invoke the relevant
  SCIF command.
- A new `--tmp-sandbox` flag has been added to the `run / shell / exec /
  instance start` commands. This will force Singularity to extract a container
  to a temporary sandbox before running it, when it would otherwise perform a
  kernel or FUSE mount.

### Bug Fixes

- Added missing `tmp sandbox` directive to `singularity.conf` template.

### Deprecated Functionality

- The experimental `--sif-fuse` flag, and `sif fuse` directive in
  `singularity.conf` are deprecated. The flag and directive were used to enable
  experimental mounting of SIF/SquashFS container images with FUSE in prior
  versions of Singularity. From 4.1, FUSE mounts are used automatically when
  kernel mounts are disabled / not available.

## 4.0.3 \[2024-01-11\]

### Bug Fixes

- Use kernel overlayfs instead of `fuse-overlayfs` when running as root user,
  regardless of unprivileged kernel overlay support.
- Execute correct `%appstart` script when using `instance start` with `--app`.

## 4.0.2 \[2023-11-16\]

### Changed defaults / behaviours

- Added `libnvidia-nvvm` to `nvliblist.conf`. Newer NVIDIA Drivers (known with
  >= 525.85.05) require this lib to compile OpenCL programs against NVIDIA GPUs,
  i.e. `libnvidia-opencl` depends on `libnvidia-nvvm`.

### Bug Fixes

- Support parentheses in `test` / `[` commands in container startup scripts,
  via dependency update of mvdan.cc/sh.
- Fix incorrect client timeout during remote build context upload.
- When user requests a bind of `/dev:/dev` or `/dev/xxx:/dev/xxx` in OCI-mode,
  ensure that it is bind mounted with appropriate flags so that it is usable in
  the container.

## 4.0.1 \[2023-10-13\]

### New Features & Functionality

- Added the upcoming NVIDIA driver library `libnvidia-gpucomp.so` to the
  list of libraries to add to NVIDIA GPU-enabled containers.

### Bug Fixes

- Don't bind `/var/tmp` on top of `/tmp` in the container, where `/var/tmp`
  resolves to same location as `/tmp`.
- Fix problem where credentials locally stored with `registry login` command
  were not usable in some execution flows. Run `registry login` again with
  latest version to ensure credentials are stored correctly.
- Don't fail in a yum bootstrap on systems where the _db_backend rpm macros is
  not defined (EL <8).

## 4.0.0 \[2023-09-19\]

### OCI-mode

Singularity 4 introduces OCI-mode as a fully supported feature. It is enabled by
using the `--oci` flag with the `run / shell / exec / pull` commands, or by
setting `oci mode = yes` in `singularity.conf`.

In OCI-mode:

- Container images from OCI sources will be `pull`-ed to an OCI-SIF file. An
  OCI-SIF file encapsulates the OCI image configuration and squashed filesystem
  using an OCI, rather than Singularity specific, structure.
- The `run / shell / exec` commands use a low-level OCI runtime (crun/runc) for container
  execution.
- Default operation is compatible with other OCI tools, similar to using
  `--compat` in Singularity's non-OCI native mode.
- OCI-modes support running existing Singularity non-OCI-SIF images, and can be
  made to imitate native mode default behavior by using the `--no-compat` flag.

OCI-mode changes from 3.11 to 4.0 include:

- `run / shell / exec` in OCI-mode now includes support for the following
  existing CLI flags:
  - `--add-caps`
  - `--drop-caps`
  - `--keep-privs`
  - `--no-privs`
  - `--overlay` from directories, bare squashfs and extfs images.
  - `--workdir`
  - `--scratch`
  - `--no-home`
  - `--no-mount` (dev cannot be disabled in OCI mode)
  - `--no-umask` (with `--no-compat`)
  - `--writable-tmpfs` (with `--no-compat`)
- Added `--device` flag to "action" commands (`run`/`exec`/`shell`) when run in
  OCI mode (`--oci`). Currently supports passing one or more (comma-separated)
  fully-qualified CDI device names, and those devices will then be made
  available inside the container.
- Added `--cdi-dirs` flag to override the default search locations for CDI
  json files, allowing, for example, users who don't have root access on their
  host machine to nevertheless create CDI mappings (into containers run with
  `--fakeroot`, for example).
- A container run as root, or with `--fakeroot`, has OCI default
  effective/permitted capabilities.
- An `--env-file` is evaluated with respect to the host environment, to match
  native mode behaviour.
- If the kernel does not support unprivileged overlays, OCI-mode will attempt to
  use `fuse-overlayfs` and `fusermount` for overlay mounting and unmounting.
- Support for thee `SINGULARITY_CONTAINLIBS` env var, to specify libraries to
  bind into `/.singularity.d/libs/` in the container.
- Support for running OCI-SIF images directly from `docker://`, `http://`,
  `https://` and `oras://` URIs.
- A new `--no-compat` flag can be used with OCI-mode to mirror singularity's
  historic native mode behavior on a variety of settings, instead of setting
  them the way other OCI runtimes typically do:
  - `$HOME`, `/tmp`, `/var/tmp` are bind mounted from the host.
  - The full `/dev` is bind mounted from the host, unless `mount dev = minimal`
    in `singularity.conf` (requires `crun`, not applied with `runc`).
  - `bind path` entries in `singularity.conf` are mounted into the container.
  - The current working directory is mounted into the container, and is the
    entry point into the container.
  - The container is read-only unless `--writable-tmpfs` is also used.
  - The host umask is propagated into the container, unless `--no-umask` is also
    used.
  - When a native (non-OCI-SIF) image is run in OCI-mode, environment variables
    will be shell evaluated on container startup.
- The `pull` command now accepts a new flag `--oci` for OCI image sources. This
  will create an OCI-SIF image rather than convert to Singularity's native
  container format.
- OCI-SIF containers can be pushed/pulled to/from OCI registries as single file
  artifacts using `oras://` URIs.
- OCI-SIF containers can be pushed/pulled to/from registries as OCI images, with
  a single squashfs layer, using `docker://` URIs.
- A new `oci mode` directive in `singularity.conf` can be set to true to enable
  OCI-mode by default. It can be negated with a new `--no-oci` command line flag.

See the [admin guide](https://docs.sylabs.io/guides/latest/admin-guide/) and
[user guide](https://docs.sylabs.io/guides/latest/user-guide/) for full
requirements of OCI-mode and usage information.

### Changed defaults / behaviours

#### Packages / Requirements

- RPM packages now use `/var/lib/singularity` (rather than `/var/singularity`)
  to store local state files.
- Bash completions are now install to the modern
  `share/bash-completion/completions` location, rather than under `etc`.
- The `--vm` and related flags to start singularity inside a VM have been
  removed. This functionality was related to the retired Singularity Desktop /
  SyOS projects.
- Singularity uses `squashfuse_ll` / `squashfuse`, which is now built from a git
  submodule unless `--without-squashfuse` is specified as an argument to
  `mconfig`. When built with `--without-squashfuse`, `squashfuse_ll` or
  `squashfuse` will be located on `PATH`. Version 0.2.0 or later is required.

#### CLI

- The commands related to OCI/Docker registries that were under `remote` have
  been moved to their own, dedicated `registry` command. Run
  `singularity help registry` for more information.
- The `remote list` subcommand now outputs only remote endpoints (with
  keyservers and OCI/Docker registries having been moved to separate commands),
  and the output has been streamlined.
- Adding a new remote endpoint using the `singularity remote add` command will
  now set the new endpoint as default. This behavior can be suppressed by
  supplying the `--no-default` (or `-n`) flag to `remote add`.
- The keyserver-related commands that were under `remote` have been moved to
  their own, dedicated `keyserver` command. Run `singularity help keyserver` for
  more information.
- Improved the clarity of `singularity key list` output.
- `--cwd` is now the preferred form of the flag for setting the container's
  working directory, though `--pwd` is still supported for compatibility.

#### Runtime Behaviour

- The way `--home` is handled when running as root (e.g. `sudo singularity`) or
  with `--fakeroot` has changed. Previously, we were only modifying the `HOME`
  environment variable in these cases, while leaving the container's
  `/etc/passwd` file unchanged (with its homedir field pointing to `/root`,
  regardless of the value passed to `--home`). With this change, both the value of
  `HOME` and the contents of `/etc/passwd` in the container will reflect the
  value passed to `--home`.
- Bind mounts are now performed in the order of their occurrence on the command
  line, or within the value of the `SINGULARITY_BIND` environment variable.
  (Previously, image-mounts were always performed first, regardless of order.)
- Default OCI config generated with `singularity mount` no longer sets any
  inheritable / ambient capabilites, matching other OCI runtimes.
- `singularity oci mount` now uses, and requires, `squashfuse_ll` or
  `squashfuse` to mount a SIF image to an OCI bundle. Note that `squashfuse_ll`
  is built with singularity unless `--without-squashfuse` is passed to
  `mconfig`.
- The current working directory is created in the container when it doesn't
  exist, so that it can be entered. You must now specify `--no-mount home,cwd`
  instead of just `--no-mount home` to avoid mounting from `$HOME` if you run
  `singularity` from inside `$HOME`.
- If the path of the current working directory in the container and on
  the host contain symlinks to different locations, it will not be mounted.

### New Features & Functionality

- Templating support for definition files: users can now define variables in
  definition files via a matching pair of double curly brackets. Variables of
  the form `{{ variable }}` will be replaced by a value defined either by a
  `variable=value` entry in the `%arguments` section of the definition file, or
  through new build options `--build-arg` or `--build-arg-file`.
- Added `--secret` flag (shorthand: `-s`) to `key remove` subcommand, to allow
  removal of a private key by fingerprint.
- Added `--private` as a synonym for `--secret` in `key list`, `key export`, and
  `key remove` subcommands.
- The `remote status` command will now print the username, realname, and email
  of the logged-in user, if available.
- The `cache` commands now accept `--type oci-sif` to list and clean cached
  OCI-SIF image conversions of OCI sources.
- The `instance start` command now accepts an optional `--app <name>` argument
  which invokes start script within the `%appstart <name>` section in the
  definition file. The `instance stop` command still only requires the instance
  name.
- A new `--no-pid` flag for `singularity run/shell/exec` disables the PID namespace
  inferred by `--containall` and `--compat`.
- A new `--platform` flag can be used to specify an `OS/Architecture[/Variant]`
  when pulling images from OCI or library sources. When pulling from library
  sources the optional variant is ignored.
- The `--arch` flag can now be used to specify a required architecture when pulling
  images from OCI, as well as library sources.
- Execution flows that unpack an image into a temporary sandbox dir can now be
  disabled, by setting "tmp sandbox = no" in `singularity.conf` or by passing
  `--no-tmp-sandbox` to the relevant `run / shell / exec` command.

### Developer / API

- Support for image driver plugins, deprecated at 3.11, has been removed.
  Unprivileged kernel overlay is supported without a plugin. In
  `singularity.conf`, the `image driver` directive has been removed, and
  `enable overlay` no longer supports the `driver` option.
- Changes in `pkg/build/types.Definition` struct. New `.FullRaw` field introduced,
  which always contains the raw data for the entire definition file. Behavior of
  `.Raw` field has changed: for multi-stage builds parsed with
  `pkg/build/types/parser.All()`, `.Raw` contains the raw content of a single
  build stage. Otherwise, it is equal to `.FullRaw`.
- The SingularityCE go module is now `github.com/sylabs/singularity/v4`,
  reflecting the major version of the application.

### Bug Fixes

- Fix interaction between `--workdir` when given relative path and `--scratch`.
- Set correct `$HOME` in `--oci` mode when `mount home = no` in
  `singularity.conf`.
- Lookup and store user/group information in stage one prior to entering any
  namespaces to fix issue with winbind not correctly lookup user/group
  information when using user namespace.
- Caching of OCI images is now architecture aware. This fixes behaviour where a
  user's home directory is shared between systems of different architectures.
- Fix compilation with the `mconfig` `-b` option (custom builddir).

-----

## 3.11.5 \[2023-09-15\]

### Changed defaults / behaviours

- If commands that expect an image file are given an OCI-SIF image, an error
  will be generated advising the user that this format is only supported in
  versions 4.0 and up.

### Bug Fixes

- Improved help text for `compile` and `install` subcommands of `plugin`
  command. Thanks to tonghuaroot
  ([https://github.com/tonghuaroot](https://github.com/tonghuaroot)) for the
  suggested improvements.

## 3.11.4 \[2023-06-22\]

### Changed defaults / behaviours

- Add `xino=on` mount option for writable kernel overlay mount points to fix
  inode numbers consistency after kernel cache flush.

### New Features & Functionality

- The `tap` CNI plugin, new to github.com/containernetworking/plugins v1.3.0,
  is now provided.
- Added `remote get-login-password` subcommand that allows the user to
  retrieve a CLI token to interact with the OCI registry of a
  Singularity Enterprise instance.
- Added `--no-setgroups` flag for `--fakeroot` builds and run/shell/exec. This
  prevents the `setgroups` syscall being used on the container process in the
  fakeroot user namespace. Maintains access from within the user namespace to
  files on the host that have permissions based on supplementary group
  membership. Note that supplementary groups are mapped to `nobody` in the
  container, and `chgrp`, `newgrp`, etc. cannot be used.
- Added ability to set a custom user config directory (default
  `$HOME/.singularity`) via the new `SINGULARITY_CONFIGDIR` environment
  variable.

### Bug Fixes

- In `--oci` mode, do not attempt to use unprivileged overlay on systems that do
  not support it.
- Fix dropped "n" characters on some platforms in definition file stored as part
  of SIF metadata.
- Pass STDIN to `--oci` containers correctly, to fix piping input to a container.
- Fix compilation on 32-bit systems.
- Fix seccomp filters to allow mknod/mknodat syscalls to create pipe/socket
  and character devices with device number 0 for fakeroot builds.
- Fix freeze when copying files between stages in an unprivileged proot build.
- Fix non-POSIX sh operator in mconfig.
- Correct internal name for CAP_BLOCK_SUSPEND.

## 3.11.3 \[2023-05-04\]

### Changed defaults / behaviours

- `--oci` mode now provides a writable container by default, using a tmpfs
  overlay. This improves parity with `--compat` mode in the native runtime, as
  `--compat` enables `--writable-tmpfs`.

### Bug Fixes

- Ensure the `allow kernel squashfs` directive in `singularity.conf` applies to
  encrypted squashfs filesystems in a SIF.

## 3.11.2 \[2023-04-27\]

### New Features & Functionality

- OCI mode now supports `--hostname` (requires UTS namespace, therefore this
  flag will infer `--uts`).
- OCI mode now supports `--scratch` (shorthand: `-S`) to mount a tmpfs scratch
  directory in the container.
- Support `--pwd` in OCI mode.
- OCI mode now supports `--home`. Supplying a single location (e.g.
  `--home /myhomedir`) will result in a new tmpfs directory being created at the
  specified location inside the container, and that dir being set as the
  in-container user's home dir. Supplying two locations separated by a colon
  (e.g. `--home /home/user:/myhomedir`) will result in the first location on the
  host being bind-mounted as the second location in-container, and set as
  the in-container user's home dir.
- OCI mode now handles `--dns` and `resolv.conf` on par with native mode: the
  `--dns` flag can be used to pass a comma-separated list of DNS servers that
  will be used in the container; if this flag is not used, the container will
  use the same `resolv.conf` settings as the host.
- Added `allow kernel squashfs` directive to `singularity.conf`. Defaults to
  `yes`. When set to no, Singularity will not mount squashfs filesystems using
  the kernel squashfs driver.
- Added `allow kernel extfs` directive to `singularity.conf`. Defaults to `yes`.
  When set to no, Singularity will not mount extfs filesystems using the kernel
  extfs driver.

### Bug Fixes

- Require `runc` in RPM packages built on SLES, not `crun`, because `crun` is
  part of the Package Hub community repository that may not be enabled.
  SingularityCE will still prefer `crun` if it has been installed.
- Use `/dev/loop-control` for loop device creation, to avoid issues with recent
  kernel patch where `max_loop` is not set.
- Always request inner userns in `--oci` mode without `--fakeroot`, so that
  inner id mapping is applied correctly.
- Use correct target uid/gid for inner id mappings in `--oci` mode.
- Avoid `runc` cgroup creation error when using `--oci` from a root-owned cgroup
  (e.g. ssh login session scope).
- Pass host's `TERM` environment variable to container in OCI mode. Can be
  overridden by setting `SINGULARITYENV_TERM` on host.
- Honour `config passwd` and `config group` directives from `singularity.conf`
  in `--oci` mode.
- Honour `mount proc` / `mount sys` / `mount tmp` / `mount home` directives from
  `singularity.conf` in `--oci` mode.
- Corrected `singularity.conf` comment, to refer to correct file as source
  of default capabilities when `root default capabilities = file`.

## 3.11.1 \[2023-03-14\]

### New Features & Functionality

- Add `setopt` definition file header for the `yum` bootstrap agent. The
  `setopt` value is passed to `yum / dnf` using the `--setopt` flag. This
  permits setting e.g. `install_weak_deps=False` to bootstrap recent versions of
  Fedora, where `systemd` (a weak dependency) cannot install correctly in the
  container. See `examples/Fedora` for an example defintion file.
- Warn user that a `yum` bootstrap of an older distro may fail if the host rpm
  `_db_backend` is not `bdb`.

### Bug Fixes

- Fix implied `--writable-tmpfs` with `--nvccli`, to avoid r/o filesytem
  error.
- Avoid incorrect error when requesting fakeroot network.
- Pass computed `LD_LIBRARY_PATH` to wrapped unsquashfs. Fixes issues where
  `unsquashfs` on host uses libraries in non-default paths.
- Show correct memory limit in `instance stats` when a limit is set.
- Ensure consistent binding of libraries under `--nv/--rocm` when duplicate
  `<library>.so[.version]` files are listed by `ldconfig -p`.
- Fix systemd cgroup manager error when running a container as a non-root
  user with `--oci`, on systems with cgroups v1 and `runc`.
- Fix joining cgroup of instance started as root, with cgroups v1,
  non-default cgroupfs manager, and no device rules.

### Changed defaults / behaviours

- Show standard output of yum bootstrap if log level is verbose or higher.

## 3.11.0 \[2023-02-10\]

### Changed defaults / behaviours

- Image driver plugins, implementing the `RegisterImageDriver` callback, are
  deprecated and will be removed in 4.0. Support for the example plugin,
  permitting Ubuntu unprivileged overlay functionality, has been replaced with
  direct support for kernel unprivileged overlay.
- When the kernel supports unprivileged overlay mounts in a user namespace, the
  container will be constructed using an overlay instead of underlay layout.
- `crun` will be used as the low-level OCI runtime, when available, rather than
  `runc`. If `crun` is not available, `runc` will be used.
- `sessiondir maxsize` in `singularity.conf` now defaults to 64 MiB for new
  installations. This is an increase from 16 MiB in prior versions.
- Instances are started in a cgroup, by default, when run as root or when
  unified cgroups v2 with systemd as manager is configured. This allows
  `singularity instance stats` to be supported by default when possible.

### New features / functionalities

#### Image Building

- Support for a custom hashbang in the `%test` section of a Singularity recipe
  (akin to the runscript and start sections).
- Non-root users can now build from a definition file, on systems that do not
  support `--fakeroot`. This requires the statically built `proot` command
  (<https://proot-me.github.io/>) to be available on the user `PATH`. These
  builds:
  - Do not support `arch` / `debootstrap` / `yum` / `zypper` bootstraps. Use
    `localimage`, `library`, `oras`, or one of the docker/oci sources.
  - Do not support `%pre` and `%setup` sections.
  - Run the `%post` sections of a build in the container as an emulated root
    user.
  - Run the `%test` section of a build as the non-root user, like `singularity
    test`.
  - Are subject to any restrictions imposed in `singularity.conf`.
  - Incur a performance penalty due to `proot`'s `ptrace` based interception of
    syscalls.
  - May fail if the `%post` script requires privileged operations that `proot`
    cannot emulate.

#### Instances

- Instances started by a non-root user can use `--apply-cgroups` to apply
  resource limits. Requires cgroups v2, and delegation configured via systemd.
- A new `instance stats` command displays basic resource usage statistics for a
  specified instance, running within a cgroup.

#### Mounts & Overlays

- `--writable-tmpfs` is now available when running unprivileged, or explicitly
  requesting a user namespace, on systems with a kernel that supports
  unprivileged overlay mounts in a user namespace.
- The `--no-mount` flag now accepts the value `bind-paths` to disable mounting
  of all `bind path` entries in `singularity.conf`.
- Persistent overlays (`--overlay`) from a directory are now available when
  running unprivileged, or explicitly requesting a user namespace, on systems
  with a kernel that supports unprivileged overlay mounts in a user namespace.
- Add `--sparse` flag to `overlay create` command to allow generation of a
  sparse ext3 overlay image.

#### OCI / Docker Compatibility

- Support for `DOCKER_HOST` parsing when using `docker-daemon://`
- `DOCKER_USERNAME` and `DOCKER_PASSWORD` supported without `SINGULARITY_` prefix.
- A new `--oci` flag for `run/exec/shell` enables the experimental OCI runtime
  mode. This mode:
  - Runs OCI container images from an OCI bundle, using `runc` or `crun`.
  - Supports `docker://`, `docker-archive:`, `docker-daemon:`, `oci:`,
    `oci-archive:` image sources.
  - Does not support running Singularity SIF, SquashFS, or EXT3 images.
  - Provides an environment similar to Singularity's native runtime, running
    with `--compat`.
  - Supports the following options / flags. Other options are not yet supported:
    - `--fakeroot` for effective root in the container. Requires subuid/subgid
      mappings.
    - Bind mounts via `--bind` or `--mount`. No image mounts.
    - Additional namespaces requests with `--net`, `--uts`, `--user`.
    - Container environment variables via `--env`, `--env-file`, and
      `SINGULARITYENV_` host env vars.
    - `--rocm` to bind ROCm GPU libraries and devices into the container.
    - `--nv` to bind Nvidia driver / basic CUDA libraries and devices into the
      container.
    - `--apply-cgroups`, and the `--cpu*`, `--blkio*`, `--memory*`,
      `--pids-limit` flags to apply resource limits.
- Instance name is available inside an instance via the new
  `SINGULARITY_INSTANCE` environment variable.

#### Signing & Verification

- The `sign` command now supports signing with non-PGP key material by
  specifying the path to a private key via the `--key` flag.
- The `verify` command now supports verification with non-PGP key material by
  specifying the path to a public key via the `--key` flag.
- The `verify` command now supports verification with X.509 certificates by
  specifying the path to a certificate via the `--certificate` flag. By default,
  the system root certificate pool is used as trust anchors unless overridden
  via the `--certificate-roots` flag. A pool of intermediate certificates that
  are not trust anchors, but can be used to form a certificate chain can also be
  specified via the `--certificate-intermediates` flag.
- Support for online verification checks of x509 certificates using OCSP
  protocol. (introduced flag: `verify --ocsp-verify`)

#### Other

- Add new Linux capabilities: `CAP_PERFMON`, `CAP_BPF`,
  `CAP_CHECKPOINT_RESTORE`.
- A new `--reproducible` flag for `./mconfig` will configure Singularity so that
  its binaries do not contain non-reproducible paths. This disables plugin
  functionality.

### Bug Fixes

- In `--rocm` mode, the whole of `/dev/dri` is now bound into the container when
  `--contain` is in use. This makes `/dev/dri/render` devices available,
  required for later ROCm versions.
- Overlay is blocked on the `panfs` filesystem, allowing sandbox directories to
  be run from `panfs` without error.
- Avoid UID / GID readonly var warnings with `--env-file`.

### Development / Testing

- Significant reduction in the use of network image sources in the e2e tests.
- Improved parallelization and use of image caches in the e2e tests.
- The `e2e-test` makefile target now accepts an argument `E2E_GROUPS` to only
  run specified groups of end to end tests. E.g. `make -C builddir e2e-test
  E2E_GROUPS=VERSION,HELP` will run end to end tests in the `VERSION` and `HELP`
  groups only.
- The `e2e-test` makefile target now accepts an argument `E2E_TESTS` which is a
  regular expression specifying the names of (top level) end to end tests that
  should be run. E.g. `make -C builddir e2e-test E2E_TESTS=^semantic` will only
  run end to end tests with a name that begins with `semantic`. These `E2E_`
  variables offer an alternative to the `-run` flag, which may be easier to use
  given the structure of e2e tests.

## 3.10.5 \[2022-01-17\]

### Security Related Fixes

- [CVE-2022-23538](https://github.com/sylabs/scs-library-client/security/advisories/GHSA-7p8m-22h4-9pj7):
  The github.com/sylabs/scs-library-client dependency included in SingularityCE
  \>=3.10.0, \<3.10.5 may leak user credentials to a third-party service via HTTP
  redirect. This issue is limited to `library://` access to specific Singularity
  Enterprise 1.x or 3rd party library configurations, which implement a
  concurrent multi-part download flow. Access to Singularity Enterprise 2.x, or
  Singularity Container Services (cloud.sylabs.io), does not trigger the
  vulnerable flow. See the linked advisory for full details.

## 3.10.4 \[2022-11-10\]

### Bug Fixes

- Ensure `make dist` doesn't include conmon binary or intermediate files.
- Do not hang on pull from http(s) source that doesn't provide a content-length.
- Avoid hang on fakeroot cleanup under high load seen on some
  distributions / kernels.

## 3.10.3 \[2022-10-06\]

### Security Related Fixes

- [CVE-2022-39237](https://github.com/sylabs/sif/security/advisories/GHSA-m5m3-46gj-wch8):
  The github.com/sylabs/sif/v2 dependency included in SingularityCE <=3.10.3
  does not verify that the hash algorithm(s) used are cryptographically secure
  when verifying digital signatures. This release updates to sif v2.8.1 which
  corrects this issue. See the linked advisory for references and a workaround.

### Bug Fixes

- Ensure bootstrap_history directory is populated with previous definition files,
  present in source containers used in a build.

## 3.10.2 \[2022-07-25\]

### New features / functionalities

- Added EL9 package builds to CI for GitHub releases.

### Bug Fixes

- Ensure no empty `if` branch is present in generated OCI image runscripts.
  Would prevent execution of container by other tools that are not using
  mvdan.cc/sh.

## 3.10.1 \[2022-07-18\]

### New features / functionalities

- Debug output can now be enabled by setting the `SINGULARITY_DEBUG` env var.
- Debug output is now shown for nested `singularity` calls, in wrapped
  `unsquashfs` image extraction, and build stages.

### Bug Fixes

- Fix test code that implied `%test -c <shell>` was supported - it is not.
- Fix compilation on `mipsel`.

## 3.10.0 \[2022-05-17\]

### Changed defaults / behaviours

- `master` branch of GitHub repository has been renamed to `main`.
- `oci mount` sets `Process.Terminal: true` when creating an OCI `config.json`,
  so that `oci run` provides expected interactive behavior by default.
- Default hostname for `oci mount` containers is now `singularity` instead of
  `mrsdalloway`.
- systemd is now supported and used as the default cgroups manager. Set
  `systemd cgroups = no` in `singularity.conf` to manage cgroups directly via
  the cgroupfs.
- The `singularity oci` command group now uses `runc` to manage containers.
- The `singularity oci` commands use `conmon` which is built from a git submodule,
  unless `--without-conmon` is specified as an argument to `mconfig`, in which
  case Singularity will search `PATH` for conmon. Version >=2.0.24 of conmon
  is required.
- The `singularity oci` flags `--sync-socket`, `--empty-process`, and
  `--timeout` have been removed.
- Don't prompt for y/n to overwrite an existing file when build is
  called from a non-interactive environment. Fail with an error.
- Plugins must be compiled from inside the SingularityCE source directory,
  and will use the main SingularityCE `go.mod` file. Required for Go 1.18
  support.
- seccomp support is not disabled automatically in the absence of
  seccomp headers at build time. Run `mconfig` using `--without-seccomp` and
  `--without-conmon` to disable seccomp support and building of `conmon`
  (which requires seccomp headers).
- SingularityCE now requires squashfs-tools >=4.3, which is satisfied by
  current EL / Ubuntu / Debian and other distributions.
- Added `--no-eval` to the list of flags set by the OCI/Docker `--compat` mode
  (see below).

### New features / functionalities

- Updated seccomp support allows use of seccomp profiles that set an error
  return code with `errnoRet` and `defaultErrnoRet`. Previously EPERM was hard
  coded. The example `etc/seccomp-profiles/default.json` has been updated.
- Native cgroups v2 resource limits can be specified using the `[unified]` key
  in a cgroups toml file applied via `--apply-cgroups`.
- The `--no-mount` flag & `SINGULARITY_NO_MOUNT` env var can now be used to
  disable a `bind path` entry from `singularity.conf` by specifying the
  absolute path to the destination of the bind.
- Non-root users can now use `--apply-cgroups` with `run/shell/exec` to limit
  container resource usage on a system using cgroups v2 and the systemd cgroups
  manager.
- Added `--cpu*`, `--blkio*`, `--memory*`, `--pids-limit` flags to apply cgroups
  resource limits to a container directly.
- Allow experimental direct mount of SIF images with `squashfuse` in
  user-namespace / no-setuid mode.
- New action flag `--no-eval` which:
  - Prevents shell evaluation of `SINGULARITYENV_ / --env / --env-file`
    environment variables as they are injected in the container, to match OCI
    behavior. *Applies to all containers*.
  - Prevents shell evaluation of the values of `CMD / ENTRYPOINT` and command
    line arguments for containers run or built directly from an OCI/Docker
    source. *Applies to newly built containers only, use `singularity inspect`
    to check version that container was built with*.
- Add support for `%files` section in remote builds, when a compatible remote is
  used.

### Bug Fixes

- Allow `newgidmap / newuidmap` that use capabilities instead of setuid root.
- Corrected `key search` output for results from some servers, and keys
  with multiple names.
- Pass through a literal `\n` in host environment variables to container.
- Address 401 error pulling from private library:// projects.
- Correctly launch CleanupHost process only when needed in `--sif-fuse` flow.
- Add specific error for unreadable image / overlay file.
- Ensure cgroups device limits are default allow per past behavior.
- Improve error message when remote build server does not support the `%files`
  section.
- Fix non-root instance join with unprivileged systemd managed cgroups, when
  join is from outside a user-owned cgroup.

## v3.9.9 \[2022-04-22\]

### Bug Fixes

- Use HEAD request when checking digest of remote OCI image sources, with GET as
  a fall-back. Greatly reduces Singularity's impact on Docker Hub API limits.

### New features / functionalities

- Add package build for Ubuntu 22.04 LTS.

## v3.9.8 \[2022-04-07\]

### Bug fixes

- Do not truncate environment variables with commas.
- Fix error when pushing to host-less `library://` URIs.

## v3.9.7 \[2022-03-23\]

### Bug fixes

- Support nvidia-container-cli v1.8.0 and above, via fix to capability set.
- Avoid cleanup panic when invalid file specified for --apply-cgroups.

## v3.9.6 \[2022-03-10\]

### New features / functionalities

- SingularityCE now supports the `riscv64` architecture.

### Bug fixes

- Correct library bindings for `unsquashfs` containment. Fixes errors where
  resolved library filename does not match library filename in binary (e.g. EL8,
  POWER9 with glibc-hwcaps).

## v3.9.5 \[2022-02-04\]

### Changed defaults / behaviours

- `make install` now installs man pages. A separate `make man` is not
  required.

### Bug fixes

- GitHub .deb packages correctly include man pages.
- Update dependency to correctly unset variables in container startup
  environment processing. Fixes regression in v3.9.2.
- Remove subshell overhead when processing large environments on container
  startup.

## v3.9.4 \[2022-01-19\]

### Bug fixes

- Address timeout in library pull single stream download.

## v3.9.3 \[2022-01-11\]

### Bug fixes

- Ensure MIGs are visible with `--nvccli` in non-contained mode, to match the
  legacy GPU binding behaviour.
- Avoid fd leak in loop device transient error path.

## v3.9.2 \[2021-12-10\]

### Bug fixes

- Ensure `gengodep` in build uses vendor dir when present.
- Fix `source` of a script on `PATH` and scoping of environment variables in
  definition files (via dependency update).
- Ensure a local build does not fail unnecessarily if a keyserver
  config cannot be retrieved from the remote endpoint.
- Correct documentation for sign command r.e. source of key index.
- Restructure loop device discovery to address an issue where a transient `EBUSY`
  error could lead to failure under Arvados. Also greedily try for a working
  loop device, rather than perform delayed retries on encountering `EAGAIN`,
  since we hold an exclusive lock which can block other processes.

## v3.9.1 \[2021-11-22\]

This is a security release for SingularityCE 3.9, addressing a security issue in
SingularityCE's dependencies.

### Security Related Fixes

- [CVE-2021-41190](https://github.com/advisories/GHSA-mc8v-mgrf-8f4m) /
  [GHSA-77vh-xpmg-72qh](https://github.com/opencontainers/image-spec/security/advisories/GHSA-77vh-xpmg-72qh):
  OCI specifications allow ambiguous documents that contain both "manifests" and
  "layers" fields. Interpretation depends on the presence / value of a
  Content-Type header. SingularityCE dependencies handling the retrieval of OCI
  images have been updated to versions that reject ambiguous documents.

## v3.9.0 \[2021-11-16\]

This is the first release of SingularityCE 3.9, the Community Edition of the
Singularity container runtime hosted at <https://github.com/sylabs/singularity>.

### Changed defaults / behaviours

- Building SingularityCE 3.9.0 requires go >=1.16. We now aim to support the
  two most recent stable versions of Go. This corresponds to the Go
  [Release Maintenance Policy](https://github.com/golang/go/wiki/Go-Release-Cycle#release-maintenance)
  and [Security Policy](https://golang.org/security), ensuring critical bug
  fixes and security patches are available for all supported language versions.
- LABELs from Docker/OCI images are now inherited. This fixes a longstanding
  regression from Singularity 2.x. Note that you will now need to use `--force`
  in a build to override a label that already exists in the source Docker/OCI
  container.
- The source paths for `%files` lines in a definition file are no longer
  interpreted by a shell. This means that environment variable substitution is
  not performed. Previously, environment variables were substituted for source
  paths, but not destination paths, leading to unexpected copy behaviour.
  Globbing for source files will now follow the Go `filepath.Match` pattern
  syntax.
- Removed `--nonet` flag, which was intended to disable networking for in-VM
  execution, but has no effect.
- `--nohttps` flag has been deprecated in favour of `--no-https`. The old flag
  is still accepted, but will display a deprecation warning.
- Paths for `cryptsetup`, `go`, `ldconfig`, `mksquashfs`, `nvidia-container-cli`,
  `unsquashfs` are now found at build time by `mconfig` and written into
  `singularity.conf`. The path to these executables can be overridden by
  changing the value in `singularity.conf`.
- When calling `ldconfig` to find GPU libraries, singularity will *not* fall back
  to `/sbin/ldconfig` if the configured `ldconfig` errors. If installing in a
  Guix/Nix on environment on top of a standard host distribution you *must* set
  `ldconfig path = /sbin/ldconfig` to use the host distribution `ldconfig` to
  find GPU libraries.
- `--nv` will not call `nvidia-container-cli` to find host libraries, unless
  the new experimental GPU setup flow that employs `nvidia-container-cli`
  for all GPU related operations is enabled (see below).
- If a container is run with `--nvcli` and `--contain`, only GPU devices
  specified via the `NVIDIA_VISIBLE_DEVICES` environment variable will be
  exposed within the container. Use `NVIDIA_VISIBLE_DEVICES=all` to access all
  GPUs inside a container run with `--nvccli`.
- Example log-plugin rewritten as a CLI callback that can log all commands
  executed, instead of only container execution, and has access to command
  arguments.
- The bundled reference CNI plugins are updated to v1.0.1. The `flannel` plugin
  is no longer included, as it is maintained as a separate plugin at:
  <https://github.com/flannel-io/cni-plugin>. If you use the flannel CNI plugin
  you should install it from this repository.
- Instances are no longer created with an IPC namespace by default. An IPC
  namespace can be specified with the `-i|--ipc` flag.
- The behaviour of the `allow container` directives in `singularity.conf` has
  been modified, to support more intuitive limitations on the usage of SIF and non-SIF
  container images. If you use these directives, *you may need to make changes
  to singularity.conf to preserve behaviour*.
  - A new `allow container sif` directive permits or denies usage of
    *unencrypted* SIF images, irrespective of the filesystem(s) inside the SIF.
  - The `allow container encrypted` directive permits or denies usage of SIF
    images with an encrypted root filesystem.
  - The `allow container squashfs/extfs` directives in `singularity.conf`
    permit or deny usage of bare SquashFS and EXT image files only.
  - The effect of the `allow container dir` directive is unchanged.

### New features / functionalities

- `--writable-tmpfs` can be used with `singularity build` to run the `%test`
  section of the build with a ephemeral tmpfs overlay, permitting tests that
  write to the container filesystem.
- The `--compat` flag for actions is a new short-hand to enable a number of
  options that increase OCI/Docker compatibility. Infers `--containall,
  --no-init, --no-umask, --writable-tmpfs`. Does not use user, uts, or
  network namespaces as these may not be supported on many installations.
- `remote add --insecure` may be used to configure endpoints that are only
  accessible via http.
- The experimental `--nvccli` flag will use `nvidia-container-cli` to setup the
  container for Nvidia GPU operation. SingularityCE will not bind GPU libraries
  itself. Environment variables that are used with Nvidia's `docker-nvidia`
  runtime to configure GPU visibility / driver capabilities & requirements are
  parsed by the `--nvccli` flag from the environment of the calling user. By
  default, the `compute` and `utility` GPU capabilities are configured. The `use
  nvidia-container-cli` option in `singularity.conf` can be set to `yes` to
  always use `nvidia-container-cli` when supported. Note that in a setuid
  install, `nvidia-container-cli` will be run as root with required ambient
  capabilities. `--nvccli` is not currently supported in the hybrid fakeroot
  (setuid install + `--fakeroot`) workflow. Please see documentation for more
  details.
- The `--apply-cgroups` flag can be used to apply cgroups resource and device
  restrictions on a system using the v2 unified cgroups hierarchy. The resource
  restrictions must still be specified in the v1 / OCI format, which will be
  translated into v2 cgroups resource restrictions, and eBPF device
  restrictions.
- A new `--mount` flag and `SINGULARITY_MOUNT` environment variable can be used
  to specify bind mounts in
  `type=bind,source=<src>,destination=<dst>[,options...]` format. This improves
  CLI compatibility with other runtimes, and allows binding paths containing
  `:` and `,` characters (using CSV style escaping).
- Perform concurrent multi-part downloads for `library://` URIs. Uses 3
  concurrent downloads by default, and is configurable in `singularity.conf` or
  via environment variables.

### Bug fixes

- The `oci` commands will operate on systems that use the v2 unified cgroups
  hierarchy.
- Ensure invalid values passed to `config global --set` cannot lead to an empty
  configuration file being written.
- An invalid remote build source (bootstrap) will be identified before
  attempting to submit the build.
- `--no-https` now applies to connections made to library services specified
  in `library://<hostname>/...` URIs.

## v3.8.4 \[2021-10-28\]

### Bug fixes

- Update `oras-go` dependency to address push failures to some registry
  configurations.
- Implement context cancellation when a signal is received in several CLI
  commands.

## v3.8.3 \[2021-09-01\]

## Bug fixes

- Fix regression when files `source`d from `%environment` contain `\` escaped
  shell builtins (fixes issue with `source` of conda profile.d script).

Additional changes include dependency updates for the SIF module (to v2.0.0),
and migration to maintained versions of other modules. There is no change to
functionality, on-disk SIF format etc.

## v3.8.2 \[2021-08-19\]

### Bug fixes

- `singularity delete` will use the correct library service when the hostname
  is specified in the `library://` URI.
- `singularity build` will use the correct library service when the hostname
  is specified in the `library://` URI / definition file.
- Fix download of default `pacman.conf` in `arch` bootstrap.
- Call `debootstrap` with correct Debian arch when it is not identical to the
  value of `runtime.GOARCH`. E.g. `ppc64el -> ppc64le`.
- When destination is omitted in `%files` entry in definition file, ensure
  globbed files are copied to correct resolved path.
- Return an error if `--tokenfile` used for `remote login` to an OCI registry,
  as this is not supported.
- Ensure repeated `remote login` to same URI does not create duplicate entries
  in `~/.singularity/remote.yaml`.
- Avoid panic when mountinfo line has a blank field.
- Properly escape single quotes in Docker `CMD` / `ENTRYPOINT` translation.
- Use host uid when choosing unsquashfs flags, to avoid selinux xattr errors
  with `--fakeroot` on non-EL/Fedora distributions with recent squashfs-tools.

## v3.8.1 \[2021-07-20\]

### Bug Fixes

- Allow escaped `\$` in a SINGULARITYENV\_ var to set a literal `$` in a
  container env var.
- Handle absolute symlinks correctly in multi-stage build `%copy from` blocks.
- Fix incorrect reference in sandbox restrictive permissions warning.

## v3.8.0 \[2021-05-26\]

This is the first release of SingularityCE 3.8.0, the Community Edition of the
Singularity container runtime hosted at <https://github.com/sylabs/singularity>.

### Changed defaults / behaviours

- The package name for this release is now `singularity-ce`. This name is used
  for the source tarball, output of an `rpmbuild`, and displayed in `--version`
  information.
- The name of the top level directory in the source tarball from `make dist` now
  includes the version string.

### New features / functionalities

- A new `overlay` command allows creation and addition of writable overlays.
- Administrators can allow named users/groups to use specific CNI network
  configurations. Managed by directives in `singularity.conf`.
- The `build` command now honors `--nv`, `--rocm`, and `--bind` flags,
  permitting builds that require GPU access or files bound in from the host.
- A library service hostname can be specified as the first component of a
  `library://` URL.
- Singularity is now relocatable for unprivileged installations only.

### Bug Fixes

- Respect http proxy server environment variables in key operations.
- When pushing SIF images to `oras://` endpoints, work around Harbor & GitLab
  failure to accept the `SifConfigMediaType`.
- Avoid a `setfsuid` compilation warning on some gcc versions.
- Fix a crash when silent/quiet log levels used on pulls from `shub://` and
  `http(s)://` URIs.
- Wait for dm device to appear when mounting an encrypted container rootfs.

### Testing / Development

Testing changes are not generally itemized. However, developers and contributors
should note that this release has modified the behavior of `make test` for ease
of use:

- `make test` runs limited unit and integration tests that will not require
  docker hub credentials.
- `make testall` runs the full unit/integration/e2e test suite that requires
  docker credentials to be set with `E2E_DOCKER_USERNAME` and
  `E2E_DOCKER_PASSWORD` environment variables.
- Fix privilege handling issue with tests on Go >=1.16.

-----

## v3.7.4 - \[2021-05-26\]

**Singularity 3.7.4 is the most recent stable release of Singularity prior to
Sylabs' fork from <https://github.com/hpcng/singularity>**

The 3.7.4 release is identical to
<https://github.com/hpcng/singularity/releases/tag/v3.7.4> and is provided for
convenience to users arriving from outdated links.

### Security Related Fixes

- [CVE-2021-32635](https://github.com/sylabs/singularity/security/advisories/GHSA-5mv9-q7fq-9394):
  Due to incorrect use of a default URL, singularity action commands
  (run/shell/exec) specifying a container using a library:// URI will always
  attempt to retrieve the container from the default remote endpoint
  (cloud.sylabs.io) rather than the configured remote endpoint. An attacker may
  be able to push a malicious container to the default remote endpoint with a
  URI that is identical to the URI used by a victim with a non-default remote
  endpoint, thus executing the malicious container.

## v3.7.3 - \[2021-04-06\]

### Security Related Fixes

- [CVE-2021-29136](https://github.com/opencontainers/umoci/security/advisories/GHSA-9m95-8hx6-7p9v):
  A dependency used by Singularity to extract docker/OCI image layers can be
  tricked into modifying host files by creating a malicious layer that has a
  symlink with the name "." (or "/"), when running as root. This vulnerability
  affects a `singularity build` or `singularity pull` as root, from a docker or
  OCI source.

## v3.7.2 - \[2021-03-09\]

### Bug Fixes

- Fix progress bar display when source image size is unknown.
- Fix a memory usage / leak issue when building from an existing image file.
- Fix to allow use of `--library` flag to point push/pull at default cloud
  library when another remote is in use.
- Address false positive loop test errors, and an e2e test registry setup issue.

## v3.7.1 - \[2021-01-12\]

### Bug Fixes

- Accommodate /sys/fs/selinux mount changes on kernel 5.9+.
- Fix loop devices file descriptor leak when shared loop devices is enabled.
- Use MaxLoopDevices variable from config file in all appropriate locations.
- Use -buildmode=default (non pie) on ppc64le to prevent crashes when using
  plugins.
- Remove spurious warning in parseTokenSection()
- e2e test fixes for new kernels, new unsquashfs version.
- Show correct web URI for detached builds against alternate remotes.

### New features / functionalities

- The singularity binary is now relocatable when built without setuid support

## v3.7.0 - \[2020-11-24\]

### New features / functionalities

- Allow configuration of global custom keyservers, separate from remote
  endpoints.
- Add a new global keyring, for public keys only (used for ECL).
- The `remote login` command now supports authentication to Docker/OCI
  registries and custom keyservers.
- New `--exclusive` option for `remote use` allows admin to lock usage to a
  specific remote.
- A new `Fingerprints:` header in definition files will check that a SIF source
  image can be verified, and is signed with keys matching all specified
  fingerprints.
- Labels can be set dynamically from a build's `%post` section by setting them
  in the `SINGULARITY_LABELS` environment variable.
- New `build-arch` label is automatically set to the architecture of the host
  during a container build.
- New `-D/--description` flag for `singularity push` sets description for a
  library container image.
- `singularity remote status` shows validity of authentication token if set.
- `singularity push` reports quota usage and URL on successful push to a library
  server that supports this.
- A new `--no-mount` flag for actions allows a user to disable
  proc/sys/dev/devpts/home/tmp/hostfs/cwd mounts, even if they are enabled in
  `singularity.conf`.

### Changed defaults / behaviours

- When actions (run/shell/exec...) are used without `--fakeroot` the umask from
  the calling environment will be propagated into the container, so that files
  are created with expected permissions. Use the new `--no-umask` flag to return
  to the previous behaviour of setting a default 0022 umask.
- Container metadata, environment, scripts are recorded in a descriptor in
  builds to SIF files, and `inspect` will use this if present.
- The `--nv` flag for NVIDIA GPU support will not resolve libraries reported by
  `nvidia-container-cli` via the ld cache. Will instead respect absolute paths
  to libraries reported by the tool, and bind all versioned symlinks to them.
- General re-work of the `remote login` flow, adds prompts and token
  verification before replacing an existing authentication token.
- The Execution Control List (ECL) now verifies container fingerprints using the
  new global keyring. Previously all users would need relevant keys in their own
  keyring.
- The SIF layer mediatype for ORAS has been changed to
  `application/vnd.sylabs.sif.layer.v1.sif` reflecting the published
  [opencontainers/artifacts](https://github.com/opencontainers/artifacts/blob/master/artifact-authors.md#defining-layermediatypes)
  value.
- `SINGULARITY_BIND` has been restored as an environment variable set within a
  running container. It now reflects all user binds requested by the `-B/--bind`
  flag, as well as via `SINGULARITY_BIND[PATHS]`.
- `singularity search` now correctly searches for container images matching the
  host architecture by default. A new `--arch` flag allows searching for other
  architectures. A new results format gives more detail about container image
  results, while users and collections are no longer returned.

### Bug Fixes

- Support larger definition files, environments etc. by passing engine
  configuration in the environment vs. via socket buffer.
- Ensure `docker-daemon:` and other source operations respect
  `SINGULARITY_TMPDIR` for all temporary files.
- Support double quoted filenames in the `%files` section of build definitions.
- Correct `cache list` sizes to show KiB with powers of 1024, matching `du` etc.
- Don't fail on `enable fusemount=no` when no fuse mounts are needed.
- Pull OCI images to the correct requested location when the cache is disabled.
- Ensure `Singularity>` prompt is set when container has no environment script,
  or singularity is called through a wrapper script.
- Avoid build failures in `yum/dnf` operations against the 'setup' package on
  `RHEL/CentOS/Fedora` by ensuring staged `/etc/` files do not match distro
  default content.
- Failed binds to `/etc/hosts` and `/etc/localtime` in a container run with
  `--contain` are no longer fatal errors.
- Don't initialize the cache for actions where it is not required.
- Increase embedded shell interpreter timeout, to allow slow-running environment
  scripts to complete.
- Correct buffer handling for key import to allow import from STDIN.
- Reset environment to avoid `LD_LIBRARY_PATH` issues when resolving
  dependencies for the `unsquashfs` sandbox.
- Fall back to `/sbin/ldconfig` if `ldconfig` on `PATH` fails while resolving
  GPU libraries. Fixes problems on systems using Nix / Guix.
- Address issues caused by error code changes in `unsquashfs` version 4.4.
- Ensure `/dev/kfd` is bound into container for ROCm when `--rocm` is used with
  `--contain`.
- Tolerate comments on `%files` sections in build definition files.
- Fix a loop device file descriptor leak.

### Known Issues

- A change in Linux kernel 5.9 causes `--fakeroot` builds to fail with a
  `/sys/fs/selinux` remount error. This will be addressed in Singularity v3.7.1.

## v3.6.4 - \[2020-10-13\]

### Security related fixes

Singularity 3.6.4 addresses the following security issue.

- [CVE-2020-15229](https://github.com/hpcng/singularity/security/advisories/GHSA-7gcp-w6ww-2xv9):
  Due to insecure handling of path traversal and the lack of path sanitization
  within unsquashfs (a distribution provided utility used by Singularity), it is
  possible to overwrite/create files on the host filesystem during the
  extraction of a crafted squashfs filesystem. Affects unprivileged execution of
  SIF / SquashFS images, and image builds from SIF / SquashFS images.

### Bug Fixes

- Update scs-library-client to support `library://` backends using an 3rd party
  S3 object store that does not strictly conform to v4 signature spec.

## v3.6.3 - \[2020-09-15\]

### Security related fixes

Singularity 3.6.3 addresses the following security issues.

- [CVE-2020-25039](https://github.com/hpcng/singularity/security/advisories/GHSA-w6v2-qchm-grj7):
  When a Singularity action command (run, shell, exec) is run with the fakeroot
  or user namespace option, Singularity will extract a container image to a
  temporary sandbox directory. Due to insecure permissions on the temporary
  directory it is possible for any user with access to the system to read the
  contents of the image. Additionally, if the image contains a world-writable
  file or directory, it is possible for a user to inject arbitrary content into
  the running container.

- [CVE-2020-25040](https://github.com/hpcng/singularity/security/advisories/GHSA-jv9c-w74q-6762):
  When a Singularity command that results in a container build operation is
  executed, it is possible for a user with access to the system to read the
  contents of the image during the build. Additionally, if the image contains a
  world-writable file or directory, it is possible for a user to inject
  arbitrary content into the running build, which in certain circumstances may
  enable arbitrary code execution during the build and/or when the built
  container is run.

## Change defaults / behaviours

- The value for maximum number of loop devices in the config file is now used
  everywhere instead of redefining this value

### Bug Fixes

- Add CAP_MKNOD in capability bounding set of RPC to fix issue with cryptsetup
  when decrypting image from within a docker container.
- Fix decryption issue when using both IPC and PID namespaces.
- Fix unsupported builtins panic from shell interpreter and add umask support
  for definition file scripts.
- Do not load keyring in prepare_linux if ECL not enabled.
- Ensure sandbox option overrides remote build destination.

## v3.6.2 - \[2020-08-25\]

### New features / functionalities

- Add --force option to `singularity delete` for non-interactive workflows.

### Change defaults / behaviours

- Default to current architecture for `singularity delete`.

### Bug Fixes

- Respect current remote for `singularity delete` command.
- Allow `rw` as a (noop) bind option.
- Fix capability handling regression in overlay mount.
- Fix LD_LIBRARY_PATH environment override regression with `--nv/--rocm`.
- Fix environment variable duplication within singularity engine.
- Use `-user-xattrs` for unsquashfs to avoid error with rootless extraction
  using unsquashfs 3.4 (Ubuntu 20.04).
- Correct `--no-home` message for 3.6 CWD behavior.
- Don't fail if parent of cache dir not accessible.
- Fix tests for Go 1.15 Ctty handling.
- Fix additional issues with test images on ARM64.
- Fix FUSE e2e tests to use container ssh_config.

## v3.6.1 - \[2020-07-21\]

### New features / functionalities

- Support compilation with `FORTIFY_SOURCE=2` and build in `pie` mode with
  `fstack-protector` enabled (#5433).

### Bug Fixes

- Provide advisory message r.e. need for `upper` and `work` to exist in overlay
  images.
- Use squashfs mem and processor limits in squashfs gzip check.
- Ensure build destination path is not an empty string - do not overwrite CWD.
- Don't unset PATH when interpreting legacy /environment files.

## v3.6.0 - \[2020-07-14\]

### Security related fixes

Singularity 3.6.0 introduces a new signature format for SIF images, and changes
to the signing / verification code to address:

- [CVE-2020-13845](https://cve.mitre.org/cgi-bin/cvename.cgi?name=2020-13845) In
  Singularity 3.x versions below 3.6.0, issues allow the ECL to be bypassed by a
  malicious user.
- [CVE-2020-13846](https://cve.mitre.org/cgi-bin/cvename.cgi?name=2020-13846) In
  Singularity 3.5 the `--all / -a` option to `singularity verify` returns
  success even when some objects in a SIF container are not signed, or cannot be
  verified.
- [CVE-2020-13847](https://cve.mitre.org/cgi-bin/cvename.cgi?name=2020-13847) In
  Singularity 3.x versions below 3.6.0, Singularity's sign and verify commands
  do not sign metadata found in the global header or data object descriptors of
  a SIF file, allowing an attacker to cause unexpected behavior. A signed
  container may verify successfully, even when it has been modified in ways that
  could be exploited to cause malicious behavior.

Please see the published security advisories at
<https://github.com/hpcng/singularity/security/advisories> for full detail of
these security issues.

Note that the new signature format is necessarily incompatible with Singularity
\< 3.6.0 - e.g. Singularity 3.5.3 cannot verify containers signed by 3.6.0.

We thank Tru Huynh for a report that led to the review of, and changes to, the
signature implementation.

### New features / functionalities

- Singularity now supports the execution of minimal Docker/OCI containers that
  do not contain `/bin/sh`, e.g. `docker://hello-world`.
- A new cache structure is used that is concurrency safe on a filesystem that
  supports atomic rename. *If you downgrade to Singularity 3.5 or older after
  using 3.6 you will need to run `singularity cache clean`.*
- A plugin system rework adds new hook points that will allow the development of
  plugins that modify behavior of the runtime. An image driver concept is
  introduced for plugins to support new ways of handling image and overlay
  mounts. *Plugins built for \<=3.5 are not compatible with 3.6*.
- The `--bind` flag can now bind directories from a SIF or ext3 image into a
  container.
- The `--fusemount` feature to mount filesystems to a container via FUSE drivers
  is now a supported feature (previously an experimental hidden flag). This
  permits users to mount e.g. `sshfs` and `cvmfs` filesystems to the container
  at runtime.
- A new `-c/--config` flag allows an alternative `singularity.conf` to be
  specified by the `root` user, or all users in an unprivileged installation.
- A new `--env` flag allows container environment variables to be set via the
  Singularity command line.
- A new `--env-file` flag allows container environment variables to be set from
  a specified file.
- A new `--days` flag for `cache clean` allows removal of items older than a
  specified number of days. Replaces the `--name` flag which is not generally
  useful as the cache entries are stored by hash, not a friendly name.
- A new '--legacy-insecure' flag to `verify` allows verification of SIF
  signatures in the old, insecure format.
- A new '-l / --logs' flag for `instance list` that shows the paths to instance
  STDERR / STDOUT log files.
- The `--json` output of `instance list` now include paths to STDERR / STDOUT
  log files.

### Changed defaults / behaviours

- New signature format (see security fixes above).
- Environment variables prefixed with `SINGULARITYENV_` always take precedence
  over variables without `SINGULARITYENV_` prefix.
- The `%post` build section inherits environment variables from the base image.
- `%files from ...` will now follow symlinks for sources that are directly
  specified, or directly resolved from a glob pattern. It will not follow
  symlinks found through directory traversal. This mirrors Docker multi-stage
  COPY behaviour.
- Restored the CWD mount behaviour of v2, implying that CWD path is not
  recreated inside container and any symlinks in the CWD path are not resolved
  anymore to determine the destination path inside container.
- The `%test` build section is executed the same manner as
  `singularity test image`.
- `--fusemount` with the `container:` default directive will foreground the FUSE
  process. Use `container-daemon:` for previous behavior.
- Fixed spacing of `singularity instance list` to be dynamically changing based
  off of input lengths instead of fixed number of spaces to account for long
  instance names.

### Deprecated / removed commands

- Removed `--name` flag for `cache clean`; replaced with `--days`.
- Deprecate `-a / --all` option to `sign/verify` as new signature behavior makes
  this the default.

### Bug Fixes

- Don't try to mount `$HOME` when it is `/` (e.g. `nobody` user).
- Process `%appinstall` sections in order when building from a definition file.
- Ensure `SINGULARITY_CONTAINER`, `SINGULARITY_ENVIRONMENT` and the custom shell
  prompt are set inside a container.
- Honor insecure registry settings from `/etc/containers/registries.conf`.
- Fix `http_proxy` env var handling in `yum` bootstrap builds.
- Disable log colorization when output location is not a terminal.
- Check encryption keys are usable before beginning an encrypted build.
- Allow app names with non-alphanumeric characters.
- Use the `base` metapackage for arch bootstrap builds - arch no longer has a
  `base` group.
- Ensure library client messages are logged with `--debug`.
- Do not mount `$HOME` with `--fakeroot --contain`.
- Fall back to underlay automatically when using a sandbox on GPFS.
- Fix Ctrl-Z handling - propagation of signal.

## v3.5.3 - \[2020-02-18\]

### Changed defaults / behaviours

The following minor behaviour changes have been made in 3.5.3 to allow correct
operation on CRAY CLE6, and correct an issue with multi-stage image builds that
was blocking use by build systems such as Spack:

- Container action scripts are no longer bound in from `etc/actions.d` on the
  host. They are created dynamically and inserted at container startup.
- `%files from ...` will no longer follow symlinks when copying between stages
  in a multi stage build, as symlinks should be copied so that they resolve
  identically in later stages. Copying `%files` from the host will still
  maintain previous behavior of following links.

### Bug Fixes

- Bind additional CUDA 10.2 libs when using the `--nv` option without
  `nvidia-container-cli`.
- Fix an NVIDIA persistenced socket bind error with `--writable`.
- Add detection of ceph to allow workarounds that avoid issues with sandboxes on
  ceph filesystems.
- Ensure setgid is inherited during make install.
- Ensure the root directory of a build has owner write permissions, regardless
  of the permissions in the bootstrap source.
- Fix a regression in `%post` and `%test` to honor the `-c` option.
- Fix an issue running `%post` when a container doesn't have `/etc/resolv.conf`
  or `/etc/hosts` files.
- Fix an issue with UID detection on RHEL6 when running instances.
- Fix a logic error when a sandbox image is in an overlay incompatible location,
  and both overlay and underlay are disabled globally.
- Fix an issue causing user namespace to always be used when `allow-setuid=no`
  was configured in a setuid installation.
- Always allow key IDs and fingerprints to be specified with or without a `0x`
  prefix when using `singularity keys`
- Fix an issue preventing joining an instance started with `--boot`.
- Provide a useful error message if an invalid library:// path is provided.
- Bring in multi-part upload client functionality that will address large image
  upload / proxied upload issues with a future update to Sylabs cloud.

In addition, numerous improvements have been made to the test suites, allowing
them to pass cleanly on a range of kernel versions and distributions that are
not covered by the open-source CI runs.

## v3.5.2 - \[2019-12-17\]

### [Security related fix](https://cve.mitre.org/cgi-bin/cvename.cgi?name=2019-19724)

- 700 permissions are enforced on `$HOME/.singularity` and
  `SINGULARITY_CACHEDIR` directories (CVE-2019-19724). Many thanks to Stuart
  Barkley for reporting this issue.

### Bug Fixes

- Fixes an issue preventing use of `.docker/config` for docker registry
  authentication.

- Fixes the `run-help` command in the unprivileged workflow.

- Fixes a regression in the `inspect` command to support older image formats.

- Adds a workaround for an EL6 kernel bug regarding shared bind mounts.

- Fixes caching of http(s) sources with conflicting filenames.

- Fixes a fakeroot sandbox build error on certain filesystems, e.g. lustre,
  GPFS.

- Fixes a fakeroot build failure to a sandbox in $HOME.

- Fixes a fakeroot build failure from a bad def file section script location.

- Fixes container execution errors when CWD is a symlink.

- Provides a useful warning r.e. possible fakeroot build issues when seccomp
  support is not available.

- Fixes an issue where the `--disable-cache` option was not being honored.

- Deprecated `--groupid` flag for `sign` and `verify`; replaced with
  `--group-id`.

- Removed useless flag `--url` for `sign`.

## v3.5.1 - \[2019-12-05\]

### New features / functionalities

A single feature has been added in the bugfix release, with specific
functionality:

- A new option `allow container encrypted` can be set to `no` in
  `singularity.conf` to prevent execution of encrypted containers.

### Bug Fixes

This point release addresses the following issues:

- Fixes a disk space leak when building from docker-archive.
- Makes container process SIGABRT return the expected code.
- Fixes the `inspect` command in unprivileged workflow.
- Sets an appropriate default umask during build stages, to avoid issues with
  very restrictive user umasks.
- Fixes an issue with build script content being consumed from STDIN.
- Corrects the behaviour of underlay with non-empty / symlinked CWD and absolute
  symlink binds targets.
- Fixes execution of containers when binding BTRFS filesystems.
- Fixes build / check failures for MIPS & PPC64.
- Ensures file ownership maintained when building image from sandbox.
- Fixes a squashfs mount error on kernel 5.4.0 and above.
- Fixes an underlay fallback problem, which prevented use of sandboxes on lustre
  filesystems.

## v3.5.0 - \[2019-11-13\]

### New features / functionalities

- New support for AMD GPUs via `--rocm` option added to bind ROCm devices and
  libraries into containers.
- Plugins can now modify Singularity behaviour with two mutators: CLI and
  Runtime.
- Introduced the `config global` command to edit `singularity.conf` settings
  from the CLI.
- Introduced the `config fakeroot` command to setup `subuid` and `subgid`
  mappings for `--fakeroot` from the Singularity CLI.

### Changed defaults / behaviours

- Go 1.13 adopted.
- Vendored modules removed from the Git tree, will be included in release
  tarballs.
- Singularity will now fail with an error if a requested bind mount cannot be
  made.
  - This is beneficial to fail fast in workflows where a task may fail a long
    way downstream if a bind mount is unavailable.
  - Any unavailable bind mount sources must be removed from `singularity.conf`.
- Docker/OCI image extraction now faithfully respects layer permissions.
  - This may lead to sandboxes that cannot be removed without modifying
    permissions.
  - `--fix-perms` option added to preserve old behaviour when building
    sandboxes.
  - Discussion issue for this change at:
    <https://github.com/sylabs/singularity/issues/4671>
- `Singularity>` prompt is always set when entering shell in a container.
- The current `umask` will be honored when building a SIF file.
- `instance exec` processes acquire cgroups set on `instance start`
- `--fakeroot` supports uid/subgid ranges >65536
- `singularity version` now reports semver compliant version information.

### Deprecated / removed commands

- Deprecated `--id` flag for `sign` and `verify`; replaced with `--sif-id`.

## v3.4.2 - \[2019-10-08\]

- This point release addresses the following issues:
  - Sets workable permissions on OCI -> sandbox rootless builds
  - Fallback correctly to user namespace for non setuid installation
  - Correctly handle the starter-suid binary for non-root installs
  - Creates CACHEDIR if it doesn't exist
  - Set apex loglevel for umoci to match singularity loglevel

## v3.4.1 - \[2019-09-17\]

- This point release addresses the following issues:
  - Fixes an issue where a PID namespace was always being used
  - Fixes compilation on non 64-bit architectures
  - Allows fakeroot builds for zypper, pacstrap, and debootstrap
  - Correctly detects seccomp on OpenSUSE
  - Honors GO_MODFLAGS properly in the mconfig generated makefile
  - Passes the Mac hostname to the VM in MacOS Singularity builds
  - Handles temporary EAGAIN failures when setting up loop devices on recent
    kernels
  - Fixes excessive memory usage in singularity push

## v3.4.0 - \[2019-08-30\]

### New features / functionalities

- New support for building and running encrypted containers with RSA keys and
  passphrases
  - `--pem-path` option added to the `build` and action commands for RSA based
    encrypted containers
  - `--passphrase` option added to `build` and action commands for passphrase
    based encrypted containers
  - `SINGULARITY_ENCRYPTION_PEM_PATH` and `SINGULARITY_ENCRYPTION_PASSPHRASE`
    environment variables added to serve same functions as above
  - `--encrypt` option added to `build` command to build an encrypted container
    when environment variables contain a secret
- New `--disable-cache` flag prevents caching of downloaded containers
- Added support for multi-line variables in singularity def-files
- Added support for 'indexed' def-file variables (like arrays)
- Added support for SUSE SLE Products
- Added the def-file variables: product, user, regcode, productpgp, registerurl,
  modules, otherurl (indexed)
- Support multiple-architecture tags in the SCS library
- Added a `--dry-run` flag to `cache clean`
- Added a `SINGULARITY_SYPGPDIR` environment variable to specify the location of
  PGP key data
- Added a `--nonet` option to the action commands to disable networking when
  running with the `--vm` option
- Added a `--long-list` flag to the `key search` command to preserve
- Added experimental, hidden `--fusemount` flag to pass a command to mount a
  libfuse3 based file system within the container

### Changed defaults / behaviors

- Runtime now properly honors `SINGULARITY_DISABLE_CACHE` environment variable
- `remote add` command now automatically attempts to login and a `--no-login`
  flag is added to disable this behavior
- Using the `pull` command to download an unsigned container no longer produces
  an error code
- `cache clean` command now prompts user before cleaning when run without
  `--force` option and is more verbose
- Shortened the default output of the `key search` command

### Deprecated / removed commands

- The `--allow-unsigned` flag to `pull` has been deprecated and will be removed
  in the future

## v3.3.0 - \[2019-06-17\]

### Changed defaults / behaviors

- Remote login and status commands will now use the default remote if a remote
  name is not supplied
- Added Singularity hub (`shub`) cache support when using the `pull` command
- Clean cache in a safer way by only deleting the cache subdirectories
- Improvements to the `cache clean` command

### New features / functionalities

- new `oras` URI for pushing and pulling SIF files to and from supported OCI
  registries
- added the `--fakeroot` option to `build`, `exec`, `run`, `shell`, `test`, and
  `instance start` commands to run container in a new user namespace as uid 0
- added the `fakeroot` network type for use with the `--network` option
- `sif` command to allow for the inspection and manipulation of SIF files with
  the following subcommands
  - `add` Add a data object to a SIF file
  - `del` Delete a specified object descriptor and data from SIF file
  - `dump` Extract and output data objects from SIF files
  - `header` Display SIF global headers
  - `info` Display detailed information of object descriptors
  - `list` List object descriptors from SIF files
  - `new` Create a new empty SIF image file
  - `setprim` Set primary system partition

## v3.2.1 - \[2019-05-28\]

- This point release fixes the following bugs:
  - Allows users to join instances with non-suid workflow
  - Removes false warning when seccomp is disabled on the host
  - Fixes an issue in the terminal when piping output to commands
  - Binds NVIDIA persistenced socket when `--nv` is invoked

## v3.2.0 - \[2019-05-14\]

### [Security related fix](https://cve.mitre.org/cgi-bin/cvename.cgi?name=2019-11328)

- Instance files are now stored in user's home directory for privacy and many
  checks have been added to ensure that a user can't manipulate files to change
  `starter-suid` behavior when instances are joined (many thanks to Matthias
  Gerstner from the SUSE security team for finding and securely reporting this
  vulnerability)

### New features / functionalities

- Introduced a new basic framework for creating and managing plugins
- Added the ability to create containers through multi-stage builds
  - Definitions now require `Bootstrap` be the first parameter of header
- Created the concept of a Sylabs Cloud "remote" endpoint and added the ability
  for users and admins to set them through CLI and conf files
- Added caching for images from Singularity Hub
- Made it possible to compile Singularity outside of `$GOPATH`
- Added a json partition to SIF files for OCI configuration when building from
  an OCI source
- Full integration with Singularity desktop for MacOS code base

### New Commands

- Introduced the `plugin` command group for creating and managing plugins

  - `compile` Compile a singularity plugin
  - `disable` disable an installed singularity plugin
  - `enable` Enable an installed singularity plugin
  - `inspect` Inspect a singularity plugin (either an installed one or an image)
  - `install` Install a singularity plugin
  - `list` List installed singularity plugins
  - `uninstall` Uninstall removes the named plugin from the system

- Introduced the `remote` command group to support management of Singularity
  endpoints:

  - `add` Create a new Sylabs Cloud remote endpoint
  - `list` List all remote endpoints that are configured
  - `login` Log into a remote endpoint using an authentication token
  - `remove` Remove an existing Sylabs Cloud remote endpoint
  - `status` Check the status of the services at an endpoint
  - `use` Set a remote endpoint to be used by default

- Added to the `key` command group to improve PGP key management:

  - `export` Export a public or private key into a specific file
  - `import` Import a local key into the local keyring
  - `remove` Remove a local public key

- Added the `Stage: <name>` keyword to the definition file header and the
  `from <stage name>` option/argument pair to the `%files` section to support
  multistage builds

### Deprecated / removed commands

- The `--token/-t` option has been deprecated in favor of the
  `singularity remote` command group

### Changed defaults / behaviors

- Ask to confirm password on a newly generated PGP key
- Prompt to push a key to the KeyStore when generated
- Refuse to push an unsigned container unless overridden with
  `--allow-unauthenticated/-U` option
- Warn and prompt when pulling an unsigned container without the
  `--allow-unauthenticated/-U` option
- `Bootstrap` must now be the first field of every header because of parser
  requirements for multi-stage builds

## v3.1.1 - \[2019-04-02\]

### New Commands

- New hidden `buildcfg` command to display compile-time parameters
- Added support for `LDFLAGS`, `CFLAGS`, `CGO_` variables in build system
- Added `--nocolor` flag to Singularity client to disable color in logging

### Removed Commands

- `singularity capability <add/drop> --desc` has been removed
- `singularity capability list <--all/--group/--user>` flags have all been
  removed

### New features / functionalities

- The `--builder` flag to the `build` command implicitly sets `--remote`
- Repeated binds no longer cause Singularity to exit and fail, just warn instead
- Corrected typos and improved docstrings throughout
- Removed warning when CWD does not exist on the host system
- Added support to spec file for RPM building on SLES 11

## v3.1.0 - \[2019-02-22\]

### New Commands

- Introduced the `oci` command group to support a new OCI compliant variant of
  the Singularity runtime:
  - `attach` Attach console to a running container process
  - `create` Create a container from a bundle directory
  - `delete` Delete container
  - `exec` Execute a command within container
  - `kill` Kill a container
  - `mount` Mount create an OCI bundle from SIF image
  - `pause` Suspends all processes inside the container
  - `resume` Resumes all processes previously paused inside the container
  - `run` Create/start/attach/delete a container from a bundle directory
  - `start` Start container process
  - `state` Query state of a container
  - `umount` Umount delete bundle
  - `update` Update container cgroups resources
- Added `cache` command group to inspect and manage cached files
  - `clean` Clean your local Singularity cache
  - `list` List your local Singularity cache

### New features / functionalities

- Can now build CLI on darwin for limited functionality on Mac
- Added the `scratch` bootstrap agent to build from anything
- Reintroduced support for zypper bootstrap agent
- Added the ability to overwrite a new `singularity.conf` when building from RPM
  if desired
- Fixed several regressions and omissions in [SCIF](https://sci-f.github.io/)
  support
- Added caching for containers pulled/built from the
  [Container Library](https://cloud.sylabs.io/library)
- Changed `keys` command group to `key` (retained hidden `keys` command for
  backward compatibility)
- Created an `RPMPREFIX` variable to allow RPMs to be installed in custom
  locations
- Greatly expanded CI unit and end-to-end testing

## v3.0.3 - \[2019-01-21\]

- Bind paths in `singularity.conf` are properly parsed and applied at runtime
- Singularity runtime will properly fail if `singularity.conf` file is not owned
  by the root user
- Several improvements to RPM packaging including using golang from epel,
  improved support for Fedora, and avoiding overwriting conf file on new RPM
  install
- Unprivileged `--contain` option now properly mounts `devpts` on older kernels
- Uppercase proxy environment variables are now rightly respected
- Add http/https protocols for singularity run/pull commands
- Update to SIF 1.0.2
- Add *noPrompt* parameter to `pkg/signing/Verify` function to enable silent
  verification

## v3.0.2 - \[2019-01-04\]

- Added the `--docker-login` flag to enable interactive authentication with
  docker registries
- Added support for pulling directly from HTTP and HTTPS
- Made minor improvements to RPM packaging and added basic support for alpine
  packaging
- The `$SINGULARITY_NOHTTPS`,`$SINGULARITY_TMPDIR`, and
  `$SINGULARITY_DOCKER_USERNAME`/`$SINGULARITY_DOCKER_PASSWORD` environment
  variables are now correctly respected
- Pulling from a private shub registry now works as expected
- Running a container with `--network="none"` no longer incorrectly fails with
  an error message
- Commands now correctly return 1 when incorrectly executed without arguments
- Progress bars no longer incorrectly display when running with `--quiet` or
  `--silent`
- Contents of `91-environment.sh` file are now displayed if appropriate when
  running `inspect --environment`

## v3.0.1 - \[2018-10-31\]

- Improved RPM packaging procedure via makeit
- Enhanced general stability of runtime

## v3.0.0 - \[2018-10-08\]

- Singularity is now written primarily in Go to bring better integration with
  the existing container ecosystem
- Added support for new URIs (`build` & `run/exec/shell/start`):
  - `library://` - Supports the
    [Sylabs.io Cloud Library](https://cloud.sylabs.io/library)
  - `docker-daemon:` - Supports images managed by the locally running docker
    daemon
  - `docker-archive:` - Supports archived docker images
  - `oci:` - Supports oci images
  - `oci-archive:` - Supports archived oci images
- Handling of `docker` & `oci` URIs/images now utilizes
  [containers/image](https://github.com/containers/image) to parse and convert
  those image types in a supported way
- Replaced `singularity instance.*` command group with `singularity instance *`
- The command `singularity help` now only provides help regarding the usage of
  the `singularity` command. To display an image's `help` message, use
  `singularity run-help <image path>` instead

### Removed Deprecated Commands

- Removed deprecated `singularity image.*` command group
- Removed deprecated `singularity create` command
- Removed deprecated `singularity bootstrap` command
- Removed deprecated `singularity mount` command
- Removed deprecated `singularity check` command

### New Commands

- Added `singularity run-help <image path>` command to output an image's `help`
  message
- Added `singularity sign <image path>` command to allow a user to
  cryptographically sign a SIF image
- Added `singularity verify <image path>` command to allow a user to verify a
  SIF image's cryptographic signatures
- Added `singularity keys` command to allow the management of `OpenPGP` key
  stores
- Added `singularity capability` command to allow fine grained control over the
  capabilities of running containers
- Added `singularity push` command to push images to the
  [Sylabs.io Cloud Library](https://cloud.sylabs.io/library)

### Changed Commands

#### Action Command Group (`run/shell/exec/instance start`)

- Added flags:
  - `--add-caps <string>`: Run the contained process with the specified
    capability set (requires root)
  - `--allow-setuid`: Allows setuid binaries to be mounted into the container
    (requires root)
  - `--apply-cgroups <path>`: Apply cgroups configuration from file to contained
    processes (requires root)
  - `--dns <string>`: Adds the comma separated list of DNS servers to the
    containers `resolv.conf` file
  - `--drop-caps <string>`: Drop the specified capabilities from the container
    (requires root)
  - `--fakeroot`: Run the container in a user namespace as `uid=0`. Requires a
    recent kernel to function properly
  - `--hostname <string>`: Set the hostname of the container
  - `--keep-privs`: Keep root user privilege inside the container (requires
    root)
  - `--network <string>`: Specify a list of comma separated network types
    ([CNI Plugins](https://github.com/containernetworking/cni)) to be present
    inside the container, each with its own dedicated interface in the container
  - `--network-args <string>`: Specify arguments to pass to CNI network plugins
    (set by `--network`)
  - `--no-privs`: Drop all privileges from root user inside the container
    (requires root)
  - `--security <string>`: Configure security features such as SELinux,
    Apparmor, Seccomp...
  - `--writable-tmpfs`: Run container with a `tmpfs` overlay
- The command `singularity instance start` now supports the `--boot` flag to
  boot the container via `/sbin/init`
- Changes to image mounting behavior:
  - All image formats are mounted as read only by default
  - `--writable` only works on images which can be mounted in read/write
    \[applicable to: `sandbox` and legacy `ext3` images\]
  - `--writable-tmpfs` runs the container with a writable `tmpfs`-based overlay
    \[applicable to: all image formats\]
  - `--overlay <string>` now specifies a list of `ext3`/`sandbox` images which
    are set as the containers overlay \[applicable to: all image formats\]

#### Build Command

- All images are now built as
  [Singularity Image Format (SIF)](https://www.sylabs.io/2018/03/sif-containing-your-containers/)
  images by default
- When building to a path that already exists, `singularity build` will now
  prompt the user if they wish to overwrite the file existing at the specified
  location
- The `-w|--writable` flag has been removed
- The `-F|--force` flag now overrides the interactive prompt and will always
  attempt to overwrite the file existing at the specified location
- The `-u|--update` flag has been added to support the workflow of running a
  definition file on top of an existing container \[implies `--sandbox`, only
  supports `sandbox` image types\]
- The `singularity build` command now supports the following flags for
  integration with the
  [Sylabs.io Cloud Library](https://cloud.sylabs.io/library):
  - `-r|--remote`: Build the image remotely on the Sylabs Remote Builder
    (currently unavailable)
  - `-d|--detached`: Detach from the `stdout` of the remote build \[requires
    `--remote`\]
  - `--builder <string>`: Specifies the URL of the remote builder to access
  - `--library <string>`: Specifies the URL of the
    [Sylabs.io Cloud Library](https://cloud.sylabs.io/library) to push the built
    image to when the build command destination is in the form
    `library://<reference>`
- The `bootstrap` keyword in the definition file now supports the following
  values:
  - `library`
  - `docker-daemon`
  - `docker-archive`
  - `oci`
  - `oci-archive`
- The `from` keyword in the definition file now correctly parses a `docker` URI
  which includes the `registry` and/or `namespace` components
- The `registry` and `namespace` keywords in the definition file are no longer
  supported. Instead, those values may all go into the `from` keyword
- Building from a tar archive of a `sandbox` no longer works
