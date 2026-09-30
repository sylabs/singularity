// Copyright (c) 2021-2026, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package image

import (
	"fmt"
	"strings"

	"github.com/go-openapi/runtime"
	"github.com/spf13/cobra"
	"github.com/sylabs/singularity/v4/enterprise-plugin/internal/pkg/api"
	"github.com/sylabs/singularity/v4/enterprise-plugin/internal/pkg/output"
	"github.com/sylabs/singularity/v4/enterprise-plugin/pkg/library/client/container"
	"github.com/sylabs/singularity/v4/enterprise-plugin/pkg/library/client/image"
	"github.com/sylabs/singularity/v4/enterprise-plugin/pkg/library/models"
	"github.com/sylabs/singularity/v4/pkg/sylog"
)

const (
	// CLITypeShort is the string representing this type in CLI commands.
	CLIType = "image"
	// CLITypePlural is the plural form of the string representing this type in CLI commands.
	CLITypePlural = "images"
	// CLITypeShort is the shortened form for this type in CLI commands.
	CLITypeShort = "img"
)

var (
	// listShortFields is the list of build metadata fields to display in short output formats.
	listShortFields = []string{"ID", "Tags", "Arch", "Description", "Size", "Signed", "Encrypted", "Uploaded"}
	// listShortHeaders is the list of build metadata field headers to display in short output formats.
	listShortHeaders = listShortFields
	// listLongFields is the list of build metadata fields to Display in long output formats.
	listLongFields = []string{"ID", "Entity", "EntityName", "Collection", "CollectionName", "Container", "ContainerName", "Tags", "Arch", "Description", "Hash", "Signed", "Encrypted", "Uploaded", "Deleted"}
	// listLongHeaders is the list of build metadata field headers to display in long output formats.
	listLongHeaders = listLongFields
)

// ActionRunner implements CRUD actions on an image.
type ActionRunner struct {
	clients  *api.Clients
	userID   string
	authInfo runtime.ClientAuthInfoWriter
}

// NewActionRunner returns an image.ActionRunner that will use the specified client and authInfo.
func NewActionRunner(clients *api.Clients, user string, authInfo runtime.ClientAuthInfoWriter) *ActionRunner {
	return &ActionRunner{clients, user, authInfo}
}

// Get implements the CLI get action for an image.
func (ar *ActionRunner) Get(cmd *cobra.Command, args []string) error {
	// No args = not supported
	if len(args) == 0 {
		return fmt.Errorf("a container ref, or images ref(s) is required")
		// 1 arg with no ':' in the ref = list images for the container
	} else if len(args) == 1 && !strings.Contains(args[0], ":") {
		return ar.getContainerImages(cmd, args)
	}
	return ar.getRefs(cmd, args)
}

func (ar *ActionRunner) getContainerImages(cmd *cobra.Command, args []string) error {
	params := container.NewListContainerImagesParams().WithRef(args[0])
	resp, err := ar.clients.LibraryClient.Container.ListContainerImages(params)
	if err != nil {
		return err
	}
	data := resp.GetPayload().Data
	return ar.outputList(cmd, data)
}

// getRefs produces a list of specified containers in the library.
func (ar *ActionRunner) getRefs(cmd *cobra.Command, args []string) error {
	data := []*models.Image{}
	for _, ref := range args {
		params := image.NewGetImageParams().WithRef(ref)
		resp, err := ar.clients.LibraryClient.Image.GetImage(params, ar.authInfo)
		if err != nil {
			sylog.Warningf("while fetching %s: %v", ref, err)
			continue
		}
		data = append(data, resp.GetPayload().Data)
	}
	return ar.outputList(cmd, data)
}

// outputList outputs a sorted list of containers in format set via --output flag.
func (ar *ActionRunner) outputList(cmd *cobra.Command, data []*models.Image) error {
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
		Transforms: output.TransformMap{
			"Description": output.Truncate50Transform,
			"Size":        output.HumanByteTransform,
		},
	}

	out, err := output.Format(format, dataGeneric, nil, formatSpec)
	if err == nil {
		cmd.Print(out)
	}
	return err
}

// Describe implements the CLI describe action for an image.
func (ar *ActionRunner) Describe(cmd *cobra.Command, args []string) error {
	params := image.NewGetImageParams().WithRef(args[0])

	resp, err := ar.clients.LibraryClient.Image.GetImage(params, ar.authInfo)
	if err != nil {
		return err
	}

	header := "Image: " + args[0]
	data := resp.GetPayload().Data
	out, err := output.DescribeStruct(header, data)
	if err != nil {
		return err
	}
	cmd.Print(out)
	return nil
}

// Delete implements the CLI delete action for an image.
func (ar *ActionRunner) Delete(*cobra.Command, []string) error {
	sylog.Errorf("Not yet implemented")
	return nil
}
