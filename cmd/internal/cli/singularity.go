// Copyright (c) 2020, Control Command Inc. All rights reserved.
// Copyright (c) 2018-2025, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package cli

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"os/user"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/google/go-containerregistry/pkg/authn"
	ggcrv1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/spf13/cobra"
	scskeyclient "github.com/sylabs/scs-key-client/client"
	scslibclient "github.com/sylabs/scs-library-client/client"
	"github.com/sylabs/singularity/v4/docs"
	"github.com/sylabs/singularity/v4/internal/pkg/buildcfg"
	"github.com/sylabs/singularity/v4/internal/pkg/ociplatform"
	"github.com/sylabs/singularity/v4/internal/pkg/plugin"
	"github.com/sylabs/singularity/v4/internal/pkg/remote"
	"github.com/sylabs/singularity/v4/internal/pkg/remote/endpoint"
	ocilauncher "github.com/sylabs/singularity/v4/internal/pkg/runtime/launcher/oci"
	"github.com/sylabs/singularity/v4/internal/pkg/util/fs"
	"github.com/sylabs/singularity/v4/internal/pkg/util/rootless"
	"github.com/sylabs/singularity/v4/pkg/cmdline"
	clicallback "github.com/sylabs/singularity/v4/pkg/plugin/callback/cli"
	"github.com/sylabs/singularity/v4/pkg/syfs"
	"github.com/sylabs/singularity/v4/pkg/sylog"
	"github.com/sylabs/singularity/v4/pkg/util/singularityconf"
	"golang.org/x/term"
)

// cmdInits holds all the init function to be called
// for commands/flags registration.
var cmdInits = make([]func(*cmdline.CommandManager), 0)

// CurrentUser holds the current user account information
var CurrentUser = getCurrentUser()

// currentRemoteEndpoint holds the current remote endpoint
var currentRemoteEndpoint *endpoint.Config

const (
	envPrefix = "SINGULARITY_"
)

// Top level options on the `singularity` root command.
var (
	debug   bool
	nocolor bool
	silent  bool
	verbose bool
	quiet   bool

	configurationFile string
)

// Common options used with multiple sub-commands.
var (
	// OCI Registry Authentication
	authConfig  authn.AuthConfig
	dockerLogin bool
	dockerHost  string
	noHTTPS     bool

	// Encryption Material
	encryptionPEMPath   string
	promptForPassphrase bool

	// Paths / file handling
	tmpDir         string
	forceOverwrite bool

	// Options controlling the unpacking of images to temporary sandboxes
	canUseTmpSandbox bool
	tmpSandbox       bool
	noTmpSandbox     bool

	// Use OCI runtime and OCI SIF?
	isOCI bool
	noOCI bool

	// Keep individual layers when creating / pulling an OCI-SIF?
	keepLayers bool

	// Platform for retrieving images
	arch     string
	platform string

	// Optional user requested authentication file for writing/reading OCI registry credentials
	reqAuthFile string
)

//
// Top level option flags
//

// -d|--debug
var singDebugFlag = cmdline.Flag{
	ID:           "singDebugFlag",
	Value:        &debug,
	DefaultValue: false,
	Name:         "debug",
	ShortHand:    "d",
	Usage:        "print debugging information (highest verbosity)",
	EnvKeys:      []string{"DEBUG"},
}

// --nocolor
var singNoColorFlag = cmdline.Flag{
	ID:           "singNoColorFlag",
	Value:        &nocolor,
	DefaultValue: false,
	Name:         "nocolor",
	Usage:        "print without color output (default False)",
}

// -s|--silent
var singSilentFlag = cmdline.Flag{
	ID:           "singSilentFlag",
	Value:        &silent,
	DefaultValue: false,
	Name:         "silent",
	ShortHand:    "s",
	Usage:        "only print errors",
}

// -q|--quiet
var singQuietFlag = cmdline.Flag{
	ID:           "singQuietFlag",
	Value:        &quiet,
	DefaultValue: false,
	Name:         "quiet",
	ShortHand:    "q",
	Usage:        "suppress normal output",
}

// -v|--verbose
var singVerboseFlag = cmdline.Flag{
	ID:           "singVerboseFlag",
	Value:        &verbose,
	DefaultValue: false,
	Name:         "verbose",
	ShortHand:    "v",
	Usage:        "print additional information",
}

