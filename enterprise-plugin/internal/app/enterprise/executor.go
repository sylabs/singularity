// Copyright (c) 2020-2026 Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package enterprise

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/blang/semver/v4"
	"github.com/go-openapi/runtime"
	"github.com/spf13/cobra"
	"github.com/sylabs/singularity/v4/enterprise-plugin/internal/pkg/api"
	"github.com/sylabs/singularity/v4/enterprise-plugin/internal/pkg/config"
)

var errUnrecognizedGitState = errors.New("unrecognized git state")

// Executor represents an executor instance.
type Executor struct {
	v         semver.Version
	gitCommit string
	builtAt   time.Time
	builtBy   string
	gitDirty  bool

	pluginMode  bool
	authInfo    runtime.ClientAuthInfoWriter
	serviceURIs *config.ServiceURIs
	clients     *api.Clients
	// rootOptions (flag vars) are currently kept as per-executor state to allow parallel tests.
	// Non root command options are now flags only, no vars.
	rootOpt rootOptions
	rootCmd *cobra.Command
}

// ExecutorOpt configures e.
type ExecutorOpt func(e *Executor) error

// OptExecutorVersion sets the semantic version of the Executor.
func OptExecutorVersion(version string) ExecutorOpt {
	return func(e *Executor) error {
		v, err := semver.Parse(version)
		if err != nil {
			return fmt.Errorf("failed to parse version '%v': %w", version, err)
		}
		e.v = v
		return nil
	}
}

// OptExecutorGitCommit sets the git commit the Executor was built from.
func OptExecutorGitCommit(commit string) ExecutorOpt {
	return func(e *Executor) error {
		e.gitCommit = commit
		return nil
	}
}

// OptExecutorGitState sets the state of the working tree when the Executor was built. The state is
// expected to be "clean" (working tree matches HEAD) or "dirty" (the working tree has local
// modifications).
func OptExecutorGitState(state string) ExecutorOpt {
	return func(e *Executor) error {
		switch state {
		case "clean":
			e.gitDirty = false
		case "dirty":
			e.gitDirty = true
		default:
			return fmt.Errorf("%w '%v'", errUnrecognizedGitState, state)
		}
		return nil
	}
}

// OptExecutorBuiltAt sets the date (in RFC3339 format) the Executor was built.
func OptExecutorBuiltAt(date string) ExecutorOpt {
	return func(e *Executor) error {
		t, err := time.Parse(time.RFC3339, date)
		if err != nil {
			return fmt.Errorf("failed to parse creation date: %w", err)
		}
		e.builtAt = t
		return nil
	}
}

// OptExecutorBuiltBy sets the entity that built the Executor.
func OptExecutorBuiltBy(entity string) ExecutorOpt {
	return func(e *Executor) error {
		e.builtBy = entity
		return nil
	}
}

// OptExecutorPluginMode instructs the executor to initialize for use as a Singularity plugin.
func OptExecutorPluginMode(pluginMode bool) ExecutorOpt {
	return func(e *Executor) error {
		e.pluginMode = pluginMode
		return nil
	}
}

// NewExecutor returns an Executor, configured according to opts.
//
// To provide build information, see OptExecutorBuiltAt, OptExecutorBuiltBy, OptExecutorGitCommit,
// OptExecutorGitState, and/or OptExecutorVersion.
func NewExecutor(opts ...ExecutorOpt) (*Executor, error) {
	e := Executor{}

	for _, opt := range opts {
		if err := opt(&e); err != nil {
			return nil, err
		}
	}

	e.initRoot()

	return &e, nil
}

// Execute executes the task specified by e.
func (e *Executor) Execute(ctx context.Context) error {
	return e.rootCmd.ExecuteContext(ctx)
}

// versionString returns the semantic version of e, or "unknown".
func (e *Executor) versionString() string {
	if e.v.Equals(semver.Version{}) {
		return "unknown"
	}
	return e.v.String()
}

// GetRootCmd returns the root command so it can be passed to Singularity for plugin use.
func (e *Executor) GetRootCmd() *cobra.Command {
	return e.rootCmd
}
