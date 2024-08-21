// Copyright (c) 2020, Control Command Inc. All rights reserved.
// Copyright (c) 2018-2023, Sylabs Inc. All rights reserved.
// Copyright (c) Contributors to the Apptainer project, established as
//   Apptainer a Series of LF Projects LLC.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package cli

import (
	"fmt"
	"os"
	"runtime"
	"syscall"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/spf13/cobra"
	"github.com/sylabs/singularity/v4/docs"
	"github.com/sylabs/singularity/v4/internal/pkg/util/fs"
	"github.com/sylabs/singularity/v4/internal/pkg/util/interactive"
	"github.com/sylabs/singularity/v4/pkg/build/types"
	"github.com/sylabs/singularity/v4/pkg/build/types/parser"
	"github.com/sylabs/singularity/v4/pkg/cmdline"
	"github.com/sylabs/singularity/v4/pkg/image"
	"github.com/sylabs/singularity/v4/pkg/sylog"
	"golang.org/x/term"
)

var buildArgs struct {
	sections        []string
	bindPaths       []string
	mounts          []string
	arch            string
	builderURL      string
	libraryURL      string
	keyServerURL    string
	webURL          string
	detached        bool
	encrypt         bool
	fakeroot        bool
	fixPerms        bool
	isJSON          bool
	noCleanUp       bool
	noTest          bool
	noSetgroups     bool
	remote          bool
	sandbox         bool
	update          bool
	nvidia          bool
	nvccli          bool
	rocm            bool
	writableTmpfs   bool     // For test section only
	buildVarArgs    []string // Variables passed to build procedure.
	buildVarArgFile string   // Variables file passed to build procedure.
}

// -s|--sandbox
var buildSandboxFlag = cmdline.Flag{
	ID:           "buildSandboxFlag",
	Value:        &buildArgs.sandbox,
	DefaultValue: false,
	Name:         "sandbox",
	ShortHand:    "s",
	Usage:        "build image as sandbox format (chroot directory structure)",
	EnvKeys:      []string{"SANDBOX"},
}

// --section
var buildSectionFlag = cmdline.Flag{
	ID:           "buildSectionFlag",
	Value:        &buildArgs.sections,
	DefaultValue: []string{"all"},
	Name:         "section",
	Usage:        "only run specific section(s) of deffile (setup, post, files, environment, test, labels, none)",
	EnvKeys:      []string{"SECTION"},
}

// --json
var buildJSONFlag = cmdline.Flag{
	ID:           "buildJSONFlag",
	Value:        &buildArgs.isJSON,
	DefaultValue: false,
	Name:         "json",
	Usage:        "interpret build definition as JSON",
	EnvKeys:      []string{"JSON"},
}

// -u|--update
var buildUpdateFlag = cmdline.Flag{
	ID:           "buildUpdateFlag",
	Value:        &buildArgs.update,
	DefaultValue: false,
	Name:         "update",
	ShortHand:    "u",
	Usage:        "run definition over existing container (skips header)",
	EnvKeys:      []string{"UPDATE"},
}

// -T|--notest
var buildNoTestFlag = cmdline.Flag{
	ID:           "buildNoTestFlag",
	Value:        &buildArgs.noTest,
	DefaultValue: false,
	Name:         "notest",
	ShortHand:    "T",
	Usage:        "build without running tests in %test section",
	EnvKeys:      []string{"NOTEST"},
}

// -r|--remote
var buildRemoteFlag = cmdline.Flag{
	ID:           "buildRemoteFlag",
	Value:        &buildArgs.remote,
	DefaultValue: false,
	Name:         "remote",
	ShortHand:    "r",
	Usage:        "build image remotely (does not require root)",
	EnvKeys:      []string{"REMOTE"},
}

// --arch
var buildArchFlag = cmdline.Flag{
	ID:           "buildArchFlag",
	Value:        &buildArgs.arch,
	DefaultValue: runtime.GOARCH,
	Name:         "arch",
	Usage:        "architecture for remote build",
	EnvKeys:      []string{"BUILD_ARCH"},
}