// -c|--config
var singConfigFileFlag = cmdline.Flag{
	ID:           "singConfigFileFlag",
	Value:        &configurationFile,
	DefaultValue: buildcfg.SINGULARITY_CONF_FILE,
	Name:         "config",
	ShortHand:    "c",
	Usage:        "specify a configuration file (for root or unprivileged installation only)",
	EnvKeys:      []string{"CONFIG_FILE"},
}

//
// Common option flags for multiple subcommands
//

// --docker-username
var dockerUsernameFlag = cmdline.Flag{
	ID:            "dockerUsernameFlag",
	Value:         &authConfig.Username,
	DefaultValue:  "",
	Name:          "docker-username",
	Usage:         "specify a username for docker authentication",
	Hidden:        true,
	EnvKeys:       []string{"DOCKER_USERNAME"},
	WithoutPrefix: true,
}

// --docker-password
var dockerPasswordFlag = cmdline.Flag{
	ID:            "dockerPasswordFlag",
	Value:         &authConfig.Password,
	DefaultValue:  "",
	Name:          "docker-password",
	Usage:         "specify a password for docker authentication",
	Hidden:        true,
	EnvKeys:       []string{"DOCKER_PASSWORD"},
	WithoutPrefix: true,
}

// --docker-login
var dockerLoginFlag = cmdline.Flag{
	ID:           "dockerLoginFlag",
	Value:        &dockerLogin,
	DefaultValue: false,
	Name:         "docker-login",
	Usage:        "login to a Docker Repository interactively",
	EnvKeys:      []string{"DOCKER_LOGIN"},
}

// --docker-host
var dockerHostFlag = cmdline.Flag{
	ID:            "dockerHostFlag",
	Value:         &dockerHost,
	DefaultValue:  "",
	Name:          "docker-host",
	Usage:         "specify a custom Docker daemon host",
	EnvKeys:       []string{"DOCKER_HOST"},
	WithoutPrefix: true,
}

// --no-https
var commonNoHTTPSFlag = cmdline.Flag{
	ID:           "commonNoHTTPSFlag",
	Value:        &noHTTPS,
	DefaultValue: false,
	Name:         "no-https",
	Usage:        "use http instead of https for docker:// oras:// and library://<hostname>/... URIs",
	EnvKeys:      []string{"NOHTTPS", "NO_HTTPS"},
}

// --nohttps (deprecated)
var commonOldNoHTTPSFlag = cmdline.Flag{
	ID:           "commonOldNoHTTPSFlag",
	Value:        &noHTTPS,
	DefaultValue: false,
	Name:         "nohttps",
	Deprecated:   "use --no-https",
	Usage:        "use http instead of https for docker:// oras:// and library://<hostname>/... URIs",
}

// --passphrase
var commonPromptForPassphraseFlag = cmdline.Flag{
	ID:           "commonPromptForPassphraseFlag",
	Value:        &promptForPassphrase,
	DefaultValue: false,
	Name:         "passphrase",
	Usage:        "prompt for an encryption passphrase",
}

// --pem-path
var commonPEMFlag = cmdline.Flag{
	ID:           "actionEncryptionPEMPath",
	Value:        &encryptionPEMPath,
	DefaultValue: "",
	Name:         "pem-path",
	Usage:        "enter an path to a PEM formatted RSA key for an encrypted container",
}

// -F|--force
var commonForceFlag = cmdline.Flag{
	ID:           "commonForceFlag",
	Value:        &forceOverwrite,
	DefaultValue: false,
	Name:         "force",
	ShortHand:    "F",
	Usage:        "overwrite an image file if it exists",
	EnvKeys:      []string{"FORCE"},
}

// --tmpdir
var commonTmpDirFlag = cmdline.Flag{
	ID:           "commonTmpDirFlag",
	Value:        &tmpDir,
	DefaultValue: os.TempDir(),
	Hidden:       true,
	Name:         "tmpdir",
	Usage:        "specify a temporary directory to use for build",
	EnvKeys:      []string{"TMPDIR"},
}

