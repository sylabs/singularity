// Copyright (c) 2023, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package library

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/google/go-containerregistry/pkg/authn"
	scslibrary "github.com/sylabs/scs-library-client/client"
	"github.com/sylabs/singularity/v4/internal/pkg/remote/endpoint"
	"github.com/sylabs/singularity/v4/pkg/sylog"
)

// libraryRegistry holds information necessary to interact with an OCI registry
// backing a library.
type libraryRegistry struct {
	library  string
	registry string
	ud       *userData
}

// newLibraryRegisty retrieves URI and authentication information for the
// backing registry of the library associated with endpoint ep.
func newLibraryRegistry(ep *endpoint.Config, LibraryConfig *scslibrary.Config) (*libraryRegistry, error) {
	epLibraryURI, err := ep.GetServiceURI(endpoint.Library)
	if err != nil {
		return nil, err
	}

	if LibraryConfig.BaseURL != epLibraryURI {
		return nil, fmt.Errorf("OCI-SIF push/pull to/from location other than current remote is not supported")
	}

	sylog.Debugf("Finding OCI registry URI")
	registryURI, err := ep.RegistryURI()
	if err != nil {
		return nil, err
	}
	ru, err := url.Parse(registryURI)
	if err != nil {
		return nil, err
	}
	registry := strings.TrimSuffix(ru.Host+ru.Path, "/")

	lr := libraryRegistry{
		library:  LibraryConfig.BaseURL,
		registry: registry,
	}

	if ep.Token != "" {
		sylog.Debugf("Fetching OCI registry token")
		ud, err := getUserData(LibraryConfig)
		if err != nil {
			return nil, err
		}
		lr.ud = ud
	}

	return &lr, nil
}

// convertRef converts the provided library ref into an OCI reference referring
// to the library's backing OCI registry.
func (lr *libraryRegistry) convertRef(libraryRef scslibrary.Ref) (string, error) {
	if libraryRef.Host != "" {
		return "", fmt.Errorf("push to location other than current remote is not supported")
	}
	ref := fmt.Sprintf("docker://%s/%s", lr.registry, libraryRef.Path)
	if len(libraryRef.Tags) > 1 {
		return "", fmt.Errorf("cannot push/pull with more than one tag")
	}
	if len(libraryRef.Tags) > 0 {
		ref = ref + ":" + libraryRef.Tags[0]
	}
	return ref, nil
}

// authConfig returns an authn.AuthConfig with current token to authenticate
// against the library's backing OCI registry, if logged in - nil otherwise.
func (lr *libraryRegistry) authConfig() *authn.AuthConfig {
	if lr.ud == nil {
		return nil
	}
	return &authn.AuthConfig{
		Username: lr.ud.Username,
		Password: lr.ud.OidcMeta.Secret,
	}
}
