// Copyright (c) 2020, Control Command Inc. All rights reserved.
// Copyright (c) 2018-2025, Sylabs Inc. All rights reserved.
// Copyright (c) Contributors to the Apptainer project, established as
//   Apptainer a Series of LF Projects LLC.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package cli

import (
	"context"
	"fmt"
	"os"
	osExec "os/exec"
	"runtime"
	"strconv"
	"strings"
	"syscall"

	"github.com/ccoveille/go-safecast"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/spf13/cobra"
	keyclient "github.com/sylabs/scs-key-client/client"
	"github.com/sylabs/singularity/v4/internal/pkg/build"
	"github.com/sylabs/singularity/v4/internal/pkg/build/args"
	bkclient "github.com/sylabs/singularity/v4/internal/pkg/build/buildkit/client"
	"github.com/sylabs/singularity/v4/internal/pkg/build/remotebuilder"
	"github.com/sylabs/singularity/v4/internal/pkg/buildcfg"
	"github.com/sylabs/singularity/v4/internal/pkg/cache"
	"github.com/sylabs/singularity/v4/internal/pkg/ociplatform"
	"github.com/sylabs/singularity/v4/internal/pkg/remote/endpoint"
	fakerootConfig "github.com/sylabs/singularity/v4/internal/pkg/runtime/engine/fakeroot/config"
	"github.com/sylabs/singularity/v4/internal/pkg/util/bin"
	"github.com/sylabs/singularity/v4/internal/pkg/util/fs"
	"github.com/sylabs/singularity/v4/internal/pkg/util/interactive"
	"github.com/sylabs/singularity/v4/internal/pkg/util/rootless"
	"github.com/sylabs/singularity/v4/internal/pkg/util/starter"
	"github.com/sylabs/singularity/v4/internal/pkg/util/user"
	"github.com/sylabs/singularity/v4/pkg/build/types"
	"github.com/sylabs/singularity/v4/pkg/image"
	"github.com/sylabs/singularity/v4/pkg/runtime/engine/config"
	"github.com/sylabs/singularity/v4/pkg/sylog"
	"github.com/sylabs/singularity/v4/pkg/util/cryptkey"
)

func fakerootExec() {
	if buildArgs.nvccli && !buildArgs.noTest {
		sylog.Warningf("Due to writable-tmpfs limitations, %%test sections will fail with --nvccli & --fakeroot")
		sylog.Infof("Use -T / --notest to disable running tests during the build")
	}

	useSuid := buildcfg.SINGULARITY_SUID_INSTALL == 1

	short := "-" + buildFakerootFlag.ShortHand
	long := "--" + buildFakerootFlag.Name
	envKey := fmt.Sprintf("SINGULARITY_%s", buildFakerootFlag.EnvKeys[0])
	fakerootEnv := os.Getenv(envKey) != ""

	argsLen := len(os.Args) - 1
	if fakerootEnv {
		argsLen = len(os.Args)
		os.Unsetenv(envKey)
	}
	args := make([]string, argsLen)
	idx := 0
	for i, arg := range os.Args {
		if i == 0 {
			path, _ := osExec.LookPath(arg)
			arg = path
		}
		if arg != short && arg != long {
			args[idx] = arg
			idx++
		}
	}

	uid, err := safecast.ToUint32(os.Getuid())
	if err != nil {
		sylog.Fatalf("while getting uid: %v", err)
	}
	user, err := user.GetPwUID(uid)
	if err != nil {
		sylog.Fatalf("failed to retrieve user information: %s", err)
	}

	// Append the user's real UID/GID to the environment as _CONTAINERS_ROOTLESS_UID/GID.
	// This is required in fakeroot builds that may use containers/image 5.7 and above.
	// https://github.com/containers/image/issues/1066
	// https://github.com/containers/image/blob/master/internal/rootless/rootless.go
	os.Setenv(rootless.UIDEnv, strconv.Itoa(os.Getuid()))
	os.Setenv(rootless.GIDEnv, strconv.Itoa(os.Getgid()))

	engineConfig := &fakerootConfig.EngineConfig{
		Args:        args,
		Envs:        os.Environ(),
		Home:        user.Dir,
		BuildEnv:    true,
		NoSetgroups: buildArgs.noSetgroups,
	}

	cfg := &config.Common{
		EngineName:   fakerootConfig.Name,
		ContainerID:  "fakeroot",
		EngineConfig: engineConfig,
	}

	err = starter.Exec(
		"Singularity fakeroot",
		cfg,
		starter.UseSuid(useSuid),
	)
	sylog.Fatalf("%s", err)
}

