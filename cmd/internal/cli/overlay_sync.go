// Copyright (c) 2024, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.
package cli

import (
	"github.com/spf13/cobra"
	"github.com/sylabs/singularity/v4/docs"
	"github.com/sylabs/singularity/v4/internal/pkg/ocisif"
	"github.com/sylabs/singularity/v4/pkg/sylog"
)

// OverlaySyncCmd is the 'overlay sync' command that updates the digest of
// an overlay in an OCI-SIF image, as recorded in the OCI manifest / config.
var OverlaySyncCmd = &cobra.Command{
	Args: cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		if err := ocisif.SyncOverlay(args[0]); err != nil {
			sylog.Fatalf("%v", err.Error())
		}
		return nil
	},
	DisableFlagsInUseLine: true,

	Use:     docs.OverlaySyncUse,
	Short:   docs.OverlaySyncShort,
	Long:    docs.OverlaySyncLong,
	Example: docs.OverlaySyncExample,
}
