// Copyright (c) 2021-2026, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package container

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
	"github.com/sylabs/singularity/v4/enterprise-plugin/pkg/library/client/container"
	"github.com/sylabs/singularity/v4/enterprise-plugin/pkg/library/models"
	"github.com/sylabs/singularity/v4/pkg/sylog"
)

const (
	// CLITypeShort is the string representing this type in CLI commands.
	CLIType = "container"
	// CLITypePlural is the plural form of the string representing this type in CLI commands.
	CLITypePlural = "containers"
	// CLITypeShort is the shortened form for this type in CLI commands.
	CLITypeShort = "con"
	// CLITypeRepo is the string representing this type in CLI commands.
	CLITypeRepo = "repository"
	// CLITypePlural is the plural form of the string representing this type in CLI commands.
	CLITypeRepoPlural = "repositories"
	// CLITypeShort is the shortened form for this type in CLI commands.
	CLITypeRepoShort = "rep"
)

var (
	// listShortFields is the list of build metadata fields to display in short output formats.
	listShortFields = []string{"ID", "Name", "Description", "Images", "Tags", "Size", "DownloadCount"}
	// listShortHeaders is the list of build metadata field headers to display in short output formats.
	listShortHeaders = listShortFields
	// listLongFields is the list of build metadata fields to Display in long output formats.
	listLongFields = []string{"ID", "Name", "Entity", "EntityName", "Collection", "CollectionName", "Description", "Images", "Tags", "Size", "DownloadCount", "ImageTags", "CreatedAt", "UpdatedAt", "Deleted"}
	// listLongHeaders is the list of build metadata field headers to display in long output formats.
	listLongHeaders = listLongFields
)

// ActionRunner implements CRUD actions on a containers.
type ActionRunner struct {
	clients  *api.Clients
	userID   string
	authInfo runtime.ClientAuthInfoWriter
}

// NewActionRunner returns a container.ActionRunner that will use the specified client and authInfo.
func NewActionRunner(clients *api.Clients, user string, authInfo runtime.ClientAuthInfoWriter) *ActionRunner {
	return &ActionRunner{clients, user, authInfo}
}

// Get implements the CLI get action for a container.
func (ar *ActionRunner) Get(cmd *cobra.Command, args []string) error {
	// No args = not supported
	if len(args) == 0 {
		return fmt.Errorf("a repository ref(s) is required")
		// 1 arg with 1 '/' in the ref = list containers for the collection
	} else if len(args) == 1 && strings.Count(args[0], "/") == 1 {
		return ar.getCollectionContainers(cmd, args)
	}
	return ar.getRefs(cmd, args)
}

// getCollectionContainers produces a list of all containers for a specific collection.
func (ar *ActionRunner) getCollectionContainers(cmd *cobra.Command, args []string) error {
	params := collection.NewListCollectionContainersParams().WithDefaults().WithRef(args[0])
	resp, err := ar.clients.LibraryClient.Collection.ListCollectionContainers(params)
	if err != nil {
		return err
	}
	data := resp.GetPayload().Data
	return ar.outputList(cmd, data)
}

// getRefs produces a list of specified containers in the library.
func (ar *ActionRunner) getRefs(cmd *cobra.Command, args []string) error {
	data := []*models.Container{}
	for _, ref := range args {
		params := container.NewGetContainerParams().WithRef(ref)
		resp, err := ar.clients.LibraryClient.Container.GetContainer(params, ar.authInfo)
		if err != nil {
			sylog.Warningf("while fetching %s: %v", ref, err)
			continue
		}
		data = append(data, resp.GetPayload().Data)
	}
	return ar.outputList(cmd, data)
}

// outputList outputs a sorted list of containers in format set via --output flag.
func (ar *ActionRunner) outputList(cmd *cobra.Command, data []*models.Container) error {
	format, err := cmd.Flags().GetString("output")
	if err != nil {
		return err
	}

	// Identify sort order - EntityName/CollectionName/Name
	indices := argsort.SortSlice(data, func(i, j int) bool {
		if data[i].EntityName > data[j].EntityName {
			return false
		}
		if data[i].EntityName < data[j].EntityName {
			return true
		}
		if data[i].CollectionName > data[j].CollectionName {
			return false
		}
		if data[i].CollectionName < data[j].CollectionName {
			return true
		}
		return *data[i].Name < *data[j].Name
	})

	dataGeneric := make([]any, len(data))
	imageCounts := make([]string, len(data))
	tagCounts := make([]string, len(data))

	// Assemble output in order
	for i, sortIdx := range indices {
		dataGeneric[i] = data[sortIdx]
		imageCounts[i] = strconv.Itoa(len(data[sortIdx].Images))
		tagCounts[i] = strconv.Itoa(len(data[sortIdx].ImageTags))
	}
	extra := map[string][]string{
		"Images": imageCounts,
		"Tags":   tagCounts,
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

	out, err := output.Format(format, dataGeneric, &extra, formatSpec)
	if err == nil {
		cmd.Print(out)
	}
	return err
}

// Describe implements the CLI describe action for a container.
func (ar *ActionRunner) Describe(cmd *cobra.Command, args []string) error {
	params := container.NewGetContainerParams().WithRef(args[0])

	resp, err := ar.clients.LibraryClient.Container.GetContainer(params, ar.authInfo)
	if err != nil {
		return err
	}

	header := "Container: " + args[0]
	data := resp.GetPayload().Data
	out, err := output.DescribeStruct(header, data)
	if err != nil {
		return err
	}
	cmd.Print(out)
	return nil
}

// Delete implements the CLI delete action for a container.
func (ar *ActionRunner) Delete(*cobra.Command, []string) error {
	sylog.Errorf("Not yet implemented")
	return nil
}
