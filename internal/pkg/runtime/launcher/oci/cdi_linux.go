// Copyright (c) 2018-2024, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

// Includes code from https://github.com/containers/podman
// Released under the Apache License Version 2.0

package oci

import (
	"fmt"

	"github.com/opencontainers/runtime-spec/specs-go"
	"tags.cncf.io/container-device-interface/pkg/cdi"
	"tags.cncf.io/container-device-interface/pkg/parser"
)

// addCDIDevices adds an array of CDI devices to an existing spec. Accepts optional, variable
// number of cdi.Option arguments (to which cdi.WithAutoRefresh(false) will be prepended).
func addCDIDevices(spec *specs.Spec, cdiDevices []string, cdiRegOptions ...cdi.Option) error {
	// Configure the CDI cache, passing a cdi.WithAutoRefresh(false) option so that CDI cache files
	// are not scanned asynchronously. (We are about to call a manual refresh, below.)
	cdiRegOptions = append([]cdi.Option{cdi.WithAutoRefresh(false)}, cdiRegOptions...)
	if err := cdi.Configure(cdiRegOptions...); err != nil {
		return fmt.Errorf("error configuring CDI cache: %w", err)
	}

	if err := cdi.Refresh(); err != nil {
		return fmt.Errorf("error refreshing CDI cache: %w", err)
	}

	for _, cdiDevice := range cdiDevices {
		if !isCDIDevice(cdiDevice) {
			return fmt.Errorf("string %#v does not represent a valid CDI device", cdiDevice)
		}
	}

	if _, err := cdi.InjectDevices(spec, cdiDevices...); err != nil {
		return fmt.Errorf("Error encountered setting up CDI devices: %w", err)
	}

	return nil
}

// isCDIDevice checks whether a string is a valid CDI device selector.
func isCDIDevice(str string) bool {
	return parser.IsQualifiedName(str)
}
