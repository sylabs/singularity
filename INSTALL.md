# Installing SingularityCE

Since you are reading this from the SingularityCE source code, it will be
assumed that you are building/compiling from source.

For full instructions on installation, including building RPMs, please check the
[installation section of the admin guide](https://sylabs.io/guides/latest/admin-guide/).

## Install system dependencies

You must first install development tools and libraries to your host.

### Debian / Ubuntu

```sh
# Ensure repositories are up-to-date
sudo apt-get update
# Install debian packages for dependencies
sudo apt-get install -y \
    autoconf \
    automake \
    cryptsetup \
    fuse2fs \
    git \
    fuse \
    libfuse-dev \
    libglib2.0-dev \
    libseccomp-dev \
    libtool \
    pkg-config \
    runc \
    squashfs-tools \
    squashfs-tools-ng \
    uidmap \
    wget \
    zlib1g-dev
```

### RHEL / Alma Linux / Rocky Linux 8+ and Fedora

```sh
# Install basic tools for compiling
sudo yum groupinstall -y 'Development Tools'
# Install RPM packages for dependencies
sudo yum install -y \
    autoconf \
    automake \
    crun \
    cryptsetup \
    fuse \
    fuse3 \
    fuse3-devel \
    git \
    glib2-devel \
    libseccomp-devel \
    libtool \
    squashfs-tools \
    wget \
    zlib-devel
```

### SLES / openSUSE Leap

```sh
sudo zypper in \
    autoconf \
    automake \
    cryptsetup \
    fuse2fs \
    fuse3 \
    fuse3-devel \
    gcc \
    gcc-c++ \
    git \
    glib2-devel \
    libseccomp-devel \
    libtool \
    make \
    pkg-config \
    runc \
    squashfs \
    wget \
    zlib-devel
```

## Install sqfstar / tar2sqfs for OCI-mode

If you intend to use the `--oci` execution mode of SingularityCE, your system
must provide either:

* `squashfs-tools / squashfs` >= 4.5, which provides the `sqfstar` utility.
  Older versions packaged by many distributions do not include `sqfstar`.
* `squashfs-tools-ng`, which provides the `tar2sqfs` utility. This is not
  packaged by all distributions.

### Debian / Ubuntu

On Debian/Ubuntu `squashfs-tools-ng` is available in the distribution
repositories. It has been included in the "Install system dependencies" step
above. No further action is necessary.

### Fedora

On Fedora, the `squashfs-tools` package includes `sqfstar`. No further action is
necessary.

### RHEL / Alma Linux / Rocky Linux

On RHEL and derivatives, the `squashfs-tools-ng` package is now
available in the EPEL repositories.

If you previously used the `dctrud/squashfs-tools-ng` COPR, you should
disable it:

```sh
sudo dnf copr remove dctrud/squashfs-tools-ng
```

Follow the [EPEL Quickstart](https://docs.fedoraproject.org/en-US/epel/#_quickstart)
for you distribution to enable the EPEL repository. Install `squashfs-tools-ng` with
`dnf` or `yum`.

```sh
sudo dnf install squashfs-tools-ng
```

### SLES / openSUSE Leap

On SLES/openSUSE, follow the instructions at the [filesystems
project](https://software.opensuse.org//download.html?project=filesystems&package=squashfs)
to obtain an more recent `squashfs` package that provides `sqfstar`.

## Install Go

Singularity is written in Go, and may require a newer version of Go than is
available in the repositories of your distribution. We recommend installing the
latest version of Go from the [official binaries](https://golang.org/dl/).

First, download the Go tar.gz archive to `/tmp`, then extract the archive to
`/usr/local`.

_**NOTE:** if you are updating Go from a older version, make sure you remove
`/usr/local/go` before reinstalling it._

```sh
export VERSION=1.22.4 OS=linux ARCH=amd64  # change this as you need

wget -O /tmp/go${VERSION}.${OS}-${ARCH}.tar.gz \
  https://dl.google.com/go/go${VERSION}.${OS}-${ARCH}.tar.gz
sudo tar -C /usr/local -xzf /tmp/go${VERSION}.${OS}-${ARCH}.tar.gz
```

Finally, add `/usr/local/go/bin` to the `PATH` environment variable:

```sh
echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc
source ~/.bashrc
```

## Install golangci-lint

If you will be making changes to the source code, and submitting PRs, you should
install `golangci-lint`, which is the linting tool used in the SingularityCE
project to ensure code consistency.

Every pull request must pass the `golangci-lint` checks, and these will be run
automatically before attempting to merge the code. If you are modifying
Singularity and contributing your changes to the repository, it's faster to run
these checks locally before uploading your pull request.

In order to download and install the latest version of `golangci-lint`, you can
run:

<!-- markdownlint-disable MD013 -->

```sh
curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(go env GOPATH)/bin
```

<!-- markdownlint-enable MD013 -->

Add `$(go env GOPATH)` to the `PATH` environment variable:

```sh
echo 'export PATH=$PATH:$(go env GOPATH)/bin' >> ~/.bashrc
source ~/.bashrc
```

## Clone the repo

With the adoption of Go modules you no longer need to clone the SingularityCE
repository to a specific location.

Clone the repository with `git` in a location of your choice:

```sh
git clone --recurse-submodules https://github.com/sylabs/singularity.git
cd singularity
```

By default your clone will be on the `main` branch which is where development
of SingularityCE happens. To build a specific version of SingularityCE, check
out a [release tag](https://github.com/sylabs/singularity/tags) before
compiling. E.g. to build the 4.2.0 release, checkout the
`v4.2.0` tag:

```sh
git checkout --recurse-submodules v4.2.0
```

## Compiling SingularityCE

You can configure, build, and install SingularityCE using the following
commands:

```sh
./mconfig
make -C builddir
sudo make -C builddir install
```

And that's it! Now you can check your SingularityCE version by running:

```sh
singularity --version
```

The `mconfig` command accepts options that can modify the build and installation
of SingularityCE. For example, to build in a different folder and to set the
install prefix to a different path:

```sh
./mconfig -b ./buildtree -p /usr/local
```

See the output of `./mconfig -h` for available options.

## Apparmor Profile (Ubuntu 24.04+)

Beginning with the 24.04 LTS release, Ubuntu does not permit applications to
create unprivileged user namespaces by default.

If you install SingularityCE from a GitHub release `.deb` package then an
apparmor profile will be installed that permits SingularityCE to create
unprivileged user namespaces.

If you install SingularityCE from source you must configure apparmor.
Create an apparmor profile file at `/etc/apparmor.d/singularity-ce`:

```sh
sudo tee /etc/apparmor.d/singularity-ce << 'EOF'
# Permit unprivileged user namespace creation for SingularityCE starter
abi <abi/4.0>,
include <tunables/global>

profile singularity-ce /usr/local/libexec/singularity/bin/starter{,-suid} flags=(unconfined) {
  userns,

  # Site-specific additions and overrides. See local/README for details.
  include if exists <local/singularity-ce>
}
EOF
```

Modify the path beginning `/usr/local` if you specified a non-default `--prefix`
when configuring and installing SingularityCE.

Reload the system apparmor profiles after you have created the file:

```sh
sudo systemctl reload apparmor
```

SingularityCE will now be able to create unprivileged user namespaces on your
system.

## Building & Installing from an RPM

On a RHEL / AlmaLinux / Rocky Linux / Fedora machine you can build a
SingularityCE into an RPM package, and install it from the RPM. This is useful
if you need to install Singularity across multiple machines, or wish to manage
all software via `yum/dnf`.

To build the RPM, you first need to install the
[system dependencies](#install-system-dependencies) and
[Go toolchain](#install-go) as shown above. The RPM spec does not declare Go as
a build dependency, as SingularityCE may require a newer version of Go than is
available in distribution / EPEL repositories. Go should be installed manually,
so that the go executable is on `$PATH` in the build environment.

Download the latest
[release tarball](https://github.com/sylabs/singularity/releases) and use it to
build and install the RPM like this:

<!-- markdownlint-disable MD013 -->

```sh
export VERSION=4.2.0 # this is the singularity version, change as you need

# Fetch the source
wget https://github.com/sylabs/singularity/releases/download/v${VERSION}/singularity-ce-${VERSION}.tar.gz
# Build the rpm from the source tar.gz
rpmbuild -tb singularity-ce-${VERSION}.tar.gz
# Install SingularityCE using the resulting rpm
sudo rpm -ivh ~/rpmbuild/RPMS/x86_64/singularity-ce-${VERSION}-1.el7.x86_64.rpm
# (Optionally) Remove the build tree and source to save space
rm -rf ~/rpmbuild singularity-ce-${VERSION}*.tar.gz
```

<!-- markdownlint-enable MD013 -->

Alternatively, to build an RPM from the latest main you can
[clone the repo as detailed above](#clone-the-repo). Then use the `rpm` make
target to build SingularityCE as an rpm package:

<!-- markdownlint-disable MD013 -->

```sh
./mconfig
make -C builddir rpm
sudo rpm -ivh ~/rpmbuild/RPMS/x86_64/singularity-ce-${VERSION}*.x86_64.rpm
```

<!-- markdownlint-enable MD013 -->

By default, the rpm will be built so that SingularityCE is installed under
`/usr/local`.

To build an rpm with an alternative install prefix set RPMPREFIX on the make
step, for example:

```sh
make -C builddir rpm RPMPREFIX=/opt/singularity-ce
```

For more information on installing/updating/uninstalling the RPM, check out our
[admin docs](https://www.sylabs.io/guides/latest/admin-guide/admin_quickstart.html).
