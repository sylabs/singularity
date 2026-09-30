// Copyright (c) 2021-2026, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package buildagent

import (
	"fmt"

	"github.com/go-openapi/runtime"
	"github.com/spf13/cobra"
	"github.com/sylabs/singularity/v4/enterprise-plugin/internal/pkg/api"
	"github.com/sylabs/singularity/v4/enterprise-plugin/internal/pkg/output"
	bmanops "github.com/sylabs/singularity/v4/enterprise-plugin/pkg/buildmanager/client/operations"
	"github.com/sylabs/singularity/v4/pkg/sylog"
)

const (
	// CLITypeShort is the string representing this type in CLI commands.
	CLIType = "build-agent"
	// CLITypePlural is the plural form of the string representing this type in CLI commands.
	CLITypePlural = "build-agents"
	// CLITypeShort is the shortened form for this type in CLI commands.
	CLITypeShort = "age"
)

var (
	// shortFields is the list of fields to display in short output formats.
	shortFields = []string{"ID", "Queue", "AddedAt", "TaskID"}
	// shortHeaders is the list of field headers to display in short output formats.
	shortHeaders = []string{"ID", "Queue", "AddedAt", "TaskID"}
	// longFields is the list of fields to Display in long output formats.
	longFields = shortFields
	// longHeaders is the list of field headers to display in long output formats.
	longHeaders = shortHeaders
	// formatSpec is the output format specification for a list of builds.
	formatSpec = output.FormatSpec{
		ShortFields:  shortFields,
		ShortHeaders: shortHeaders,
		LongFields:   longFields,
		LongHeaders:  longHeaders,
		Transforms: map[string]output.TransformFunc{
			"AddedAt": output.TimezoneTransform,
		},
	}
)

// ActionRunner implements CRUD actions on a buildagent.
type ActionRunner struct {
	clients  *api.Clients
	userID   string
	authInfo runtime.ClientAuthInfoWriter
}

// NewActionRunner returns a buildagent.ActionRunner that will use the specified client and authInfo.
func NewActionRunner(clients *api.Clients, user string, authInfo runtime.ClientAuthInfoWriter) *ActionRunner {
	return &ActionRunner{clients, user, authInfo}
}

// Get implements the CLI get action for a build agent.
func (ar *ActionRunner) Get(cmd *cobra.Command, args []string) error {
	// get build-agent
	// returns a list
	if len(args) != 1 {
		return fmt.Errorf("please specify a single builder pool to view agents")
	}

	return ar.getList(cmd, args)
}

func (ar *ActionRunner) getList(cmd *cobra.Command, args []string) error {
	resp, err := ar.clients.BuildManClient.Operations.GetAgentPool(bmanops.NewGetAgentPoolParams().WithName(args[0]), ar.authInfo)
	if err != nil {
		return err
	}

	data := resp.GetPayload().Data
	dataGeneric := make([]any, len(data))
	for idx, val := range data {
		dataGeneric[idx] = val
	}

	format, err := cmd.Flags().GetString("output")
	if err != nil {
		return err
	}

	out, err := output.Format(format, dataGeneric, nil, formatSpec)
	if err == nil {
		cmd.Print(out)
	}
	return err
}

// Describe implements the CLI describe action for a buildagent.
func (ar *ActionRunner) Describe(*cobra.Command, []string) error {
	sylog.Errorf("Not implemented. Please use 'get build-agent <pool>' to see build agent details.")
	return nil
}

// Delete implements the CLI delete action for a build agent.
func (ar *ActionRunner) Delete(*cobra.Command, []string) error {
	sylog.Errorf("Not yet implemented")
	return nil
}
