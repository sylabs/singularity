// Copyright (c) 2017-2025, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package cli

import (
	"context"
	"crypto"
	"fmt"
	"os"

	"github.com/sigstore/cosign/v2/pkg/cosign"
	"github.com/sigstore/sigstore/pkg/cryptoutils"
	"github.com/sigstore/sigstore/pkg/signature"
	"github.com/spf13/cobra"
	"github.com/sylabs/singularity/v4/docs"
	cosignsignature "github.com/sylabs/singularity/v4/internal/pkg/cosign"
	sifsignature "github.com/sylabs/singularity/v4/internal/pkg/signature"
	"github.com/sylabs/singularity/v4/internal/pkg/sypgp"
	"github.com/sylabs/singularity/v4/pkg/cmdline"
	"github.com/sylabs/singularity/v4/pkg/sylog"
)

var (
	priKeyPath string
	priKeyIdx  int
	signAll    bool
	useCosign  bool
)

// -g|--group-id
var signSifGroupIDFlag = cmdline.Flag{
	ID:           "signSifGroupIDFlag",
	Value:        &sifGroupID,
	DefaultValue: uint32(0),
	Name:         "group-id",
	ShortHand:    "g",
	Usage:        "sign objects with the specified group ID",
}

// --groupid (deprecated)
var signOldSifGroupIDFlag = cmdline.Flag{
	ID:           "signOldSifGroupIDFlag",
	Value:        &sifGroupID,
	DefaultValue: uint32(0),
	Name:         "groupid",
	Usage:        "sign objects with the specified group ID",
	Deprecated:   "use '--group-id'",
}

// -i| --sif-id
var signSifDescSifIDFlag = cmdline.Flag{
	ID:           "signSifDescSifIDFlag",
	Value:        &sifDescID,
	DefaultValue: uint32(0),
	Name:         "sif-id",
	ShortHand:    "i",
	Usage:        "sign object with the specified ID",
}

// --id (deprecated)
var signSifDescIDFlag = cmdline.Flag{
	ID:           "signSifDescIDFlag",
	Value:        &sifDescID,
	DefaultValue: uint32(0),
	Name:         "id",
	Usage:        "sign object with the specified ID",
	Deprecated:   "use '--sif-id'",
}

// --key
var signPrivateKeyFlag = cmdline.Flag{
	ID:           "privateKeyFlag",
	Value:        &priKeyPath,
	DefaultValue: "",
	Name:         "key",
	Usage:        "path to the private key file",
	EnvKeys:      []string{"SIGN_KEY"},
}

// -k|--keyidx
var signKeyIdxFlag = cmdline.Flag{
	ID:           "signKeyIdxFlag",
	Value:        &priKeyIdx,
	DefaultValue: 0,
	Name:         "keyidx",
	ShortHand:    "k",
	Usage:        "PGP private key to use (index from 'key list --secret')",
}

// -a|--all (deprecated)
var signAllFlag = cmdline.Flag{
	ID:           "signAllFlag",
	Value:        &signAll,
	DefaultValue: false,
	Name:         "all",
	ShortHand:    "a",
	Usage:        "sign all objects",
	Deprecated:   "now the default behavior",
}

// -c|--cosign
var cosignFlag = cmdline.Flag{
	ID:           "cosignFlag",
	Value:        &useCosign,
	DefaultValue: false,
	Name:         "cosign",
	ShortHand:    "c",
	Usage:        "sign an OCI-SIF with a cosign-compatible sigstore signature",
}

func init() {
	addCmdInit(func(cmdManager *cmdline.CommandManager) {
		cmdManager.RegisterCmd(SignCmd)

		cmdManager.RegisterFlagForCmd(&signSifGroupIDFlag, SignCmd)
		cmdManager.RegisterFlagForCmd(&signOldSifGroupIDFlag, SignCmd)
		cmdManager.RegisterFlagForCmd(&signSifDescSifIDFlag, SignCmd)
		cmdManager.RegisterFlagForCmd(&signSifDescIDFlag, SignCmd)
		cmdManager.RegisterFlagForCmd(&signPrivateKeyFlag, SignCmd)
		cmdManager.RegisterFlagForCmd(&signKeyIdxFlag, SignCmd)
		cmdManager.RegisterFlagForCmd(&signAllFlag, SignCmd)
		cmdManager.RegisterFlagForCmd(&cosignFlag, SignCmd)
	})
}

