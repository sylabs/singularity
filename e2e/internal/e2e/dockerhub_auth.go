// Copyright (c) 2023, Sylabs Inc. All rights reserved.
// Copyright (c) 2020, Control Command Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package e2e

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/docker/cli/cli/config/configfile"
	"github.com/docker/cli/cli/config/types"
	"github.com/sylabs/singularity/internal/pkg/util/user"
	"github.com/sylabs/singularity/pkg/syfs"
)

const dockerHub = "docker.io"

func SetupDockerHubCredentials(t *testing.T) {
	var unprivUser, privUser *user.User

	username := os.Getenv("E2E_DOCKER_USERNAME")
	pass := os.Getenv("E2E_DOCKER_PASSWORD")

	if username == "" && pass == "" {
		t.Log("No DockerHub credentials supplied, DockerHub rate limits could be hit")
		return
	}

	unprivUser = CurrentUser(t)
	writeDockerHubCredentials(t, unprivUser.Dir, username, pass)
	Privileged(func(t *testing.T) {
		privUser = CurrentUser(t)
		writeDockerHubCredentials(t, privUser.Dir, username, pass)
	})(t)
}

func writeDockerHubCredentials(t *testing.T, dir, username, pass string) {
	configPath := filepath.Join(dir, ".singularity", syfs.DockerConfFile)

	cf := configfile.ConfigFile{
		AuthConfigs: map[string]types.AuthConfig{
			dockerHub: {
				Username: username,
				Password: pass,
			},
		},
	}

	configData, err := json.Marshal(cf)
	if err != nil {
		t.Error(err)
	}

	os.WriteFile(configPath, configData, 0o600)
}
