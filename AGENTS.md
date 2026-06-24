# AGENTS.md

This file provides instructions for AI agents working in the SingularityCE
repository.

## Overview

SingularityCE is a container runtime, focused on HPC and scientific computing
use-cases.

## Quick-Start

Install prerequisites listed in [INSTALL.md](INSTALL.md)

```sh
./mconfig -v                        # Configure
make -C builddir                    # Build
./builddir/singularity --version    # Smoke test
sudo make -C builddir install       # Install
make -C builddir check              # Lint
```

By default `make install` installs into system-wide paths.

Use `./mconfig -p <dir>` to configure for install to an alternative location.

## Conventions

- Size/type: large multi-directory systems project with Go, C, shell, and
  packaging metadata.
- Primary language/runtime: Go using 1.25+ syntax.
- Build system: `mconfig` generates `builddir/Makefile`, then
  `make -C builddir ...`.
- CI system: CircleCI only (`.circleci/config.yml`).
- Lint: `golangci-lint` or `make -C builddir check`
- Formatting: `gofumpt`
- Platform assumptions: Linux, often with sudo/root privileges for tests.
- C code: the `cmd/starter/` runtime starter is C. Do not modify C code
  without understanding the security implications — the starter runs as
  a setuid binary.

## Unit / Integration Tests

Prefer table-driven tests. Use `stretchr/testify` for assertions.

Run unit & integration tests using `scripts/go-test` wrapper script and standard
`go test` arguments / flags.

```sh
# After configure and build steps in Quick Start

# Run all unit tests
scripts/go-test -v ./...

# Run single `TestPrefix` test from pkg/sylog
scripts/go-test -v ./pkg/sylog -run TestPrefix
```

If test calls `test.EnsurePrivilege(t)` then use `scripts/go-test -v -sudo`.

## End-to-end Tests

End-to-end tests are written in the `e2e/` directory.

When writing end-to-end tests follow instructions in
[e2e/README.md](e2e/README.md), which documents test profiles (`UserProfile`,
`RootProfile`, `FakerootProfile`, `OCIUserProfile`, etc.), helpers, and
table-driven test patterns.

**DO NOT** use `t.TempDir()` in code inside `e2e/`. Use `e2e.MakeTempDir()`
instead.

Always use `scripts/e2e-test` wrapper to run end-to-end tests.

Test names are prefixed with `TestE2E/SEQ` or `TestE2E/PAR`.

```sh
# After configure, build, and install steps in Quick Start

# Sequential e2e tests using testhelper.NoParallel
scripts/e2e-test -v -run TestE2E/SEQ/<GROUP>/<NAME>

# Example
scripts/e2e-test -v -run TestE2E/SEQ/ACTIONS/umask

# Parallel e2e tests
scripts/e2e-test -v -run TestE2E/PAR/<GROUP>/<NAME>

# Example
scripts/e2e-test -v -run TestE2E/PAR/ACTIONS/shell
```

## Code Structure

```example
singularity
├── .circleci                              # CircleCI CI/CD configuration
├── builddir                               # Generated build files
├── cmd                                    # Top-level CLI code
│   ├── bash_completion                    # Bash completions generator
│   ├── docs                               # man / markdown CLI docs generator
│   ├── internal
│   │   └── cli                            # singularity CLI Cobra commands
│   ├── singularity                        # Main singularity CLI entry-point
│   ├── singularity-buildkitd              # Customised buildkitd executable for Dockerfile builds
│   └── starter                            # Runtime starter entry-point
├── debian                                 # Deb packaging for Ubuntu
├── dist
│   └── rpm                                # RPM packaging for RHEL / AlmaLinux / Rocky Linux
├── docs                                   # CLI docstrings
├── e2e                                    # End-to-end test suite
├── etc                                    # Template configuration files
├── examples                               # Example container definition files
├── internal
│   ├── app
│   │   ├── singularity                    # singularity CLI implementation
│   │   └── starter                        # Runtime starter implementation
│   └── pkg                                # Private runtime, image, utility, etc. packages.
├── makeit                                 # makeit Makefile generator tool
├── mlocal                                 # Checks and templates for ./mconfig build configuration
├── pkg                                    # Public types and utility packages.
├── scripts                                # Build, test, development wrapper scripts
├── test                                   # Test fixtures
└── third_party                            # Third-party software
```

## Security

Consider to avoid false positives:

- SingularityCE does not aim to secure / fully sandbox containers run as the
  host root user.
- Design choices including broad access to `/dev`, default `$HOME` / CWD binds,
  unrestricted user binds, no default seccomp profile by default are
  intentional. Singularity originated to allow direct replacement of
  non-containerised application with containers, on standard multi-user HPC
  systems, with default configuration.
- Denial of service by a user against their own job/session is not in-scope.
- uid/gid, fsuid, and capabilities vary between the different processes spawned
  by the starter. The effective uid/gid, fsuid, and capabilities must be
  identified to confirm a vulnerability.
- Environment is wiped before privileged code runs by the C starter's
  `cleanenv()`.
- External binaries that are run in a privileged context in setuid mode are not
  found on `$PATH`. See special handling in `bin.FindBin()` etc.
- SingularityCE container builds are not intended to be fully sandboxed. E.g.
  the `%pre` / `%setup` blocks in a definition file intentionally run commands
  directly on the host.

Required security analysis context:

- Installation mode: setuid vs non-setuid
- Any non-default configuration (`singularity.conf` etc.).
- Execution mode: native, OCI, user namespace, fakeroot, sudo/root.
- Exact process involved: starter, RPC server, master, container process, build
  helper, external tool.
- uid/euid/suid, gid/egid/sgid, fsuid/fsgid, and effective/permitted
  capabilities at the vulnerable operation.
- Whether the operation occurs before or after starter `cleanenv()` and
  privilege drop.
- Whether the attacker is already host root, controls the definition file,
  controls the image, or controls the container command.
- A minimal reproducer showing unauthorized host impact from an unprivileged
  user.

## Resources

- [Contributor's Guide](CONTRIBUTING.md)
- [User Documentation](https://github.com/sylabs/singularity-userdocs)
- [Admin Documentation](https://github.com/sylabs/singularity-admindocs)
