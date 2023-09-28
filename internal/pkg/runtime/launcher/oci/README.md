# internal/pkg/runtime/launcher/oci

This package contains routines that configure and launch a container in an OCI
bundle format, using a low-level OCI runtime, either `crun` or `runc` at this
time. `crun` is currently preferred. `runc` is used where `crun` is not
available.

**Note** - at present, all functionality works with either `crun` or `runc`.
However, in future `crun` may be required for all functionality, as `runc` does
not support some limited ID mappings etc. that may be beneficial in an HPC
scenario.

The package contrasts with `internal/pkg/runtime/launcher/native` which executes
Singularity format containers (SIF/Sandbox/squashfs/ext3), using one of our own
runtime engines (`internal/pkg/runtime/engine/*`).

There are two flows that are implemented here.

* Basic OCI runtime operations against an existing bundle, which will be executed
  via the `singularity oci` command group. These are not widely used by
  end-users of singularity.
* A `Launcher`, that implements an `Exec` function that will be called by
  'actions' (run/shell/exec) in `--oci` mode, and will:
  * Prepare an OCI bundle according to `launcher.Options` passed through from
    the CLI layer.
  * Execute the bundle, interactively, via the OCI Run operation.

**Note** - this area of code is under heavy development for experimental
introduction in CE 3.11. It is likely that it will be heavily refactored, and
split, in future.

## Basic OCI Operations

The following files implement basic OCI operations on a runtime bundle:

### `oci_linux.go`

Defines constants, path resolution, and minimal bundle locking functions.

### `oci_runc_linux.go`

Holds implementations of the Run / Start / Exec / Kill / Delete / Pause / Resume
/ State OCI runtime operations.

See
<https://github.com/opencontainers/runtime-spec/blob/main/runtime.md#operations>

These functions are thin wrappers around the `runc`/`crun` operations of the
same name.

### `oci_conmon_linux.go`

Hold an implementation of the Create OCI runtime operation. This calls out to
`conmon`, which in turn calls `crun` or `runc`.

`conmon` is used to manage logging and console streams for containers that are
started backgrounded, so we don't have to do that ourselves.

### `oci_attach_linux.go`

Implements an `Attach` function, which can attach the user's console to the
streams of a container running in the background, which is being monitored by
conmon.

### Testing

End-to-end flows of basic OCI operations on an existing bundle are tested in the
OCI group of the e2e suite, `e2e/oci`.

## Launcher Flow

The `Launcher` type connects the standard singularity CLI actions
(run/shell/exec), to execution of an OCI container in a native bundle. Invoked
with the `--oci` flag, this is in contrast to running a Singularity format
container, with Singularity's own runtime engine.

### `spec_linux.go`

Provides a minimal OCI runtime spec, that will form the basis of container
execution that is roughly comparable to running a native singularity container
with `--compat` (`--containall`).

### `mounts_linux.go`

Provides code handling the addition of required mounts to the OCI runtime spec.

### `process_linux.go`

Provides code handling configuration of container process execution, including
user mapping.

### `launcher_linux.go`

Implements `Launcher.Exec`, which is called from the CLI layer. It will:

* Create a temporary bundle directory.
* Use `pkg/ocibundle/native` to retrieve the specified image, and extract it in
  the temporary bundle.
* Configure the container by creating an appropriate runtime spec.
* Call the interactive OCI Run function to execute the container with `crun` or
  `runc`.

### Namespace Considerations

An OCI container started via `Launch.Exec` as a non-root user always uses at
least one user namespace.

The user namespace is created *prior to* calling `runc` or `crun`, so we'll call
it an *outer* user namespace.

Creation of this outer user namespace is via using the `RunNS` function, instead
of `Run`. The `RunNS` function executes the Singularity `starter` binary, with a
minimal configuration of the fakeroot engine (
`internal/pkg/runtime/engine/fakeroot/config`).

The `starter` will create a user namespace and ID mapping, and will then execute
`singularity oci run` to perform the basic OCI Run operation against the bundle
that the `Launcher.Exec` function has prepared.

The outer user namespace from which `runc` or `crun` is called *always* maps the
host user id to root inside the userns.

When a container is run in `--fakeroot` mode, the outer user namespace is the
only user namespace. The OCI runtime config does not request any additional
userns or ID mapping be performed by `crun` / `runc`.

When a container is **not** run in `--fakeroot` mode, the OCI runtime config for
the bundle requests that `crun` / `runc`:

* Create another, inner, user namespace for the container.
* Apply an ID mapping which reverses the 'fakeroot' outer ID mapping.

I.E. when a container runs without `--fakeroot`, the ID mapping is:

* User ID on host (1001)
* Root in outer user namespace (0)
* User ID in container (1001)

### Testing

End-to-end testing of ßthe launcher flow is via the `e2e/actions` suite. Tests
prefixed `oci`.
