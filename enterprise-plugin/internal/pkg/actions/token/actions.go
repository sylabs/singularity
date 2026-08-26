// Copyright (c) 2021-2026, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package token

import (
	"fmt"
	"strings"

	"github.com/blang/semver/v4"
	"github.com/go-openapi/runtime"
	"github.com/spf13/cobra"
	"github.com/sylabs/singularity/v4/enterprise-plugin/internal/pkg/api"
	"github.com/sylabs/singularity/v4/enterprise-plugin/internal/pkg/output"
	"github.com/sylabs/singularity/v4/enterprise-plugin/internal/pkg/user"
	"github.com/sylabs/singularity/v4/enterprise-plugin/pkg/consentservice/client/users"
	tokops "github.com/sylabs/singularity/v4/enterprise-plugin/pkg/tokenservice/client/operations"
	"github.com/sylabs/singularity/v4/enterprise-plugin/pkg/tokenservice/models"
	"github.com/sylabs/singularity/v4/pkg/sylog"
)

const (
	// CLITypeShort is the string representing this type in CLI commands.
	CLIType = "token"
	// CLITypePlural is the plural form of the string representing this type in CLI commands.
	CLITypePlural = "tokens"
	// CLITypeShort is the shortened form for this type in CLI commands.
	CLITypeShort = "tok"
)

var (
	// MinServiceVersion is the minimum supported version of the token-service.
	// Older versions do not alow auth with a CLI token.
	MinServiceVersion = semver.MustParse("1.5.7-0")
	// ErrInsufficientVersion is reported if the token service version is not new enough, or cannot be obtained.
	ErrInsufficientVersion = fmt.Errorf("token-service version too old or unknown - %v is required for token functionality", MinServiceVersion)
	// listShortFields is the list of token fields to display in short output formats.
	listShortFields = []string{"ID", "Subject", "Label", "Expiration", "IsRevoked"}
	// listShortHeaders is the list of token field headers to display in short output formats.
	listShortHeaders = listShortFields
	// listLongFields is the list of token fields to Display in long output formats.
	listLongFields = []string{"ID", "Subject", "Label", "IssuedAt", "Expiration", "IsRevoked", "RevokedAt"}
	// listLongHeaders is the list of token field headers to display in long output formats.
	listLongHeaders = listLongFields
	// autoTokenPrefixes is the list of prefixes for remote build related tokens that should not be displayed unless requested.
	autoTokenPrefixes = []string{
		"Token for user",
		"Ephemeral token for user",
		"Remote Build",
	}
)

// ActionRunner implements CRUD actions on a token.
type ActionRunner struct {
	clients  *api.Clients
	userID   string
	authInfo runtime.ClientAuthInfoWriter
}

// NewActionRunner returns a token.ActionRunner that will use the specified client and authInfo.
func NewActionRunner(clients *api.Clients, user string, authInfo runtime.ClientAuthInfoWriter) *ActionRunner {
	// Check version...
	params := tokops.NewGetVersionParams()
	resp, err := clients.TokenClient.Operations.GetVersion(params)
	// TODO - handle this with nice messages once the mock is separated per service, and returns useful version info
	if err != nil {
		sylog.Warningf("%v", ErrInsufficientVersion)
	} else {
		ver := resp.GetPayload().Data.Version
		curSemver, err := semver.ParseTolerant(ver)
		if err != nil {
			sylog.Warningf("%v", ErrInsufficientVersion)
		}
		if curSemver.Compare(MinServiceVersion) < 0 {
			sylog.Warningf("%v", ErrInsufficientVersion)
		}
	}

	return &ActionRunner{clients, user, authInfo}
}

// Get implements the CLI get action for a token.
func (ar *ActionRunner) Get(cmd *cobra.Command, args []string) error {
	// No args = list of all builds with filtering
	if len(args) == 0 {
		return ar.getList(cmd)
	}
	// Args = list of specified builds
	return ar.getRefs(cmd, args)
}

// getList produces a list of tokens with filtering.
func (ar *ActionRunner) getList(cmd *cobra.Command) error {
	uid, err := cmd.Flags().GetString("user")
	if err != nil {
		return err
	}
	showDisabled, err := cmd.Flags().GetBool("show-disabled-tokens")
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

	if uid != "" {
		sylog.Infof("Showing results for user: %s, use --user to see others.", uid)
	}

	params := tokops.NewGetTokensParams().WithSub(&uid)

	if showDisabled {
		trueStr := "true"
		params = params.WithAll(&trueStr)
	}

	resp, err := ar.clients.TokenClient.Operations.GetTokens(params, ar.authInfo)
	if err != nil {
		return err
	}
	data := resp.GetPayload()

	return ar.outputList(cmd, data)
}

