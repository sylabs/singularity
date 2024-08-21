// Copyright (c) 2024, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package cli

import (
	"github.com/spf13/cobra"
	"github.com/sylabs/singularity/v4/docs"
	"github.com/sylabs/singularity/v4/internal/app/singularity"
	"github.com/sylabs/singularity/v4/pkg/sylog"
)

// DataPackageCmd is the 'data package' command to package a file/dir into an OCI-SIF data container.
var DataPackageCmd = &cobra.Command{
	Args: cobra.ExactArgs(2),
	RunE: func(_ *cobra.Command, args []string) error {
		if err := singularity.DataPackage(args[0], args[1]); err != nil {
			sylog.Fatalf("%v", err.Error())
		}
		return nil
	},
	DisableFlagsInUseLine: true,

	Use:     docs.DataPackageUse,
	Short:   docs.DataPackageShort,
	Long:    docs.DataPackageLong,
	Example: docs.DataPackageExample,
}