// --oci
var commonOCIFlag = cmdline.Flag{
	ID:           "actionOCI",
	Value:        &isOCI,
	DefaultValue: false,
	Name:         "oci",
	Usage:        "Launch container with OCI runtime (experimental)",
	EnvKeys:      []string{"OCI"},
}

// --no-oci
var commonNoOCIFlag = cmdline.Flag{
	ID:           "commonNoOCI",
	Value:        &noOCI,
	DefaultValue: false,
	Name:         "no-oci",
	Usage:        "Launch container with native runtime",
	EnvKeys:      []string{"NO_OCI"},
}

// --keep-layers
var commonKeepLayersFlag = cmdline.Flag{
	ID:           "keepLayers",
	Value:        &keepLayers,
	DefaultValue: false,
	Name:         "keep-layers",
	Usage:        "Keep layers when creating an OCI-SIF. Do not squash to a single layer.",
	EnvKeys:      []string{"KEEP_LAYERS"},
}

// --tmp-sandbox
var actionTmpSandbox = cmdline.Flag{
	ID:           "actionTmpSandbox",
	Value:        &tmpSandbox,
	DefaultValue: false,
	Name:         "tmp-sandbox",
	Usage:        "Forces unpacking of images into temporary sandbox dirs when a kernel or FUSE mount would otherwise be used.",
	EnvKeys:      []string{"TMP_SANDBOX"},
}

// --no-tmp-sandbox
var actionNoTmpSandbox = cmdline.Flag{
	ID:           "actionNoTmpSandbox",
	Value:        &noTmpSandbox,
	DefaultValue: false,
	Name:         "no-tmp-sandbox",
	Usage:        "Prohibits unpacking of images into temporary sandbox dirs",
	EnvKeys:      []string{"NO_TMP_SANDBOX"},
}

// --arch
var commonArchFlag = cmdline.Flag{
	ID:           "commonArchFlag",
	Value:        &arch,
	DefaultValue: "",
	Name:         "arch",
	Usage:        "architecture to use when pulling images",
	EnvKeys:      []string{"PULL_ARCH", "ARCH"},
}

// --platform
var commonPlatformFlag = cmdline.Flag{
	ID:           "commonPlatformFlag",
	Value:        &platform,
	DefaultValue: "",
	Name:         "platform",
	Usage:        "platform (OS/Architecture/Variant) to use when pulling images",
	EnvKeys:      []string{"PLATFORM"},
}

// --authfile
var commonAuthFileFlag = cmdline.Flag{
	ID:           "commonAuthFileFlag",
	Value:        &reqAuthFile,
	DefaultValue: "",
	Name:         "authfile",
	Usage:        "Docker-style authentication file to use for writing/reading OCI registry credentials",
	EnvKeys:      []string{"AUTHFILE"},
}

func getCurrentUser() *user.User {
	usr, err := user.Current()
	if err != nil {
		sylog.Fatalf("Couldn't determine user account information: %v", err)
	}
	return usr
}

func addCmdInit(cmdInit func(*cmdline.CommandManager)) {
	cmdInits = append(cmdInits, cmdInit)
}

func setSylogMessageLevel() {
	var level int

	if debug {
		level = 5
		// Propagate debug flag to nested `singularity` calls.
		os.Setenv("SINGULARITY_DEBUG", "1")
	} else if verbose {
		level = 4
	} else if quiet {
		level = -1
	} else if silent {
		level = -3
	} else {
		level = 1
	}

	color := true //nolint:staticcheck
	if nocolor || !term.IsTerminal(2) {
		color = false
	}

	sylog.SetLevel(level, color)
}

// handleRemoteConf will make sure your 'remote.yaml' config file
// has the correct permission.
func handleRemoteConf(remoteConfFile string) error {
	// Only check the permission if it exists.
	if fs.IsFile(remoteConfFile) {
		sylog.Debugf("Ensuring file permission of 0600 on %s", remoteConfFile)
		if err := fs.EnsureFileWithPermission(remoteConfFile, 0o600); err != nil {
			return fmt.Errorf("unable to correct the permission on %s: %w", remoteConfFile, err)
		}
	}
	return nil
}

