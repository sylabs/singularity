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
// initDelete creates the delete command to delete a specific object.
//
// Should eventually take multiple forms of input including type/name pairs and
// the library URLs many are already familiar with. E.g. both
// `delete library://user/collection/container` and
// `delete container user/collection/container`.
func (e *Executor) initDelete() *cobra.Command {
	cmd := cobra.Command{
		Args:  cobra.MinimumNArgs(1),
		Use:   "delete [flags] <type> [identifiers]...",
		Short: "Delete specific items, of a specified type.",
		Long: `Delete specific items, of a specified type:

	tokens       / token       / tok     - User authentication tokens`,
		Example: `
	
	# Delete (revoke) a specific authentication token
	enterprise delete token 614a452fbe17a2b0f416b5e3
	# Delete (revoke) all authentication tokens for the current user
	enterprise delete tokens --all
	# Delete (revoke) all authentication tokens for a different user (admin only)
	enterprise delete tokens --all --user 507f1f77bcf86cd799439aaa
	# Delete (revoke) all authentication tokens for all users except the current user (admin only)
	enterprise delete tokens --all --user all`,
		RunE: e.delete,
	}
	cmd.PersistentFlags().String("user", "", "Limit results to specified user")
	cmd.PersistentFlags().Bool("all", false, "Delete all items")
	return &cmd
}

func (e *Executor) delete(cmd *cobra.Command, args []string) error {
	ar, err := actions.NewRunner(args[0], e.clients, e.rootOpt.authUser, e.authInfo)
	if err != nil {
		return err
	}
	return ar.Delete(cmd, args[1:])
}
