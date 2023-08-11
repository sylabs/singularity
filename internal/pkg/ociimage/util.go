// Copyright (c) 2019-2023, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package ociimage

import (
	"fmt"
	"strings"

	"github.com/containers/image/v5/docker"
	dockerarchive "github.com/containers/image/v5/docker/archive"
	dockerdaemon "github.com/containers/image/v5/docker/daemon"
	ociarchive "github.com/containers/image/v5/oci/archive"
	ocilayout "github.com/containers/image/v5/oci/layout"
	"github.com/containers/image/v5/signature"
	"github.com/containers/image/v5/types"
)

// defaultPolicy is Singularity's default OCI signature verifiction policy - accept anything.
func defaultPolicy() (*signature.PolicyContext, error) {
	policy := &signature.Policy{Default: []signature.PolicyRequirement{signature.NewPRInsecureAcceptAnything()}}
	return signature.NewPolicyContext(policy)
}

// parseImageRef parses a uri-like OCI image reference into a containers/image types.ImageReference.
func ParseImageRef(imageRef string) (types.ImageReference, error) {
	parts := strings.SplitN(imageRef, ":", 2)
	if len(parts) < 2 {
		return nil, fmt.Errorf("could not parse image ref: %s", imageRef)
	}

	var srcRef types.ImageReference
	var err error

	switch parts[0] {
	case "docker":
		srcRef, err = docker.ParseReference(parts[1])
	case "docker-archive":
		srcRef, err = dockerarchive.ParseReference(parts[1])
	case "docker-daemon":
		srcRef, err = dockerdaemon.ParseReference(parts[1])
	case "oci":
		srcRef, err = ocilayout.ParseReference(parts[1])
	case "oci-archive":
		srcRef, err = ociarchive.ParseReference(parts[1])
	default:
		return nil, fmt.Errorf("cannot create an OCI container from %s source", parts[0])
	}
	if err != nil {
		return nil, fmt.Errorf("invalid image source: %v", err)
	}

	return srcRef, nil
}
