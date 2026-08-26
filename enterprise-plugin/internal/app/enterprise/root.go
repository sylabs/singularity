// Copyright (c) 2020-2026 Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package enterprise

import (
	"errors"
	"os"

	httptransport "github.com/go-openapi/runtime/client"
	"github.com/spf13/cobra"
	"github.com/sylabs/singularity/v4/enterprise-plugin/internal/pkg/api"
	"github.com/sylabs/singularity/v4/enterprise-plugin/internal/pkg/config"
	"github.com/sylabs/singularity/v4/pkg/sylog"
	useragent "github.com/sylabs/singularity/v4/pkg/util/user-agent"
	"golang.org/x/sys/unix"
)

const (
	// defaultURI points at the live Sylabs service.
	defaultURI = "cloud.sylabs.io"
)

// ErrNoEndpointURI is returned if we attempt to initialize clients without a valid URI.
var ErrNoEndpointURI = errors.New("no endpoint uri provided")

// rootOptions holds the options for the root command of the plugin / CLI
// These will be populated from config -> env var -> root command flags.
type rootOptions struct {
	// uri is the address of the enterprise frontend for service discovery.
	uri string
	// authToken is the credential used when accessing services.
	authToken string
	// authUser is the user ID from the authToken sub claim, if there is one.
	authUser string
	// SingRemote indicates whether to use the current Singularity remote config.
	// This will contact the enterprise install that is `remote use`-d in Singularity.
	singRemote bool
	// Nocolor indicates whether to disable color for Sylog messages.
	nocolor bool
	// Debug indicates whether to set the minimum log level to DEBUG for Sylog messages.
	debug bool
	// Debug indicates whether to set the minimum log level to VERBOSE for Sylog messages.
	verbose bool
	// Debug indicates whether to set the minimum log level to WARNING for Sylog messages.
	quiet bool
	// Debug indicates whether to set the minimum log level to ERROR for Sylog messages.
	silent bool
}

// initRoot initializes the root command `enterprise` for the CLI.. or `singularity enterprise` for the plugin.
// Note that global options that can be set through conf file, env var, and the root command flags are kept as
// Executor.options.
func (e *Executor) initRoot() {
	cmd := cobra.Command{
		Use:     "enterprise",
		Short:   "Singularity Enterprise management commands",
		Long:    "Singularity Enterprise management commands",
		Version: e.versionString(),
		// We use sylog to show the errors, so prevent duplicates here
		SilenceErrors: true,
		SilenceUsage:  true,
		PersistentPreRun: func(cmd *cobra.Command, _ []string) {
			// If we are in standalone mode, we must set the log level for Sylog, and our user-agent string.
			if !e.pluginMode {
				if err := config.InitializeConfig(cmd); err != nil {
					sylog.Fatalf("While reading configuration: %v", err)
				}
				e.setSylogLevel()
				useragent.InitValue("enterprise", e.versionString())
			}
			// If needed, load endpoint and token from the currently in-use Singularity managed remote
			if e.rootOpt.singRemote {
				uri, token, err := config.GetSingularityRemote("")
				if err != nil {
					sylog.Fatalf("Could not load remote configuration: %v", err)
				}
				e.rootOpt.uri = uri
				e.rootOpt.authToken = token
			}
			// Now initialize a set of service clients using the URI / token we have
			if e.rootOpt.authToken != "" {
				u, err := config.UserFromToken(e.rootOpt.authToken)
				if err != nil {
					sylog.Warningf("Could not determine user ID: %v", err)
				}
				e.rootOpt.authUser = u
				sylog.Debugf("User ID: %s", e.rootOpt.authUser)
				e.authInfo = httptransport.BearerToken(e.rootOpt.authToken)
			}
			var err error
			e.serviceURIs, err = config.GetServiceURIs(e.rootOpt.uri)
			if err != nil {
				sylog.Fatalf("Could not fetch service URIs from Enterprise: %v", err)
			}
			e.clients, err = api.NewClients(e.serviceURIs, e.authInfo)
			if err != nil {
				sylog.Fatalf("Could not initialize: %v", err)
			}
		},
	}

	// Only present flags for endpoint and log level configuration in CLI mode
	// We will *always* use the active Singularity managed remote in plugin mode.
	if !e.pluginMode {
		cmd.PersistentFlags().StringVar(&e.rootOpt.uri, "uri", defaultURI, "Base URI for the Singularity Enterprise / Sylabs Cloud instance.")
		cmd.PersistentFlags().StringVar(&e.rootOpt.authToken, "token", "", "Authentication token (CAUTION - specifying on the command line is insecure!)")
		cmd.PersistentFlags().BoolVar(&e.rootOpt.singRemote, "singularity-remote", false, "Use singularity remote configuration")
		cmd.PersistentFlags().BoolVarP(&e.rootOpt.debug, "debug", "d", false, "print debugging information (highest verbosity)")
		cmd.PersistentFlags().BoolVarP(&e.rootOpt.verbose, "verbose", "v", false, "print additional information")
		cmd.PersistentFlags().BoolVarP(&e.rootOpt.quiet, "quiet", "q", false, "suppress normal output")
		cmd.PersistentFlags().BoolVarP(&e.rootOpt.silent, "silent", "s", false, "only print errors")
		cmd.PersistentFlags().BoolVarP(&e.rootOpt.nocolor, "nocolor", "", false, "print without color output")
	} else {
		e.rootOpt.singRemote = true
	}

	// Register subcommands
	cmd.AddCommand(e.initVersion())
	cmd.AddCommand(e.initStatus())
	cmd.AddCommand(e.initLogtest())
	cmd.AddCommand(e.initGet())
	cmd.AddCommand(e.initDescribe())
	cmd.AddCommand(e.initDelete())

	// Main command output should go to Stdout, not Stderr
	cmd.SetOut(os.Stdout)

	e.rootCmd = &cmd
}

// setSylogLevel sets the logging level for Sylog according to our local options.
func (e *Executor) setSylogLevel() {
	logColor := isTerminal(2) && !e.rootOpt.nocolor
	switch {
	case e.rootOpt.debug:
		sylog.SetLevel(int(sylog.DebugLevel), logColor)
	case e.rootOpt.verbose:
		sylog.SetLevel(int(sylog.VerboseLevel), logColor)
	case e.rootOpt.quiet:
		sylog.SetLevel(int(sylog.InfoLevel), logColor)
	case e.rootOpt.silent:
		sylog.SetLevel(int(sylog.ErrorLevel), logColor)
	default:
		sylog.SetLevel(int(sylog.InfoLevel), logColor)
	}
}

// Detect a terminal in the same manner as golang/x/term.
// We need to avoid importing due to a dependency mess with Singularity's
// golang/x/crypto replacement.
func isTerminal(fd int) bool {
	_, err := unix.IoctlGetTermios(fd, unix.TCGETS)
	return err == nil
}
