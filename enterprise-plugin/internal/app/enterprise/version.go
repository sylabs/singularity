// Copyright (c) 2020-2026, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package enterprise

import (
	"fmt"
	"runtime"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
)

func (e *Executor) initVersion() *cobra.Command {
	cmd := cobra.Command{
		Args: cobra.ExactArgs(0),

		Use:     "version",
		Short:   "Display version info",
		Long:    "Display version and build information",
		Example: "version",
		RunE:    e.version,
	}
	return &cmd
}

func (e *Executor) version(cmd *cobra.Command, _ []string) error {
	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
	defer w.Flush()

	fmt.Fprintf(w, "Version:\t%v\n", e.versionString())
	if !e.builtAt.IsZero() {
		fmt.Fprintf(w, "Built:\t%v\n", e.builtAt.Format(time.UnixDate))
	}
	if e.builtBy != "" {
		fmt.Fprintf(w, "By:\t%v\n", e.builtBy)
	}
	if e.gitCommit != "" {
		format := "Commit:\t%v\n"
		if e.gitDirty {
			format = "Commit:\t%v (dirty)\n"
		}
		fmt.Fprintf(w, format, e.gitCommit)
	}
	fmt.Fprintf(w, "Runtime:\t%v (%v/%v)\n", runtime.Version(), runtime.GOOS, runtime.GOARCH)

	return nil
}
