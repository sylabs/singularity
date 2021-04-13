// Copyright (c) 2018-2020, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package gpu

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/sylabs/singularity/pkg/sylog"
	"github.com/sylabs/singularity/pkg/util/capabilities"
)

// NvidiaContainerCLIAmbientCaps is the ambient capability set required by nvidia-container-cli
var NvidiaContainerCLIAmbientCaps = []uintptr{
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

// NvidiaPaths returns a list of Nvidia libraries/binaries that should be
// mounted into the container in order to use Nvidia GPUs
func NvidiaPaths(configFilePath, userEnvPath string) ([]string, []string, error) {
	if userEnvPath != "" {
		oldPath := os.Getenv("PATH")
		os.Setenv("PATH", userEnvPath)
		defer os.Setenv("PATH", oldPath)
	}

	// Parse nvidia-container-cli for the necessary binaries/libs, fallback to a
	// list of required binaries/libs if the nvidia-container-cli is unavailable
	nvidiaFiles, err := nvidiaContainerCli("list", "--binaries", "--libraries")
	if err != nil {
		sylog.Verbosef("nvidiaContainerCli returned: %v", err)
		sylog.Verbosef("Falling back to nvliblist.conf")

		nvidiaFiles, err = gpuliblist(configFilePath)
		if err != nil {
			return nil, nil, fmt.Errorf("could not read %s: %v", filepath.Base(configFilePath), err)
		}
	}

	return paths(nvidiaFiles)
}

// NvidiaIpcsPath returns list of nvidia ipcs driver.
func NvidiaIpcsPath(envPath string) []string {
	const persistencedSocket = "/var/run/nvidia-persistenced/socket"

	if envPath != "" {
		oldPath := os.Getenv("PATH")
		os.Setenv("PATH", envPath)
		defer os.Setenv("PATH", oldPath)
	}

	var nvidiaFiles []string
	nvidiaFiles, err := nvidiaContainerCli("list", "--ipcs")
	if err != nil {
		sylog.Verbosef("nvidiaContainerCli returned: %v", err)
		sylog.Verbosef("Falling back to default path %s", persistencedSocket)

		// nvidia-container-cli may not be installed, check
		// default path
		_, err := os.Stat(persistencedSocket)
		if os.IsNotExist(err) {
			sylog.Verbosef("persistenced socket %s not found", persistencedSocket)
		} else {
			nvidiaFiles = append(nvidiaFiles, persistencedSocket)
		}
	}

	return nvidiaFiles
}

// HasNvidiaContainerCli returns true if `nvidia-container-cli` is available.
func HasNvidiaContainerCli() bool {
	_, err := exec.LookPath("nvidia-container-cli")
	return err == nil
}

// nvidiaContainerCli runs `nvidia-container-cli list` and returns list of
// libraries, ipcs and binaries for proper NVIDIA work. This may return duplicates!
func nvidiaContainerCli(args ...string) ([]string, error) {
	nvidiaCLIPath, err := exec.LookPath("nvidia-container-cli")
	if err != nil {
		return nil, fmt.Errorf("could not find nvidia-container-cli: %v", err)
	}

	var out bytes.Buffer
	cmd := exec.Command(nvidiaCLIPath, args...)
	cmd.Stdout = &out
	err = cmd.Run()
	if err != nil {
		return nil, fmt.Errorf("could not execute nvidia-container-cli list: %v", err)
	}

	var libs []string
	scanner := bufio.NewScanner(&out)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if strings.Contains(line, ".so") {
			// Handle the library reported by nvidia-container-cli
			libs = append(libs, line)
			// Look for and add any symlinks for this library
			soPath := strings.SplitAfter(line, ".so")[0]
			soPaths, err := soLinks(soPath)
			if err != nil {
				sylog.Errorf("while finding links for %s: %v", soPath, err)
			}
			libs = append(libs, soPaths...)
		} else {
			// this is a binary -> need full path
			libs = append(libs, line)
		}
	}
	return libs, nil
}

// NvidiaDevices return list of all non-GPU nvidia devices present on host. If withGPU
// is true all GPUs are included in the resulting list as well.
func NvidiaDevices(withGPU bool) ([]string, error) {
	nvidiaGlob := "/dev/nvidia*"
	if !withGPU {
		nvidiaGlob = "/dev/nvidia[^0-9]*"
	}
	devs, err := filepath.Glob(nvidiaGlob)
	if err != nil {
		return nil, fmt.Errorf("could not list nvidia devices: %v", err)
	}
	return devs, nil
}

// NVidiaContainerCLIConfigure calls out to the nvidia-container-cli configure operation.
// This sets up the GPU with the container. Note that the ability to set a fairly broad set of
// ambient capabilities is required. This function will error if the bounding set does not include
// NvidiaContainerCLIAmbientCaps.
func NVidiaContainerCLIConfigure(pathEnv string, flags []string, rootfs string, runAsRoot bool) error {
	oldPath := os.Getenv("PATH")
	os.Setenv("PATH", pathEnv)
	defer os.Setenv("PATH", oldPath)

	nccBin, err := exec.LookPath("nvidia-container-cli")
	if err != nil {
		return err
	}

	nccArgs := []string{"--debug=/tmp/singularity-nvcli-debug", "--user", "configure"}
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
	cmd.SysProcAttr.AmbientCaps = NvidiaContainerCLIAmbientCaps
	stdoutStderr, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("nvidia-container-cli failed with %v: %s", err, stdoutStderr)
	}
	return nil
}
