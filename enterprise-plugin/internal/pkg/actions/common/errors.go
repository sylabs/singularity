// Copyright (c) 2021-2026, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

//revive:disable:var-naming
package common

import "errors"

var (
	ErrUnknownType  = errors.New("unknown type")
	ErrAuthRequired = errors.New("an authentication token is required")
	ErrListOnly     = errors.New("cannot retrieve a single object, use get to list all")
)
