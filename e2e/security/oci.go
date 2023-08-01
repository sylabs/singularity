// Copyright (c) 2023, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package security

import (
	"fmt"
	"testing"

	"github.com/sylabs/singularity/v4/e2e/internal/e2e"
	"github.com/sylabs/singularity/v4/pkg/util/capabilities"
)

const (
	// Default OCI capabilities as visible in /proc/status
	ociDefaultCapString = "00000020a80425fb"
	// DefaultOCI capabilities with CAP_CHOWN dropped
	ociDropChownString = "00000020a80425fa"
	// DefaultOCI capabilities with CAP_SYS_ADMIN added
	ociAddSysAdminString = "00000020a82425fb"
	capSysAdminString    = "0000000000200000"
	// No capabilities, as visible in /proc/status
	nullCapString = "0000000000000000"
)

func (c ctx) ociCapabilities(t *testing.T) {
	e2e.EnsureOCISIF(t, c.env)
	imageRef := "oci-sif:" + c.env.OCISIFPath

	var rootCaps uint64
	var err error
	e2e.Privileged(func(t *testing.T) {
		rootCaps, err = capabilities.GetProcessEffective()
		if err != nil {
			t.Fatalf("Could not get CapEff: %v", err)
		}
	})(t)
	fullCapString := fmt.Sprintf("%0.16x", rootCaps)

	tests := []struct {
		name       string
		options    []string
		profiles   []e2e.Profile
		expectInh  string
		expectPrm  string
		expectEff  string
		expectBnd  string
		expectAmb  string
		expectExit int
	}{
		{
			name:      "DefaultUser",
			profiles:  []e2e.Profile{e2e.OCIUserProfile},
			expectInh: nullCapString,
			expectPrm: nullCapString,
			expectEff: nullCapString,
			expectBnd: ociDefaultCapString,
			expectAmb: nullCapString,
		},
		{
			name:      "DefaultRoot",
			profiles:  []e2e.Profile{e2e.OCIRootProfile, e2e.OCIFakerootProfile},
			expectInh: nullCapString,
			expectPrm: ociDefaultCapString,
			expectEff: ociDefaultCapString,
			expectBnd: ociDefaultCapString,
			expectAmb: nullCapString,
		},
		{
			name:      "NoPrivs",
			options:   []string{"--no-privs"},
			profiles:  []e2e.Profile{e2e.OCIRootProfile, e2e.OCIFakerootProfile, e2e.OCIUserProfile},
			expectInh: nullCapString,
			expectPrm: nullCapString,
			expectEff: nullCapString,
			expectBnd: nullCapString,
			expectAmb: nullCapString,
		},
		{
			name:      "KeepPrivsUser",
			options:   []string{"--keep-privs"},
			profiles:  []e2e.Profile{e2e.OCIUserProfile},
			expectInh: nullCapString,
			expectPrm: nullCapString,
			expectEff: nullCapString,
			expectBnd: fullCapString,
			expectAmb: nullCapString,
		},
		{
			name:      "KeepPrivsRoot",
			options:   []string{"--keep-privs"},
			profiles:  []e2e.Profile{e2e.OCIRootProfile, e2e.OCIFakerootProfile},
			expectInh: nullCapString,
			expectPrm: fullCapString,
			expectEff: fullCapString,
			expectBnd: fullCapString,
			expectAmb: nullCapString,
		},
		{
			name:      "DropChownUser",
			options:   []string{"--drop-caps", "CAP_CHOWN"},
			profiles:  []e2e.Profile{e2e.OCIUserProfile},
			expectInh: nullCapString,
			expectPrm: nullCapString,
			expectEff: nullCapString,
			expectBnd: ociDropChownString,
			expectAmb: nullCapString,
		},
		{
			name:      "DropChownRoot",
			options:   []string{"--drop-caps", "CAP_CHOWN"},
			profiles:  []e2e.Profile{e2e.OCIRootProfile, e2e.OCIFakerootProfile},
			expectInh: nullCapString,
			expectPrm: ociDropChownString,
			expectEff: ociDropChownString,
			expectBnd: ociDropChownString,
			expectAmb: nullCapString,
		},
		{
			name:      "AddSysAdminUser",
			options:   []string{"--add-caps", "CAP_SYS_ADMIN"},
			profiles:  []e2e.Profile{e2e.OCIUserProfile},
			expectInh: capSysAdminString,
			expectPrm: capSysAdminString,
			expectEff: capSysAdminString,
			expectBnd: ociAddSysAdminString,
			expectAmb: capSysAdminString,
		},
		{
			name:      "AddSysAdminRoot",
			options:   []string{"--add-caps", "CAP_SYS_ADMIN"},
			profiles:  []e2e.Profile{e2e.OCIRootProfile, e2e.OCIFakerootProfile},
			expectInh: nullCapString,
			expectPrm: ociAddSysAdminString,
			expectEff: ociAddSysAdminString,
			expectBnd: ociAddSysAdminString,
			expectAmb: nullCapString,
		},
	}

	e2e.EnsureImage(t, c.env)

	for _, tt := range tests {
		for _, p := range tt.profiles {
			args := append(tt.options, imageRef, "grep", "^Cap...:", "/proc/self/status")
			c.env.RunSingularity(
				t,
				e2e.AsSubtest(tt.name+"/"+p.String()),
				e2e.WithProfile(p),
				e2e.WithCommand("exec"),
				e2e.WithArgs(args...),
				e2e.ExpectExit(tt.expectExit,
					e2e.ExpectOutput(e2e.ContainMatch, "CapInh:\t"+tt.expectInh),
					e2e.ExpectOutput(e2e.ContainMatch, "CapPrm:\t"+tt.expectPrm),
					e2e.ExpectOutput(e2e.ContainMatch, "CapEff:\t"+tt.expectEff),
					e2e.ExpectOutput(e2e.ContainMatch, "CapBnd:\t"+tt.expectBnd),
					e2e.ExpectOutput(e2e.ContainMatch, "CapAmb:\t"+tt.expectAmb),
				),
			)
		}
	}
}