// -d|--detached
var buildDetachedFlag = cmdline.Flag{
	ID:           "buildDetachedFlag",
	Value:        &buildArgs.detached,
	DefaultValue: false,
	Name:         "detached",
	ShortHand:    "d",
	Usage:        "submit build job and print build ID (no real-time logs and requires --remote)",
	EnvKeys:      []string{"DETACHED"},
}

// --builder
var buildBuilderFlag = cmdline.Flag{
	ID:           "buildBuilderFlag",
	Value:        &buildArgs.builderURL,
	DefaultValue: "",
	Name:         "builder",
	Usage:        "remote Build Service URL, setting this implies --remote",
	EnvKeys:      []string{"BUILDER"},
}

// --library
var buildLibraryFlag = cmdline.Flag{
	ID:           "buildLibraryFlag",
	Value:        &buildArgs.libraryURL,
	DefaultValue: "",
	Name:         "library",
	Usage:        "container Library URL",
	EnvKeys:      []string{"LIBRARY"},
}

// --disable-cache
var buildDisableCacheFlag = cmdline.Flag{
	ID:           "buildDisableCacheFlag",
	Value:        &disableCache,
	DefaultValue: false,
	Name:         "disable-cache",
	Usage:        "do not use cache or create cache",
	EnvKeys:      []string{"DISABLE_CACHE"},
}

// --no-cleanup
var buildNoCleanupFlag = cmdline.Flag{
	ID:           "buildNoCleanupFlag",
	Value:        &buildArgs.noCleanUp,
	DefaultValue: false,
	Name:         "no-cleanup",
	Usage:        "do NOT clean up bundle after failed build, can be helpful for debugging",
	EnvKeys:      []string{"NO_CLEANUP"},
}

// --fakeroot
var buildFakerootFlag = cmdline.Flag{
	ID:           "buildFakerootFlag",
	Value:        &buildArgs.fakeroot,
	DefaultValue: false,
	Name:         "fakeroot",
	ShortHand:    "f",
	Usage:        "build using user namespace to fake root user (requires a privileged installation)",
	EnvKeys:      []string{"FAKEROOT"},
}

// --no-setgroups
var buildNoSetgroupsFlag = cmdline.Flag{
	ID:           "buildNoSetgroupsFlag",
	Value:        &buildArgs.noSetgroups,
	DefaultValue: false,
	Name:         "no-setgroups",
	Usage:        "disable setgroups when entering --fakeroot user namespace",
	EnvKeys:      []string{"NO_SETGROUPS"},
}

// -e|--encrypt
var buildEncryptFlag = cmdline.Flag{
	ID:           "buildEncryptFlag",
	Value:        &buildArgs.encrypt,
	DefaultValue: false,
	Name:         "encrypt",
	ShortHand:    "e",
	Usage:        "build an image with an encrypted file system",
}

// TODO: Deprecate at 3.6, remove at 3.8
// --fix-perms
var buildFixPermsFlag = cmdline.Flag{
	ID:           "fixPermsFlag",
	Value:        &buildArgs.fixPerms,
	DefaultValue: false,
	Name:         "fix-perms",
	Usage:        "ensure owner has rwX permissions on all container content for oci/docker sources",
	EnvKeys:      []string{"FIXPERMS"},
}

// --nv
var buildNvFlag = cmdline.Flag{
	ID:           "nvFlag",
	Value:        &buildArgs.nvidia,
	DefaultValue: false,
	Name:         "nv",
	Usage:        "inject host Nvidia libraries during build for post and test sections (not supported with remote build)",
	EnvKeys:      []string{"NV"},
}

// --nvccli
var buildNvCCLIFlag = cmdline.Flag{
	ID:           "buildNvCCLIFlag",
	Value:        &buildArgs.nvccli,
	DefaultValue: false,
	Name:         "nvccli",
	Usage:        "use nvidia-container-cli for GPU setup (experimental)",
	EnvKeys:      []string{"NVCCLI"},
}

// --rocm
var buildRocmFlag = cmdline.Flag{
	ID:           "rocmFlag",
	Value:        &buildArgs.rocm,
	DefaultValue: false,
	Name:         "rocm",
	Usage:        "inject host Rocm libraries during build for post and test sections (not supported with remote build)",
	EnvKeys:      []string{"ROCM"},
}

