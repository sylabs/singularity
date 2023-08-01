// Copyright (c) 2019-2022, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package main

import (
	"log"

	"github.com/sylabs/singularity/v4/internal/pkg/cgroups"
	pluginapi "github.com/sylabs/singularity/v4/pkg/plugin"
	clicallback "github.com/sylabs/singularity/v4/pkg/plugin/callback/cli"
	"github.com/sylabs/singularity/v4/pkg/runtime/engine/config"
	singularity "github.com/sylabs/singularity/v4/pkg/runtime/engine/singularity/config"
	"github.com/sylabs/singularity/v4/pkg/sylog"
)

// Plugin is the only variable which a plugin MUST export.
// This symbol is accessed by the plugin framework to initialize the plugin
var Plugin = pluginapi.Plugin{
	Manifest: pluginapi.Manifest{
		Name:        "github.com/sylabs/singularity/config-example-plugin",
		Author:      "Sylabs Team",
		Version:     "0.1.0",
		Description: "This is a short example config plugin for Singularity",
	},
	Callbacks: []pluginapi.Callback{
		(clicallback.SingularityEngineConfig)(callbackCgroups),
	},
}

func callbackCgroups(common *config.Common) {
	c, ok := common.EngineConfig.(*singularity.EngineConfig)
	if !ok {
		log.Printf("Unexpected engine config")
		return
	}
	cfg := cgroups.Config{
		Devices: nil,
		Memory: &cgroups.LinuxMemory{
			Limit: &[]int64{1024 * 1}[0],
		},
	}

	data, err := cfg.MarshalJSON()
	if err != nil {
		sylog.Errorf("While Marshalling cgroups config to JSON: %s", err)
		return
	}
	sylog.Infof("Overriding cgroups config")
	c.SetCgroupsJSON(data)
}
