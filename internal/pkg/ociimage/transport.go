// Copyright (c) 2019-2023, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package ociimage

import (
	"errors"
	"fmt"
	"strings"

	"github.com/containers/image/v5/types"
	"github.com/google/go-containerregistry/pkg/authn"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/sylabs/singularity/v4/pkg/util/slice"
)

var ociTransports = []string{"docker", "docker-archive", "docker-daemon", "oci", "oci-archive"}

var errUnsupportedTransport = errors.New("unsupported transport")

// SupportedTransport returns whether or not the transport given is supported. To fit within a switch/case
// statement, this function will return transport if it is supported
func SupportedTransport(transport string) string {
	if slice.ContainsString(ociTransports, transport) {
		return transport
	}
	return ""
}

// TransportOptions provides authentication, platform etc. configuration for
// interactions with image transports.
type TransportOptions struct {
	// AuthConfig provides optional credentials to be used when interacting with
	// an image transport.
	AuthConfig *authn.AuthConfig
	// AuthFilePath provides an optional path to a file containing credentials
	// to be used when interacting with an image transport.
	AuthFilePath string
	// Insecure should be set to true in order to interact with a registry via
	// http, or without TLS certificate verification.
	Insecure bool
	// DockerDaemonHost provides the URI to use when interacting with a Docker
	// daemon.
	DockerDaemonHost string
	// Platform specifies the OS / Architecture / Variant that the pulled images
	// should satisfy.
	Platform v1.Platform
	// UserAgent will be set on HTTP(S) request made by transports.
	UserAgent string
	// TmpDir is a location in which a transport can create temporary files.
	TmpDir string
}

// SystemContext returns a containers/image/v5 types.SystemContext struct for
// compatibility with operations that still use containers/image.
//
// Deprecated: for containers/image compatibility only. To be removed in
// SingularityCE v5.
func (t *TransportOptions) SystemContext() types.SystemContext {
	sc := types.SystemContext{
		AuthFilePath:            t.AuthFilePath,
		BigFilesTemporaryDir:    t.TmpDir,
		DockerRegistryUserAgent: t.UserAgent,
		OSChoice:                t.Platform.OS,
		ArchitectureChoice:      t.Platform.Architecture,
		VariantChoice:           t.Platform.Variant,
		DockerDaemonHost:        t.DockerDaemonHost,
	}

	if t.AuthConfig != nil {
		sc.DockerAuthConfig = &types.DockerAuthConfig{
			Username:      t.AuthConfig.Username,
			Password:      t.AuthConfig.Password,
			IdentityToken: t.AuthConfig.IdentityToken,
		}
	}

	if t.Insecure {
		sc.DockerInsecureSkipTLSVerify = types.NewOptionalBool(true)
		sc.DockerDaemonInsecureSkipTLSVerify = true
		sc.OCIInsecureSkipTLSVerify = true
	}

	return sc
}

// TransportOptionsFromSystemContext returns a TransportOptions struct
// initialized from a containers/image SystemContext. If the SystemContext is
// nil, then nil is returned.
//
// Deprecated: for containers/image compatibility only. To be removed in
// SingularityCE v5.
func TransportOptionsFromSystemContext(sc *types.SystemContext) *TransportOptions {
	if sc == nil {
		return nil
	}

	tOpts := TransportOptions{
		AuthFilePath: sc.AuthFilePath,
		TmpDir:       sc.BigFilesTemporaryDir,
		UserAgent:    sc.DockerRegistryUserAgent,
		Platform: v1.Platform{
			OS:           sc.OSChoice,
			Architecture: sc.ArchitectureChoice,
			Variant:      sc.VariantChoice,
		},
		Insecure: sc.DockerInsecureSkipTLSVerify == types.OptionalBoolTrue || sc.DockerDaemonInsecureSkipTLSVerify || sc.OCIInsecureSkipTLSVerify,
	}

	if sc.DockerAuthConfig != nil {
		tOpts.AuthConfig = &authn.AuthConfig{
			Username:      sc.DockerAuthConfig.Username,
			Password:      sc.DockerAuthConfig.Password,
			IdentityToken: sc.DockerAuthConfig.IdentityToken,
		}
	}

	return &tOpts
}

// URItoSourceSinkRef parses a uri-like OCI image reference into a SourceSink and ref
func URItoSourceSinkRef(imageURI string) (SourceSink, string, error) {
	parts := strings.SplitN(imageURI, ":", 2)
	if len(parts) < 2 {
		return UnknownSourceSink, "", fmt.Errorf("could not parse image ref: %s", imageURI)
	}

	switch parts[0] {
	case "docker":
		// Remove slashes from docker:// URI
		parts[1] = strings.TrimPrefix(parts[1], "//")
		return RegistrySourceSink, parts[1], nil
	case "docker-archive":
		return TarballSourceSink, parts[1], nil
	case "docker-daemon":
		return DaemonSourceSink, parts[1], nil
	case "oci":
		return OCISourceSink, parts[1], nil
	}

	return UnknownSourceSink, "", errUnsupportedTransport
}
