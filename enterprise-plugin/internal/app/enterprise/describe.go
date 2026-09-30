// Copyright (c) 2021-2026, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package enterprise

import (
	"github.com/spf13/cobra"
	"github.com/sylabs/singularity/v4/enterprise-plugin/internal/pkg/actions"
)

// TODO
// initDescribe creates the describe command to display a specific object.
//
// Should eventually take multi forms of input including type/name pairs and
// the library URLs many are already familiar with.
func (e *Executor) initDescribe() *cobra.Command {
	cmd := cobra.Command{
		Args:  cobra.ExactArgs(2),
		Use:   "describe",
		Short: "Displays a specific item in an enterprise service",

		Long: `
  Displays a specific item in an enterprise service:

	projects     / project     / prj     - Library projects
	repositories / repository  / rep     - Library repository
	images       / image       / img     - Library images

	builds       / build       / bld     - Remote builder builds
	build-agents / build-agent / age     - Remote builder build agents

	keys         / key                   - Key service keys

	users        / user        / usr     - User accounts
	tokens       / token       / tok     - User authentication tokens

    Legacy Enterprise 1.x library support:

	entities     / entity      / ent     - Library entities
	collections  / collection  / col     - Library collections
	containers   / container   / con     - Library containers
	images       / image       / img     - Library images`,
		Example: `
	# Show the detail of the repository 'suzy/linux/test' in the library
	enterprise describe repository suzy/linux/test

	# Show the detail about the user account 'user123' (admin only)
	enterprise describe user user123`,
		RunE: e.describe,
	}
	return &cmd
}

func (e *Executor) describe(cmd *cobra.Command, args []string) error {
	ar, err := actions.NewRunner(args[0], e.clients, e.rootOpt.authUser, e.authInfo)
	if err != nil {
		return err
	}
	return ar.Describe(cmd, args[1:])
}
