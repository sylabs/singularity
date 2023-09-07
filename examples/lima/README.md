# SingularityCE Lima Virtual Machine

This directory contains an example [lima VM](https://lima-vm.io/)
template, which can be used to easily install and work with
SingularityCE inside a Linux virtual machine on a macOS system.

The template:

* Is based on AlmaLinux 9.
* Supports both Intel and Silicon (ARM64) Macs.
* Installs the latest stable release of SingularityCE that has been
  published to the Fedora EPEL repositories.

To create a SingularityCE VM using the template:

* [Install homebrew on your Mac](https://brew.sh)
* Install lima with `brew install lima`
* Run `limactl start ./singularity-ce.yml`

Configuration of the VM will take a couple of minutes.

You can then enter the VM interactively, and run `singularity`
commmands inside it:

```shell
limactl shell singularity-ce
singularity run library://alpine
```

Your home directory will be shared from your Mac, into the VM. However, since
macOS places home directories under `/Users` (rather than `/home`),
SingularityCE will not mount your home directory in the container unless you
explicitly specify your macOS homedir, as shown here:

```shell
   limactl shell singularity-ce
   singularity run -H /Users/myuser library://alpine
```

Alternatively, to run SingularityCE directly:

```shell
limactl shell singularity-ce singularity run library://alpine
```

Or, with homedir mounting:

```shell
limactl shell singularity-ce singularity run -H /Users/myuser library://alpine
```

To stop the VM:

```shell
limactl stop singularity-ce
```

To delete the VM:

```shell
limactl delete singularity-ce
```
