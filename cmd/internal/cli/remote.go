// Copyright (c) 2019-2025, Sylabs Inc. All rights reserved.
// Copyright (c) 2020, Control Command Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package cli

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/sylabs/singularity/v4/docs"
	"github.com/sylabs/singularity/v4/internal/app/singularity"
	"github.com/sylabs/singularity/v4/internal/pkg/client/library"
	"github.com/sylabs/singularity/v4/internal/pkg/remote"
	"github.com/sylabs/singularity/v4/pkg/cmdline"
	"github.com/sylabs/singularity/v4/pkg/syfs"
	"github.com/sylabs/singularity/v4/pkg/sylog"
)

const (
	remoteWarning = "no authentication token, log in with `singularity remote login`"
)

var (
	loginTokenFile      string
	loginUsername       string
	loginPassword       string
	remoteConfig        string
	loginPasswordStdin  bool
	loginInsecure       bool
	remoteNoLogin       bool
	global              bool
	remoteUseExclusive  bool
	remoteAddInsecure   bool
	remoteAddNotDefault bool
)

// assemble values of remoteConfig for user/sys locations
var remoteConfigUser = syfs.RemoteConf()

// -g|--global
var remoteGlobalFlag = cmdline.Flag{
	ID:           "remoteGlobalFlag",
	Value:        &global,
	DefaultValue: false,
	Name:         "global",
	ShortHand:    "g",
	Usage:        "edit the list of globally configured remote endpoints",
}

// -c|--config
var remoteConfigFlag = cmdline.Flag{
	ID:           "remoteConfigFlag",
	Value:        &remoteConfig,
	DefaultValue: remoteConfigUser,
	Name:         "config",
	ShortHand:    "c",
	Usage:        "path to the file holding remote endpoint configurations",
}

// --tokenfile
var remoteTokenFileFlag = cmdline.Flag{
	ID:           "remoteTokenFileFlag",
	Value:        &loginTokenFile,
	DefaultValue: "",
	Name:         "tokenfile",
	Usage:        "path to the file holding auth token for login (remote endpoints only)",
}

// --no-login
var remoteNoLoginFlag = cmdline.Flag{
	ID:           "remoteNoLoginFlag",
	Value:        &remoteNoLogin,
	DefaultValue: false,
	Name:         "no-login",
	Usage:        "skip automatic login step",
}

// -u|--username
var remoteLoginUsernameFlag = cmdline.Flag{
	ID:           "remoteLoginUsernameFlag",
	Value:        &loginUsername,
	DefaultValue: "",
	Name:         "username",
	ShortHand:    "u",
	Usage:        "username to authenticate with",
	EnvKeys:      []string{"LOGIN_USERNAME"},
}

// -p|--password
var remoteLoginPasswordFlag = cmdline.Flag{
	ID:           "remoteLoginPasswordFlag",
	Value:        &loginPassword,
	DefaultValue: "",
	Name:         "password",
	ShortHand:    "p",
	Usage:        "password / token to authenticate with",
	EnvKeys:      []string{"LOGIN_PASSWORD"},
}

// --password-stdin
var remoteLoginPasswordStdinFlag = cmdline.Flag{
	ID:           "remoteLoginPasswordStdinFlag",
	Value:        &loginPasswordStdin,
	DefaultValue: false,
	Name:         "password-stdin",
	Usage:        "take password from standard input",
}

// -i|--insecure
var remoteLoginInsecureFlag = cmdline.Flag{
	ID:           "remoteLoginInsecureFlag",
	Value:        &loginInsecure,
	DefaultValue: false,
	Name:         "insecure",
	ShortHand:    "i",
	Usage:        "allow insecure login",
	EnvKeys:      []string{"LOGIN_INSECURE"},
}

// -e|--exclusive
var remoteUseExclusiveFlag = cmdline.Flag{
	ID:           "remoteUseExclusiveFlag",
	Value:        &remoteUseExclusive,
	DefaultValue: false,
	Name:         "exclusive",
	ShortHand:    "e",
	Usage:        "set the endpoint as exclusive (root user only, imply --global)",
}

// -i|--insecure
var remoteAddInsecureFlag = cmdline.Flag{
	ID:           "remoteAddInsecureFlag",
	Value:        &remoteAddInsecure,
	DefaultValue: false,
	Name:         "insecure",
	ShortHand:    "i",
	Usage:        "allow connection to an insecure http remote",
}

// -i|--insecure
var remoteAddNotDefaultFlag = cmdline.Flag{
	ID:           "remoteAddNotDefaultFlag",
	Value:        &remoteAddNotDefault,
	DefaultValue: false,
	Name:         "no-default",
	ShortHand:    "n",
	Usage:        "do not designate the newly-added remote endpoint as the default",
}