// -B|--bind
var buildBindFlag = cmdline.Flag{
	ID:           "buildBindFlag",
	Value:        &buildArgs.bindPaths,
	DefaultValue: []string{},
	Name:         "bind",
	ShortHand:    "B",
	Usage: "a user-bind path specification. spec has the format src[:dest[:opts]]," +
		"where src and dest are outside and inside paths. If dest is not given," +
		"it is set equal to src. Mount options ('opts') may be specified as 'ro'" +
		"(read-only) or 'rw' (read/write, which is the default)." +
		"Multiple bind paths can be given by a comma separated list. (not supported with remote build)",
	EnvKeys:    []string{"BIND", "BINDPATH"},
	EnvHandler: cmdline.EnvAppendValue,
}

// --mount
var buildMountFlag = cmdline.Flag{
	ID:           "buildMountFlag",
	Value:        &buildArgs.mounts,
	DefaultValue: []string{},
	Name:         "mount",
	Usage:        "a mount specification e.g. 'type=bind,source=/opt,destination=/hostopt'.",
	EnvKeys:      []string{"MOUNT"},
	Tag:          "<spec>",
	EnvHandler:   cmdline.EnvAppendValue,
	StringArray:  true,
}

// --writable-tmpfs
var buildWritableTmpfsFlag = cmdline.Flag{
	ID:           "buildWritableTmpfsFlag",
	Value:        &buildArgs.writableTmpfs,
	DefaultValue: false,
	Name:         "writable-tmpfs",
	Usage:        "during the %test section, makes the file system accessible as read-write with non persistent data (with overlay support only)",
	EnvKeys:      []string{"WRITABLE_TMPFS"},
}

// --build-arg
var buildVarArgsFlag = cmdline.Flag{
	ID:           "buildVarArgsFlag",
	Value:        &buildArgs.buildVarArgs,
	DefaultValue: []string{},
	Name:         "build-arg",
	Usage:        "provide value to replace {{ variable }} entries in build definition file, in variable=value format",
}

// --build-arg-file
var buildVarArgFileFlag = cmdline.Flag{
	ID:           "buildVarArgFileFlag",
	Value:        &buildArgs.buildVarArgFile,
	DefaultValue: "",
	Name:         "build-arg-file",
	Usage:        "specifies a file containing variable=value lines to replace '{{ variable }}' with value in build definition files",
}

