# Log Plugin

This logging plugin will write audit log entries to syslog. Multiple different
outputs can be configured in a YAML format configuration file, each with its own
template for the log entries.

For developers, a `Makefile` is provided to compile the plugin, and install it /
uninstall it along with the default config and template files.

You *must* have a SingularityCE 4.x.x installed, with the source tree that
matches the installed binaries still in place.

```sh
# Build the plugin
$ make
Using Singularity source directory: /home/dave/Git_sylabs/singularity
 COMPILE singularity plugin compile

# Install the plugin
$ sudo make install
Using Singularity source directory: /home/dave/Git_sylabs/singularity
 COMPILE singularity plugin compile
 INSTALL singularity plugin install
ENABLED  NAME
    yes  sylabs.io/log-plugin


# Uninstall the plugin
$ sudo make uninstall
Using Singularity source directory: /home/dave/Git_sylabs/singularity
 UNINSTALL singularity plugin uninstall
singularity plugin uninstall sylabs.io/log-plugin
Uninstalled plugin "sylabs.io/log-plugin".

# Clean up build files
$ make clean
 CLEAN
```

Plugin configuration is placed in the `plugins/sylabs.io/log-plugin` subdir
of Singularity's etc directory, e.g.:

```sh
/usr/local/etc/singularity/plugins/sylabs.io/log-plugin/
```

`config.yml` is the default/example configuration file, and will be
copied to the plugin configuration location by `make install`.

`default.tpl` is the default/example output template, and will be
copied to the plugin configuration location by `make install`.

The example `config.yml` and `default.tpl` will generate log messages
in the format:

<!-- markdownlint-disable line-length -->
```sh
Aug 05 19:02:18 z2 singularity[2505827]: uid=1000 gid=1000 user="dtrudg-sylabs" group="dtrudg-sylabs" exe="/usr/bin/singularity" version="4.3.9-1-resolute" cwd="/home/dtrudg-sylabs/test" command="run" args=["alpine_latest.sif"] flags=["userns=\"true\""] image="/home/dtrudg-sylabs/test/alpine_latest.sif" sif-uuid="46bff61b-7129-4707-9d54-c36c7fb09c04" LOGNAME="dtrudg-sylabs"
```
<!-- markdownlint-enable line-length -->

The following variables are available to use in the template:

```go
// UID is the numeric user id of the account that ran singularity
UID int
// GID is the numeric group id of the account that ran singularity
GID int
// User is the text user name of the account that ran singularity
User string
// Group is the text group name of the account that ran singularity
Group string
// Executable is the full path to the singularity executable that was run
Executable string
// Version is the version number of the singularity executable that was run
Version string
// CWD is the current working directory
CWD string
// Command is the top level command passed to singularity, e.g. run or build
Command string
// Args is a list of arguments passed to the command
Args []string
// Flags is a list of flags passed to the command
Flags []string
// Image is the path / URL of the container image, if applicable
Image string
// SIFUUID is the unique identifier (UUID) of the SIF image, if applicable
SIFUUID string
// Env gives access to log arbitrary environment variables.
// Map of NAME -> VALUE
Env map[string]string
```
