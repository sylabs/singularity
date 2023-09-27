// Copyright (c) 2023, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package ociauth

import (
	"context"
	"fmt"
	"net/http"
	"os"

	"github.com/docker/cli/cli/config"
	"github.com/docker/cli/cli/config/configfile"
	"github.com/docker/cli/cli/config/types"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote/transport"
	"github.com/sylabs/singularity/v4/internal/pkg/util/fs"
	"github.com/sylabs/singularity/v4/pkg/syfs"
	"github.com/sylabs/singularity/v4/pkg/sylog"
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

func LoginAndStore(registry, username, password string, insecure bool, reqAuthFile string) error {
	if err := checkOCILogin(registry, username, password, insecure); err != nil {
		return err
	}

	cf, err := ConfigFileFromPath(ChooseAuthFile(reqAuthFile))
	if err != nil {
		return fmt.Errorf("while loading existing OCI registry credentials from %q: %w", ChooseAuthFile(reqAuthFile), err)
	}

	creds := cf.GetCredentialsStore(registry)

	// DockerHub requires special logic for historical reasons.
	serverAddress := registry
	if serverAddress == name.DefaultRegistry {
		serverAddress = authn.DefaultAuthKey
	}

	if err := creds.Store(types.AuthConfig{
		Username:      username,
		Password:      password,
		ServerAddress: serverAddress,
	}); err != nil {
		return fmt.Errorf("while trying to store new credentials: %w", err)
	}

	sylog.Infof("Token stored in %s", cf.Filename)

	return nil
}

func checkOCILogin(regName string, username, password string, insecure bool) error {
	regOpts := []name.Option{}
	if insecure {
		regOpts = []name.Option{name.Insecure}
	}
	reg, err := name.NewRegistry(regName, regOpts...)
	if err != nil {
		return err
	}

	auth := authn.FromConfig(authn.AuthConfig{
		Username: username,
		Password: password,
	})

	// Creating a new transport pings the registry and works through auth flow.
	_, err = transport.NewWithContext(context.TODO(), reg, auth, http.DefaultTransport, nil)
	if err != nil {
		return err
	}

	return nil
}
