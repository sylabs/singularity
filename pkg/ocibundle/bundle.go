// Copyright (c) 2019, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package ocibundle

import (
	"context"

	imgspecv1 "github.com/opencontainers/image-spec/specs-go/v1"
	specs "github.com/opencontainers/runtime-spec/specs-go"
)

// Bundle defines an OCI bundle interface to create/delete OCI bundles
type Bundle interface {
	Create(context.Context, *specs.Spec) error
	Update(context.Context, *specs.Spec) error
	ImageSpec() *imgspecv1.Image
	Delete(ctx context.Context) error
	Path() string
}
