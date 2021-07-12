// Copyright (c) 2018-2020, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package gpu

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"

	"github.com/sylabs/singularity/internal/pkg/util/fs"
	"github.com/sylabs/singularity/pkg/util/capabilities"
	"github.com/sylabs/singularity/pkg/util/slice"
)

// NVDriverCapabilities is the set of driver capabilities supported by nvidia-container-cli.
// See: https://github.com/nvidia/nvidia-container-runtime#nvidia_driver_capabilities
var NVDriverCapabilities = []string{
	"compute",
	"compat32",
	"graphics",
	"utility",
	"video",
	"display",
}

// NVDriverDefaultCapabilities is the default set of nvidia-container-cli driver capabilities.
// It is used if NVIDIA_DRIVER_CAPABILITIES is not set.
// See: https://github.com/nvidia/nvidia-container-runtime#nvidia_driver_capabilities
var NVDriverDefaultCapabilities = []string{
	"compute",
	"utility",
}

// NVCLIAmbientCaps is the ambient capability set required by nvidia-container-cli.
var NVCLIAmbientCaps = []uintptr{
	uintptr(capabilities.Map["CAP_KILL"].Value),
	uintptr(capabilities.Map["CAP_SETUID"].Value),
	uintptr(capabilities.Map["CAP_SETGID"].Value),
	uintptr(capabilities.Map["CAP_SYS_CHROOT"].Value),
	uintptr(capabilities.Map["CAP_CHOWN"].Value),
	uintptr(capabilities.Map["CAP_FOWNER"].Value),
	uintptr(capabilities.Map["CAP_MKNOD"].Value),
	uintptr(capabilities.Map["CAP_SYS_ADMIN"].Value),
	uintptr(capabilities.Map["CAP_DAC_READ_SEARCH"].Value),
	uintptr(capabilities.Map["CAP_SYS_PTRACE"].Value),
	uintptr(capabilities.Map["CAP_DAC_OVERRIDE"].Value),
	uintptr(capabilities.Map["CAP_SETPCAP"].Value),
}

// HasNVCLI returns true if `nvidia-container-cli` is available.
func HasNVCLI() bool {
	_, err := exec.LookPath("nvidia-container-cli")
	return err == nil
}

// NVCLIConfigure calls out to the nvidia-container-cli configure operation.
// This sets up the GPU with the container. Note that the ability to set a fairly broad set of
// ambient capabilities is required. This function will error if the bounding set does not include
// NvidiaContainerCLIAmbientCaps.
func NVCLIConfigure(pathEnv string, flags []string, rootfs string, runAsRoot bool) error {
	oldPath := os.Getenv("PATH")
	os.Setenv("PATH", pathEnv)
	defer os.Setenv("PATH", oldPath)

	nccBin, err := exec.LookPath("nvidia-container-cli")
	if err != nil {
		return err
	}

	// The nvidia-container-cli binary must be owned by root, as it is called with broad
	// capabilities, and as root in the setuid flow.
	if !fs.IsOwner(nccBin, 0) {
		return fmt.Errorf("nvidia-container-cli is not owned by root user")
	}

	nccArgs := []string{"configure"}
	nccArgs = append(nccArgs, flags...)
	nccArgs = append(nccArgs, rootfs)

	cmd := exec.Command(nccBin, nccArgs...)
	cmd.Env = os.Environ()

	// We need to run nvidia-container-cli as root when we are in the setuid flow
	// without a usernamepace in play.
	if runAsRoot {
		cmd.SysProcAttr = &syscall.SysProcAttr{
			Credential: &syscall.Credential{Uid: 0, Gid: 0},
		}
	} else {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.AmbientCaps = NVCLIAmbientCaps
	stdoutStderr, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("nvidia-container-cli failed with %v: %s", err, stdoutStderr)
	}
	return nil
}

// NVCLIEnvToFlags reads the environment variables supported by nvidia-container-runtime
// and converts them to flags for nvidia-container-cli.
// See: https://github.com/nvidia/nvidia-container-runtime#environment-variables-oci-spec
func NVCLIEnvToFlags() (flags []string, err error) {
	// We don't support cgroups related usage yet.
	flags = []string{"--no-cgroups"}

	// We use the host ldconfig by prefixing '@'
	// On Debian/Ubuntu `ldconfig` is a script... we need ldconfig.real
	ldConfig, err := exec.LookPath("ldconfig.real")
	if err != nil {
		ldConfig, err = exec.LookPath("ldconfig")
		if err != nil {
			return nil, fmt.Errorf("could not lookup ldconfig: %v", err)
		}
	}
	flags = append(flags, "--ldconfig=@"+ldConfig)

	if val := os.Getenv("NVIDIA_VISIBLE_DEVICES"); val != "" {
		flags = append(flags, "--device="+val)
	}

	if val := os.Getenv("NVIDIA_MIG_CONFIG_DEVICES"); val != "" {
		flags = append(flags, "--mig-config="+val)
	}

	if val := os.Getenv("NVIDIA_MIG_MONITOR_DEVICES"); val != "" {
		flags = append(flags, "--mig-monitor="+val)
	}

	// Driver capabilities have a default, but can be overridden.
	caps := NVDriverDefaultCapabilities
	if val := os.Getenv("NVIDIA_DRIVER_CAPABILITIES"); val != "" {
		caps = strings.Split(val, ",")
	}

	for _, cap := range caps {
		if slice.ContainsString(NVDriverCapabilities, cap) {
			flags = append(flags, "--"+cap)
		} else {
			return nil, fmt.Errorf("unknown NVIDIA_DRIVER_CAPABILITIES value: %s", cap)
		}
	}

	// One --require flag for each NVIDIA_REQUIRE_* environment
	// https://github.com/nvidia/nvidia-container-runtime#nvidia_require_
	if val := os.Getenv("NVIDIA_DISABLE_REQUIRE"); val == "" {
		for _, e := range os.Environ() {
			if strings.HasPrefix(e, "NVIDIA_REQUIRE_") {
				req := strings.SplitN(e, "=", 2)[1]
				flags = append(flags, "--require="+req)
			}
		}
	}

	return flags, nil
}
