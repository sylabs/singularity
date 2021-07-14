// Copyright (c) 2018-2020, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package gpu

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/sylabs/singularity/pkg/sylog"
)

// NvidiaPaths returns a list of Nvidia libraries/binaries that should be
// mounted into the container in order to use Nvidia GPUs
func NvidiaPaths(configFilePath, userEnvPath string) ([]string, []string, error) {
	if userEnvPath != "" {
		oldPath := os.Getenv("PATH")
		os.Setenv("PATH", userEnvPath)
		defer os.Setenv("PATH", oldPath)
	}

	nvidiaFiles, err := gpuliblist(configFilePath)
	if err != nil {
		return nil, nil, fmt.Errorf("could not read %s: %v", filepath.Base(configFilePath), err)
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
	_, err := os.Stat(persistencedSocket)
	if os.IsNotExist(err) {
		sylog.Verbosef("persistenced socket %s not found", persistencedSocket)
	} else {
		nvidiaFiles = append(nvidiaFiles, persistencedSocket)
	}

	return nvidiaFiles
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