// handleConfDir tries to create the user's configuration directory and handles
// messages and/or errors.
func handleConfDir(confDir string) error {
	if err := fs.Mkdir(confDir, 0o700); err != nil {
		if os.IsExist(err) {
			sylog.Debugf("%s already exists. Not creating.", confDir)
			fi, err := os.Stat(confDir)
			if err != nil {
				return fmt.Errorf("failed to retrieve information for %s: %s", confDir, err)
			}
			if fi.Mode().Perm() != 0o700 {
				sylog.Debugf("Enforce permission 0700 on %s", confDir)
				// enforce permission on user configuration directory
				if err := os.Chmod(confDir, 0o700); err != nil {
					// best effort as chmod could fail for various reasons (eg: readonly FS)
					sylog.Warningf("Couldn't enforce permission 0700 on %s: %s", confDir, err)
				}
			}
		} else {
			sylog.Debugf("Could not create %s: %s", confDir, err)
		}
	} else {
		sylog.Debugf("Created %s", confDir)
	}
	return nil
}

func persistentPreRun(*cobra.Command, []string) error {
	setSylogMessageLevel()
	sylog.Debugf("Singularity version: %s", buildcfg.PACKAGE_VERSION)

	if os.Geteuid() != 0 && buildcfg.SINGULARITY_SUID_INSTALL == 1 {
		if configurationFile != singConfigFileFlag.DefaultValue {
			return fmt.Errorf("--config requires to be root or an unprivileged installation")
		}
	}

	sylog.Debugf("Parsing configuration file %s", configurationFile)
	config, err := singularityconf.Parse(configurationFile)
	if err != nil {
		return fmt.Errorf("couldn't parse configuration file %s: %s", configurationFile, err)
	}
	singularityconf.SetCurrentConfig(config)

	// Honor 'oci mode' in singularity.conf, and allow negation with `--no-oci`.
	if isOCI && noOCI {
		return fmt.Errorf("--oci and --no-oci cannot be used together")
	}
	isOCI = isOCI || config.OCIMode
	if noOCI {
		isOCI = false
	}

	// --keep-layers is only valid in OCI mode, as native SIFs do not hold layers.
	if keepLayers && !isOCI {
		sylog.Fatalf("--keep-layers is only supported when creating OCI-SIF images (--oci mode)")
	}

	// Honor 'tmp sandbox' in singularity.conf, and allow negation with
	// `--no-tmp-sandbox`.
	canUseTmpSandbox = config.TmpSandboxAllowed
	if noTmpSandbox {
		canUseTmpSandbox = false
	}

	// If we need to enter a namespace (oci-mode) do the re-exec now, before any
	// other handling happens.
	if err := maybeReExec(); err != nil {
		return err
	}

	// Handle the config dir (~/.singularity),
	// then check the remove conf file permission.
	if err := handleConfDir(syfs.ConfigDir()); err != nil {
		return fmt.Errorf("while handling config dir: %w", err)
	}
	if err := handleRemoteConf(syfs.RemoteConf()); err != nil {
		return fmt.Errorf("while handling remote config: %w", err)
	}
	return nil
}

