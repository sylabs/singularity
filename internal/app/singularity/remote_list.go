// Copyright (c) 2019-2023, Sylabs Inc. All rights reserved.
// Copyright (c) 2020, Control Command Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package singularity

import (
	"fmt"
	"os"
	"sort"
	"text/tabwriter"

	"github.com/samber/lo"
	"github.com/sylabs/singularity/internal/pkg/remote"
	"github.com/sylabs/singularity/internal/pkg/remote/endpoint"
)

const listLine = "%s\t%s\t%s\t%s\t%s\t%s\n"

// RemoteList prints information about remote configurations
func RemoteList(usrConfigFile string) error {
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

	// get system remote-endpoint configuration
	cSys, err := remote.GetSysConfig()
	if err != nil {
		return fmt.Errorf("while trying to access system remote-endpoint config: %w", err)
	}

	// get default remote endpoint
	defaultRemote, err := c.GetDefaultWithSys(cSys)
	if err != nil {
		return fmt.Errorf("error getting default remote-endpoint: %w", err)
	}

	// list remote-endpoints in alphanumeric order
	remoteNames := lo.Keys(c.Remotes)[:]
	sysRemoteNames := []string{}
	if cSys != nil {
		sysRemoteNames = lo.Keys(cSys.Remotes)
	}

	sort.Strings(remoteNames)
	sort.Strings(sysRemoteNames)

	fmt.Println()
	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, listLine, "NAME", "URI", "DEFAULT?", "GLOBAL?", "EXCLUSIVE?", "SECURE?")
	printLine := func(name string, r *endpoint.Config, isGlobal bool) {
		globalStr := ""
		if isGlobal {
			globalStr = "✓"
		}
		excl := ""
		if r.Exclusive {
			excl = "✓"
		}
		secure := "✓"
		if r.Insecure {
			secure = "✗!"
		}
		isDefault := ""
		if r == defaultRemote {
			isDefault = "✓"
		}

		fmt.Fprintf(tw, listLine, name, r.URI, isDefault, globalStr, excl, secure)
	}

	for _, n := range remoteNames {
		printLine(n, c.Remotes[n], false)
	}
	for _, n := range sysRemoteNames {
		printLine(n, cSys.Remotes[n], true)
	}
	tw.Flush()

	return nil
}
