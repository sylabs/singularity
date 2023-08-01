// Copyright (c) 2019-2023, Sylabs Inc. All rights reserved.
// Copyright (c) 2020, Control Command Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package singularity

import (
	"fmt"
	"net/url"
	"os"
	"text/tabwriter"

	"github.com/sylabs/singularity/v4/internal/pkg/remote"
	"github.com/sylabs/singularity/v4/internal/pkg/remote/credential"
)

// RegistryList prints information about remote configurations
func RegistryList(usrConfigFile string) (err error) {
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

	var registryCredentials []*credential.Config
	for _, cred := range c.Credentials {
		u, err := url.Parse(cred.URI)
		if err != nil {
			return err
		}

		switch u.Scheme {
		case "oras", "docker":
			registryCredentials = append(registryCredentials, cred)
		}
	}

	if len(registryCredentials) < 1 {
		fmt.Println()
		fmt.Println("(no registries with stored login information found)")
		fmt.Println()

		return nil
	}

	fmt.Println()
	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "%s\t%s\n", "URI", "SECURE?")
	for _, r := range registryCredentials {
		secure := "✓"
		if r.Insecure {
			secure = "✗!"
		}
		fmt.Fprintf(tw, "%s\t%s\n", r.URI, secure)
	}
	tw.Flush()
	fmt.Println()

	return nil
}