// Init initializes and registers all singularity commands.
func Init(loadPlugins bool) {
	cmdManager := cmdline.NewCommandManager(singularityCmd)

	singularityCmd.Flags().SetInterspersed(false)
	singularityCmd.PersistentFlags().SetInterspersed(false)

	templateFuncs := template.FuncMap{
		"TraverseParentsUses": TraverseParentsUses,
	}
	cobra.AddTemplateFuncs(templateFuncs)

	singularityCmd.SetHelpTemplate(docs.HelpTemplate)
	singularityCmd.SetUsageTemplate(docs.UseTemplate)

	vt := fmt.Sprintf("%s version {{printf \"%%s\" .Version}}\n", buildcfg.PACKAGE_NAME)
	singularityCmd.SetVersionTemplate(vt)

	// set persistent pre run function here to avoid initialization loop error
	singularityCmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		if err := cmdManager.UpdateCmdFlagFromEnv(singularityCmd, envPrefix); err != nil {
			sylog.Fatalf("While parsing global environment variables: %s", err)
		}
		if err := cmdManager.UpdateCmdFlagFromEnv(cmd, envPrefix); err != nil {
			sylog.Fatalf("While parsing environment variables: %s", err)
		}
		if err := persistentPreRun(cmd, args); err != nil {
			sylog.Fatalf("While initializing: %s", err)
		}
		return nil
	}

	cmdManager.RegisterFlagForCmd(&singDebugFlag, singularityCmd)
	cmdManager.RegisterFlagForCmd(&singNoColorFlag, singularityCmd)
	cmdManager.RegisterFlagForCmd(&singSilentFlag, singularityCmd)
	cmdManager.RegisterFlagForCmd(&singQuietFlag, singularityCmd)
	cmdManager.RegisterFlagForCmd(&singVerboseFlag, singularityCmd)
	cmdManager.RegisterFlagForCmd(&singConfigFileFlag, singularityCmd)

	cmdManager.RegisterCmd(VersionCmd)

	// register all others commands/flags
	for _, cmdInit := range cmdInits {
		cmdInit(cmdManager)
	}

	// load plugins and register commands/flags if any
	//nolint:forcetypeassert
	if loadPlugins {
		callbackType := (clicallback.Command)(nil)
		callbacks, err := plugin.LoadCallbacks(callbackType)
		if err != nil {
			sylog.Fatalf("Failed to load plugins callbacks '%T': %s", callbackType, err)
		}
		for _, c := range callbacks {
			c.(clicallback.Command)(cmdManager)
		}
	}

	// any error reported by command manager is considered as fatal
	cliErrors := len(cmdManager.GetError())
	if cliErrors > 0 {
		for _, e := range cmdManager.GetError() {
			sylog.Errorf("%s", e)
		}
		sylog.Fatalf("CLI command manager reported %d error(s)", cliErrors)
	}
}

// singularityCmd is the base command when called without any subcommands
var singularityCmd = &cobra.Command{
	TraverseChildren:      true,
	DisableFlagsInUseLine: true,
	RunE: func(_ *cobra.Command, _ []string) error {
		return cmdline.CommandError("invalid command")
	},

	Use:           docs.SingularityUse,
	Version:       buildcfg.PACKAGE_VERSION,
	Short:         docs.SingularityShort,
	Long:          docs.SingularityLong,
	Example:       docs.SingularityExample,
	SilenceErrors: true,
	SilenceUsage:  true,
}

// RootCmd returns the root singularity cobra command.
func RootCmd() *cobra.Command {
	return singularityCmd
}

// ExecuteSingularity adds all child commands to the root command and sets
// flags appropriately. This is called by main.main(). It only needs to happen
// once to the root command (singularity).
func ExecuteSingularity() {
	loadPlugins := true

	// we avoid to load installed plugins to not double load
	// them during execution of plugin compile and plugin install
	args := os.Args
	if len(args) > 1 {
		loadPlugins = !strings.HasPrefix(args[1], "plugin")
	}

	Init(loadPlugins)

	// Setup a cancellable context that will trap Ctrl-C / SIGINT
	ctx := context.Background()
	ctx, cancel := context.WithCancel(ctx)
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	defer func() {
		signal.Stop(c)
		cancel()
	}()
	go func() {
		select {
		case <-c:
			sylog.Debugf("User requested cancellation with interrupt")
			cancel()
		case <-ctx.Done():
		}
	}()

	if err := singularityCmd.ExecuteContext(ctx); err != nil {
		// Find the subcommand to display more useful help, and the correct
		// subcommand name in messages - i.e. 'run' not 'singularity'
		// This is required because we previously used ExecuteC that returns the
		// subcommand... but there is no ExecuteC that variant accepts a context.
		subCmd, _, subCmdErr := singularityCmd.Find(args[1:])
		if subCmdErr != nil {
			singularityCmd.Printf("Error: %v\n\n", subCmdErr)
		}

		name := subCmd.Name()
		switch err.(type) {
		case cmdline.FlagError:
			usage := subCmd.Flags().FlagUsagesWrapped(getColumns())
			singularityCmd.Printf("Error for command %q: %s\n\n", name, err)
			singularityCmd.Printf("Options for %s command:\n\n%s\n", name, usage)
		case cmdline.CommandError:
			singularityCmd.Println(subCmd.UsageString())
		default:
			singularityCmd.Printf("Error for command %q: %s\n\n", name, err)
			singularityCmd.Println(subCmd.UsageString())
		}
		singularityCmd.Printf("Run '%s --help' for more detailed usage information.\n",
			singularityCmd.CommandPath())
		os.Exit(1)
	}
}

