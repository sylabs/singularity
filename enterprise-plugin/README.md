# Singularity Enterprise Mangement Plugin

Target: SingularityCE 4.x & Singularity Enterprise 2.x

## Building & Testing

For developers, a `Makefile` is provided to compile the plugin, and install it /
uninstall it. The Makefile will also build the standalone CLI version.

You *must* have a SingularityCE 4.x installed, with the source tree that
matches the installed binaries still in place.

The plugin will add a `singularity enterprise` subcommand.

The plugin will *always* respect the currently configured Singularity
remote, and singularity's own log level etc. No additional
configuration is read from files or environment variables.

```sh
# Build the plugin
$ make
Using Singularity source directory: /home/dave/Git_sylabs/singularity-pro3
 COMPILE singularity plugin compile

# Install the plugin
$ sudo make install
Using Singularity source directory: /home/dave/Git_sylabs/singularity-pro3
 COMPILE singularity plugin compile
 INSTALL singularity plugin install
ENABLED  NAME
    yes  sylabs.io/enterprise-plugin

# Use the plugin
$ singularity enterprise
Singularity enterprise management commands (plugin)

Usage:
  singularity enterprise

Description:

Options:
  -h, --help   help for enterprise

Available Commands:
  status      Check the status of the Singularity Enterprise services, and
              your authentication token


For additional help or support, please visit https://www.sylabs.io/docs/

# Uninstall the plugin
$ sudo make uninstall
Using Singularity source directory: /home/dave/Git_sylabs/singularity
 UNINSTALL singularity plugin uninstall
singularity plugin uninstall sylabs.io/enterprise-plugin
Uninstalled plugin "sylabs.io/enterprise-plugin".

# Clean up build files
$ make clean
 CLEAN
```

### Mock Services

The plugin must talk to a set of mock Singularity Enterprise services during
integration test execution. The mock services are run using using
[outofcoffee/imposter](https://github.com/outofcoffee/imposter/).

Imposter is setup using a number of `xxxxx-config.yml` files in the `api/`
directory. These configure imposter to present mock services using the OpenAPI
spec files, as well as a mock remote endpoint configuration.

Start imposter from the root directory of this project with:

```sh
docker run \
     -ti \
     -p 8080:8080 \
     -v $(pwd)/api/:/opt/imposter/config \
     outofcoffee/imposter:2.6.4
```

For interactive use, the CLI or plugin can then be pointed to the mock services
by specifying the remote URI `http://localhost:8080`, e.g.:

```sh
$ singularity enterprise --output json --uri http://localhost:8080 get build-agents default
INFO:    get command called
[
  {
    "addedAt": "2018-10-28T16:45:03.6037837Z",
    "address": "8a62f2d3b523",
    "id": "0a6bccb3-094f-4c65-a8cd-2903e4672968",
    "queue": "0a6bccb3-094f-4c65-a8cd-2903e4672968",
    "taskID": "5af05d5e9293620001b41d2c",
    "taskedAt": "2018-10-28T16:51:02.8492307Z"
  }
]
```

### Tests

Tests are run for the CLI along with the plugin via `make test` and `make
integration-test`. You can update golden files with `make test-update`. When
running `make test-update` ensure that the mock services are running.

If running tests from the `go test` CLI, or inside an editor / IDE, ensure you
always specify the `sylog` build tag, and the `enterprise_integration` build tag
when working with integration tests.

If you cannot run the mock services locally on port 8080 you may specify an
alternative URI with the `-URI` flag when running `go test`.

**NOTE** - The tests assume UTC timezone in order to match golden files.
`TZ=UTC` is set from the Makefile when running `make test` and `make
test-update`. If you are running tests directly, you may need to `export TZ=UTC`
to avoid failures.

### OpenAPI Specs / Client Codegen

The basic functional code for the service API clients is generated from OpenAPI
sepcification files provided in the repository of each service this project
interacts with.

A script `swagger-codegen.sh` is provided which will:

- Temporarily shallow clone each service repository.
- Copy the service API definitions from these repositories into `api/`,
  replacing prior versions.
- Run `swagger generate client` to re-generate client code in `pkg/`.

This script should be run when the API of a service is materially changed.
