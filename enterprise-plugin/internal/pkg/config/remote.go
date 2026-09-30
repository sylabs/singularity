// Copyright (c) 2020-2026 Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package config

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	useragent "github.com/sylabs/singularity/v4/pkg/util/user-agent"

	"github.com/SermoDigital/jose/jws"
	"github.com/sylabs/singularity/v4/internal/pkg/remote"
	"github.com/sylabs/singularity/v4/internal/pkg/remote/endpoint"
	"github.com/sylabs/singularity/v4/pkg/syfs"
	"github.com/sylabs/singularity/v4/pkg/sylog"
)

const (
	defaultTimeout    = 10 * time.Second
	serviceConfigPath = "/assets/config/config.prod.json"
)

// GetSingularityRemoteYaml returns the path to the singularity installation's remote.yml.
func GetSingularityRemoteYaml() (confPath string, err error) {
	// TODO - consider handling for when there is no remote config yet.
	// Should we bale out or construct the default like Singularity
	// would?
	confPath = syfs.RemoteConf()
	_, err = os.Stat(confPath)
	if err != nil {
		return "", fmt.Errorf("while reading remote config: %v", err)
	}
	return confPath, nil
}

// GetSingularityRemote returns the base URI and token for the specified remote name.
// If the remote name is "" then use the default remote.
// We do not return a Singularity endpoint construct as we want to avoid the handling
// of overridden keyservers etc - we are *only* working with the enterprise install.
func GetSingularityRemote(name string) (baseURI, authToken string, err error) {
	usrConfigFile, err := GetSingularityRemoteYaml()
	if err != nil {
		return "", "", err
	}

	if name != "" {
		sylog.Debugf("Loading Singularity remote config: %s", name)
	} else {
		sylog.Debugf("Loading default Singularity remote config")
	}

	// opening config file
	file, err := os.OpenFile(usrConfigFile, os.O_RDONLY|os.O_CREATE, 0o600)
	if err != nil {
		return "", "", fmt.Errorf("while opening remote config file: %s", err)
	}
	defer file.Close()

	// read file contents to config struct
	c, err := remote.ReadFrom(file)
	if err != nil {
		return "", "", fmt.Errorf("while parsing remote config data: %s", err)
	}

	var e *endpoint.Config
	if name == "" {
		e, err = c.GetDefault()
	} else {
		e, err = c.GetRemote(name)
	}

	if err != nil {
		return "", "", err
	}

	return e.URI, e.Token, nil
}

type ServiceConfigResponse struct {
	LibraryAPI struct {
		URI string `json:"uri"`
	} `json:"libraryAPI"`
	BuilderAPI struct {
		URI        string `json:"uri"`
		ManagerURI string `json:"managerUri"`
	} `json:"builderAPI"`
	KeystoreAPI struct {
		URI string `json:"uri"`
	} `json:"keystoreAPI"`
	ConsentAPI struct {
		URI string `json:"uri"`
	} `json:"consentAPI"`
	TokenAPI struct {
		URI string `json:"uri"`
	} `json:"tokenAPI"`
}

type ServiceURIs struct {
	LibraryURI        string
	BuildManagerURI   string
	BuildServerURI    string
	KeyServiceURI     string
	ConsentServiceURI string
	TokenServiceURI   string
}

// GetServiceURIs returns a ServiceURIs struct container the URI of each component service of Enterprise.
func GetServiceURIs(remoteURI string) (uris *ServiceURIs, err error) {
	client := &http.Client{
		Timeout: defaultTimeout,
	}

	// TODO
	// Quick fix to allow forcing an `http://` URL here at present.
	// Consider flag or other
	// approach, which needs to tie in with https://github.com/sylabs/singularity/issues/61
	url := remoteURI + serviceConfigPath
	if !strings.HasPrefix(url, "http://") {
		url = "https://" + url
	}

	sylog.Debugf("Retrieving service URIs from config at %s", url)

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", useragent.Value())

	res, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error making request to server: %s", err)
	} else if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("error response from server: %s", err)
	}

	defer res.Body.Close()
	b, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, fmt.Errorf("while reading response body: %v", err)
	}

	var sr ServiceConfigResponse
	if err := json.Unmarshal(b, &sr); err != nil {
		return nil, fmt.Errorf("jsonresp: failed to unmarshal response: %v", err)
	}

	uris = &ServiceURIs{
		LibraryURI:        sr.LibraryAPI.URI,
		BuildServerURI:    sr.BuilderAPI.URI,
		BuildManagerURI:   sr.BuilderAPI.ManagerURI,
		KeyServiceURI:     sr.KeystoreAPI.URI,
		ConsentServiceURI: sr.ConsentAPI.URI,
		TokenServiceURI:   sr.TokenAPI.URI,
	}

	return uris, nil
}

// UserFromToken returns the user ID from the token sub claim.
func UserFromToken(token string) (user string, err error) {
	j, err := jws.ParseJWT([]byte(token))
	if err != nil {
		return "", fmt.Errorf("while parsing auth token: %v", err)
	}

	user, ok := j.Claims().Get("sub").(string)
	if !ok {
		return "", fmt.Errorf("could not extract user ID from token - missing sub claim")
	}

	return user, nil
}