// GenBashCompletion writes the bash completion file to w.
func GenBashCompletion(w io.Writer) error {
	Init(false)
	return singularityCmd.GenBashCompletion(w)
}

// TraverseParentsUses walks the parent commands and outputs a properly formatted use string
func TraverseParentsUses(cmd *cobra.Command) string {
	if cmd.HasParent() {
		return TraverseParentsUses(cmd.Parent()) + cmd.Use + " "
	}

	return cmd.Use + " "
}

// VersionCmd displays installed singularity version
var VersionCmd = &cobra.Command{
	DisableFlagsInUseLine: true,
	Run: func(_ *cobra.Command, _ []string) {
		fmt.Println(buildcfg.PACKAGE_VERSION)
	},

	Use:   "version",
	Short: "Show the version for Singularity",
}

func loadRemoteConf(filepath string) (*remote.Config, error) {
	f, err := os.OpenFile(filepath, os.O_RDONLY, 0o600)
	if err != nil {
		return nil, fmt.Errorf("while opening remote config file: %s", err)
	}
	defer f.Close()

	c, err := remote.ReadFrom(f)
	if err != nil {
		return nil, fmt.Errorf("while parsing remote config data: %s", err)
	}

	return c, nil
}

// sylabsRemote returns the remote in use or an error
func sylabsRemote() (*endpoint.Config, error) {
	var c *remote.Config

	// try to load both remotes, check for errors, sync if both exist,
	// if neither exist return errNoDefault to return to old auth behavior
	cSys, sysErr := loadRemoteConf(remote.SystemConfigPath)
	cUsr, usrErr := loadRemoteConf(syfs.RemoteConf())
	if sysErr != nil && usrErr != nil {
		return endpoint.DefaultEndpointConfig, nil
	} else if sysErr != nil {
		c = cUsr
	} else if usrErr != nil {
		c = cSys
	} else {
		// sync cUsr with system config cSys
		if err := cUsr.SyncFrom(cSys); err != nil {
			return nil, err
		}
		c = cUsr
	}

	ep, err := c.GetDefault()
	if err == remote.ErrNoDefault {
		// all remotes have been deleted, fix that by returning
		// the default remote endpoint to avoid side effects when
		// pulling from library or with remote build
		if len(c.Remotes) == 0 {
			return endpoint.DefaultEndpointConfig, nil
		}
		// otherwise notify users about available endpoints and
		// invite them to select one of them
		help := "use 'singularity remote use <endpoint>', available endpoints are: "
		endpoints := make([]string, 0, len(c.Remotes))
		for name := range c.Remotes {
			endpoints = append(endpoints, name)
		}
		help += strings.Join(endpoints, ", ")
		return nil, fmt.Errorf("no default endpoint set: %s", help)
	}

	return ep, err
}

func singularityExec(image string, args []string) (string, error) {
	// Record from stdout and store as a string to return as the contents of the file.
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	abspath, err := filepath.Abs(image)
	if err != nil {
		return "", fmt.Errorf("while determining absolute path for %s: %v", image, err)
	}

	// re-use singularity exec to grab image file content,
	// we reduce binds to the bare minimum with options below
	cmdArgs := []string{"exec", "--contain", "--no-home", "--no-nv", "--no-rocm", abspath}
	cmdArgs = append(cmdArgs, args...)

	singularityCmd := filepath.Join(buildcfg.BINDIR, "singularity")

	cmd := exec.Command(singularityCmd, cmdArgs...)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	// move to the root to not bind the current working directory
	// while inspecting the image
	cmd.Dir = "/"

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("unable to process command: %s: error output:\n%s", err, stderr.String())
	}

	return stdout.String(), nil
}

// CheckRoot ensures that a command is executed with root privileges.
func CheckRoot(cmd *cobra.Command, _ []string) {
	if os.Geteuid() != 0 {
		sylog.Fatalf("%q command requires root privileges", cmd.CommandPath())
	}
}

// CheckRootOrUnpriv ensures that a command is executed with root
// privileges or that Singularity is installed unprivileged.
func CheckRootOrUnpriv(cmd *cobra.Command, _ []string) {
	if os.Geteuid() != 0 && buildcfg.SINGULARITY_SUID_INSTALL == 1 {
		sylog.Fatalf("%q command requires root privileges or an unprivileged installation", cmd.CommandPath())
	}
}