func init() {
	addCmdInit(func(cmdManager *cmdline.CommandManager) {
		cmdManager.RegisterCmd(RemoteCmd)
		cmdManager.RegisterSubCmd(RemoteCmd, RemoteAddCmd)
		cmdManager.RegisterSubCmd(RemoteCmd, RemoteRemoveCmd)
		cmdManager.RegisterSubCmd(RemoteCmd, RemoteUseCmd)
		cmdManager.RegisterSubCmd(RemoteCmd, RemoteListCmd)
		cmdManager.RegisterSubCmd(RemoteCmd, RemoteLoginCmd)
		cmdManager.RegisterSubCmd(RemoteCmd, RemoteLogoutCmd)
		cmdManager.RegisterSubCmd(RemoteCmd, RemoteStatusCmd)
		cmdManager.RegisterSubCmd(RemoteCmd, RemoteGetLoginPasswordCmd)

		// default location of the remote.yaml file is the user directory
		cmdManager.RegisterFlagForCmd(&remoteConfigFlag, RemoteCmd)
		// use tokenfile to log in to a remote
		cmdManager.RegisterFlagForCmd(&remoteTokenFileFlag, RemoteLoginCmd, RemoteAddCmd)
		// add --global flag to remote add/remove/use commands
		cmdManager.RegisterFlagForCmd(&remoteGlobalFlag, RemoteAddCmd, RemoteRemoveCmd, RemoteUseCmd)
		// add --insecure, --no-login flags to add command
		cmdManager.RegisterFlagForCmd(&remoteNoLoginFlag, RemoteAddCmd)
		cmdManager.RegisterFlagForCmd(&remoteAddInsecureFlag, RemoteAddCmd)
		cmdManager.RegisterFlagForCmd(&remoteAddNotDefaultFlag, RemoteAddCmd)

		cmdManager.RegisterFlagForCmd(&remoteLoginUsernameFlag, RemoteLoginCmd)
		cmdManager.RegisterFlagForCmd(&remoteLoginPasswordFlag, RemoteLoginCmd)
		cmdManager.RegisterFlagForCmd(&remoteLoginPasswordStdinFlag, RemoteLoginCmd)
		cmdManager.RegisterFlagForCmd(&remoteLoginInsecureFlag, RemoteLoginCmd)

		cmdManager.RegisterFlagForCmd(&remoteUseExclusiveFlag, RemoteUseCmd)
	})
}

// RemoteCmd singularity remote [...]
var RemoteCmd = &cobra.Command{
	Run: nil,

	Use:     docs.RemoteUse,
	Short:   docs.RemoteShort,
	Long:    docs.RemoteLong,
	Example: docs.RemoteExample,

	DisableFlagsInUseLine: true,
}

// setGlobalRemoteConfig will assign the appropriate value to remoteConfig if the global flag is set
func setGlobalRemoteConfig(_ *cobra.Command, _ []string) {
	if !global {
		return
	}

	if os.Getuid() != 0 {
		sylog.Fatalf("Unable to modify global endpoint configuration file: not root user")
	}

	// set remoteConfig value to the location of the global remote.yaml file
	remoteConfig = remote.SystemConfigPath
}

// RemoteGetLoginPasswordCmd singularity remote get-login-password
var RemoteGetLoginPasswordCmd = &cobra.Command{
	DisableFlagsInUseLine: true,

	Use:     docs.RemoteGetLoginPasswordUse,
	Short:   docs.RemoteGetLoginPasswordShort,
	Long:    docs.RemoteGetLoginPasswordLong,
	Example: docs.RemoteGetLoginPasswordExample,

	Run: func(_ *cobra.Command, _ []string) {
		defaultConfig := ""

		config, err := getLibraryClientConfig(defaultConfig)
		if err != nil {
			sylog.Errorf("Error initializing config: %v", err)
		}

		password, err := library.GetOCIToken(config)
		if err != nil {
			sylog.Errorf("error: %v", err)
		}
		if password != "" {
			fmt.Println(password)
		}
	},
}

// RemoteAddCmd singularity remote add [remoteName] [remoteURI]
var RemoteAddCmd = &cobra.Command{
	Args:   cobra.ExactArgs(2),
	PreRun: setGlobalRemoteConfig,
	Run: func(_ *cobra.Command, args []string) {
		name := args[0]
		uri := args[1]
		makeDefault := !remoteAddNotDefault
		if err := singularity.RemoteAdd(remoteConfig, name, uri, global, remoteAddInsecure, makeDefault); err != nil {
			sylog.Fatalf("%s", err)
		}
		sylog.Infof("Remote %q added.", name)

		// ensure that this was not called with global flag, otherwise this will store the token in the
		// world readable config
		if global && !remoteNoLogin {
			sylog.Infof("Global option detected. Will not automatically log into remote.")
		} else if !remoteNoLogin {
			loginArgs := &singularity.LoginArgs{
				Name:      name,
				Tokenfile: loginTokenFile,
			}
			if err := singularity.RemoteLogin(remoteConfig, loginArgs); err != nil {
				sylog.Fatalf("%s", err)
			}
		}
	},

	Use:     docs.RemoteAddUse,
	Short:   docs.RemoteAddShort,
	Long:    docs.RemoteAddLong,
	Example: docs.RemoteAddExample,

	DisableFlagsInUseLine: true,
}

