# Release Procedures

The release procedures below can be performed by a project member with
"maintainer" or higher privileges on the GitHub repository. They assume that you
will be working in an up-to-date local clone of the GitHub repository, where the
`upstream` remote points to `github.com/sylabs/singularity`.

There are two different release procedures:

* A new patch release, e.g. releasing 3.11.3 when the current release is 3.11.2,
  follows a short procedure that can be completed within a day.
* A new major or minor version, e.g. releasing 3.12.0 when the current release
  is 3.11.1, requires a longer procedure that takes a minimum of 2 weeks. This
  includes a release candidate period, creation of a new release branch, new
  documentation branches etc.

## Patch Releases

A patch release is made from an existing `release-XXX` branch, to provide fixes
for the current stable major.minor version of SingularityCE.

### Prior to Patch Release

1. Set a target date for the patch release.
1. Use a GitHub milestone to track issues and PRs that will form part of the
   patch release.
1. Cherry pick fixes from the `main` branch onto the `release-XXX` branch.
1. Ensure that the `CHANGELOG.md` is kept up-to-date on the `release-XXX`
   branch, with all relevant changes listed under a "Changes Since Last Release"
   section.
1. Monitor and merge dependabot updates, such that a release is made with as
   up-to-date versions of dependencies as possible. This lessens the burden in
   addressing patch release fixes that require dependency updates, as we use
   several dependencies that move quickly.
1. Identify and manually test any changes that are not exercised by the CI
   pipeline. E.g. cgroups v1 code.

### Creating the Patch Release

1. Check the version of Go installed in the CI environment from
   `.circleci/config.yml`. Ensure that it is the latest release, as it will be
   used to build our release packages. Update if necessary via a PR against the
   release branch.
1. Modify the `README.md`, `INSTALL.md`, `CHANGELOG.md` via PR against the
   release branch, so that they reflect the version to be released. Also modify
   Go version mentioned in `INSTALL.md`, if that has changed since last release.
1. After all PRs are merged, apply an annotated tag to the HEAD of the release
   branch via `git tag -a -m "SingularityCE v3.11.3" v3.11.3`.
1. Push the tag to the GitHub repository via `git push upstream v3.11.3`.
1. Download the rpm and deb packages, which are generated as CI job artifacts,
   from the CircleCI workflow for the tag.
1. Create a tarball locally, via
   `git clean -fdx && ./mconfig -v && make -C builddir dist`.
1. Test installation locally from the tarball.
1. Compute the sha256sums of the tarball and packages, e.g. by collecting them
   in a single directory and running `sha256sum * > sha256sums`.
1. Create a GitHub release, using the previous release as a guide and
   incorporating `CHANGELOG.md` information. Attach the tarball, rpms, debs, and
   `sha256sums` to it.
1. Notify the community about the release via the Google Group and Slack.

### After the Patch Release

1. Create and merge a PR from the `release-XXX` branch into `main`, so that the
   changelog is synchronized etc.

## Major / Minor Releases

A Major release (e.g. 3.11 -> 4.0), or minor release (e.g. 3.11 -> 3.12)
requires a minimum of two weeks to complete. This allows time for feedback on at
least one release candidate to be received from the user community.

### Prior to Release

1. Set a target date for the release candidate (if required) and release.
   Generally 2 weeks from RC -> release is appropriate for new 3.X minor
   versions. A major release may need a longer RC period.
1. Aim to specifically discuss the release timeline and progress in community
   meetings at least 2 months prior to the scheduled date.
1. Use a GitHub milestone to track issues and PRs that will form part of the
   next release.
1. Ensure that the `CHANGELOG.md` is kept up-to-date on the `main` branch,
   with all relevant changes listed under a "Changes Since Last Release"
   section.
1. Monitor and merge dependabot updates, such that a release is made with as
   up-to-date versions of dependencies as possible. This lessens the burden in
   addressing patch release fixes that require dependency updates, as we use
   several dependencies that move quickly.

### Creating the Release Branch and Release Candidate

