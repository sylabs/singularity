// Copyright (c) 2025, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package cli

import (
	"os"

	"github.com/sigstore/cosign/v2/pkg/cosign"
	"github.com/sigstore/sigstore/pkg/cryptoutils"
	"github.com/spf13/cobra"
	"github.com/sylabs/singularity/v4/docs"
	"github.com/sylabs/singularity/v4/internal/pkg/util/fs"
	"github.com/sylabs/singularity/v4/pkg/cmdline"
	"github.com/sylabs/singularity/v4/pkg/sylog"
)

var (
	cosignKeyPairPrefix string

	// --output-key-prefix
	keyOutputKeyPrefixFlag = cmdline.Flag{
		ID:           "keyOutputKeyPrefixFlag",
		Value:        &cosignKeyPairPrefix,
		DefaultValue: "singularity-cosign",
		Name:         "output-key-prefix",
		Usage:        "prefix for .key / .pub files",
	}

	// KeyNewPairCmd is 'singularity key newpair' and generate a new OpenPGP key pair
	KeyGenerateCosignKeyPairCmd = &cobra.Command{
		Args:                  cobra.ExactArgs(0),
		DisableFlagsInUseLine: true,
		Run:                   runGenerateCosignKeyPairCmd,
		Use:                   docs.KeyGenerateCosignKeyPairUse,
		Short:                 docs.KeyGenerateCosignKeyPairShort,
		Long:                  docs.KeyGenerateCosignKeyPairLong,
		Example:               docs.KeyGenerateCosignKeyPairExample,
	}
)

func runGenerateCosignKeyPairCmd(_ *cobra.Command, _ []string) {
	priKey := cosignKeyPairPrefix + ".key"
	pubKey := cosignKeyPairPrefix + ".pub"

	exists, err := fs.PathExists(priKey)
	if err != nil {
		sylog.Fatalf("%v", err)
	}
	if exists {
		sylog.Fatalf("file exists, will not overwrite: %s", priKey)
	}

	exists, err = fs.PathExists(pubKey)
	if err != nil {
		sylog.Fatalf("%v", err)
	}
	if exists {
		sylog.Fatalf("file exists, will not overwrite: %s", pubKey)
	}

	sylog.Infof("Creating cosign key-pair %s.key/.pub", cosignKeyPairPrefix)

	kb, err := cosign.GenerateKeyPair(cryptoutils.GetPasswordFromStdIn)
	if err != nil {
		sylog.Fatalf("%v", err)
	}

	if err := os.WriteFile(priKey, kb.PrivateBytes, 0o600); err != nil {
		sylog.Fatalf("%v", err)
	}
	if err := os.WriteFile(pubKey, kb.PublicBytes, 0o644); err != nil {
		sylog.Fatalf("%v", err)
	}
}
