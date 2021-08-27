// Copyright (c) 2019-2021, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

//go:build !linux
// +build !linux

package sources

import (
	"context"
	"fmt"

	"github.com/sylabs/singularity/pkg/build/types"
)

// Pack puts relevant objects in a Bundle.
func (p *SIFPacker) Pack(context.Context) (*types.Bundle, error) {
	return nil, fmt.Errorf("unsupported on this platform")
}
