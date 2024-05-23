// Copyright (c) 2024, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.package singularity

package singularity

import (
	"github.com/sylabs/singularity/v4/pkg/sylog"
)

// DataPackage packages src into a data container at dst.
func DataPackage(src, dst string) error {
	sylog.Fatalf("package %s -> %s: not implemented", src, dst)
	return nil
}