// getRefs produces a list of specified tokens.
func (ar *ActionRunner) getRefs(cmd *cobra.Command, args []string) error {
	data := []*models.TokenDetails{}
	for _, ref := range args {
		params := tokops.NewGetTokenParams().WithJti(ref)
		resp, err := ar.clients.TokenClient.Operations.GetToken(params, ar.authInfo)
		if err != nil {
			sylog.Warningf("while fetching %s: %v", ref, err)
			continue
		}
		data = append(data, resp.GetPayload())
	}
	return ar.outputList(cmd, data)
}

// outputList outputs a sorted list of tokens in format set via --output flag.
func (ar *ActionRunner) outputList(cmd *cobra.Command, data []*models.TokenDetails) error {
	format, err := cmd.Flags().GetString("output")
	if err != nil {
		return err
	}

	showAuto, err := cmd.Flags().GetBool("show-auto-tokens")
	if err != nil {
		return err
	}

	dataGeneric := make([]any, 0, len(data))
	for _, val := range data {
		skip := false
		if !showAuto {
			for _, ap := range autoTokenPrefixes {
				if strings.HasPrefix(val.Label, ap) {
					skip = true
					break
				}
			}
			if skip {
				continue
			}
		}

		dataGeneric = append(dataGeneric, val)
	}

	formatSpec := output.FormatSpec{
		ShortFields:  listShortFields,
		ShortHeaders: listShortHeaders,
		LongFields:   listLongFields,
		LongHeaders:  listLongHeaders,
		Transforms: output.TransformMap{
			"Subject":    output.UsernameTransform,
			"Label":      output.Truncate50Transform,
			"IssuedAt":   output.TimezoneTransform,
			"Expiration": output.TimezoneTransform,
			"RevokedAt":  output.TimezoneTransform,
		},
		Clients: ar.clients,
	}

	out, err := output.Format(format, dataGeneric, nil, formatSpec)
	if err == nil {
		cmd.Print(out)
	}
	return err
}

// Describe implements the CLI describe action for a token.
func (ar *ActionRunner) Describe(cmd *cobra.Command, args []string) error {
	params := tokops.NewGetTokenParams().WithJti(args[0])

	resp, err := ar.clients.TokenClient.Operations.GetToken(params, ar.authInfo)
	if err != nil {
		return err
	}

	header := "Token: " + args[0]
	data := resp.GetPayload()
	out, err := output.DescribeStruct(header, data)
	if err != nil {
		return err
	}
	cmd.Print(out)
	return nil
}

// Delete implements the CLI delete action for a token.
func (ar *ActionRunner) Delete(cmd *cobra.Command, args []string) error {
	uid, err := cmd.Flags().GetString("user")
	if err != nil {
		return err
	}
	// Resolve it to an ID if it's not empty or  "all"
	if uid != "" && uid != "all" {
		uid, err = user.UsernameToID(uid, ar.clients)
		if err != nil {
			return fmt.Errorf("while looking up user %s: %v", uid, err)
		}
	}

	all, err := cmd.Flags().GetBool("all")
	if err != nil {
		return err
	}

	if all && uid == "" {
		return fmt.Errorf("a --user must be specfied to revoke --all of their tokens")
	}

	if !all && len(args) < 1 {
		return fmt.Errorf("a token ID must be specified when not revoking --all tokens for a --user")
	}

	if all && uid == "all" {
		return ar.revokeAllTokens()
	}
	if all {
		return ar.revokeAllUserTokens(uid)
	}
	return ar.revokeTokens(args)
}

func (ar *ActionRunner) revokeTokens(tokens []string) error {
	for _, t := range tokens {
		sylog.Infof("Revoking token %s", t)
		params := tokops.NewRevokeTokenParams().WithJti(t)
		_, err := ar.clients.TokenClient.Operations.RevokeToken(params, ar.authInfo)
		if err != nil {
			return err
		}
	}
	return nil
}

func (ar *ActionRunner) revokeAllUserTokens(user string) error {
	sylog.Infof("Revoking all tokens for user %s", user)
	params := tokops.NewRevokeTokenParams().WithSub(&user).WithJti("all")
	_, err := ar.clients.TokenClient.Operations.RevokeToken(params, ar.authInfo)
	return err
}

func (ar *ActionRunner) revokeAllTokens() error {
	params := users.NewListUsersParams()
	resp, err := ar.clients.ConsentClient.Users.ListUsers(params, ar.authInfo)
	if err != nil {
		return err
	}
	data := resp.GetPayload().Data
	for _, u := range data {
		if u.ID == ar.userID {
			sylog.Infof("Not revoking all tokens for user %s, as they are running this command", u.ID)
			continue
		}
		err := ar.revokeAllUserTokens(u.ID)
		if err != nil {
			return err
		}
	}
	return nil
}