func runBuild(cmd *cobra.Command, args []string) {
	if buildArgs.nvidia {
		if buildArgs.remote {
			sylog.Fatalf("--nv option is not supported for remote build")
		}
		if isOCI {
			sylog.Fatalf("--nv option is not supported for OCI builds from Dockerfiles")
		}
		os.Setenv("SINGULARITY_NV", "1")
	}
	if buildArgs.nvccli {
		if buildArgs.remote {
			sylog.Fatalf("--nvccli option is not supported for remote build")
		}
		if isOCI {
			sylog.Fatalf("--nvccli option is not supported for OCI builds from Dockerfiles")
		}
		os.Setenv("SINGULARITY_NVCCLI", "1")
	}
	if buildArgs.rocm {
		if buildArgs.remote {
			sylog.Fatalf("--rocm option is not supported for remote build")
		}
		if isOCI {
			sylog.Fatalf("--rocm option is not supported for OCI builds from Dockerfiles")
		}
		os.Setenv("SINGULARITY_ROCM", "1")
	}
	if len(buildArgs.bindPaths) > 0 {
		if buildArgs.remote {
			sylog.Fatalf("-B/--bind option is not supported for remote build")
		}
		if isOCI {
			sylog.Fatalf("-B/--bind option is not supported for OCI builds from Dockerfiles")
		}
		os.Setenv("SINGULARITY_BINDPATH", strings.Join(buildArgs.bindPaths, ","))
	}
	if len(buildArgs.mounts) > 0 {
		if buildArgs.remote {
			sylog.Fatalf("--mount option is not supported for remote build")
		}
		if isOCI {
			sylog.Fatalf("--mount option is not supported for OCI builds from Dockerfiles")
		}
		os.Setenv("SINGULARITY_MOUNT", strings.Join(buildArgs.mounts, "\n"))
	}
	if buildArgs.writableTmpfs {
		if buildArgs.remote {
			sylog.Fatalf("--writable-tmpfs option is not supported for remote build")
		}
		if buildArgs.fakeroot {
			sylog.Fatalf("--writable-tmpfs option is not supported for fakeroot build")
		}
		if isOCI {
			sylog.Fatalf("--writable-tmpfs option is not supported for OCI builds from Dockerfiles")
		}
		os.Setenv("SINGULARITY_WRITABLE_TMPFS", "1")
	}

	if cmd.Flags().Lookup("authfile").Changed && buildArgs.remote {
		sylog.Fatalf("Custom authfile is not supported for remote build")
	}

	if buildArgs.arch != runtime.GOARCH && !buildArgs.remote && !isOCI {
		sylog.Fatalf("Requested architecture (%s) does not match host (%s). Cannot build locally.", buildArgs.arch, runtime.GOARCH)
	}

	dest := args[0]
	spec := args[1]

	// Non-remote build with def file as source
	rootNeeded := !buildArgs.remote && fs.IsFile(spec) && !isImage(spec) && !isOCI

	if rootNeeded && syscall.Getuid() != 0 && !buildArgs.fakeroot {
		prootPath, err := bin.FindBin("proot")
		if err != nil {
			sylog.Fatalf("--remote, --fakeroot, or the proot command are required to build this source as a non-root user")
		}
		os.Setenv("SINGULARITY_PROOT", prootPath)
		sylog.Infof("Using proot to build unprivileged. Not all builds are supported. If build fails, use --remote or --fakeroot.")
	}

	// check if target collides with existing file
	if err := checkBuildTarget(dest); err != nil {
		sylog.Fatalf("While checking build target: %s", err)
	}

	if buildArgs.remote {
		runBuildRemote(cmd.Context(), cmd, dest, spec)
		return
	}

	authConf, err := makeOCICredentials(cmd)
	if err != nil {
		sylog.Fatalf("While creating Docker credentials: %v", err)
	}

	if isOCI {
		reqArch := ""
		if cmd.Flags().Lookup("arch").Changed {
			reqArch = buildArgs.arch
		}
		wd, err := os.Getwd()
		if err != nil {
			sylog.Fatalf("While trying to determine current dir: %v", err)
		}

		bkOpts := &bkclient.Opts{
			AuthConf:        authConf,
			ReqAuthFile:     reqAuthFile,
			BuildVarArgs:    buildArgs.buildVarArgs,
			BuildVarArgFile: buildArgs.buildVarArgFile,
			ReqArch:         reqArch,
			KeepLayers:      keepLayers,
			ContextDir:      wd,
			DisableCache:    disableCache,
		}
		if err := bkclient.Run(cmd.Context(), bkOpts, dest, spec); err != nil {
			sylog.Fatalf("%v", err)
		}
	} else {
		runBuildLocal(cmd.Context(), authConf, cmd, dest, spec)
	}

	sylog.Infof("Build complete: %s", dest)
}

