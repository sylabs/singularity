// Copyright (c) 2020, Control Command Inc. All rights reserved.
// Copyright (c) 2019-2020, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package cli

import (
	"github.com/spf13/cobra"
	"github.com/sylabs/singularity/v4/docs"
	"github.com/sylabs/singularity/v4/internal/pkg/buildcfg"
	"github.com/sylabs/singularity/v4/internal/pkg/sypgp"
	"github.com/sylabs/singularity/v4/pkg/cmdline"
	"github.com/sylabs/singularity/v4/pkg/sylog"
)

var secretRemove bool

// -s|--secret
var keyRemoveSecretFlag = cmdline.Flag{
	ID:           "keyRemoveSecretFlag",
	Value:        &secretRemove,
	DefaultValue: false,
	Name:         "secret",
	ShortHand:    "s",
	Usage:        "remove a secret key (synonym for --private)",
}

// --private
var keyRemovePrivateFlag = cmdline.Flag{
	ID:           "keyRemovePrivateFlag",
	Value:        &secretRemove,
	DefaultValue: false,
	Name:         "private",
	Usage:        "remove a private key (synonym for --secret)",
}

func init() {
	addCmdInit(func(cmdManager *cmdline.CommandManager) {
		cmdManager.RegisterFlagForCmd(&keyRemoveSecretFlag, KeyRemoveCmd)
		cmdManager.RegisterFlagForCmd(&keyRemovePrivateFlag, KeyRemoveCmd)
	})
}

// KeyRemoveCmd is `singularity key remove <fingerprint>' command
var KeyRemoveCmd = &cobra.Command{
	PreRun:                checkGlobal,
	Args:                  cobra.ExactArgs(1),
	DisableFlagsInUseLine: true,
	Run: func(_ *cobra.Command, args []string) {
		var opts []sypgp.HandleOpt
		path := ""

		if keyGlobalPubKey {
			path = buildcfg.SINGULARITY_CONFDIR
			opts = append(opts, sypgp.GlobalHandleOpt())
		}

		keyring := sypgp.NewHandle(path, opts...)
		if secretRemove {
			err := keyring.RemovePrivKey(args[0])
			if err != nil {
				sylog.Fatalf("Unable to remove private key: %s", err)
			}
		} else {
			err := keyring.RemovePubKey(args[0])
			if err != nil {
				sylog.Fatalf("Unable to remove public key: %s", err)
			}
		}
	},

	Use:     docs.KeyRemoveUse,
	Short:   docs.KeyRemoveShort,
	Long:    docs.KeyRemoveLong,
	Example: docs.KeyRemoveExample,
}
