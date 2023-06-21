// Copyright (c) 2023, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package security

import (
	"testing"

	"github.com/sylabs/singularity/e2e/internal/e2e"
)

const (
	// Default OCI capabilities as visible in /proc/status
	ociDefaultCapString = "00000020a80425fb"
	// No capabilities, as visible in /proc/status
	nullCapString = "0000000000000000"
)

func (c ctx) ociCapabilities(t *testing.T) {
	e2e.EnsureOCIArchive(t, c.env)
	imageRef := "oci-archive:" + c.env.OCIArchivePath

	tests := []struct {
		name       string
		profile    e2e.Profile
		expectInh  string
		expectPrm  string
		expectEff  string
		expectBnd  string
		expectAmb  string
		expectExit int
	}{
		{
			name:      "DefaultUser",
			profile:   e2e.OCIUserProfile,
			expectInh: nullCapString,
			expectPrm: nullCapString,
			expectEff: nullCapString,
			expectBnd: ociDefaultCapString,
			expectAmb: nullCapString,
		},
		{
			name:      "DefaultRoot",
			profile:   e2e.OCIRootProfile,
			expectInh: nullCapString,
			expectPrm: ociDefaultCapString,
			expectEff: ociDefaultCapString,
			expectBnd: ociDefaultCapString,
			expectAmb: nullCapString,
		},
		{
			name:      "DefaultFakeroot",
			profile:   e2e.OCIRootProfile,
			expectInh: nullCapString,
			expectPrm: ociDefaultCapString,
			expectEff: ociDefaultCapString,
			expectBnd: ociDefaultCapString,
			expectAmb: nullCapString,
		},
	}

	e2e.EnsureImage(t, c.env)

	for _, tt := range tests {
		args := []string{imageRef, "grep", "^Cap...:", "/proc/self/status"}
		c.env.RunSingularity(
			t,
			e2e.AsSubtest(tt.name),
			e2e.WithProfile(tt.profile),
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