func runBuildRemote(ctx context.Context, cmd *cobra.Command, dst, spec string) {
	// building encrypted containers on the remote builder is not currently supported
	if buildArgs.encrypt {
		sylog.Fatalf("Building encrypted container with the remote builder is not currently supported.")
	}

	if (len(buildArgs.buildVarArgs) > 1) || (buildArgs.buildVarArgFile != "") {
		sylog.Fatalf("The remote builder does not currently support build-argument substitution (--build-arg / --build-arg-file).")
	}

	// TODO - the keyserver config needs to go to the remote builder for fingerprint verification at
	// build time to be fully supported.

	lc, err := getLibraryClientConfig(buildArgs.libraryURL)
	if err != nil {
		sylog.Fatalf("Unable to get library client configuration: %v", err)
	}
	buildArgs.libraryURL = lc.BaseURL

	baseURI, authToken, err := getBuilderClientConfig(buildArgs.builderURL)
	if err != nil {
		sylog.Fatalf("Unable to get builder client configuration: %v", err)
	}
	buildArgs.builderURL = baseURI

	// To provide a web link to detached remote builds we need to know the web frontend URI.
	// We only know this working forward from a remote config, and not if the user has set custom
	// service URLs, since there is no straightforward foolproof way to work back from them to a
	// matching frontend URL.
	if !cmd.Flag("builder").Changed && !cmd.Flag("library").Changed {
		webURL, err := currentRemoteEndpoint.GetURL()
		if err != nil {
			sylog.Fatalf("Unable to find remote web URI %v", err)
		}
		buildArgs.webURL = webURL
	}

	// submitting a remote build requires a valid authToken
	if authToken == "" {
		sylog.Fatalf("Unable to submit build job: %v", remoteWarning)
	}

	def, err := definitionFromSpec(spec)
	if err != nil {
		sylog.Fatalf("Unable to build from %s: %v", spec, err)
	}

	// Ensure that the definition bootstrap source is valid before we submit a remote build
	if _, err := build.NewConveyorPacker(def); err != nil {
		sylog.Fatalf("Unable to build from %s: %v", spec, err)
	}
	if bs, ok := def.Header["bootstrap"]; ok && bs == "localimage" {
		sylog.Fatalf("Building from a \"localimage\" source with the remote builder is not supported.")
	}

	// path SIF from remote builder should be placed
	rbDst := dst
	if buildArgs.sandbox {
		if strings.HasPrefix(dst, "library://") {
			// image destination is the library.
			sylog.Fatalf("Library URI detected as destination, sandbox builds are incompatible with library destinations.")
		}

		// create temporary file to download sif
		f, err := os.CreateTemp(tmpDir, "remote-build-")
		if err != nil {
			sylog.Fatalf("Could not create temporary directory: %s", err)
		}
		f.Close()

		// override remote build destation to temporary file for conversion to a sandbox
		rbDst = f.Name()
		sylog.Debugf("Overriding remote build destination to temporary file: %s", rbDst)

		// remove downloaded sif
		defer os.Remove(rbDst)

		// build from sif downloaded in tmp location
		defer func() {
			sylog.Debugf("Building sandbox from downloaded SIF")
			imgCache := getCacheHandle(cache.Config{Disable: disableCache})
			if imgCache == nil {
				sylog.Fatalf("failed to create an image cache handle")
			}

			d, err := types.NewDefinitionFromURI("localimage" + "://" + rbDst)
			if err != nil {
				sylog.Fatalf("Unable to create definition for sandbox build: %v", err)
			}

			b, err := build.New(
				[]types.Definition{d},
				build.Config{
					Dest:      dst,
					Format:    "sandbox",
					NoCleanUp: buildArgs.noCleanUp,
					Opts: types.Options{
						ImgCache: imgCache,
						NoCache:  disableCache,
						TmpDir:   tmpDir,
						Update:   buildArgs.update,
						Force:    forceOverwrite,
					},
				})
			if err != nil {
				sylog.Fatalf("Unable to create build: %v", err)
			}

			if err = b.Full(ctx); err != nil {
				sylog.Fatalf("While performing build: %v", err)
			}
		}()
	}

	b, err := remotebuilder.New(rbDst, buildArgs.libraryURL, def, buildArgs.detached, forceOverwrite, buildArgs.builderURL, authToken, buildArgs.arch, buildArgs.webURL)
	if err != nil {
		sylog.Fatalf("Failed to create builder: %v", err)
	}
	err = b.Build(ctx)
	if err != nil {
		sylog.Fatalf("While performing build: %v", err)
	}
}