// getKeyServerClientOpts returns client options for keyserver access.
// A "" value for uri will return client options for the current endpoint.
// A specified uri will return client options for that keyserver.
func getKeyserverClientOpts(uri string, op endpoint.KeyserverOp) ([]scskeyclient.Option, error) {
	if currentRemoteEndpoint == nil {
		var err error

		// if we can load config and if default endpoint is set, use that
		// otherwise fall back on regular authtoken and URI behavior
		currentRemoteEndpoint, err = sylabsRemote()
		if err != nil {
			return nil, fmt.Errorf("unable to load remote configuration: %v", err)
		}
	}
	if currentRemoteEndpoint == endpoint.DefaultEndpointConfig {
		sylog.Warningf("No default remote in use, falling back to default keyserver: %s", endpoint.SCSDefaultKeyserverURI)
	}

	return currentRemoteEndpoint.KeyserverClientOpts(uri, op)
}

// getLibraryClientConfig returns client config for library server access.
// A "" value for uri will return client config for the current endpoint.
// A specified uri will return client options for that library server.
func getLibraryClientConfig(uri string) (*scslibclient.Config, error) {
	if currentRemoteEndpoint == nil {
		var err error

		// if we can load config and if default endpoint is set, use that
		// otherwise fall back on regular authtoken and URI behavior
		currentRemoteEndpoint, err = sylabsRemote()
		if err != nil {
			return nil, fmt.Errorf("unable to load remote configuration: %v", err)
		}
	}
	if currentRemoteEndpoint == endpoint.DefaultEndpointConfig {
		sylog.Warningf("No default remote in use, falling back to default library: %s", endpoint.SCSDefaultLibraryURI)
	}

	return currentRemoteEndpoint.LibraryClientConfig(uri)
}

// getBuilderClientConfig returns the base URI and auth token to use for build server access. A ""
// value for uri will use the current endpoint. A specified uri will return client options for that
// build server.
func getBuilderClientConfig(uri string) (baseURI, authToken string, err error) {
	if currentRemoteEndpoint == nil {
		var err error

		// if we can load config and if default endpoint is set, use that
		// otherwise fall back on regular authtoken and URI behavior
		currentRemoteEndpoint, err = sylabsRemote()
		if err != nil {
			return "", "", fmt.Errorf("unable to load remote configuration: %v", err)
		}
	}
	if currentRemoteEndpoint == endpoint.DefaultEndpointConfig {
		sylog.Warningf("No default remote in use, falling back to default builder: %s", endpoint.SCSDefaultBuilderURI)
	}

	return currentRemoteEndpoint.BuilderClientConfig(uri)
}

func maybeReExec() error {
	sylog.Debugf("Checking whether to re-exec")
	// The OCI runtime must always be launched where the effective uid/gid is 0 (root or fake-root).
	if isOCI && !rootless.InNS() {
		// If we need to, enter a new cgroup now, to workaround an issue with crun container cgroup creation (#1538).
		if err := ocilauncher.CrunNestCgroup(); err != nil {
			return fmt.Errorf("while applying crun cgroup workaround: %w", err)
		}
		// If we are root already, run the launcher in a new mount namespace only.
		if os.Geteuid() == 0 {
			return rootless.RunInMountNS(os.Args[1:])
		}
		// If we are not root, re-exec in a root-mapped user namespace and mount namespace.
		return rootless.ExecWithFakeroot(os.Args[1:])
	}
	return nil
}

// getOCIPlatform returns the appropriate OCI platform to use according to `--arch` and `--platform`
func getOCIPlatform() ggcrv1.Platform {
	var (
		p   *ggcrv1.Platform
		err error
	)
	if arch != "" && platform != "" {
		err = fmt.Errorf("--arch and --platform cannot be used together")
	}
	if arch == "" && platform == "" {
		p, err = ociplatform.DefaultPlatform()
	}
	if arch != "" {
		p, err = ociplatform.PlatformFromArch(arch)
	}
	if platform != "" {
		p, err = ociplatform.PlatformFromString(platform)
	}
	if err != nil {
		sylog.Fatalf("%v", err)
	}
	return *p
}
