// Copyright (c) 2020-2026, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package main

import (
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/spf13/pflag"

	"github.com/sylabs/sif/v2/pkg/sif"
	"github.com/sylabs/singularity/v4/internal/pkg/buildcfg"
	"github.com/sylabs/singularity/v4/pkg/sylog"
)

// LogData provides items that can be used in an output template to produce
// a log message.
type LogData struct {
	// UID is the numeric user id of the account that ran singularity
	UID int
	// GID is the numeric group id of the account that ran singularity
	GID int
	// User is the text user name of the account that ran singularity
	User string
	// Group is the text group name of the account that ran singularity
	Group string
	// Executable is the full path to the singularity executable that was run
	Executable string
	// Version is the version number of the singularity executable that was run
	Version string
	// CWD is the current working directory
	CWD string
	// Command is the top level command passed to singularity, e.g. run or build
	Command string
	// Args is a list of arguments passed to the command
	Args []string
	// Flags is a list of flags passed to the command
	Flags []string
	// Image is the path / URL of the container image, if applicable
	Image string
	// SIFUUID is the unique identifier (UUID) of the SIF image, if applicable
	SIFUUID string
	// Env gives access to log arbitrary environment variables.
	// Map of NAME -> VALUE
	Env map[string]string
}

// populateData fills the LogData struct with values
func (p *pluginImplementation) populateData() {
	p.populateAccount()
	p.populateSingularity()
	p.populateEnvironment()
}

// populateAccount fills user / group information in the LogData struct
func (p *pluginImplementation) populateAccount() {
	p.data.UID = os.Getuid()
	p.data.GID = os.Getgid()

	p.data.User = "UNKNOWN"
	p.data.Group = "UNKNOWN"

	u, err := user.Current()
	if err != nil {
		sylog.Warningf("Plugin %s: could not populate user information: %s", pluginName, err)
	} else {
		p.data.User = u.Username
	}

	g, err := user.LookupGroupId(strconv.Itoa(p.data.GID))
	if err != nil {
		sylog.Warningf("Plugin %s: could not populate group information: %s", pluginName, err)
	} else {
		p.data.Group = g.Name
	}
}

// populateSingularity fills information about the singularity invocation
func (p *pluginImplementation) populateSingularity() {
	exe, err := os.Executable()
	if err != nil {
		sylog.Warningf("Plugin %s: could not populate executable information: %s", pluginName, err)
	}
	exe, err = filepath.Abs(exe)
	if err != nil {
		sylog.Warningf("Plugin %s: could not populate executable information: %s", pluginName, err)
	}

	p.data.Executable = exe
	p.data.Version = buildcfg.PACKAGE_VERSION
	p.data.Command = p.command.Name()

	// Commands that are 'actions' act on a container images so get the Image path / URI and SIF UUID
	// if those are applicable
	for _, action := range p.commandManager.GetCmdGroup("actions_instance") {
		if action.Name() == p.command.Name() {
			// URL-like... keep as is
			if strings.Contains(p.args[0], "://") {
				p.data.Image = p.args[0]
			} else {
				// Otherwise treat as a file
				p.data.SIFUUID = getImageUUID(p.args[0])
				p.data.Image, _ = filepath.Abs(p.args[0])
			}
			break
		}
	}

	p.data.Args = p.args

	// Walk the flags to make a simple list of them
	var flags []string
	p.command.Flags().Visit(func(f *pflag.Flag) {
		flags = append(flags, fmt.Sprintf("%s=%q", f.Name, f.Value))
	})
	p.data.Flags = flags

	// Current working directory if possible
	p.data.CWD = "UNKNOWN"
	wd, err := os.Getwd()
	if err == nil {
		p.data.CWD = wd
	}
}

// populateEnvironment add a map of env vars to p.data
func (p *pluginImplementation) populateEnvironment() {
	p.data.Env = envMap()
}

// getImageUUID returns the SIF UUID for the container in use, if applicable.
func getImageUUID(imgPath string) string {
	c, err := sif.LoadContainerFromPath(imgPath, sif.OptLoadWithFlag(os.O_RDONLY))
	if err != nil {
		return ""
	}
	defer func() {
		if err := c.UnloadContainer(); err != nil {
			sylog.Warningf("Unable to unload container SIF: %v", err)
		}
	}()
	return c.ID()
}

// envMap processes NAME=VAR pairs from the environment into a map
func envMap() map[string]string {
	getenvironment := func(data []string, getkeyval func(item string) (key, val string)) map[string]string {
		items := make(map[string]string)
		for _, item := range data {
			key, val := getkeyval(item)
			items[key] = val
		}
		return items
	}

	return getenvironment(os.Environ(), func(item string) (key, val string) {
		splits := strings.SplitN(item, "=", 2)
		key = splits[0]
		val = splits[1]
		return
	})
}