func runBuildLocal(ctx context.Context, authConf *authn.AuthConfig, cmd *cobra.Command, dst, spec string) {
	var keyInfo *cryptkey.KeyInfo
	if buildArgs.encrypt || promptForPassphrase || cmd.Flags().Lookup("pem-path").Changed {
		if os.Getuid() != 0 {
			sylog.Fatalf("You must be root to build an encrypted container")
		}

		k, err := getEncryptionMaterial(cmd)
		if err != nil {
			sylog.Fatalf("While handling encryption material: %v", err)
		}
		keyInfo = k
	} else {
		_, passphraseEnvOK := os.LookupEnv("SINGULARITY_ENCRYPTION_PASSPHRASE")
		_, pemPathEnvOK := os.LookupEnv("SINGULARITY_ENCRYPTION_PEM_PATH")
		if passphraseEnvOK || pemPathEnvOK {
			sylog.Warningf("Encryption related env vars found, but --encrypt was not specified. NOT encrypting container.")
		}
	}

	imgCache := getCacheHandle(cache.Config{Disable: disableCache})
	if imgCache == nil {
		sylog.Fatalf("Failed to create an image cache handle")
	}

	err := checkSections()
	if err != nil {
		sylog.Fatalf("Could not check build sections: %v", err)
	}

	// parse definition to determine build source
	buildArgsMap, err := args.ReadBuildArgs(buildArgs.buildVarArgs, buildArgs.buildVarArgFile)
	if err != nil {
		sylog.Fatalf("While processing the definition file: %v", err)
	}
	defs, err := build.MakeAllDefs(spec, buildArgsMap)
	if err != nil {
		sylog.Fatalf("Unable to build from %s: %v", spec, err)
	}

	authToken := ""
	hasLibrary := false
	hasSIF := false

	for _, d := range defs {
		// If there's a library source we need the library client, and it'll be a SIF
		if d.Header["bootstrap"] == "library" {
			hasLibrary = true
			hasSIF = true
			break
		}
		// Certain other bootstrap sources may result in a SIF image source
		if d.Header["bootstrap"] == "localimage" || d.Header["bootstrap"] == "oras" || d.Header["bootstrap"] == "shub" {
			hasSIF = true
		}
	}

	// We only need to initialize the library client if we have a library source
	// in our definition file.
	if hasLibrary {
		lc, err := getLibraryClientConfig(buildArgs.libraryURL)
		if err != nil {
			sylog.Fatalf("Unable to get library client configuration: %v", err)
		}
		buildArgs.libraryURL = lc.BaseURL
		authToken = lc.AuthToken
	}

	// We only need to initialize the key server client if we have a source
	// in our definition file that could provide a SIF. Only SIFs verify in the build.
	var ko []keyclient.Option
	if hasSIF {
		ko, err = getKeyserverClientOpts(buildArgs.keyServerURL, endpoint.KeyserverVerifyOp)
		if err != nil {
			// Do not hard fail if we can't get a keyserver config.
			// Verification can use the local keyring still.
			sylog.Warningf("Unable to get key server client configuration: %v", err)
		}
	}

	buildFormat := "sif"
	sandboxTarget := false
	if buildArgs.sandbox {
		buildFormat = "sandbox"
		sandboxTarget = true
	}

	dp, err := ociplatform.DefaultPlatform()
	if err != nil {
		sylog.Fatalf("%v", err)
	}

	b, err := build.New(
		defs,
		build.Config{
			Dest:      dst,
			Format:    buildFormat,
			NoCleanUp: buildArgs.noCleanUp,
			Opts: types.Options{
				ImgCache:          imgCache,
				TmpDir:            tmpDir,
				NoCache:           disableCache,
				Update:            buildArgs.update,
				Force:             forceOverwrite,
				Sections:          buildArgs.sections,
				NoTest:            buildArgs.noTest,
				NoHTTPS:           noHTTPS,
				LibraryURL:        buildArgs.libraryURL,
				LibraryAuthToken:  authToken,
				KeyServerOpts:     ko,
				OCIAuthConfig:     authConf,
				DockerDaemonHost:  dockerHost,
				DockerAuthFile:    reqAuthFile,
				EncryptionKeyInfo: keyInfo,
				FixPerms:          buildArgs.fixPerms,
				SandboxTarget:     sandboxTarget,
				// Only perform a build with the host DefaultPlatform at present.
				// TODO: rework --arch handling for remote builds so that local builds can specify --arch and --platform.
				Platform: *dp,
			},
		})
	if err != nil {
		sylog.Fatalf("Unable to create build: %v", err)
	}

	if err = b.Full(ctx); err != nil {
		sylog.Fatalf("While performing build: %v", err)
	}
}

