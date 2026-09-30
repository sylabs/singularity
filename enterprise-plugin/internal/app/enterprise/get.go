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
// initGet creates the get command to display a specific object, or all
// of a type of object if none specified.
//
// Should eventually take multi forms of input including type/name pairs and
// the library URLs many are already familiar with. E.g. both
// `get library://user/collection/container` and
// `get container user/collection/container` for specific items and
// `get container` for all items.
func (e *Executor) initGet() *cobra.Command {
	cmd := cobra.Command{
		Args:  cobra.MinimumNArgs(1),
		Use:   "get [flags] <type> [identifiers]...",
		Short: "List all items, or specific items, of a specified type.",
		Long: `
  List all items, or specific items, of a specified type:

	projects     / project     / prj     - Library projects
	repositories / repository  / rep     - Library repositories
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
	# Get list of all projects in the library
	enterprise get projects
	# Get list with detail about 'suzy' and 'alfred' in the library
	enterprise get project suzy alfred

	# Get a list of repositories under 'suzy/linux'
	enterprise get repositories suzy/linux
	# Get list with detail about the repositories 'suzy/linux/alpine' and
	# 'suzy/linux/ubuntu'
	enterprise get repositories suzy/linux/alpine suzy/linux/ubuntu

	# Get a list of container images for the repository 'suzy/linux/alpine'
	enterprise get images suzy/linux/alpine
	# Get list with detail about the images 'suzy/linux/alpine:latest'
	# and 'suzy/linux/alpine:v1'
	enterprise get containers suzy/linux/alpine:latest suzy/linux/alpine:v1

	# Get list of recent builds for the currently authenticated user
	enterprise get builds
	# Get list of recent builds for another user (admin only)
	enterprise get builds --user 507f1f77bcf86cd799439aaa
	enterprise get builds --user user123
	# Get list of recent builds for all users (admin only)
	enterprise get builds --all

	# Get list of build agents in the default pool (admin only)
	enterprise get build-agents default

	# Get list of keys for the currently authenticated user
	enterprise get keys
	# Get list of keys for a specific user
	enterprise get keys --user 507f1f77bcf86cd799439aaa
	enterprise get keys --user user123
	# Get list of keys matching a search term
	enterprise get keys test-key
	# Get list of keys for all users (admin only)
	enterprise get keys --all

	# Get list of user accounts (admin only)
	enterprise get users
	# Get list with detail about specified users
	enterprise get users 507f1f77bcf86cd799439aaa 507f1f77bcf86cd799439aab
	enterprise get users user123 user456

	# Get list of current user's authentication tokens 
	enterprise get tokens
	# Get list of current user's authentication tokens, including auto-generated
	# remote build tokens
	enterprise get tokens --show-auto-tokens
	# Get list of current user's authentication tokens, including disabled (revoked / expired) tokens
	enterprise get tokens --show-disabled-tokens
	# Get list with detail about specified authentication tokens
	enterprise get tokens 614a452fbe17a2b0f416b5e3 614a452fbe17a2b0f416b5f0 
	# Get list of authentication tokens for another user (admin only)
	enterprise get tokens --user 507f1f77bcf86cd799439aaa
	enterprise get tokens --user user123
`,
		RunE: e.get,
	}

	cmd.PersistentFlags().String("user", "", "Limit results to specified user")
	cmd.PersistentFlags().Bool("all", false, "Show results for all users")
	cmd.PersistentFlags().String("from", "", "Limit results to specified start date/time")
	cmd.PersistentFlags().String("to", "", "Limit results to specified end date/time")
	cmd.PersistentFlags().StringP("output", "o", "short", "output format (short/long/json/csv)")
	cmd.PersistentFlags().Bool("show-auto-tokens", false, "Show auto-generated remote build tokens")
	cmd.PersistentFlags().Bool("show-disabled-tokens", false, "Show disabled (revoked & expired) tokens")

	return &cmd
}

func (e *Executor) get(cmd *cobra.Command, args []string) error {
	ar, err := actions.NewRunner(args[0], e.clients, e.rootOpt.authUser, e.authInfo)
	if err != nil {
		return err
	}
	return ar.Get(cmd, args[1:])
}
