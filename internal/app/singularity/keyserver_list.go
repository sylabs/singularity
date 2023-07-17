// Copyright (c) 2019-2023, Sylabs Inc. All rights reserved.
// Copyright (c) 2020, Control Command Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package singularity

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/sylabs/singularity/internal/pkg/remote"
	"github.com/sylabs/singularity/internal/pkg/remote/endpoint"
)

// KeyserverList prints information about remote configurations
func KeyserverList(remoteName string, usrConfigFile string) (err error) {
	c := &remote.Config{}

	// opening config file
	file, err := os.OpenFile(usrConfigFile, os.O_RDONLY|os.O_CREATE, 0o600)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("no remote configurations")
		}
		return fmt.Errorf("while opening remote config file: %s", err)
	}
	defer file.Close()

	// read file contents to config struct
	c, err = remote.ReadFrom(file)
	if err != nil {
		return fmt.Errorf("while parsing remote config data: %s", err)
	}

	if err := syncSysConfig(c); err != nil {
		return err
	}

	var ep *endpoint.Config
	if remoteName == "" {
		ep, err = c.GetDefault()
	} else {
		ep, err = c.GetRemote(remoteName)
	}

	if err != nil {
		return fmt.Errorf("endpoint not found: %w", err)
	} else if !ep.System {
		return fmt.Errorf("current endpoint is not a system defined endpoint")
	}

	if err := ep.UpdateKeyserversConfig(); err != nil {
		return err
	}

	fmt.Println()
	fmt.Println("Keyservers")
	fmt.Println("==========")

	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", "URI", "GLOBAL?", "SECURE?", "ORDER")
	order := 1
	for _, kc := range ep.Keyservers {
		if kc.Skip {
			continue
		}
		secure := "✓"
		if kc.Insecure {
			secure = ""
		}
		fmt.Fprintf(tw, "%s\t✓\t%s\t%d", kc.URI, secure, order)
		if !kc.External {
			fmt.Fprintf(tw, "*\n")
		} else {
			fmt.Fprintf(tw, "\n")
		}
		order++
	}
	tw.Flush()

	fmt.Println()
	fmt.Println("* Active cloud services keyserver")

	return nil
}
