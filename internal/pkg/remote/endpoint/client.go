// Copyright (c) 2020-2023, Sylabs Inc. All rights reserved.
// Copyright (c) 2020, Control Command Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package endpoint

import (
	"fmt"
	"net/http"
	"strings"

	golog "github.com/go-log/log"
	keyclient "github.com/sylabs/scs-key-client/client"
	libclient "github.com/sylabs/scs-library-client/client"
	remoteutil "github.com/sylabs/singularity/v4/internal/pkg/remote/util"
	"github.com/sylabs/singularity/v4/pkg/sylog"
	useragent "github.com/sylabs/singularity/v4/pkg/util/user-agent"
)

func (ep *Config) KeyserverClientOpts(uri string, op KeyserverOp) ([]keyclient.Option, error) {
	// empty uri means to use the default endpoint
	isDefault := uri == ""

	if err := ep.UpdateKeyserversConfig(); err != nil {
		return nil, err
	}

	var primaryKeyserver *ServiceConfig

	for _, kc := range ep.Keyservers {
		if kc.Skip {
			continue
		}
		primaryKeyserver = kc
		break
	}

	// shouldn't happen
	if primaryKeyserver == nil {
		return nil, fmt.Errorf("no primary keyserver configured")
	}

	var keyservers []*ServiceConfig

	if isDefault {
		uri = primaryKeyserver.URI

		if op == KeyserverVerifyOp {
			// verify operation can query multiple keyserver, the token
			// is automatically set by the custom client
			keyservers = ep.Keyservers
		} else {
			// use the primary keyserver
			keyservers = []*ServiceConfig{
				primaryKeyserver,
			}
		}
	} else if ep.Exclusive {
		available := make([]string, 0)
		found := false
		for _, kc := range ep.Keyservers {
			if kc.Skip {
				continue
			}
			available = append(available, kc.URI)
			if remoteutil.SameKeyserver(uri, kc.URI) {
				found = true
				break
			}
		}
		if !found {
			list := strings.Join(available, ", ")
			return nil, fmt.Errorf(
				"endpoint is set as exclusive by the system administrator: only %q can be used",
				list,
			)
		}
	} else {
		keyservers = []*ServiceConfig{
			{
				URI:      uri,
				External: true,
			},
		}
	}

	co := []keyclient.Option{
		keyclient.OptBaseURL(uri),
		keyclient.OptUserAgent(useragent.Value()),
		keyclient.OptHTTPClient(newClient(keyservers, op)),
	}
	return co, nil
}

func (ep *Config) LibraryClientConfig(uri string) (*libclient.Config, error) {
	// empty uri means to use the default endpoint
	isDefault := uri == ""

	config := &libclient.Config{
		BaseURL:   uri,
		UserAgent: useragent.Value(),
		Logger:    (golog.Logger)(sylog.DebugLogger{}),
		// TODO - probably should establish an appropriate client timeout here.
		HTTPClient: &http.Client{},
	}

	if isDefault {
		libURI, err := ep.GetServiceURI(Library)
		if err != nil {
			return nil, fmt.Errorf("unable to get library service URI: %v", err)
		}
		config.AuthToken = ep.Token
		config.BaseURL = libURI
	} else if ep.Exclusive {
		libURI, err := ep.GetServiceURI(Library)
		if err != nil {
			return nil, fmt.Errorf("unable to get library service URI: %v", err)
		}
		if !remoteutil.SameURI(uri, libURI) {
			return nil, fmt.Errorf(
				"endpoint is set as exclusive by the system administrator: only %q can be used",
				libURI,
			)
		}
	}

	return config, nil
}

// BuilderClientConfig returns the baseURI and authToken associated with ep, in the context of uri.
func (ep *Config) BuilderClientConfig(uri string) (baseURI, authToken string, err error) {
	// empty uri means to use the default endpoint
	isDefault := uri == ""

	baseURI = uri

	if isDefault {
		buildURI, err := ep.GetServiceURI(Builder)
		if err != nil {
			return "", "", fmt.Errorf("unable to get builder service URI: %v", err)
		}
		authToken = ep.Token
		baseURI = buildURI
	} else if ep.Exclusive {
		buildURI, err := ep.GetServiceURI(Builder)
		if err != nil {
			return "", "", fmt.Errorf("unable to get builder service URI: %v", err)
		}
		if !remoteutil.SameURI(uri, buildURI) {
			return "", "", fmt.Errorf(
				"endpoint is set as exclusive by the system administrator: only %q can be used",
				buildURI,
			)
		}
	}

	return baseURI, authToken, nil
}

// RegistryURI returns the URI of the backing OCI registry for the library service, associated with ep.
func (ep *Config) RegistryURI() (string, error) {
	registryURI, err := ep.getServiceConfigVal(Library, RegistryURIConfigKey)
	if err != nil {
		return "", err
	}
	if registryURI == "" {
		return "", fmt.Errorf("library does not provide an OCI registry")
	}
	return registryURI, nil
}
