// Copyright (c) 2021-2026, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package build

import (
	"fmt"
	"time"

	"github.com/go-openapi/runtime"
	"github.com/spf13/cobra"
	"github.com/sylabs/singularity/v4/enterprise-plugin/internal/pkg/api"
	"github.com/sylabs/singularity/v4/enterprise-plugin/internal/pkg/output"
	"github.com/sylabs/singularity/v4/enterprise-plugin/internal/pkg/user"
	bsrvops "github.com/sylabs/singularity/v4/enterprise-plugin/pkg/buildserver/client/operations"
	"github.com/sylabs/singularity/v4/enterprise-plugin/pkg/buildserver/models"
	"github.com/sylabs/singularity/v4/pkg/sylog"
)

const (
	// CLITypeShort is the string representing this type in CLI commands.
	CLIType = "build"
	// CLITypePlural is the plural form of the string representing this type in CLI commands.
	CLITypePlural = "builds"
	// CLITypeShort is the shortened form for this type in CLI commands.
	CLITypeShort = "bld"
)

var (
	// listShortFields is the list of build metadata fields to display in short output formats.
	listShortFields = []string{"ID", "CreatedBy", "IsComplete", "SubmitTime", "CompleteTime", "LibraryRef"}
	// listShortHeaders is the list of build metadata field headers to display in short output formats.
	listShortHeaders = []string{"ID", "CreatedBy", "IsComplete", "SubmitTime", "CompleteTime", "LibraryRef"}
	// listLongFields is the list of build metadata fields to Display in long output formats.
	listLongFields = []string{"ID", "CreatedBy", "IsComplete", "SubmitTime", "StartTime", "CompleteTime", "LibraryRef", "LibraryURL", "ImageSize", "ImageChecksum"}
	// listLongHeaders is the list of build metadata field headers to display in long output formats.
	listLongHeaders = []string{"ID", "CreatedBy", "IsComplete", "SubmitTime", "StartTime", "CompleteTime", "LibraryRef", "LibraryURL", "ImageSize", "ImageChecksum"}
)

// ActionRunner implements CRUD actions on a build.
type ActionRunner struct {
	clients  *api.Clients
	userID   string
	authInfo runtime.ClientAuthInfoWriter
}

// NewActionRunner returns a build.ActionRunner that will use the specified client and authInfo.
func NewActionRunner(clients *api.Clients, user string, authInfo runtime.ClientAuthInfoWriter) *ActionRunner {
	return &ActionRunner{clients, user, authInfo}
}

// Get implements the CLI get action for a build.
func (ar *ActionRunner) Get(cmd *cobra.Command, args []string) error {
	// No args = list of all builds with filtering
	if len(args) == 0 {
		return ar.getList(cmd)
	}
	// Args = list of specified builds
	return ar.getRefs(cmd, args)
}

// getList produces a list of remote builds with filtering.
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
		sylog.Infof("Showing results for user: %s, use --all or --user to see others.", uid)
	}

	from, err := cmd.Flags().GetString("from")
	if err != nil {
		return err
	}
	to, err := cmd.Flags().GetString("to")
	if err != nil {
		return err
	}
	if to == "" && from == "" {
		from = time.Now().AddDate(0, 0, -1).Format(time.RFC3339)
		sylog.Infof("Showing results for last hour, use --to / --from to see more.")
	}

	params := bsrvops.NewGetBuildsParams().
		WithUser(&uid).
		WithFrom(&from).
		WithTo(&to)

	resp, err := ar.clients.BuildSrvClient.Operations.GetBuilds(params, ar.authInfo)
	if err != nil {
		return err
	}
	data := resp.GetPayload().Data

	return ar.outputList(cmd, data)
}

// getRefs produces a list of specified builds.
func (ar *ActionRunner) getRefs(cmd *cobra.Command, args []string) error {
	data := []*models.BuildMetadata{}
	for _, ref := range args {
		params := bsrvops.NewGetBuildParams().WithID(ref)
		resp, err := ar.clients.BuildSrvClient.Operations.GetBuild(params, ar.authInfo)
		if err != nil {
			sylog.Warningf("while fetching %s: %v", ref, err)
			continue
		}
		data = append(data, resp.GetPayload().Data)
	}
	return ar.outputList(cmd, data)
}

// outputList outputs a sorted list of builds in format set via --output flag.
func (ar *ActionRunner) outputList(cmd *cobra.Command, data []*models.BuildMetadata) error {
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
		Transforms: output.TransformMap{
			"CreatedBy":    output.UsernameTransform,
			"SubmitTime":   output.TimezoneTransform,
			"StartTime":    output.TimezoneTransform,
			"CompleteTime": output.TimezoneTransform,
		},
		Clients: ar.clients,
	}

	out, err := output.Format(format, dataGeneric, nil, formatSpec)
	if err == nil {
		cmd.Print(out)
	}
	return err
}

// Describe implements the CLI describe action for a build.
func (ar *ActionRunner) Describe(cmd *cobra.Command, args []string) error {
	params := bsrvops.NewGetBuildParams().WithID(args[0])

	resp, err := ar.clients.BuildSrvClient.Operations.GetBuild(params, ar.authInfo)
	if err != nil {
		return err
	}

	header := "Build: " + args[0]
	data := resp.GetPayload().Data
	out, err := output.DescribeStruct(header, data)
	if err != nil {
		return err
	}
	cmd.Print(out)
	return nil
}

// Delete implements the CLI delete action for a build.
func (ar *ActionRunner) Delete(*cobra.Command, []string) error {
	sylog.Errorf("Not yet implemented")
	return nil
}
