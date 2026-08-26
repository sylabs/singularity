// Copyright (c) 2021-2026, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package enterprise

import (
	"github.com/spf13/cobra"
	"github.com/sylabs/singularity/v4/pkg/sylog"
)

// logtest is a *hidden* command that will display standard output, and a messsage
// at each of the sylog levels, so we can verify logging matches Singularity
// behavior in tests.
func (e *Executor) initLogtest() *cobra.Command {
	cmd := cobra.Command{
		Args: cobra.ExactArgs(0),

		Use:     "logtest",
		Short:   "Displays all sylog levels for test purposes",
		Long:    "Displays all sylog levels for test purposes",
		Example: "logtest",
		RunE:    e.logtest,
		Hidden:  true,
	}
	return &cmd
}

func (e *Executor) logtest(cmd *cobra.Command, _ []string) error {
	cmd.Println("Test Println")
	sylog.Warningf("Test Warningf")
	sylog.Errorf("Test Errorf")
	sylog.Infof("Test Infof")
	sylog.Verbosef("Test Verbosef")
	sylog.Debugf("Test Debugf")
	return nil
}
