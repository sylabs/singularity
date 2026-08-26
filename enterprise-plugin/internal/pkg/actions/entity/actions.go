// Copyright (c) 2021-2026, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package entity

import (
	"github.com/go-openapi/runtime"
	"github.com/mkmik/argsort"
	"github.com/spf13/cobra"
	"github.com/sylabs/singularity/v4/enterprise-plugin/internal/pkg/api"
	"github.com/sylabs/singularity/v4/enterprise-plugin/internal/pkg/output"
	"github.com/sylabs/singularity/v4/enterprise-plugin/pkg/library/client/entity"
	"github.com/sylabs/singularity/v4/enterprise-plugin/pkg/library/models"
	"github.com/sylabs/singularity/v4/pkg/sylog"
)

const (
	// CLIType is the string representing this type in CLI commands.
	CLIType = "entity"
	// CLITypePlural is the plural form of the string representing this type in CLI commands.
	CLITypePlural = "entities"
	// CLITypeShort is the shortened form for this type in CLI commands.
	CLITypeShort = "ent"
	// CLITypeProject is the string representing the project alternate in CLI commands.
	CLITypeProject = "project"
	// CLITypeProjectPlural is the plural form of the string representing the project alternate in CLI commands.
	CLITypeProjectPlural = "projects"
	// CLITypeProjectShort is the shortened form for the project alternate in CLI commands.
	CLITypeProjectShort = "prj"
)

var (
	// listShortFields is the list of entity fields to display in short output formats.
	listShortFields = []string{"ID", "Name", "Owner"}
	// listShortHeaders is the list of entity field headers to display in short output formats.
	listShortHeaders = listShortFields
	// listLongFields is the list of entity fields to Display in long output formats.
	listLongFields = []string{"ID", "Name", "Owner", "CreatedAt", "UpdatedAt", "DefaultPrivate"}
	// listLongHeaders is the list of entity field headers to display in long output formats.
	listLongHeaders = listLongFields
)

// ActionRunner implements CRUD actions on a entity.
type ActionRunner struct {
	clients  *api.Clients
	userID   string
	authInfo runtime.ClientAuthInfoWriter
}

// NewActionRunner returns a collection.ActionRunner that will use the specified client and authInfo.
func NewActionRunner(clients *api.Clients, userID string, authInfo runtime.ClientAuthInfoWriter) *ActionRunner {
	return &ActionRunner{clients, userID, authInfo}
}

// Get implements the CLI get action for a entity.
func (ar *ActionRunner) Get(cmd *cobra.Command, args []string) error {
	// No args = list of all entities
	if len(args) == 0 {
		return ar.getAll(cmd)
	}
	// Args = list of specified entities
	return ar.getRefs(cmd, args)
}

// getAll produces a list of all entities in the library.
func (ar *ActionRunner) getAll(cmd *cobra.Command) error {
	params := entity.NewListEntitiesParams()
	resp, err := ar.clients.LibraryClient.Entity.ListEntities(params)
	if err != nil {
		return err
	}
	data := resp.GetPayload().Data
	return ar.outputList(cmd, data)
}

// getRefs produces a list of specified entities in the library.
func (ar *ActionRunner) getRefs(cmd *cobra.Command, args []string) error {
	data := []*models.Entity{}
	for _, ref := range args {
		params := entity.NewGetEntityParams().WithRef(ref)
		resp, err := ar.clients.LibraryClient.Entity.GetEntity(params)
		if err != nil {
			sylog.Warningf("while fetching %s: %v", ref, err)
			continue
		}
		data = append(data, resp.GetPayload().Data)
	}
	return ar.outputList(cmd, data)
}

// outputList outputs a sorted list of entities in format set via --output flag.
func (ar *ActionRunner) outputList(cmd *cobra.Command, data []*models.Entity) error {
	format, err := cmd.Flags().GetString("output")
	if err != nil {
		return err
	}

	// Identify sort order - Name
	indices := argsort.SortSlice(data, func(i, j int) bool {
		return *data[i].Name < *data[j].Name
	})
	// Assemble output in order
	dataGeneric := make([]any, len(data))
	for i, sortIdx := range indices {
		dataGeneric[i] = data[sortIdx]
	}

	formatSpec := output.FormatSpec{
		ShortFields:  listShortFields,
		ShortHeaders: listShortHeaders,
		LongFields:   listLongFields,
		LongHeaders:  listLongHeaders,
		Clients:      ar.clients,
		Transforms: output.TransformMap{
			"Description": output.Truncate50Transform,
			"Size":        output.HumanByteTransform,
			"CreatedAt":   output.TimezoneTransform,
			"UpdatedAt":   output.TimezoneTransform,
		},
	}

	out, err := output.Format(format, dataGeneric, nil, formatSpec)
	if err == nil {
		cmd.Print(out)
	}
	return err
}

// Describe implements the CLI describe action for a entity.
func (ar *ActionRunner) Describe(cmd *cobra.Command, args []string) error {
	params := entity.NewGetEntityParams().WithRef(args[0])

	resp, err := ar.clients.LibraryClient.Entity.GetEntity(params)
	if err != nil {
		return err
	}

	header := "Entity: " + args[0]
	data := resp.GetPayload().Data
	out, err := output.DescribeStruct(header, data)
	if err != nil {
		return err
	}
	cmd.Print(out)
	return nil
}

// Delete implements the CLI delete action for a entity.
func (ar *ActionRunner) Delete(*cobra.Command, []string) error {
	sylog.Errorf("Not yet implemented")
	return nil
}
