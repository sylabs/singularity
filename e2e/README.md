# End-to-End Testing

This package contains the end-to-end (e2e) tests for `singularity`.

## Contributing

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

```go
type ctx struct {
    env e2e.TestEnv
}
```

- Add tests, which are member functions of the `ctx`. The following example
  tests that when an environment variable is set, it can be echoed from a
  `singularity exec`.

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

- Putting this altogether, the `e2e/env/env.go` will look like:

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

## Running

To run all end to end tests, use the `e2e-tests` make target:

```sh
make -C builddir e2e-test
```

To run all tests in a specific group, or groups, specify the group names (comma
seperated) in an `E2E_GROUPS=` argument to the make target:

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
