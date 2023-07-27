// Copyright (c) 2020, Control Command Inc. All rights reserved.
// Copyright (c) 2019-2021, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package singularity

import (
	"fmt"
	"io"
	"os"

	"github.com/sylabs/singularity/internal/pkg/remote"
)

// RemoteUse sets remote to use
func RemoteUse(usrConfigFile, name string, global, makeExclusive bool) (err error) {
	if makeExclusive {
		if os.Getuid() != 0 {
			return fmt.Errorf("unable to set endpoint as exclusive: not root user")
		}
		global = true
		usrConfigFile = remote.SystemConfigPath
	}

	// system config should be world readable
	perm := os.FileMode(0o600)
	if global {
		perm = os.FileMode(0o644)
	}

	// opening config file
	file, err := os.OpenFile(usrConfigFile, os.O_RDWR|os.O_CREATE, perm)
	if err != nil {
		return fmt.Errorf("while opening remote config file: %s", err)
	}
	defer file.Close()

	// read file contents to config struct
	c, err := remote.ReadFrom(file)
	if err != nil {
		return fmt.Errorf("while parsing remote config data: %s", err)
	}

	if err := c.SetDefault(name, makeExclusive); err != nil {
		return err
	}

	// truncating file before writing new contents and syncing to commit file
	if err := file.Truncate(0); err != nil {
		return fmt.Errorf("while truncating remote config file: %s", err)
	}

	if n, err := file.Seek(0, io.SeekStart); err != nil || n != 0 {
		return fmt.Errorf("failed to reset %s cursor: %s", file.Name(), err)
	}

	if _, err := c.WriteTo(file); err != nil {
		return fmt.Errorf("while writing remote config to file: %s", err)
	}

	if err := file.Sync(); err != nil {
		return fmt.Errorf("failed to flush remote config file %s: %s", file.Name(), err)
	}

	return nil
}