When a new major or minor version of SingularityCE is issued the release process
begins by branching, and then issuing, a release candidate for broader testing.

1. From a repository that is up-to-date with the `main` branch, create a release
   branch e.g. `git checkout upstream/main -b release-3.12`.
1. Push the release branch to the GitHub repository via `git push upstream
   release-3.12`.
1. Examine the GitHub branch protection rules, to extend them to the new release
   branch if needed.
1. Update the `.github/dependabot.yml` configuration so that dependabot is
   tracking the new stable release branch. Do not remove the previous stable
   release branch from the configuration yet, as it should be monitored until
   the final release of a new version.
1. Check the version of Go installed in the CI environment from
   `.circleci/config.yml`. Ensure that it is the latest release, as it will be
   used to build our release packages. Update if necessary via a PR against the
   release branch.
1. Modify the `README.md`, `INSTALL.md`, `CHANGELOG.md` via PR against the
   release branch, so that they reflect the release candidate version to be
   issued. Also modify Go version mentioned in `INSTALL.md`, if that has changed
   since last release.
1. Apply an annotated tag via
   `git tag -a -m "SingularityCE v3.11.0 Release Candidate 1" v3.11.0-rc.1`.
1. Push the tag via `git push upstream v3.11.0-rc.1`.
1. Download the rpm and deb packages, which are generated as CI job artifacts,
   from the CircleCI workflow for the tag.
1. Create a tarball locally, via
   `git clean -fdx && ./mconfig -v && make -C builddir dist`.
1. Test intallation locally from the tarball.
1. Compute the sha256sums of the tarball and packages, e.g. by collecting them
   in a single directory and running `sha256sum * > sha256sums`.
1. Create a GitHub pre-release, using the previous release as a guide and
   incorporating `CHANGELOG.md` information. Make sure it is clear to users that
   the release candidate is not for production use. Attach the tarball, rpms,
   debs, and `sha256sums` to it.
1. Notify the community about the release candidate via the Google Group and Slack.

There will often be multiple release candidates issued prior to the final
release of a new major or minor version. Each new RC follows step 5 and above.

### Creating a Final Major / Minor Release

1. Ensure the user and admin documentation is up-to-date for the new version,
   has been branched, and tagged.
   * [User Docs](https://www.sylabs.io/guides/latest/user-guide/) can be edited
     [here](https://github.com/sylabs/singularity-userdocs)
   * [Admin Docs](https://www.sylabs.io/guides/latest/admin-guide/) can be edited
     [here](https://github.com/sylabs/singularity-admindocs)
1. Ensure the user and admin documentation has been deployed to the sylabs.io
   website.
1. Modify the `README.md`, `INSTALL.md`, `CHANGELOG.md` via PR against the
   release branch, so that they reflect the version to be released. Also modify
   Go version mentioned in `INSTALL.md`, if that has changed since last release.
1. Apply an annotated tag via `git tag -a -m "SingularityCE v3.12.0" v3.12.0`.
1. Push the tag via `git push upstream v3.12.0`.
1. Download the rpm and deb packages, which are generated as CI job artifacts,
   from the CircleCI workflow for the tag.
1. Create a tarball locally, via
   `git clean -fdx && ./mconfig -v && make -C builddir dist`.
1. Test intallation locally from the tarball.
1. Compute the sha256sums of the tarball and packages, e.g. by collecting them
   in a single directory and running `sha256sum * > sha256sums`.
1. Create a GitHub release, using the previous release as a guide and
   incorporating `CHANGELOG.md` information. Attach the tarball, rpms, debs, and
   `sha256sums` to it.
1. Notify the community about the release candidate via the Google Group and Slack.

### After the Release

1. Create and merge a PR from the `release-XXX` branch into `main`, so that
   history from the RC process etc. is captured on `main`.
1. Move the prior `release-XXX` branch to `vault/release-XXX`.
1. Update the `.github/dependabot.yml` configuration to remove the prior stable
   release branch.
1. Start scheduling / setting up milestones etc. to track the next release!
