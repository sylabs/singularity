// Copyright (c) 2024, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.
package cli

import (
	"os"

	"github.com/spf13/cobra"
	"github.com/sylabs/singularity/v4/docs"
	"github.com/sylabs/singularity/v4/internal/pkg/ocisif"
	"github.com/sylabs/singularity/v4/pkg/sylog"
)

// OverlaySyncCmd is the 'overlay sync' command that updates the digest of
// an overlay in an OCI-SIF image, as recorded in the OCI manifest / config.
var OverlaySealCmd = &cobra.Command{
	Args: cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		tmpEnv := os.Getenv("SINGULARITY_TMPDIR")
		if err := ocisif.SealOverlay(args[0], tmpEnv); err != nil {
			sylog.Fatalf("%v", err.Error())
		}
		return nil
	},
	DisableFlagsInUseLine: true,

	Use:     docs.OverlaySealUse,
	Short:   docs.OverlaySealShort,
	Long:    docs.OverlaySealLong,
	Example: docs.OverlaySealExample,
}
