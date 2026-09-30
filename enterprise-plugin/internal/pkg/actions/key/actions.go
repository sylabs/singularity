// Copyright (c) 2021-2026, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package key

import (
	"fmt"

	"github.com/go-openapi/runtime"
	"github.com/spf13/cobra"
	"github.com/sylabs/singularity/v4/enterprise-plugin/internal/pkg/api"
	"github.com/sylabs/singularity/v4/enterprise-plugin/internal/pkg/output"
	"github.com/sylabs/singularity/v4/enterprise-plugin/internal/pkg/user"
	keyops "github.com/sylabs/singularity/v4/enterprise-plugin/pkg/keyservice/client/operations"
	"github.com/sylabs/singularity/v4/enterprise-plugin/pkg/keyservice/models"
	"github.com/sylabs/singularity/v4/pkg/sylog"
)

const (
	// CLITypeShort is the string representing this type in CLI commands.
	CLIType = "key"
	// CLITypePlural is the plural form of the string representing this type in CLI commands.
	CLITypePlural = "keys"
	// CLITypeShort is the shortened form for this type in CLI commands.
	CLITypeShort = "key"
)

var (
	// listShortFields is the list of build metadata fields to display in short output formats.
	listShortFields = []string{"Fingerprint", "Names", "ExpirationTime"}
	// listShortHeaders is the list of build metadata field headers to display in short output formats.
	listShortHeaders = listShortFields
	// listLongFields is the list of build metadata fields to Display in long output formats.
	listLongFields = []string{"Fingerprint", "KeyIDShort", "Names", "CreationTime", "ExpirationTime", "Algorithm", "BitLength"}
	// listLongHeaders is the list of build metadata field headers to display in long output formats.
	listLongHeaders = listLongFields
)

// ActionRunner implements CRUD actions on a key.
type ActionRunner struct {
	clients  *api.Clients
	userID   string
	authInfo runtime.ClientAuthInfoWriter
}

// NewActionRunner returns a key.ActionRunner that will use the specified client and authInfo.
func NewActionRunner(clients *api.Clients, user string, authInfo runtime.ClientAuthInfoWriter) *ActionRunner {
	return &ActionRunner{clients, user, authInfo}
}

// Get implements the CLI get action for a key.
func (ar *ActionRunner) Get(cmd *cobra.Command, args []string) error {
	// No args = list of all keys with filtering
	if len(args) == 0 {
		return ar.getList(cmd)
	}
	// Args = list of specified keys
	return ar.getRefs(cmd, args)
}

// getList produces a list of keys with filtering.
func (ar *ActionRunner) getList(cmd *cobra.Command) error {
	uid, err := cmd.Flags().GetString("user")
	if err != nil {
		return err
	}
	// If user specified, check it's a BSON ID or translate to ID from username
	if uid != "" {
		uid, err = user.UsernameToID(uid, ar.clients)
		if err != nil {
			return fmt.Errorf("while looking up user: %v", err)
		}
	}
	// Default to user from auth token if not overridden here
	if uid == "" && ar.userID != "" {
		uid = ar.userID
	}
	// --all overrides default or other --user
	all, err := cmd.Flags().GetBool("all")
	if err != nil {
		return err
	}
	if all {
		uid = ""
	}
	if uid != "" {
		sylog.Infof("Showing results for user: %s", uid)
	}

	params := keyops.NewListKeysParams().
		WithCreatedby(&uid)

	resp, err := ar.clients.KeyClient.Operations.ListKeys(params, ar.authInfo)
	if err != nil {
		return err
	}

	data := resp.GetPayload().Data
	return ar.outputList(cmd, data)
}

// getRefs produces a list of keys matching specified queries.
func (ar *ActionRunner) getRefs(cmd *cobra.Command, args []string) error {
	for _, ref := range args {
		params := keyops.NewPksLookupParams().WithOp("index").WithSearch(ref)
		resp, err := ar.clients.KeyClient.Operations.PksLookup(params, ar.authInfo)
		if err != nil {
			sylog.Warningf("while fetching %s: %v", ref, err)
			continue
		}
		cmd.Print(resp.GetPayload())
	}
	return nil
}

// outputList outputs a list of keys in format set via --output flag.
func (ar *ActionRunner) outputList(cmd *cobra.Command, data []*models.KeysResponseDataItems0) error {
	format, err := cmd.Flags().GetString("output")
	if err != nil {
		return err
	}

	dataGeneric := make([]any, len(data))
	for idx, val := range data {
		dataGeneric[idx] = val
	}

	formatSpec := output.FormatSpec{
		ShortFields:  listShortFields,
		ShortHeaders: listShortHeaders,
		LongFields:   listLongFields,
		LongHeaders:  listLongHeaders,
		Clients:      ar.clients,
		Transforms: map[string]output.TransformFunc{
			"CreationTime":   output.TimezoneTransform,
			"ExpirationTime": output.TimezoneTransform,
		},
	}

	out, err := output.Format(format, dataGeneric, nil, formatSpec)
	if err == nil {
		cmd.Print(out)
	}
	return err
}

// Describe implements the CLI describe action for a key.
func (ar *ActionRunner) Describe(*cobra.Command, []string) error {
	sylog.Errorf("Not yet implemented")
	return nil
}

// Delete implements the CLI delete action for a key.
func (ar *ActionRunner) Delete(*cobra.Command, []string) error {
	sylog.Errorf("Not yet implemented")
	return nil
}