// SignCmd singularity sign
var SignCmd = &cobra.Command{
	DisableFlagsInUseLine: true,
	Args:                  cobra.ExactArgs(1),

	Run: func(cmd *cobra.Command, args []string) {
		// args[0] contains image path
		doSignCmd(cmd, args[0])
	},

	Use:     docs.SignUse,
	Short:   docs.SignShort,
	Long:    docs.SignLong,
	Example: docs.SignExample,
}

func doSignCmd(cmd *cobra.Command, cpath string) {
	if useCosign {
		if priKeyPath == "" {
			sylog.Fatalf("--cosign signatures require a private --key to be specified")
		}
		if priKeyIdx != 0 {
			sylog.Fatalf("--keyidx not supported: --cosign signatures use a private --key, not the PGP keyring")
		}
		if signAll || sifGroupID != 0 || sifDescID != 0 {
			sylog.Fatalf("--cosign signatures sign an OCI image, specifying SIF descriptors / groups is not supported")
		}
		err := signCosign(cmd.Context(), cpath, priKeyPath)
		if err != nil {
			sylog.Fatalf("%v", err)
		}
		return
	}

	err := signSIF(cmd, cpath)
	if err != nil {
		sylog.Fatalf("%v", err)
	}
}

func signSIF(cmd *cobra.Command, cpath string) error {
	var opts []sifsignature.SignOpt

	// Set key material.
	switch {
	case cmd.Flag(signPrivateKeyFlag.Name).Changed:
		sylog.Infof("Signing image with key material from '%v'", priKeyPath)

		s, err := signature.LoadSignerFromPEMFile(priKeyPath, crypto.SHA256, cryptoutils.GetPasswordFromStdIn)
		if err != nil {
			return fmt.Errorf("Failed to load key material: %v", err)
		}
		opts = append(opts, sifsignature.OptSignWithSigner(s))

	default:
		sylog.Infof("Signing image with PGP key material")

		// Set entity selector option, and ensure the entity is decrypted.
		var f sypgp.EntitySelector
		if cmd.Flag(signKeyIdxFlag.Name).Changed {
			f = selectEntityAtIndex(priKeyIdx)
		} else {
			f = selectEntityInteractive()
		}
		f = decryptSelectedEntityInteractive(f)
		opts = append(opts, sifsignature.OptSignEntitySelector(f))
	}

	// Set group option, if applicable.
	if cmd.Flag(signSifGroupIDFlag.Name).Changed || cmd.Flag(signOldSifGroupIDFlag.Name).Changed {
		opts = append(opts, sifsignature.OptSignGroup(sifGroupID))
	}

	// Set object option, if applicable.
	if cmd.Flag(signSifDescSifIDFlag.Name).Changed || cmd.Flag(signSifDescIDFlag.Name).Changed {
		opts = append(opts, sifsignature.OptSignObjects(sifDescID))
	}

	// Sign the image.
	if err := sifsignature.Sign(cmd.Context(), cpath, opts...); err != nil {
		return fmt.Errorf("Failed to sign container: %w", err)
	}
	sylog.Infof("Signature created and applied to image '%v'", cpath)
	return nil
}

func signCosign(ctx context.Context, sifPath, keyPath string) error {
	sylog.Infof("Sigstore/cosign compatible signature, using key material from '%v'", priKeyPath)
	kb, err := os.ReadFile(keyPath)
	if err != nil {
		return fmt.Errorf("failed to load key material: %w", err)
	}

	pass, err := cryptoutils.GetPasswordFromStdIn(false)
	if err != nil {
		return fmt.Errorf("couldn't read key password: %w", err)
	}

	sv, err := cosign.LoadPrivateKey(kb, pass)
	if err != nil {
		return fmt.Errorf("failed to open OCI-SIF: %w", err)
	}

	return cosignsignature.SignOCISIF(ctx, sifPath, sv)
}