// RemoteRemoveCmd singularity remote remove [remoteName]
var RemoteRemoveCmd = &cobra.Command{
	Args:   cobra.ExactArgs(1),
	PreRun: setGlobalRemoteConfig,
	Run: func(_ *cobra.Command, args []string) {
		name := args[0]
		if err := singularity.RemoteRemove(remoteConfig, name); err != nil {
			sylog.Fatalf("%s", err)
		}
		sylog.Infof("Remote %q removed.", name)
	},

	Use:     docs.RemoteRemoveUse,
	Short:   docs.RemoteRemoveShort,
	Long:    docs.RemoteRemoveLong,
	Example: docs.RemoteRemoveExample,

	DisableFlagsInUseLine: true,
}

// RemoteUseCmd singularity remote use [remoteName]
var RemoteUseCmd = &cobra.Command{
	Args:   cobra.ExactArgs(1),
	PreRun: setGlobalRemoteConfig,
	Run: func(_ *cobra.Command, args []string) {
		name := args[0]
		if err := singularity.RemoteUse(remoteConfig, name, global, remoteUseExclusive); err != nil {
			sylog.Fatalf("%s", err)
		}
		sylog.Infof("Remote %q now in use.", name)
	},

	Use:     docs.RemoteUseUse,
	Short:   docs.RemoteUseShort,
	Long:    docs.RemoteUseLong,
	Example: docs.RemoteUseExample,

	DisableFlagsInUseLine: true,
}

// RemoteListCmd singularity remote list
var RemoteListCmd = &cobra.Command{
	Args: cobra.ExactArgs(0),
	Run: func(_ *cobra.Command, _ []string) {
		if err := singularity.RemoteList(remoteConfig); err != nil {
			sylog.Fatalf("%s", err)
		}
	},

	Use:     docs.RemoteListUse,
	Short:   docs.RemoteListShort,
	Long:    docs.RemoteListLong,
	Example: docs.RemoteListExample,

	DisableFlagsInUseLine: true,
}

// RemoteLoginCmd singularity remote login [remoteName]
var RemoteLoginCmd = &cobra.Command{
	Args: cobra.RangeArgs(0, 1),
	Run: func(_ *cobra.Command, args []string) {
		loginArgs := new(singularity.LoginArgs)

		// default to empty string to signal to RemoteLogin to use default remote
		if len(args) > 0 {
			loginArgs.Name = args[0]
		}

		loginArgs.Username = loginUsername
		loginArgs.Password = loginPassword
		loginArgs.Tokenfile = loginTokenFile
		loginArgs.Insecure = loginInsecure

		if loginPasswordStdin {
			p, err := io.ReadAll(os.Stdin)
			if err != nil {
				sylog.Fatalf("Failed to read password from stdin: %s", err)
			}
			loginArgs.Password = strings.TrimSuffix(string(p), "\n")
			loginArgs.Password = strings.TrimSuffix(loginArgs.Password, "\r")
		}

		if err := singularity.RemoteLogin(remoteConfig, loginArgs); err != nil {
			sylog.Fatalf("%s", err)
		}
	},

	Use:     docs.RemoteLoginUse,
	Short:   docs.RemoteLoginShort,
	Long:    docs.RemoteLoginLong,
	Example: docs.RemoteLoginExample,

	DisableFlagsInUseLine: true,
}

// RemoteLogoutCmd singularity remote logout [remoteName|serviceURI]
var RemoteLogoutCmd = &cobra.Command{
	Args: cobra.RangeArgs(0, 1),
	Run: func(_ *cobra.Command, args []string) {
		// default to empty string to signal to RemoteLogin to use default remote
		name := ""
		if len(args) > 0 {
			name = args[0]
		}

		if err := singularity.RemoteLogout(remoteConfig, name); err != nil {
			sylog.Fatalf("%s", err)
		}
		sylog.Infof("Logout succeeded")
	},

	Use:     docs.RemoteLogoutUse,
	Short:   docs.RemoteLogoutShort,
	Long:    docs.RemoteLogoutLong,
	Example: docs.RemoteLogoutExample,

	DisableFlagsInUseLine: true,
}

// RemoteStatusCmd singularity remote status [remoteName]
var RemoteStatusCmd = &cobra.Command{
	Args: cobra.RangeArgs(0, 1),
	Run: func(_ *cobra.Command, args []string) {
		// default to empty string to signal to RemoteStatus to use default remote
		name := ""
		if len(args) > 0 {
			name = args[0]
		}

		if err := singularity.RemoteStatus(remoteConfig, name); err != nil {
			sylog.Fatalf("%s", err)
		}
	},

	Use:     docs.RemoteStatusUse,
	Short:   docs.RemoteStatusShort,
	Long:    docs.RemoteStatusLong,
	Example: docs.RemoteStatusExample,

	DisableFlagsInUseLine: true,
}