func init() {
	addCmdInit(func(cmdManager *cmdline.CommandManager) {
		cmdManager.RegisterCmd(buildCmd)

		cmdManager.RegisterFlagForCmd(&buildArchFlag, buildCmd)
		cmdManager.RegisterFlagForCmd(&buildBuilderFlag, buildCmd)
		cmdManager.RegisterFlagForCmd(&buildDetachedFlag, buildCmd)
		cmdManager.RegisterFlagForCmd(&buildDisableCacheFlag, buildCmd)
		cmdManager.RegisterFlagForCmd(&buildEncryptFlag, buildCmd)
		cmdManager.RegisterFlagForCmd(&buildFakerootFlag, buildCmd)
		cmdManager.RegisterFlagForCmd(&buildNoSetgroupsFlag, buildCmd)
		cmdManager.RegisterFlagForCmd(&buildFixPermsFlag, buildCmd)
		cmdManager.RegisterFlagForCmd(&buildJSONFlag, buildCmd)
		cmdManager.RegisterFlagForCmd(&buildLibraryFlag, buildCmd)
		cmdManager.RegisterFlagForCmd(&buildNoCleanupFlag, buildCmd)
		cmdManager.RegisterFlagForCmd(&buildNoTestFlag, buildCmd)
		cmdManager.RegisterFlagForCmd(&buildRemoteFlag, buildCmd)
		cmdManager.RegisterFlagForCmd(&buildSandboxFlag, buildCmd)
		cmdManager.RegisterFlagForCmd(&buildSectionFlag, buildCmd)
		cmdManager.RegisterFlagForCmd(&buildUpdateFlag, buildCmd)
		cmdManager.RegisterFlagForCmd(&commonForceFlag, buildCmd)
		cmdManager.RegisterFlagForCmd(&commonNoHTTPSFlag, buildCmd)
		cmdManager.RegisterFlagForCmd(&commonTmpDirFlag, buildCmd)

		cmdManager.RegisterFlagForCmd(&dockerHostFlag, buildCmd)
		cmdManager.RegisterFlagForCmd(&dockerUsernameFlag, buildCmd)
		cmdManager.RegisterFlagForCmd(&dockerPasswordFlag, buildCmd)
		cmdManager.RegisterFlagForCmd(&dockerLoginFlag, buildCmd)

		cmdManager.RegisterFlagForCmd(&commonPromptForPassphraseFlag, buildCmd)
		cmdManager.RegisterFlagForCmd(&commonPEMFlag, buildCmd)

		cmdManager.RegisterFlagForCmd(&buildNvFlag, buildCmd)
		cmdManager.RegisterFlagForCmd(&buildNvCCLIFlag, buildCmd)
		cmdManager.RegisterFlagForCmd(&buildRocmFlag, buildCmd)
		cmdManager.RegisterFlagForCmd(&buildBindFlag, buildCmd)
		cmdManager.RegisterFlagForCmd(&buildMountFlag, buildCmd)
		cmdManager.RegisterFlagForCmd(&buildWritableTmpfsFlag, buildCmd)
		cmdManager.RegisterFlagForCmd(&buildVarArgsFlag, buildCmd)
		cmdManager.RegisterFlagForCmd(&buildVarArgFileFlag, buildCmd)

		cmdManager.RegisterFlagForCmd(&commonOCIFlag, buildCmd)
		cmdManager.RegisterFlagForCmd(&commonNoOCIFlag, buildCmd)

		cmdManager.RegisterFlagForCmd(&commonAuthFileFlag, buildCmd)
		cmdManager.RegisterFlagForCmd(&commonKeepLayersFlag, buildCmd)
	})
}

// buildCmd represents the build command.
var buildCmd = &cobra.Command{
	DisableFlagsInUseLine: true,
	Args:                  cobra.ExactArgs(2),

	Use:              docs.BuildUse,
	Short:            docs.BuildShort,
	Long:             docs.BuildLong,
	Example:          docs.BuildExample,
	PreRun:           preRun,
	Run:              runBuild,
	TraverseChildren: true,
}

func preRun(cmd *cobra.Command, _ []string) {
	if isOCI {
		if buildArgs.remote {
			sylog.Fatalf("Remote OCI builds from Dockerfiles are not supported.")
		}

		return
	}

	if buildArgs.noSetgroups && !buildArgs.fakeroot {
		sylog.Warningf("--no-setgroups only applies to --fakeroot builds")
	}

	if buildArgs.fakeroot && !buildArgs.remote {
		fakerootExec()
	}

	// Always perform remote build when builder flag is set
	if cmd.Flags().Lookup("builder").Changed {
		cmd.Flags().Lookup("remote").Value.Set("true")
	}
}

