// Copyright (c) 2019-2023, Sylabs Inc. All rights reserved.
// Copyright (c) 2020, Control Command Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package cli

import (
	"os"

	"github.com/spf13/cobra"
	"github.com/sylabs/singularity/docs"
	"github.com/sylabs/singularity/internal/app/singularity"
	"github.com/sylabs/singularity/pkg/cmdline"
	"github.com/sylabs/singularity/pkg/sylog"
)

var (
	keyserverInsecure bool
	keyserverOrder    uint32
)

// -i|--insecure
var keyserverInsecureFlag = cmdline.Flag{
	ID:           "keyserverInsecureFlag",
	Value:        &keyserverInsecure,
	DefaultValue: false,
	Name:         "insecure",
	ShortHand:    "i",
	Usage:        "allow insecure connection to keyserver",
}

// -o|--order
var keyserverOrderFlag = cmdline.Flag{
	ID:           "keyserverOrderFlag",
	Value:        &keyserverOrder,
	DefaultValue: uint32(0),
	Name:         "order",
	ShortHand:    "o",
	Usage:        "define the keyserver order",
}

func init() {
	addCmdInit(func(cmdManager *cmdline.CommandManager) {
		cmdManager.RegisterCmd(KeyserverCmd)
		cmdManager.RegisterSubCmd(KeyserverCmd, KeyserverAddCmd)
		cmdManager.RegisterSubCmd(KeyserverCmd, KeyserverRemoveCmd)
		cmdManager.RegisterSubCmd(KeyserverCmd, KeyserverListCmd)

		cmdManager.RegisterFlagForCmd(&keyserverOrderFlag, KeyserverAddCmd)
		cmdManager.RegisterFlagForCmd(&keyserverInsecureFlag, KeyserverAddCmd)
	})
}

// KeyserverCmd singularity keyserver [...]
var KeyserverCmd = &cobra.Command{
	Run: nil,

	Use:     docs.KeyserverUse,
	Short:   docs.KeyserverShort,
	Long:    docs.KeyserverLong,
	Example: docs.KeyserverExample,

	DisableFlagsInUseLine: true,
}

// KeyserverAddCmd singularity keyserver add [option] <keyserver_url>
var KeyserverAddCmd = &cobra.Command{
	Args:   cobra.RangeArgs(1, 2),
	PreRun: setKeyserver,
	Run: func(cmd *cobra.Command, args []string) {
		uri := args[0]
		name := ""
		if len(args) > 1 {
			name = args[0]
			uri = args[1]
		}

		if cmd.Flag(keyserverOrderFlag.Name).Changed && keyserverOrder == 0 {
			sylog.Fatalf("order must be > 0")
		}

		if err := singularity.KeyserverAdd(name, uri, keyserverOrder, keyserverInsecure); err != nil {
			sylog.Fatalf("%s", err)
		}
	},

	Use:     docs.KeyserverAddUse,
	Short:   docs.KeyserverAddShort,
	Long:    docs.KeyserverAddLong,
	Example: docs.KeyserverAddExample,

	DisableFlagsInUseLine: true,
}

// KeyserverRemoveCmd singularity remote remove-keyserver [remoteName] <keyserver_url>
var KeyserverRemoveCmd = &cobra.Command{
	Args:   cobra.RangeArgs(1, 2),
	PreRun: setKeyserver,
	Run: func(cmd *cobra.Command, args []string) {
		uri := args[0]
		name := ""
		if len(args) > 1 {
			name = args[0]
			uri = args[1]
		}

		if err := singularity.RemoteRemoveKeyserver(name, uri); err != nil {
			sylog.Fatalf("%s", err)
		}
	},

	Use:     docs.KeyserverRemoveUse,
	Short:   docs.KeyserverRemoveShort,
	Long:    docs.KeyserverRemoveLong,
	Example: docs.KeyserverRemoveExample,

	DisableFlagsInUseLine: true,
}

func setKeyserver(_ *cobra.Command, _ []string) {
	if uint32(os.Getuid()) != 0 {
		sylog.Fatalf("Unable to modify keyserver configuration: not root user")
	}
}

// KeyserverListCmd singularity remote list
var KeyserverListCmd = &cobra.Command{
	Args: cobra.RangeArgs(0, 1),
	Run: func(cmd *cobra.Command, args []string) {
		remoteName := ""
		if len(args) > 0 {
			remoteName = args[0]
		}
		if err := singularity.KeyserverList(remoteName, remoteConfig); err != nil {
			sylog.Fatalf("%s", err)
		}
	},

	Use:     docs.KeyserverListUse,
	Short:   docs.KeyserverListShort,
	Long:    docs.KeyserverListLong,
	Example: docs.KeyserverListExample,

	DisableFlagsInUseLine: true,
}
