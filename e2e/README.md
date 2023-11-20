# End-to-End Testing

This package contains the end-to-end (e2e) tests for `singularity`.

## Contributing

### Introduction

The e2e tests are split into *groups*, which gather tests that exercise a
specific area of functionality.

For this example, we're going to use a a group called `ENV` which would hold
tests relevant to environment variable handling.

- Add your group into the `e2eGroups` or `e2eGroupsNoPIDNS` struct in
  `suite.go`. This is a map from the name of the group (in upper case), to the
  `E2ETests` function declared in the group's package.

  You should generally use `e2eGroups`. The `e2eGroupsNoPIDNS`
  struct is only for groups that cannot be run within a PID namespace -
  specifically tests that involve systemd cgroups handling.

```go
var e2eGroups = map[string]testhelper.Group{
    ...
    "ENV":        env.E2ETests,
```

- Now create a directory for your group.

```sh
mkdir -p e2e/env
```

- Create a source file that will hold your groups's tests.

```sh
touch e2e/env/env.go
```

- Optionally create a source file to include helpers for your group's test.

```sh
touch e2e/env/env_utils.go
```

- Add a package declaration to the test file (e2e/env/env.go) that matches what
  you put in `suite.go`

```go
package env
```

- Declare a ctx struct that holds the `e2e.TestEnv` structure, and can be
  extended with any other group-specific variables.
  - For more information on `e2e.TestEnv`, see the relevant
    [subsection](#initialization-and-the-e2etestenv-struct) below.

```go
type ctx struct {
    env e2e.TestEnv
}
```

- Add tests, which are member functions (=methods) of the `ctx`. The following
  example tests that when an environment variable is set, it can be echoed from
  a `singularity exec`.

```go
func (c ctx) echoEnv(t *testing.T) {
    e2e.EnsureImage(t, c.env)

    env := []string{`FOO=BAR`}
    c.env.RunSingularity(
        t,
        e2e.WithProfile(e2e.UserProfile),
        e2e.WithCommand("exec"),
        e2e.WithEnv(env),
        e2e.WithArgs("/bin/sh", "-c" "echo $FOO"),
        e2e.ExpectExit(
            0,
            e2e.ExpectOutput(e2e.ExactMatch, "BAR"),
        ),
    )
}

```

Note that we use a helper function `e2e.EnsureImage` to make sure the test image
that we will run has been created. We use the `c.env.RunSingularity` function to
actually execute `singularity`, and specify arguments, expected output etc.

- Now add a public `E2ETests` function to the package, which returns a
  `testhelper.Tests` struct, holding the test we want to run:

```go
func E2ETests(env e2e.TestEnv) testhelper.Tests {
    c := ctx{
        env: env,
    }

    return testhelper.Tests{
        "environment echo": c.echoEnv,
    }
}
```

- Putting this all together, the `e2e/env/env.go` will look like:

```go
package env 

import (
    "testing"

    "github.com/sylabs/singularity/e2e/internal/e2e"
    "github.com/sylabs/singularity/e2e/internal/testhelper"
)

type ctx struct {
    env e2e.TestEnv
}

func (c ctx) echoEnv(t *testing.T) {
    e2e.EnsureImage(t, c.env)

    env := []string{`FOO=BAR`}
    c.env.RunSingularity(
        t,
        e2e.WithProfile(e2e.UserProfile),
        e2e.WithCommand("exec"),
        e2e.WithEnv(env),
        e2e.WithArgs("/bin/sh", "-c" "echo $FOO"),
        e2e.ExpectExit(
            0,
            e2e.ExpectOutput(e2e.ExactMatch, "BAR"),
        ),
    )
}

func E2ETests(env e2e.TestEnv) testhelper.Tests {
    c := ctx{
        env: env,
    }

    return testhelper.Tests{
        "environment echo": c.echoEnv,
    }
}
```

### Initialization and the e2e.TestEnv struct

The `e2e.TestEnv` struct is created and initialized in the e2e.Run() function,
defined in the e2e/suite.go file. This function initializes many fields of the
struct and carries out initialization procedures that are necessary for the
entire e2e suite to work. Here are a few examples:

- Creating a temporary test directory (intended to serve as the parent dir for
  any more specific temporary subdirs that may be needed in the course of
  specific tests), and setting the `TestDir` field of the struct to point to
  that directory.
- Setting up "fake" home directories for the current user and for root, so that
  actions that affect the home dir (caches, tokens, config changes, etc.) will
  not affect the files in the users' real home dirs.
  - This is achieved by creating temporary homedirs for the user and for root,
    and bind-mouting them over the real ones (`$HOME` and `/root`,
    respectively).
  - Because the e2e suite is run inside a dedicated mount namespace, this
    bind-mount does not affect the "outside world."
  - The actual function that is called to set up these fake homedirs is
    SetupHomeDirectories(), defined in e2e/internal/e2e/home.go
- Blank/default versions of the following are set up & placed in the
  aforementioned temporary `TestDir`:
  - `singularity.conf`
  - `remote.yaml`
  - plugin dir
  - ECL configuration
  - Global keyring
- If the E2E_DOCKER_USERNAME and E2E_DOCKER_PASSWORD environment variables are
  set, they will be used to generate `docker-config.json` files, which will be
  placed inside the `.singularity` subdir of the "fake" user homedir and the
  fake `/root` (see above).

#### The local Docker/OCI registry

Next, a local Docker/OCI registry is spun up for testing purposes. The host &
address for this local testing registry is stored in `testenv.TestRegistry`
(note that the string stored here does *not* contain the `docker://` transport
prefix).

A few images are immediately pushed to this testing registry; others are only
generated on demand, using the `testenv.EnsureXYZ()` functions, discussed below.

The images immediately pushed to the testing registry (in what follows,
`<registryURI>` should be understood as shorthand for
`"docker://"+testenv.TestRegistry`):

- `<registryURI>/my-alpine:latest`: Created by copying `docker://alpine:latest`
  at runtime.
- `<registryURI>/aufs-sanity:latest`: An image with many small layers, useful
  for testing overlay and `--keep-layers` behaviors. Created by copying
  `docker://sylabsio/aufs-sanity:latest` at runtime.
- `<registryURI>/private/e2eprivrepo/my-alpine:latest`: Another copy of
  `docker://alpine:latest` created at runtime, but pushed into a private
  location in the testing repo that requires authentication.
  - To push the private image, e2e.Run() makes use of the
    PrivateRepoLogin()/PrivateRepoLogout() functions defined in
    e2e/internal/e2e/private_repo.go. See the comments on those functions for
    more information.

The following URIs are then stored in `testenv` fields for convenience:

- `testenv.TestRegistryImage` = `<registryURI>/my-alpine:latest`
- `testenv.TestRegistryLayeredImage` = `<registryURI>/aufs-sanity:latest`
- `testenv.TestRegistryPrivURI` = `<registryURI>`
  - At present, this is simply identical to `"docker://"+testenv.TestRegistry`.
    But the test suite is written so that this *could* point at a different
    registry.
- `testenv.TestRegistryPrivPath` = `testenv.TestRegistryPrivURI+"/private/e2eprivrepo"`
- `testenv.TestRegistryPrivImage` = `testenv.TestRegistryPrivPath+"docker://%s/my-alpine:latest"`

#### The EnsureXYZ() functions

Aside from the images copied as part of setting up [the local Docker/OCI
registry](#the-local-dockeroci-registry), the e2e suite makes a series of other
images available on-demand: these images are copied or built only when a
particular EnsureXYZ() function is called.

Below is a description of each of the EnsureXYZ() functions, and the image they
create. These functions are defined in e2e/internal/e2e/image.go, and they use
mutexes to ensure they are concurrency-safe.

- EnsureImage():
  - Builds the "main" test image, whose definition is located in
    e2e/testdata/Singularity.
  - The image is saved to a file named "test.sif" inside `testenv.TestDir`.
  - The path to this image file is saved in `testenv.ImagePath`.
- EnsureOCIArchive():
  - Copies `testenv.TestRegistryImage` from the local testing registry to an OCI
    archive (a local .tar file with the contents of the OCI image, to be used
    via URIs with the `oci-archive:` transport prefix).
  - The image is saved to a file named "oci.tar" inside `testenv.TestDir`.
  - The path to this image file is saved in `testenv.OCIArchivePath`.

## Running

To run all end to end tests, use the `e2e-tests` make target:

```sh
make -C builddir e2e-test
```

To run all tests in a specific group, or groups, specify the group names (comma
separated) in an `E2E_GROUPS=` argument to the make target:

```sh
# Only run tests in the VERSION and HELP groups
make -C builddir e2e-test E2E_GROUPS=VERSION,HELP
```

To run specific top-level tests (as defined in the `testhelper.Tests` struct
returned by eache group's `E2ETests` function) supply a regular expression in an
`E2E_TESTS` argument to the make target:

```sh
# Only run e2e tests with a name that begins with 'semantic'
make -C builddir e2e-test E2E_TESTS=^semantic
```

You can combine the `E2E_GROUPS` and `E2E_TESTS` arguments to limit the tests
that are run:

```sh
# Only run e2e tests in the VERSION group that have a name that begins with 'semantic'
make -C builddir e2e-test E2E_GROUP=VERSION E2E_TESTS=^semantic
```