// checkBuildTarget makes sure output target doesn't exist, or is ok to overwrite.
// And checks that update flag will update an existing directory.
func checkBuildTarget(path string) error {
	abspath, err := fs.Abs(path)
	if err != nil {
		return fmt.Errorf("failed to get absolute path for %q: %v", path, err)
	}

	if !buildArgs.sandbox && buildArgs.update {
		return fmt.Errorf("only sandbox update is supported: --sandbox flag is missing")
	}
	if f, err := os.Stat(abspath); err == nil {
		if buildArgs.update && !f.IsDir() {
			return fmt.Errorf("only sandbox update is supported: %s is not a directory", abspath)
		}
		// check if the sandbox image being overwritten looks like a Singularity
		// image and inform users to check its content and use --force option if
		// the sandbox image is not a Singularity image
		if f.IsDir() && !forceOverwrite {
			files, err := os.ReadDir(abspath)
			if err != nil {
				return fmt.Errorf("could not read sandbox directory %s: %s", abspath, err)
			} else if len(files) > 0 {
				required := 0
				for _, f := range files {
					switch f.Name() {
					case ".singularity.d", "dev", "proc", "sys":
						required++
					}
				}
				if required != 4 {
					return fmt.Errorf("%s is not empty and is not a Singularity sandbox, check its content first and use --force if you want to overwrite it", abspath)
				}
			}
		}
		if !buildArgs.update && !forceOverwrite {
			// If non-interactive, die... don't try to prompt the user y/n
			if !term.IsTerminal(syscall.Stdin) {
				return fmt.Errorf("build target '%s' already exists. Use --force if you want to overwrite it", f.Name())
			}

			question := fmt.Sprintf("Build target '%s' already exists and will be deleted during the build process. Do you want to continue? [y/N] ", f.Name())

			img, err := image.Init(abspath, false)
			if err != nil {
				if err != image.ErrUnknownFormat {
					return fmt.Errorf("while determining '%s' format: %s", f.Name(), err)
				}
				// unknown image file format
				question = fmt.Sprintf("Build target '%s' may be a definition file or a text/binary file that will be overwritten. Do you still want to overwrite it? [y/N] ", f.Name())
			} else {
				img.File.Close()
			}

			input, err := interactive.AskYNQuestion("n", "%s", question)
			if err != nil {
				return fmt.Errorf("while reading the input: %s", err)
			}
			if input != "y" {
				return fmt.Errorf("stopping build")
			}
			forceOverwrite = true
		}
	} else if os.IsNotExist(err) && buildArgs.update && buildArgs.sandbox {
		return fmt.Errorf("could not update sandbox %s: doesn't exist", abspath)
	}
	return nil
}

// definitionFromSpec is specifically for parsing specs for the remote builder
// it uses a different version the definition struct and parser
func definitionFromSpec(spec string) (types.Definition, error) {
	// Try spec as URI first
	def, err := types.NewDefinitionFromURI(spec)
	if err == nil {
		return def, nil
	}

	// Try spec as local file
	var isValid bool
	isValid, err = parser.IsValidDefinition(spec)
	if err != nil {
		return types.Definition{}, err
	}

	if isValid {
		sylog.Debugf("Found valid definition: %s\n", spec)
		// File exists and contains valid definition
		var defFile *os.File
		defFile, err = os.Open(spec)
		if err != nil {
			return types.Definition{}, err
		}

		defer defFile.Close()

		return parser.ParseDefinitionFile(defFile)
	}

	// File exists and does NOT contain a valid definition
	// local image or sandbox
	def = types.Definition{
		Header: map[string]string{
			"bootstrap": "localimage",
			"from":      spec,
		},
	}

	return def, nil
}

// makeOCICredentials creates an *authn.AuthConfig that should be used for
// explicit OCI/Docker registry authentication when appropriate. If
// `--docker-login` has been specified then interactive authentication will be
// performed. If `--docker-login` has not been specified, and explicit
// credentials have not been supplied via env-vars/flags, then a nil AuthConfig
// will be returned.
func makeOCICredentials(cmd *cobra.Command) (*authn.AuthConfig, error) {
	usernameFlag := cmd.Flags().Lookup("docker-username")
	passwordFlag := cmd.Flags().Lookup("docker-password")

	var err error
	if dockerLogin {
		if !usernameFlag.Changed {
			authConfig.Username, err = interactive.AskQuestion("Enter Docker/OCI registry username: ")
			if err != nil {
				return &authConfig, err
			}
			usernameFlag.Value.Set(authConfig.Username)
			usernameFlag.Changed = true
		}

		authConfig.Password, err = interactive.AskQuestionNoEcho("Enter Docker / OCI registry password: ")
		if err != nil {
			return &authConfig, err
		}
		passwordFlag.Value.Set(authConfig.Password)
		passwordFlag.Changed = true
	}

	if usernameFlag.Changed || passwordFlag.Changed {
		return &authConfig, nil
	}

	// If a username / password have not been explicitly set, return a nil
	// pointer, which will mean containers/image falls back to looking for
	// .docker/config.json
	return nil, nil
}
