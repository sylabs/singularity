# SingularityCE

[![CircleCI](https://circleci.com/gh/sylabs/singularity/tree/master.svg?style=svg)](https://circleci.com/gh/sylabs/singularity/tree/master)

- [Documentation](https://www.sylabs.io/docs/)
- [Support](#support)
- [Community Meetings / Minutes / Roadmap](https://github.com/sylabs/singularityce-community)
- [Project License](LICENSE.md)
- [Guidelines for Contributing](CONTRIBUTING.md)
- [Code of Conduct](CODE_OF_CONDUCT.md)
- [Citation](#citing-singularity)

## What is SingularityCE?

SingularityCE is the Community Edition of Singularity, an open source container
platform designed to be simple, fast, and secure. Many container platforms are
available, but SingularityCE is designed for ease-of-use on shared systems and in
high performance computing (HPC) environments. It features:

- An immutable single-file container image format, supporting cryptographic
  signatures and encryption.
- Integration over isolation by default. Easily make use of GPUs, high speed
  networks, parallel filesystems on a cluster or server.
- Mobility of compute. The single file SIF container format is easy to transport
  and share.
- A simple, effective security model. You are the same user inside a container
  as outside, and cannot gain additional privilege on the host system by
  default.

SingularityCE is open source software, distributed under the [BSD License](LICENSE.md).

Check out [talks about Singularity](https://www.sylabs.io/videos) and some
[use cases of Singularity](https://sylabs.io/case-studies) on our website.

## Getting Started with SingularityCE

To install SingularityCE from source, see the
[installation instructions](INSTALL.md). For other installation options, see
[our guide](https://www.sylabs.io/guides/latest/admin-guide/).

System administrators can learn how to configure SingularityCE, and get an
overview of its architecture and security features in the
[administrator guide](https://www.sylabs.io/guides/latest/admin-guide/).

For users, see the [user guide](https://www.sylabs.io/guides/latest/user-guide/)
for details on how to run and build containers with SingularityCE.

## Contributing to SingularityCE

Community contributions are always greatly appreciated. To start developing
SingularityCE, check out the [guidelines for contributing](CONTRIBUTING.md).

Please note we have a [code of conduct](CODE_OF_CONDUCT.md). Please follow it in
all your interactions with the project members and users.

Our roadmap, other documents, and user/developer meeting information can be
found in the
[singularityce-community repository](https://github.com/sylabs/singularityce-community).

We also welcome contributions to our
[user guide](https://github.com/sylabs/singularity-userdocs) and
[admin guide](https://github.com/sylabs/singularity-admindocs).

## Support

To get help with SingularityCE, check out the community spaces detailed at our
[Community Portal](https://www.sylabs.io/singularity/community/).

See also our [Support Guidelines](SUPPORT.md) for further information about the
best place, and how, to raise different kinds of issues and questions.

For additional support, [contact us](https://www.sylabs.io/contact/) to receive
more information.

## Go Version Compatibility

SingularityCE aims to maintain support for the two most recent stable versions
of Go. This corresponds to the Go
[Release Maintenance Policy](https://github.com/golang/go/wiki/Go-Release-Cycle#release-maintenance)
and [Security Policy](https://golang.org/security), ensuring critical bug
fixes and security patches are available for all supported language versions.

## Citing Singularity

The SingularityCE software may be cited using our Zenodo DOI `10.5281/zenodo.5564905`:

> SingularityCE Developers (2021) SingularityCE. 10.5281/zenodo.5564905
> <https://doi.org/10.5281/zenodo.5564905>

This is an 'all versions' DOI for referencing SingularityCE in a manner that is
not version-specific. You may wish to reference the particular version of
SingularityCE used in your work. Zenodo creates a unique DOI for each release,
and these can be found in the 'Versions' sidebar on the [Zenodo record page](https://doi.org/10.5281/zenodo.5564905).

Please also consider citing the original publication describing Singularity:

> Kurtzer GM, Sochat V, Bauer MW (2017) Singularity: Scientific containers for
> mobility of compute. PLoS ONE 12(5): e0177459.
> <https://doi.org/10.1371/journal.pone.0177459>

## License

_Unless otherwise noted, this project is licensed under a 3-clause BSD license
found in the [license file](LICENSE.md)._
