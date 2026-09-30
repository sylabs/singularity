// Copyright (c) 2021-2026, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package collection

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/go-openapi/runtime"
	"github.com/mkmik/argsort"
	"github.com/spf13/cobra"
	"github.com/sylabs/singularity/v4/enterprise-plugin/internal/pkg/api"
	"github.com/sylabs/singularity/v4/enterprise-plugin/internal/pkg/output"
	"github.com/sylabs/singularity/v4/enterprise-plugin/pkg/library/client/collection"
	"github.com/sylabs/singularity/v4/enterprise-plugin/pkg/library/client/entity"
	"github.com/sylabs/singularity/v4/enterprise-plugin/pkg/library/models"
	"github.com/sylabs/singularity/v4/pkg/sylog"
)

const (
	// CLITypeShort is the string representing this type in CLI commands.
	CLIType = "collection"
	// CLITypePlural is the plural form of the string representing this type in CLI commands.
	CLITypePlural = "collections"
	// CLITypeShort is the shortened form for this type in CLI commands.
	CLITypeShort = "col"
)

var (
	// listShortFields is the list of build metadata fields to display in short output formats.
	listShortFields = []string{"ID", "Name", "Num. Containers"}
	// listShortHeaders is the list of build metadata field headers to display in short output formats.
	listShortHeaders = listShortFields
	// listLongFields is the list of build metadata fields to Display in long output formats.
	listLongFields = []string{"ID", "Name", "Entity", "EntityName", "Num. Containers", "Containers"}
	// listLongHeaders is the list of build metadata field headers to display in long output formats.
	listLongHeaders = listLongFields
)

// ActionRunner implements CRUD actions on a collection.
type ActionRunner struct {
	clients  *api.Clients
	userID   string
	authInfo runtime.ClientAuthInfoWriter
}

// NewActionRunner returns a collection.ActionRunner that will use the specified client and authInfo.
func NewActionRunner(clients *api.Clients, user string, authInfo runtime.ClientAuthInfoWriter) *ActionRunner {
	return &ActionRunner{clients, user, authInfo}
}

// Get implements the CLI get action for a collection.
func (ar *ActionRunner) Get(cmd *cobra.Command, args []string) error {
	// No args = not supported (in shim)
	if len(args) == 0 {
		return fmt.Errorf("an entity/project ref, or collection ref(s) is required")
		// 1 arg with no '/' in the ref = list collections for the entity
	} else if len(args) == 1 && !strings.Contains(args[0], "/") {
		return ar.getEntityCollections(cmd, args)
	}
	return ar.getRefs(cmd, args)
}

// getEntityCollections produces a list of all collections for a specific entity.
func (ar *ActionRunner) getEntityCollections(cmd *cobra.Command, args []string) error {
	params := entity.NewListEntityCollectionsParams().WithRef(args[0])
	resp, err := ar.clients.LibraryClient.Entity.ListEntityCollections(params)
	if err != nil {
		return err
	}
	data := resp.GetPayload().Data
	return ar.outputList(cmd, data)
}

// getRefs produces a list of specified collections in the library.
func (ar *ActionRunner) getRefs(cmd *cobra.Command, args []string) error {
	data := []*models.Collection{}
	for _, ref := range args {
		params := collection.NewGetCollectionParams().WithRef(ref)
		resp, err := ar.clients.LibraryClient.Collection.GetCollection(params)
		if err != nil {
			sylog.Warningf("while fetching %s: %v", ref, err)
			continue
		}
		data = append(data, resp.GetPayload().Data)
	}
	return ar.outputList(cmd, data)
}

// outputList outputs a sorted list of collections in format set via --output flag.
func (ar *ActionRunner) outputList(cmd *cobra.Command, data []*models.Collection) error {
	format, err := cmd.Flags().GetString("output")
	if err != nil {
		return err
	}

	// Identify sort order - EntityName/Name
	indices := argsort.SortSlice(data, func(i, j int) bool {
		if data[i].EntityName > data[j].EntityName {
			return false
		}
		if data[i].EntityName < data[j].EntityName {
			return true
		}
		return *data[i].Name < *data[j].Name
	})

	dataGeneric := make([]any, len(data))
	containerCount := make([]string, len(data))

	// Assemble output in order
	for i, sortIdx := range indices {
		dataGeneric[i] = data[sortIdx]
		containerCount[i] = strconv.Itoa(len(data[sortIdx].Containers))
	}
	extra := map[string][]string{
		"Num. Containers": containerCount,
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
		},
	}

	out, err := output.Format(format, dataGeneric, &extra, formatSpec)
	if err == nil {
		cmd.Print(out)
	}
	return err
}

// Describe implements the CLI describe action for a collection.
func (ar *ActionRunner) Describe(cmd *cobra.Command, args []string) error {
	params := collection.NewGetCollectionParams().WithRef(args[0])

	resp, err := ar.clients.LibraryClient.Collection.GetCollection(params)
	if err != nil {
		return err
	}

	header := "Collection: " + args[0]
	data := resp.GetPayload().Data
	out, err := output.DescribeStruct(header, data)
	if err != nil {
		return err
	}
	cmd.Print(out)
	return nil
}

// Delete implements the CLI delete action for a collection.
func (ar *ActionRunner) Delete(*cobra.Command, []string) error {
	sylog.Errorf("Not yet implemented")
	return nil
}
