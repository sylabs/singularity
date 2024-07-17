// Copyright (c) 2018-2024, Sylabs Inc. All rights reserved.
// Copyright (c) Contributors to the Apptainer project, established as
//   Apptainer a Series of LF Projects LLC.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package cli

import (
	"github.com/spf13/cobra"
	"github.com/sylabs/singularity/v4/docs"
	"github.com/sylabs/singularity/v4/internal/app/singularity"
	"github.com/sylabs/singularity/v4/internal/pkg/runtime/launcher"
	"github.com/sylabs/singularity/v4/pkg/cmdline"
	"github.com/sylabs/singularity/v4/pkg/sylog"
)

func init() {
	addCmdInit(func(cmdManager *cmdline.CommandManager) {
		cmdManager.RegisterFlagForCmd(&instanceStartPidFileFlag, instanceStartCmd, instanceRunCmd)
	})
}

// --pid-file
var instanceStartPidFile string

var instanceStartPidFileFlag = cmdline.Flag{
	ID:           "instanceStartPidFileFlag",
	Value:        &instanceStartPidFile,
	DefaultValue: "",
	Name:         "pid-file",
	Usage:        "write instance PID to the file with the given name",
	EnvKeys:      []string{"PID_FILE"},
}

// singularity instance start
var instanceStartCmd = &cobra.Command{
	Args:                  cobra.MinimumNArgs(2),
	PreRun:                actionPreRun,
	DisableFlagsInUseLine: true,
	Run: func(cmd *cobra.Command, args []string) {
		if isOCI {
			sylog.Fatalf("Instances are not yet supported in OCI-mode. Omit --oci, or use --no-oci, to start a non-OCI Singularity container.")
		}

		ep := launcher.ExecParams{
			Image:    args[0],
			Action:   "start",
			Instance: args[1],
			Args:     args[2:],
		}
		if err := launchContainer(cmd, ep); err != nil {
			sylog.Fatalf("%s", err)
		}

		if instanceStartPidFile != "" {
			err := singularity.WriteInstancePidFile(ep.Instance, instanceStartPidFile)
			if err != nil {
				sylog.Warningf("Failed to write pid file: %v", err)
			}
		}
	},

	Use:     docs.InstanceStartUse,
	Short:   docs.InstanceStartShort,
	Long:    docs.InstanceStartLong,
	Example: docs.InstanceStartExample,
}

// singularity instance run
var instanceRunCmd = &cobra.Command{
	Args:                  cobra.MinimumNArgs(2),
	PreRun:                actionPreRun,
	DisableFlagsInUseLine: true,
	Run: func(cmd *cobra.Command, args []string) {
		if isOCI {
			sylog.Fatalf("Instances are not yet supported in OCI-mode. Omit --oci, or use --no-oci, to start a non-OCI Singularity container.")
		}

		ep := launcher.ExecParams{
			Image:    args[0],
			Action:   "run",
			Instance: args[1],
			Args:     args[2:],
		}
		if err := launchContainer(cmd, ep); err != nil {
			sylog.Fatalf("%s", err)
		}

		if instanceStartPidFile != "" {
			err := singularity.WriteInstancePidFile(ep.Instance, instanceStartPidFile)
			if err != nil {
				sylog.Warningf("Failed to write pid file: %v", err)
			}
		}
	},
	Use:     docs.InstanceRunUse,
	Short:   docs.InstanceRunShort,
	Long:    docs.InstanceRunLong,
	Example: docs.InstanceRunExample,
}
