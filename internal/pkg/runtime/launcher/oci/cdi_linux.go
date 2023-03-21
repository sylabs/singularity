// Copyright (c) 2018-2022, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

// Includes code from https://github.com/containers/podman
// Released under the Apache License Version 2.0

package oci

import (
	"fmt"
	"sync"

	"github.com/container-orchestrated-devices/container-device-interface/pkg/cdi"
	"github.com/opencontainers/runtime-spec/specs-go"
)

// A container to hold the CDI registry, plus a sync.Once object to ensure we only have to ask for it once
var regSyncContainer struct {
	reg      cdi.Registry
	initOnce sync.Once
	err      error
}

// addCDIDevices adds an array of CDI devices to an existing spec.
func addCDIDevices(spec *specs.Spec, cdiDevices []string) error {
	regSyncContainer.initOnce.Do(func() {
		// Get the CDI registry, passing a cdi.WithAutoRefresh(false) option so that CDI registry files are not scanned asynchronously. (We are about to call a manual refresh, below.)
		regSyncContainer.reg = cdi.GetRegistry(cdi.WithAutoRefresh(false))
		regSyncContainer.err = regSyncContainer.reg.Refresh()
	})

	if regSyncContainer.err != nil {
		return fmt.Errorf("Error encountered refreshing the CDI registry during initialization: %v", regSyncContainer.err)
	}

	for _, cdiDevice := range cdiDevices {
		if !isCDIDevice(cdiDevice) {
			return fmt.Errorf("string %#v does not represent a valid CDI device", cdiDevice)
		}
	}

	if _, err := regSyncContainer.reg.InjectDevices(spec, cdiDevices...); err != nil {
		return fmt.Errorf("Error encountered setting up CDI devices: %w", err)
	}

	return nil
}

// isCDIDevice checks whether a string is a valid CDI device selector.
func isCDIDevice(str string) bool {
	return cdi.IsQualifiedName(str)
}
