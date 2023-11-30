// Copyright (c) 2023, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package main

import (
	"context"
	"os"

	bkdaemon "github.com/sylabs/singularity/v4/internal/pkg/build/buildkit/daemon"
	"github.com/sylabs/singularity/v4/internal/pkg/buildcfg"
	"github.com/sylabs/singularity/v4/pkg/sylog"
	"github.com/sylabs/singularity/v4/pkg/util/singularityconf"
)

func main() {
	if len(os.Args) < 2 || len(os.Args) > 3 {
		sylog.Fatalf("%s: usage: %s <socket-uri> [architecture]", bkdaemon.DaemonName, os.Args[0])
	}

	bkSocket := os.Args[1]
	bkArch := ""
	if len(os.Args) == 3 {
		bkArch = os.Args[2]
	}

	sylog.Debugf("%s: parsing configuration file %s", bkdaemon.DaemonName, buildcfg.SINGULARITY_CONF_FILE)
	config, err := singularityconf.Parse(buildcfg.SINGULARITY_CONF_FILE)
	if err != nil {
		sylog.Fatalf("%s: couldn't parse configuration file %s: %v", bkdaemon.DaemonName, buildcfg.SINGULARITY_CONF_FILE, err)
	}
	singularityconf.SetCurrentConfig(config)

	daemonOpts := &bkdaemon.Opts{
		ReqArch: bkArch,
	}
	if err := bkdaemon.Run(context.Background(), daemonOpts, bkSocket); err != nil {
		sylog.Fatalf("%s: %v", bkdaemon.DaemonName, err)
	}
}
