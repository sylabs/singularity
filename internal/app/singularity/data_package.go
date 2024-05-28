// Copyright (c) 2024, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.package singularity

package singularity

import (
	"fmt"
	"os"

	"github.com/sylabs/singularity/v4/internal/pkg/ocisif"
	"github.com/sylabs/singularity/v4/pkg/sylog"
)

// DataPackage packages src into a data container at dst.
func DataPackage(src, dst string) error {
	if _, err := os.Stat(dst); !os.IsNotExist(err) {
		return fmt.Errorf("%s already exists - will not overwrite", dst)
	}

	tmpEnv := os.Getenv("SINGULARITY_TMPDIR")
	tmpDir, err := os.MkdirTemp(tmpEnv, "data-package-")
	if err != nil {
		return fmt.Errorf("while creating temporary directory: %w", err)
	}
	defer func() {
		if err := os.RemoveAll(tmpDir); err != nil {
			sylog.Errorf("while removing temporary directory: %v", err)
		}
	}()

	return ocisif.WriteDataContainerFromPath(src, dst, tmpDir)
}
