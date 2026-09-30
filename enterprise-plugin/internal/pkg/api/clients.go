// Copyright (c) 2020-2026 Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

//revive:disable:var-naming
package api

import (
	"errors"
	"fmt"
	"io"
	"net/url"

	"github.com/go-openapi/runtime"
	httptransport "github.com/go-openapi/runtime/client"
	"github.com/go-openapi/strfmt"
	"github.com/sylabs/singularity/v4/enterprise-plugin/internal/pkg/config"
	"github.com/sylabs/singularity/v4/pkg/sylog"

	buildmanagerclient "github.com/sylabs/singularity/v4/enterprise-plugin/pkg/buildmanager/client"
	buildserverclient "github.com/sylabs/singularity/v4/enterprise-plugin/pkg/buildserver/client"
	consentclient "github.com/sylabs/singularity/v4/enterprise-plugin/pkg/consentservice/client"
	keyclient "github.com/sylabs/singularity/v4/enterprise-plugin/pkg/keyservice/client"
	libraryclient "github.com/sylabs/singularity/v4/enterprise-plugin/pkg/library/client"
	tokenclient "github.com/sylabs/singularity/v4/enterprise-plugin/pkg/tokenservice/client"

	librarymodels "github.com/sylabs/singularity/v4/enterprise-plugin/pkg/library/models"
)

// ErrNoURI is returned when an operation requires a URI but none was provided.
var ErrNoURI = errors.New("no URI provided")

// Clients holds the set of go-swagger API clients for the Singularity Enterprise services.
type Clients struct {
	LibraryClient  *libraryclient.SylabsContainerLibraryAPIOCISHIM
	BuildSrvClient *buildserverclient.SylabsRemoteBuildServer
	BuildManClient *buildmanagerclient.SylabsRemoteBuildManager
	KeyClient      *keyclient.SylabsKeyServiceAPI
	ConsentClient  *consentclient.SylabsConsentServiceAPI
	TokenClient    *tokenclient.SylabsTokenServiceAPI
}

// NewClients returns a set of initialized API clients for services with URIs specified in sURIs.
func NewClients(sURIs *config.ServiceURIs, authInfo runtime.ClientAuthInfoWriter) (c *Clients, err error) {
	c = new(Clients)
	if err := c.init(sURIs, authInfo); err != nil {
		return nil, err
	}
	return c, nil
}

// init initializes a client for each Enterprise service, in the Clients struct.
func (c *Clients) init(sURIs *config.ServiceURIs, authInfo runtime.ClientAuthInfoWriter) (err error) {
	if sURIs.LibraryURI == "" {
		return fmt.Errorf("library: %w", ErrNoURI)
	}
	sylog.Debugf("Initializing library client for URI %s", sURIs.LibraryURI)
	c.LibraryClient, err = newLibraryClient(sURIs.LibraryURI, authInfo)
	if err != nil {
		return fmt.Errorf("while initializing library client: %v", err)
	}

	if sURIs.BuildServerURI == "" {
		return fmt.Errorf("build server: %w", ErrNoURI)
	}
	sylog.Debugf("Initializing build server client for URI %s", sURIs.BuildServerURI)
	c.BuildSrvClient, err = newBuildServerClient(sURIs.BuildServerURI, authInfo)
	if err != nil {
		return fmt.Errorf("while initializing build server client: %v", err)
	}

	if sURIs.BuildManagerURI == "" {
		return fmt.Errorf("build manager: %w", ErrNoURI)
	}
	sylog.Debugf("Initializing build manager client for URI %s", sURIs.BuildManagerURI)
	c.BuildManClient, err = newBuildManagerClient(sURIs.BuildManagerURI, authInfo)
	if err != nil {
		return fmt.Errorf("while initializing build manager client: %v", err)
	}

	if sURIs.KeyServiceURI == "" {
		return fmt.Errorf("key service: %w", ErrNoURI)
	}
	sylog.Debugf("Initializing key service client for URI %s", sURIs.KeyServiceURI)
	c.KeyClient, err = newKeyClient(sURIs.KeyServiceURI, authInfo)
	if err != nil {
		return fmt.Errorf("while initializing key client: %v", err)
	}

	if sURIs.ConsentServiceURI == "" {
		return fmt.Errorf("consent service: %w", ErrNoURI)
	}
	sylog.Debugf("Initializing consent service client for URI %s", sURIs.ConsentServiceURI)
	c.ConsentClient, err = newConsentClient(sURIs.ConsentServiceURI, authInfo)
	if err != nil {
		return fmt.Errorf("while initializing consent client: %v", err)
	}

	if sURIs.TokenServiceURI == "" {
		return fmt.Errorf("token service: %w", ErrNoURI)
	}
	sylog.Debugf("Initializing token service client for URI %s", sURIs.TokenServiceURI)
	c.TokenClient, err = newTokenClient(sURIs.TokenServiceURI, authInfo)
	if err != nil {
		return fmt.Errorf("while initializing token client: %v", err)
	}

	return nil
}

