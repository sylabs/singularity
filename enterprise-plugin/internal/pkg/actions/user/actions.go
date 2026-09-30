// Copyright (c) 2021-2026, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package user

import (
	"fmt"

	"github.com/go-openapi/runtime"
	"github.com/spf13/cobra"
	"github.com/sylabs/singularity/v4/enterprise-plugin/internal/pkg/api"
	"github.com/sylabs/singularity/v4/enterprise-plugin/internal/pkg/output"
	"github.com/sylabs/singularity/v4/enterprise-plugin/internal/pkg/user"
	"github.com/sylabs/singularity/v4/enterprise-plugin/pkg/consentservice/client/users"
	"github.com/sylabs/singularity/v4/enterprise-plugin/pkg/consentservice/models"
	"github.com/sylabs/singularity/v4/pkg/sylog"
)

const (
	// CLITypeShort is the string representing this type in CLI commands.
	CLIType = "user"
	// CLITypePlural is the plural form of the string representing this type in CLI commands.
	CLITypePlural = "users"
	// CLITypeShort is the shortened form for this type in CLI commands.
	CLITypeShort = "usr"
)

var (
	// listShortFields is the list of user metadata fields to display in short output formats.
	listShortFields = []string{"ID", "Username", "Email", "Provider", "Active", "Groups", "AllRoles"}
	// listShortHeaders is the list of user metadata field headers to display in short output formats.
	listShortHeaders = listShortFields
	// listLongFields is the list of user metadata fields to Display in long output formats.
	listLongFields = []string{"ID", "Username", "Email", "Provider", "ProviderID", "Active", "Groups", "Roles", "AllRoles", "Quota.MaxBuildTime", "Quota.MaxBuildTimeCumulative", "Quota.MaxLibrarySize", "CreatedAt", "DeactivatedAt"}
	// listLongHeaders is the list of user metadata field headers to display in long output formats.
	listLongHeaders = listLongFields
)

// ActionRunner implements CRUD actions on a user.
type ActionRunner struct {
	clients  *api.Clients
	userID   string
	authInfo runtime.ClientAuthInfoWriter
}

// NewActionRunner returns a user.ActionRunner that will use the specified client and authInfo.
func NewActionRunner(clients *api.Clients, user string, authInfo runtime.ClientAuthInfoWriter) *ActionRunner {
	return &ActionRunner{clients, user, authInfo}
}

// Get implements the CLI get action for a user.
func (ar *ActionRunner) Get(cmd *cobra.Command, args []string) error {
	// No args = list of all users
	if len(args) == 0 {
		return ar.getList(cmd)
	}
	// Args = list of specified users
	return ar.getRefs(cmd, args)
}

func (ar *ActionRunner) getList(cmd *cobra.Command) error {
	params := users.NewListUsersParams()
	resp, err := ar.clients.ConsentClient.Users.ListUsers(params, ar.authInfo)
	if err != nil {
		return err
	}
	data := resp.GetPayload().Data
	return ar.outputList(cmd, data)
}

// getRefs produces a list of specified users.
func (ar *ActionRunner) getRefs(cmd *cobra.Command, args []string) (err error) {
	data := []*models.User{}
	for _, ref := range args {
		ref, err = user.UsernameToID(ref, ar.clients)
		if err != nil {
			return fmt.Errorf("while looking up user: %v", err)
		}
		params := users.NewGetUserParams().WithUserID(ref)
		resp, err := ar.clients.ConsentClient.Users.GetUser(params, ar.authInfo)
		if err != nil {
			sylog.Warningf("while fetching %s: %v", ref, err)
			continue
		}
		data = append(data, resp.GetPayload().Data)
	}
	return ar.outputList(cmd, data)
}

// outputList outputs a sorted list of users in format set via --output flag.
func (ar *ActionRunner) outputList(cmd *cobra.Command, data []*models.User) error {
	format, err := cmd.Flags().GetString("output")
	if err != nil {
		return err
	}
	dataGeneric := make([]any, len(data))
	quotaBuild := make([]string, len(data))
	quotaBuildCumul := make([]string, len(data))
	quotaLibrary := make([]string, len(data))
	for idx, val := range data {
		dataGeneric[idx] = val
		quotaBuild[idx] = fmt.Sprintf("%0.0f", val.Quotas.MaxBuildTime)
		quotaBuildCumul[idx] = fmt.Sprintf("%0.0f", val.Quotas.MaxBuildTimeCumulative)
		quotaLibrary[idx] = output.HumanByteTransform(fmt.Sprintf("%0.0f", val.Quotas.MaxLibrarySize*1024), nil)
	}

	extra := map[string][]string{
		"Quota.MaxBuildTime":           quotaBuild,
		"Quota.MaxBuildTimeCumulative": quotaBuildCumul,
		"Quota.MaxLibrarySize":         quotaLibrary,
	}

	formatSpec := output.FormatSpec{
		ShortFields:  listShortFields,
		ShortHeaders: listShortHeaders,
		LongFields:   listLongFields,
		LongHeaders:  listLongHeaders,
		Clients:      ar.clients,
		Transforms: map[string]output.TransformFunc{
			"CreatedAt":     output.TimezoneTransform,
			"DeactivatedAt": output.TimezoneTransform,
		},
	}

	out, err := output.Format(format, dataGeneric, &extra, formatSpec)
	if err == nil {
		cmd.Print(out)
	}
	return err
}

// Describe implements the CLI describe action for a user.
func (ar *ActionRunner) Describe(cmd *cobra.Command, args []string) error {
	uid, err := user.UsernameToID(args[0], ar.clients)
	if err != nil {
		return fmt.Errorf("while looking up user: %v", err)
	}

	params := users.NewGetUserParams().WithUserID(uid)

	resp, err := ar.clients.ConsentClient.Users.GetUser(params, ar.authInfo)
	if err != nil {
		return err
	}

	header := "User: " + args[0]
	data := resp.GetPayload().Data
	out, err := output.DescribeStruct(header, data)
	if err != nil {
		return err
	}
	cmd.Print(out)
	return nil
}

// Delete implements the CLI delete action for a user.
func (ar *ActionRunner) Delete(*cobra.Command, []string) error {
	sylog.Errorf("Not yet implemented")
	return nil
}
