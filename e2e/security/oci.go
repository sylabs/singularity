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

var (
	// Default OCI capabilities as visible in /proc/status
	ociDefaultCaps = uint64(0x00000020a80425fb)
	// Default OCI capabilities with CAP_CHOWN dropped
	ociDropChown = uint64(0x00000020a80425fa)
	// Default OCI capabilities with CAP_SYS_ADMIN added
	ociAddSysAdmin = uint64(0x00000020a82425fb)
	capSysAdmin    = uint64(0x0000000000200000)
	// No capabilities, as visible in /proc/status
	nullCaps = uint64(0x0000000000000000)
)

func (c ctx) ociCapabilities(t *testing.T) {
	e2e.EnsureOCISIF(t, c.env)
	imageRef := "oci-sif:" + c.env.OCISIFPath

	var fullCaps uint64
	var err error
	e2e.Privileged(func(t *testing.T) {
		fullCaps, err = capabilities.GetProcessEffective()
		if err != nil {
			t.Fatalf("Could not get CapEff: %v", err)
		}
	})(t)

	tests := []struct {
		name       string
		options    []string
		profiles   []e2e.Profile
		expectInh  uint64
		expectPrm  uint64
		expectEff  uint64
		expectBnd  uint64
		expectAmb  uint64
		expectExit int
	}{
		{
			name:      "DefaultUser",
			profiles:  []e2e.Profile{e2e.OCIUserProfile},
			expectInh: nullCaps,
			expectPrm: nullCaps,
			expectEff: nullCaps,
			expectBnd: ociDefaultCaps,
			expectAmb: nullCaps,
		},
		{
			name:      "DefaultRoot",
			profiles:  []e2e.Profile{e2e.OCIRootProfile, e2e.OCIFakerootProfile},
			expectInh: nullCaps,
			expectPrm: ociDefaultCaps,
			expectEff: ociDefaultCaps,
			expectBnd: ociDefaultCaps,
			expectAmb: nullCaps,
		},
		{
			name:      "NoPrivs",
			options:   []string{"--no-privs"},
			profiles:  []e2e.Profile{e2e.OCIRootProfile, e2e.OCIFakerootProfile, e2e.OCIUserProfile},
			expectInh: nullCaps,
			expectPrm: nullCaps,
			expectEff: nullCaps,
			expectBnd: nullCaps,
			expectAmb: nullCaps,
		},
		{
			name:      "KeepPrivsUser",
			options:   []string{"--keep-privs"},
			profiles:  []e2e.Profile{e2e.OCIUserProfile},
			expectInh: nullCaps,
			expectPrm: nullCaps,
			expectEff: nullCaps,
			expectBnd: fullCaps,
			expectAmb: nullCaps,
		},
		{
			name:      "KeepPrivsRoot",
			options:   []string{"--keep-privs"},
			profiles:  []e2e.Profile{e2e.OCIRootProfile, e2e.OCIFakerootProfile},
			expectInh: nullCaps,
			expectPrm: fullCaps,
			expectEff: fullCaps,
			expectBnd: fullCaps,
			expectAmb: nullCaps,
		},
		{
			name:      "DropChownUser",
			options:   []string{"--drop-caps", "CAP_CHOWN"},
			profiles:  []e2e.Profile{e2e.OCIUserProfile},
			expectInh: nullCaps,
			expectPrm: nullCaps,
			expectEff: nullCaps,
			expectBnd: ociDropChown,
			expectAmb: nullCaps,
		},
		{
			name:      "DropChownRoot",
			options:   []string{"--drop-caps", "CAP_CHOWN"},
			profiles:  []e2e.Profile{e2e.OCIRootProfile, e2e.OCIFakerootProfile},
			expectInh: nullCaps,
			expectPrm: ociDropChown,
			expectEff: ociDropChown,
			expectBnd: ociDropChown,
			expectAmb: nullCaps,
		},
		{
			name:      "AddSysAdminUser",
			options:   []string{"--add-caps", "CAP_SYS_ADMIN"},
			profiles:  []e2e.Profile{e2e.OCIUserProfile},
			expectInh: capSysAdmin,
			expectPrm: capSysAdmin,
			expectEff: capSysAdmin,
			expectBnd: ociAddSysAdmin,
			expectAmb: capSysAdmin,
		},
		{
			name:      "AddSysAdminRoot",
			options:   []string{"--add-caps", "CAP_SYS_ADMIN"},
			profiles:  []e2e.Profile{e2e.OCIRootProfile, e2e.OCIFakerootProfile},
			expectInh: nullCaps,
			expectPrm: ociAddSysAdmin,
			expectEff: ociAddSysAdmin,
			expectBnd: ociAddSysAdmin,
			expectAmb: nullCaps,
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
					e2e.ExpectOutput(e2e.ContainMatch,
						fmt.Sprintf("CapInh:\t%0.16x", tt.expectInh&fullCaps)),
					e2e.ExpectOutput(e2e.ContainMatch,
						fmt.Sprintf("CapPrm:\t%0.16x", tt.expectPrm&fullCaps)),
					e2e.ExpectOutput(e2e.ContainMatch,
						fmt.Sprintf("CapEff:\t%0.16x", tt.expectEff&fullCaps)),
					e2e.ExpectOutput(e2e.ContainMatch,
						fmt.Sprintf("CapBnd:\t%0.16x", tt.expectBnd&fullCaps)),
					e2e.ExpectOutput(e2e.ContainMatch,
						fmt.Sprintf("CapAmb:\t%0.16x", tt.expectAmb&fullCaps)),
				),
			)
		}
	}
}
