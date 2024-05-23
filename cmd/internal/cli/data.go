// Copyright (c) 2024, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package cli

import (
	"errors"

	"github.com/spf13/cobra"
	"github.com/sylabs/singularity/v4/docs"
	"github.com/sylabs/singularity/v4/pkg/cmdline"
)

func init() {
	addCmdInit(func(cmdManager *cmdline.CommandManager) {
		cmdManager.RegisterCmd(DataCmd)
		cmdManager.RegisterSubCmd(DataCmd, DataPackageCmd)
	})
}

// DataCmd is the 'data' command that provides management of data containers.
var DataCmd = &cobra.Command{
	RunE: func(_ *cobra.Command, _ []string) error {
		return errors.New("Invalid command")
	},
	DisableFlagsInUseLine: true,

	Use:     docs.DataUse,
	Short:   docs.DataShort,
	Long:    docs.DataLong,
	Example: docs.DataExample,
}
