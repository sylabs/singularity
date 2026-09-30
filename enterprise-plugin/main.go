// Copyright (c) 2020-2026, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

// Package main implements a Singularity plugin for Enterprise management.
// Plugin-specific code should be kept minimal, as we also build cmd/enterprise,
// which is a stand-alone CLI for the same functionality.
package main

import (
	"fmt"

	"github.com/sylabs/singularity/v4/enterprise-plugin/internal/app/enterprise"
	"github.com/sylabs/singularity/v4/internal/pkg/buildcfg"
	"github.com/sylabs/singularity/v4/pkg/cmdline"
	pluginapi "github.com/sylabs/singularity/v4/pkg/plugin"
	clicallback "github.com/sylabs/singularity/v4/pkg/plugin/callback/cli"
	"github.com/sylabs/singularity/v4/pkg/sylog"
)

var (
	pluginName        = "sylabs.io/enterprise-plugin"
	pluginAuthor      = "Sylabs"
	pluginVersion     = "1.0.0"
	pluginDescription = "A Singularity Enterprise management plugin"
)

// Plugin is the only variable which a plugin MUST export.
// This symbol is accessed by the plugin framework to initialize the plugin.
// Appears unused to the linter, but is consumed by Singularity when plugin loaded.
var Plugin = pluginapi.Plugin{
	Manifest: pluginapi.Manifest{
		Name:        pluginName,
		Author:      pluginAuthor,
		Version:     pluginVersion,
		Description: pluginDescription,
	},
	Callbacks: []pluginapi.Callback{
		clicallback.Command(callBackEnterprise),
	},
}

// callBackEnterprise registers the `enterprise` sub-command with Singularity
// Note that we are using the standard cobra registration of sub-commands for
// `enterprise` itself in `Executor.initRoot()`, prior to registering with the
// Singularity CLI. We are avoiding calling out to the customized Singularity
// command manager for this, in an attempt to use fewer parts of the Singularity
// code base when `enterprise` is built as a stand-alone CLI tool.
func callBackEnterprise(manager *cmdline.CommandManager) {
	opts := make([]enterprise.ExecutorOpt, 0, 3)
	opts = append(opts, enterprise.OptExecutorPluginMode(true))
	opts = append(opts, enterprise.OptExecutorVersion(pluginVersion))
	opts = append(opts, enterprise.OptExecutorBuiltBy(
		fmt.Sprintf("%s %s", buildcfg.PACKAGE_NAME, buildcfg.PACKAGE_VERSION)))

	e, err := enterprise.NewExecutor(opts...)
	if err != nil {
		sylog.Errorf("%s: %v", pluginName, err)
		return
	}

	manager.RegisterCmd(e.GetRootCmd())
}