func newLibraryClient(uri string, authInfo runtime.ClientAuthInfoWriter) (lc *libraryclient.SylabsContainerLibraryAPIOCISHIM, err error) {
	u, err := url.Parse(uri)
	if err != nil {
		return nil, err
	}
	r := httptransport.New(u.Host, u.Path, []string{u.Scheme})
	r.Consumers["text/html"] = LibraryFallbackConsumer()
	r.Consumers["text/plain"] = LibraryFallbackConsumer()
	if authInfo != nil {
		r.DefaultAuthentication = authInfo
	}
	return libraryclient.New(r, strfmt.Default), nil
}

func newBuildServerClient(uri string, authInfo runtime.ClientAuthInfoWriter) (bsc *buildserverclient.SylabsRemoteBuildServer, err error) {
	u, err := url.Parse(uri)
	if err != nil {
		return nil, err
	}
	r := httptransport.New(u.Host, u.Path, []string{u.Scheme})
	if authInfo != nil {
		r.DefaultAuthentication = authInfo
	}
	return buildserverclient.New(r, strfmt.Default), nil
}

func newBuildManagerClient(uri string, authInfo runtime.ClientAuthInfoWriter) (bmc *buildmanagerclient.SylabsRemoteBuildManager, err error) {
	u, err := url.Parse(uri)
	if err != nil {
		return nil, err
	}
	r := httptransport.New(u.Host, u.Path, []string{u.Scheme})
	if authInfo != nil {
		r.DefaultAuthentication = authInfo
	}
	return buildmanagerclient.New(r, strfmt.Default), nil
}

func newKeyClient(uri string, authInfo runtime.ClientAuthInfoWriter) (kc *keyclient.SylabsKeyServiceAPI, err error) {
	u, err := url.Parse(uri)
	if err != nil {
		return nil, err
	}
	r := httptransport.New(u.Host, u.Path, []string{u.Scheme})
	if authInfo != nil {
		r.DefaultAuthentication = authInfo
	}
	r.Consumers["application/pgp-keys"] = runtime.TextConsumer()
	return keyclient.New(r, strfmt.Default), nil
}

func newConsentClient(uri string, authInfo runtime.ClientAuthInfoWriter) (cc *consentclient.SylabsConsentServiceAPI, err error) {
	u, err := url.Parse(uri)
	if err != nil {
		return nil, err
	}
	r := httptransport.New(u.Host, u.Path, []string{u.Scheme})
	if authInfo != nil {
		r.DefaultAuthentication = authInfo
	}
	return consentclient.New(r, strfmt.Default), nil
}

func newTokenClient(uri string, authInfo runtime.ClientAuthInfoWriter) (tc *tokenclient.SylabsTokenServiceAPI, err error) {
	u, err := url.Parse(uri)
	if err != nil {
		return nil, err
	}
	r := httptransport.New(u.Host, u.Path, []string{u.Scheme})
	if authInfo != nil {
		r.DefaultAuthentication = authInfo
	}
	return tokenclient.New(r, strfmt.Default), nil
}

// LibraryFallbackConsumer will be registered to handle responses that don't include data, or have straight
// text / HTML etc. This is necessary to properly present bare errors from the oci-library-shim that don't fit the
// Sylabs JSON response structure. These can occur when attempting to fetch a container by id (suppported by v1) rather
// than a delimited repository path. Without a custom handler a confusing unmarshalling error is displayed.
func LibraryFallbackConsumer() runtime.Consumer {
	return runtime.ConsumerFunc(func(_ io.Reader, data any) error {
		switch data.(type) {
		case *librarymodels.Json400error, *librarymodels.Json401error, *librarymodels.Json403error,
			*librarymodels.Json404error, *librarymodels.Json409error, *librarymodels.Json500error,
			*librarymodels.Json501error, *librarymodels.Json507error:
			return nil
		}

		return fmt.Errorf("%v (%T) is not supported by the LibraryFallBackConsumer", data, data)
	})
}
