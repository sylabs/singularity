// Copyright (c) 2018-2021, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package main

import (
	"github.com/sylabs/singularity/v4/cmd/internal/cli"
	"github.com/sylabs/singularity/v4/internal/pkg/buildcfg"
	useragent "github.com/sylabs/singularity/v4/pkg/util/user-agent"
)

func main() {
	useragent.InitValue(buildcfg.PACKAGE_NAME, buildcfg.PACKAGE_VERSION)

	// In cmd/internal/cli/singularity.go
	cli.ExecuteSingularity()
}