func checkSections() error {
	var all, none bool
	for _, section := range buildArgs.sections {
		if section == "none" {
			none = true
		}
		if section == "all" {
			all = true
		}
	}

	if all && len(buildArgs.sections) > 1 {
		return fmt.Errorf("section specification error: cannot have all and any other option")
	}
	if none && len(buildArgs.sections) > 1 {
		return fmt.Errorf("section specification error: cannot have none and any other option")
	}

	return nil
}

func isImage(spec string) bool {
	i, err := image.Init(spec, false)
	if i != nil {
		_ = i.File.Close()
	}
	return err == nil
}

// getEncryptionMaterial handles the setting of encryption environment and flag parameters to eventually be
// passed to the crypt package for handling.
// This handles the SINGULARITY_ENCRYPTION_PASSPHRASE/PEM_PATH envvars outside of cobra in order to
// enforce the unique flag/env precedence for the encryption flow
func getEncryptionMaterial(cmd *cobra.Command) (*cryptkey.KeyInfo, error) {
	passphraseFlag := cmd.Flags().Lookup("passphrase")
	PEMFlag := cmd.Flags().Lookup("pem-path")
	passphraseEnv, passphraseEnvOK := os.LookupEnv("SINGULARITY_ENCRYPTION_PASSPHRASE")
	pemPathEnv, pemPathEnvOK := os.LookupEnv("SINGULARITY_ENCRYPTION_PEM_PATH")

	// checks for no flags/envvars being set
	if !PEMFlag.Changed && !pemPathEnvOK && !passphraseFlag.Changed && !passphraseEnvOK {
		return nil, nil
	}

	// order of precedence:
	// 1. PEM flag
	// 2. Passphrase flag
	// 3. PEM envvar
	// 4. Passphrase envvar

	if PEMFlag.Changed {
		exists, err := fs.PathExists(encryptionPEMPath)
		if err != nil {
			sylog.Fatalf("Unable to verify existence of %s: %v", encryptionPEMPath, err)
		}

		if !exists {
			sylog.Fatalf("Specified PEM file %s: does not exist.", encryptionPEMPath)
		}

		sylog.Verbosef("Using pem path flag for encrypted container")

		// Check it's a valid PEM public key we can load, before starting the build (#4173)
		if cmd.Name() == "build" {
			if _, err := cryptkey.LoadPEMPublicKey(encryptionPEMPath); err != nil {
				sylog.Fatalf("Invalid encryption public key: %v", err)
			}
			// or a valid private key before launching the engine for actions on a container (#5221)
		} else {
			if _, err := cryptkey.LoadPEMPrivateKey(encryptionPEMPath); err != nil {
				sylog.Fatalf("Invalid encryption private key: %v", err)
			}
		}

		return &cryptkey.KeyInfo{Format: cryptkey.PEM, Path: encryptionPEMPath}, nil
	}

	if passphraseFlag.Changed {
		sylog.Verbosef("Using interactive passphrase entry for encrypted container")
		passphrase, err := interactive.AskQuestionNoEcho("Enter encryption passphrase: ")
		if err != nil {
			return nil, err
		}
		if passphrase == "" {
			sylog.Fatalf("Cannot encrypt container with empty passphrase")
		}
		return &cryptkey.KeyInfo{Format: cryptkey.Passphrase, Material: passphrase}, nil
	}

	if pemPathEnvOK {
		exists, err := fs.PathExists(pemPathEnv)
		if err != nil {
			sylog.Fatalf("Unable to verify existence of %s: %v", pemPathEnv, err)
		}

		if !exists {
			sylog.Fatalf("Specified PEM file %s: does not exist.", pemPathEnv)
		}

		sylog.Verbosef("Using pem path environment variable for encrypted container")
		return &cryptkey.KeyInfo{Format: cryptkey.PEM, Path: pemPathEnv}, nil
	}

	if passphraseEnvOK {
		sylog.Verbosef("Using passphrase environment variable for encrypted container")
		return &cryptkey.KeyInfo{Format: cryptkey.Passphrase, Material: passphraseEnv}, nil
	}

	return nil, nil
}
