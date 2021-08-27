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
)

func (cp *ArchConveyorPacker) prepareFakerootEnv(context.Context) (func(), error) {
	return nil, fmt.Errorf("fakeroot not supported on this platform")
}
