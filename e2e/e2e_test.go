// Copyright (c) 2019-2022, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

//go:build e2e_test

package e2e

import (
	"testing"

	// This import will execute a CGO section with the help of a C constructor
	// section "init". As we always require to run e2e tests as root, the C part
	// is responsible of finding the original user who executes tests; it will
	// also create a dedicated mount namespace for the e2e tests, and a PID
	// namespace if "SINGULARITY_E2E_NO_PID_NS" is not set. Finally, it will
	// restore identity to the original user but will retain privileges for
	// Privileged method enabling the execution of a function with root
	// privileges when required
	_ "github.com/sylabs/singularity/e2e/internal/e2e/init"
)

func TestE2E(t *testing.T) {
	RunE2ETests(t)
}
