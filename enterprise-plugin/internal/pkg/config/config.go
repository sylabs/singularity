// Copyright (c) 2020-2026 Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package config

import (
	"fmt"
	"os"
	"path"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"github.com/sylabs/singularity/v4/pkg/sylog"
)

const (
	envPrefix = "ENTERPRISE"
	confFile  = "enterprise"
	confPath  = ".config/sylabs"
)

// InitializeConfig handle options from a config file and env vars
// This approach binds viper config directly onto existing flags
// belonging to the cobra command cmd. This allows us to handle this
// configuration selectively for standalon operation only without
// any changes to the flag / option handling under singularity plugin
// mode.
//
// MIT Licensed
// https://github.com/carolynvs/stingoftheviper/blob/main/LICENSE
func InitializeConfig(cmd *cobra.Command) error {
	v := viper.New()

	// Set the base name of the config file, without the file extension.
	v.SetConfigName(confFile)

	// Currently use home directory .config/sylabs only
	// TODO - handle XDG config dirs properly here
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	v.AddConfigPath(path.Join(home, confPath))

	// Attempt to read the config file, gracefully ignoring errors
	// caused by a config file not being found. Return an error
	// if we cannot parse the config file.
	if err := v.ReadInConfig(); err != nil {
		// It's okay if there isn't a config file
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return err
		}
	}

	// When we bind flags to environment variables expect that the
	// environment variables are prefixed, e.g. a flag like --number
	// binds to an environment variable STING_NUMBER. This helps
	// avoid conflicts.
	v.SetEnvPrefix(envPrefix)

	// Bind to environment variables
	// Works great for simple config names, but needs help for names
	// like --favorite-color which we fix in the bindFlags function
	v.AutomaticEnv()

	// Bind the current command's flags to viper
	bindFlags(cmd, v)

	return nil
}

// Bind each cobra flag to its associated viper configuration (config file and environment variable)
//
// MIT Licensed
// https://github.com/carolynvs/stingoftheviper/blob/main/LICENSE
func bindFlags(cmd *cobra.Command, v *viper.Viper) {
	cmd.Flags().VisitAll(func(f *pflag.Flag) {
		// Environment variables can't have dashes in them, so bind them to their equivalent
		// keys with underscores, e.g. --favorite-color to STING_FAVORITE_COLOR
		if strings.Contains(f.Name, "-") {
			envVarSuffix := strings.ToUpper(strings.ReplaceAll(f.Name, "-", "_"))
			if err := v.BindEnv(f.Name, fmt.Sprintf("%s_%s", envPrefix, envVarSuffix)); err != nil {
				sylog.Warningf("Could not bind environment variable %s: %v", f.Name, err)
			}
		}

		// Apply the viper config value to the flag when the flag is not set and viper has a value
		if !f.Changed && v.IsSet(f.Name) {
			val := v.Get(f.Name)
			if err := cmd.Flags().Set(f.Name, fmt.Sprintf("%v", val)); err != nil {
				sylog.Warningf("Could not set option %s: %v", f.Name, err)
			}
		}
	})
}
