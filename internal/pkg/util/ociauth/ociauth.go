// Copyright (c) 2023, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package ociauth

import (
	"os"

	"github.com/docker/cli/cli/config"
	"github.com/docker/cli/cli/config/configfile"
	"github.com/sylabs/singularity/v4/internal/pkg/util/fs"
	"github.com/sylabs/singularity/v4/pkg/syfs"
)

// ConfigFileFromPath creates a configfile.Configfile object (part of docker/cli
// API) associated with the auth file at path.
func ConfigFileFromPath(path string) (*configfile.ConfigFile, error) {
	cf := configfile.New(path)
	if fs.IsFile(path) {
		f, err := os.Open(path)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		cf, err = config.LoadFromReader(f)
		if err != nil {
			return nil, err
		}
		cf.Filename = path
	}

	return cf, nil
}

// ChooseAuthFile returns reqAuthFile if it is not empty, or else the default
// location of the OCI registry auth file.
func ChooseAuthFile(reqAuthFile string) string {
	if reqAuthFile != "" {
		return reqAuthFile
	}

	return syfs.DockerConf()
}
