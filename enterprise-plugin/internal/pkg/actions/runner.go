// Copyright (c) 2021-2026, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package actions

import (
	"fmt"

	"github.com/go-openapi/runtime"
	"github.com/spf13/cobra"
	"github.com/sylabs/singularity/v4/enterprise-plugin/internal/pkg/actions/build"
	"github.com/sylabs/singularity/v4/enterprise-plugin/internal/pkg/actions/buildagent"
	"github.com/sylabs/singularity/v4/enterprise-plugin/internal/pkg/actions/collection"
	"github.com/sylabs/singularity/v4/enterprise-plugin/internal/pkg/actions/common"
	"github.com/sylabs/singularity/v4/enterprise-plugin/internal/pkg/actions/container"
	"github.com/sylabs/singularity/v4/enterprise-plugin/internal/pkg/actions/entity"
	"github.com/sylabs/singularity/v4/enterprise-plugin/internal/pkg/actions/image"
	"github.com/sylabs/singularity/v4/enterprise-plugin/internal/pkg/actions/key"
	"github.com/sylabs/singularity/v4/enterprise-plugin/internal/pkg/actions/token"
	"github.com/sylabs/singularity/v4/enterprise-plugin/internal/pkg/actions/user"
	"github.com/sylabs/singularity/v4/enterprise-plugin/internal/pkg/api"
)

var (
	// CLITypes is the list of types that can be specified for actions on the CLI.
	CLITypes = []string{
		build.CLIType,
		buildagent.CLIType,
		entity.CLIType,
		entity.CLITypeProject,
		collection.CLIType,
		container.CLIType,
		container.CLITypeRepo,
		image.CLIType,
		key.CLIType,
		token.CLIType,
		user.CLIType,
	}
	// CLITypesPlural is the list of pluralized types that can be specified for actions on the CLI.
	CLITypesPlural = []string{
		build.CLITypePlural,
		buildagent.CLITypePlural,
		entity.CLITypePlural,
		entity.CLITypeProjectPlural,
		collection.CLITypePlural,
		container.CLITypePlural,
		container.CLITypeRepoPlural,
		image.CLITypePlural,
		key.CLITypePlural,
		token.CLITypePlural,
		user.CLITypePlural,
	}
	// CLITypesShort is the list of short form types that can be specified for actions on the CLI.
	CLITypesShort = []string{
		build.CLITypeShort,
		buildagent.CLITypeShort,
		entity.CLITypeShort,
		entity.CLITypeProjectShort,
		collection.CLITypeShort,
		container.CLITypeShort,
		container.CLITypeRepoShort,
		image.CLITypeShort,
		key.CLITypeShort,
		token.CLITypeShort,
		user.CLITypeShort,
	}
)

// Runner can perform the basic CLI actions for a specific type.
type Runner interface {
	Get(cmd *cobra.Command, args []string) error
	Describe(cmd *cobra.Command, args []string) error
	Delete(cmd *cobra.Command, args []string) error
}

// NewRunner returns an implementation of the ActionRunner interface on the specified cliType.
func NewRunner(cliType string, clients *api.Clients, userID string, authInfo runtime.ClientAuthInfoWriter) (ar Runner, err error) {
	switch cliType {
	case build.CLIType, build.CLITypePlural, build.CLITypeShort:
		return build.NewActionRunner(clients, userID, authInfo), nil
	case buildagent.CLIType, buildagent.CLITypePlural, buildagent.CLITypeShort:
		return buildagent.NewActionRunner(clients, userID, authInfo), nil
	case entity.CLIType, entity.CLITypePlural, entity.CLITypeShort,
		entity.CLITypeProject, entity.CLITypeProjectPlural, entity.CLITypeProjectShort:
		return entity.NewActionRunner(clients, userID, authInfo), nil
	case collection.CLIType, collection.CLITypePlural, collection.CLITypeShort:
		return collection.NewActionRunner(clients, userID, authInfo), nil
	case container.CLIType, container.CLITypePlural, container.CLITypeShort,
		container.CLITypeRepo, container.CLITypeRepoPlural, container.CLITypeRepoShort:
		return container.NewActionRunner(clients, userID, authInfo), nil
	case image.CLIType, image.CLITypePlural, image.CLITypeShort:
		return image.NewActionRunner(clients, userID, authInfo), nil
	case key.CLIType, key.CLITypePlural:
		return key.NewActionRunner(clients, userID, authInfo), nil
	case token.CLIType, token.CLITypePlural, token.CLITypeShort:
		return token.NewActionRunner(clients, userID, authInfo), nil
	case user.CLIType, user.CLITypePlural, user.CLITypeShort:
		return user.NewActionRunner(clients, userID, authInfo), nil
	}
	return nil, fmt.Errorf("%w %q", common.ErrUnknownType, cliType)
}
