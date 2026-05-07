// Copyright (c) 2020, Control Command Inc. All rights reserved.
// Copyright (c) 2018-2026, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/sylabs/singularity/v4/docs"
	"github.com/sylabs/singularity/v4/internal/pkg/cache"
	"github.com/sylabs/singularity/v4/internal/pkg/client/library"
	"github.com/sylabs/singularity/v4/internal/pkg/client/net"
	"github.com/sylabs/singularity/v4/internal/pkg/client/oci"
	"github.com/sylabs/singularity/v4/internal/pkg/client/oras"
	"github.com/sylabs/singularity/v4/internal/pkg/client/shub"
	"github.com/sylabs/singularity/v4/internal/pkg/ociimage"
	"github.com/sylabs/singularity/v4/internal/pkg/ocisif"
	"github.com/sylabs/singularity/v4/internal/pkg/remote/credential/ociauth"
	"github.com/sylabs/singularity/v4/internal/pkg/runtime/launcher"
	"github.com/sylabs/singularity/v4/internal/pkg/runtime/launcher/native"
	ocilauncher "github.com/sylabs/singularity/v4/internal/pkg/runtime/launcher/oci"
	"github.com/sylabs/singularity/v4/internal/pkg/util/uri"
	bndocisif "github.com/sylabs/singularity/v4/pkg/ocibundle/ocisif"
	"github.com/sylabs/singularity/v4/pkg/sylog"
	useragent "github.com/sylabs/singularity/v4/pkg/util/user-agent"
)

const (
	defaultPath = "/bin:/usr/bin:/sbin:/usr/sbin:/usr/local/bin:/usr/local/sbin"
)

func getCacheHandle(cfg cache.Config) *cache.Handle {
	h, err := cache.New(cache.Config{
		ParentDir: os.Getenv(cache.DirEnv),
		Disable:   cfg.Disable,
	})
	if err != nil {
		sylog.Fatalf("Failed to create an image cache handle: %s", err)
	}

	return h
}

type contextKey string

const (
	keyOrigImageURI contextKey = "origImageURI"
	keyPullTempDir  contextKey = "pullTempDir"
)

// actionPreRun will:
//   - do the proper path unsetting;
//   - and implement flag inferences for:
//     --compat
//     --hostname
//   - retrieve remote images to the cache or a temporary directory for execution
func actionPreRun(cmd *cobra.Command, args []string) {
	// For compatibility - we still set USER_PATH so it will be visible in the
	// container, and can be used there if needed. USER_PATH is not used by
	// singularity itself in 3.9+
	userPath := strings.Join([]string{os.Getenv("PATH"), defaultPath}, ":")
	os.Setenv("USER_PATH", userPath)

	// --compat infers other options that give increased OCI / Docker compatibility
	// Excludes uts/user/net namespaces as these are restrictive for many Singularity
	// installs.
	if isCompat {
		if noCompat {
			sylog.Fatalf("Cannot use --no-compat with --compat: incompatible options")
		}
		isContainAll = true
		isWritableTmpfs = true
		noInit = true
		noUmask = true
		noEval = true
	}

	// --hostname requires UTS namespace
	if len(hostname) > 0 {
		utsNamespace = true
	}

	// Store the original image URI in the command context, so it can be used by
	// any fallback logic.
	origImageURI := args[0]
	cmd.SetContext(context.WithValue(cmd.Context(), keyOrigImageURI, &origImageURI))

	// Replace remote URI with a local image path, pulling to cache or a
	// temporary directory as needed.
	localImage, pullTempDir := uriToImage(cmd.Context(), cmd, origImageURI)
	args[0] = localImage

	// Track the pullTempDir (if set) in the context, so it can be cleaned up on container exit.
	cmd.SetContext(context.WithValue(cmd.Context(), keyPullTempDir, &pullTempDir))
}

func uriToCacheImage(ctx context.Context, refType string, cmd *cobra.Command, imgCache *cache.Handle, pullFrom string) (string, error) {
	switch refType {
	case uri.Library:
		return handleLibrary(ctx, imgCache, "", pullFrom)
	case uri.Oras:
		ociAuth, err := makeOCICredentials(cmd)
		if err != nil {
			return "", fmt.Errorf("while creating docker credentials: %v", err)
		}
		return oras.PullToCache(ctx, imgCache, pullFrom, ociAuth, reqAuthFile)
	case uri.Shub:
		return shub.PullToCache(ctx, imgCache, pullFrom, noHTTPS)
	case ociimage.SupportedTransport(refType):
		return handleOCI(ctx, cmd, imgCache, "", pullFrom)
	case uri.HTTP:
		return net.PullToCache(ctx, imgCache, pullFrom)
	case uri.HTTPS:
		return net.PullToCache(ctx, imgCache, pullFrom)
	default:
		return "", fmt.Errorf("unsupported transport type: %s", refType)
	}
}

func uriToTempImage(ctx context.Context, refType string, cmd *cobra.Command, imgCache *cache.Handle, pullFrom string) (string, string, error) {
	pullTempDir, err := os.MkdirTemp(tmpDir, "singularity-action-pull-")
	if err != nil {
		return "", "", fmt.Errorf("unable to create temporary directory: %w", err)
	}
	tmpImage := filepath.Join(pullTempDir, "image")
	sylog.Debugf("Cache disabled, pulling image to temporary file: %s", tmpImage)

	switch refType {
	case uri.Library:
		_, err = handleLibrary(ctx, imgCache, tmpImage, pullFrom)
	case uri.Oras:
		ociAuth, authErr := makeOCICredentials(cmd)
		if authErr != nil {
			return "", "", fmt.Errorf("while creating docker credentials: %v", authErr)
		}
		_, err = oras.PullToFile(ctx, imgCache, tmpImage, pullFrom, ociAuth, reqAuthFile)
	case uri.Shub:
		_, err = shub.PullToFile(ctx, imgCache, tmpImage, pullFrom, noHTTPS)
	case ociimage.SupportedTransport(refType):
		_, err = handleOCI(ctx, cmd, imgCache, tmpImage, pullFrom)
	case uri.HTTP:
		_, err = net.PullToFile(ctx, imgCache, tmpImage, pullFrom)
	case uri.HTTPS:
		_, err = net.PullToFile(ctx, imgCache, tmpImage, pullFrom)
	default:
		return "", "", fmt.Errorf("unsupported transport type: %s", refType)
	}

	return tmpImage, pullTempDir, err
}

func handleLibrary(ctx context.Context, imgCache *cache.Handle, tmpImage, pullFrom string) (string, error) {
	r, err := library.NormalizeLibraryRef(pullFrom)
	if err != nil {
		return "", err
	}

	// Default "" = use current remote endpoint
	var libraryURI string
	if r.Host != "" {
		if noHTTPS {
			libraryURI = "http://" + r.Host
		} else {
			libraryURI = "https://" + r.Host
		}
	}

	c, err := getLibraryClientConfig(libraryURI)
	if err != nil {
		return "", err
	}

	pullOpts := library.PullOptions{
		Endpoint:      currentRemoteEndpoint,
		LibraryConfig: c,
		// false to allow OCI execution of native SIF from library
		RequireOciSif: false,
		KeepLayers:    keepLayers,
		TmpDir:        tmpDir,
		Platform:      getOCIPlatform(),
	}

	var imagePath string
	if tmpImage == "" {
		imagePath, err = library.PullToCache(ctx, imgCache, r, pullOpts)
	} else {
		imagePath, err = library.PullToFile(ctx, imgCache, tmpImage, r, pullOpts)
	}

	if err != nil && err != library.ErrLibraryPullUnsigned {
		return "", err
	}
	if err == library.ErrLibraryPullUnsigned {
		sylog.Warningf("Skipping container verification")
	}
	return imagePath, nil
}

func handleOCI(ctx context.Context, cmd *cobra.Command, imgCache *cache.Handle, tmpImage, pullFrom string) (string, error) {
	ociAuth, err := makeOCICredentials(cmd)
	if err != nil {
		sylog.Fatalf("While creating Docker credentials: %v", err)
	}

	pullOpts := oci.PullOptions{
		TmpDir:      tmpDir,
		OciAuth:     ociAuth,
		DockerHost:  dockerHost,
		NoHTTPS:     noHTTPS,
		OciSif:      isOCI,
		KeepLayers:  keepLayers,
		Platform:    getOCIPlatform(),
		ReqAuthFile: reqAuthFile,
	}

	if tmpImage == "" {
		return oci.PullToCache(ctx, imgCache, pullFrom, pullOpts)
	}
	return oci.PullToFile(ctx, imgCache, tmpImage, pullFrom, pullOpts)
}

// uriToImage will pull a remote image to the cache, or a temporary directory if
// the cache is disabled. It returns a path to the pulled image, and the
// temporary directory that should be removed when the container exits, where
// applicable.
func uriToImage(ctx context.Context, cmd *cobra.Command, origImageURI string) (imagePath, tempDir string) {
	refType, _ := uri.Split(origImageURI)
	// If joining an instance (instance://xxx), or we have a bare filename then
	// no retrieval / conversion is required.
	if refType == "instance" || refType == "" {
		return origImageURI, ""
	}

	imgCache := getCacheHandle(cache.Config{Disable: disableCache})
	if imgCache == nil {
		sylog.Fatalf("failed to create a new image cache handle")
	}

	// If the cache is disabled, then we pull to a temporary location, which
	// will need to be removed on container exit. Otherwise, we pull to the
	// cache and run directly from there.
	var err error
	if disableCache {
		imagePath, tempDir, err = uriToTempImage(ctx, refType, cmd, imgCache, origImageURI)
	} else {
		imagePath, err = uriToCacheImage(ctx, refType, cmd, imgCache, origImageURI)
	}

	// If we are in OCI mode, then we can still attempt to run from a directory
	// bundle if tar->squashfs conversion in OCI-SIF creation fails. This
	// fallback is important while sqfstar/tar2sqfs are not bundled, and not
	// available in common distros.
	if errors.Is(err, ocisif.ErrFailedSquashfsConversion) {
		if !canUseTmpSandbox {
			sylog.Errorf("%v", err)
			sylog.Fatalf("OCI-SIF could not be created, and fallback to temporary sandbox dir disallowed")
		}
		sylog.Warningf("%v", err)
		sylog.Warningf("OCI-SIF could not be created, falling back to unpacking OCI bundle in temporary sandbox dir")
		return origImageURI, ""
	}

	if err != nil {
		sylog.Fatalf("Unable to handle %s uri: %v", origImageURI, err)
	}

	return imagePath, tempDir
}

func pullTempDirFromContext(ctx context.Context) string {
	pullTempDirPtr := ctx.Value(keyPullTempDir)
	if pullTempDirPtr != nil {
		pullTempDir, ok := pullTempDirPtr.(*string)
		if !ok {
			sylog.Fatalf("unable to recover pull temp dir (expected string, found: %T) from context", pullTempDirPtr)
		}
		return *pullTempDir
	}
	return ""
}

// ExecCmd represents the exec command
var ExecCmd = &cobra.Command{
	DisableFlagsInUseLine: true,
	TraverseChildren:      true,
	Args:                  cobra.MinimumNArgs(2),
	PreRun:                actionPreRun,
	Run: func(cmd *cobra.Command, args []string) {
		// singularity exec <image> <command> [args...]
		ep := launcher.ExecParams{
			Image:       args[0],
			PullTempDir: pullTempDirFromContext(cmd.Context()),
			Action:      "exec",
			Process:     args[1],
			Args:        args[2:],
		}
		if err := launchContainer(cmd, ep); err != nil {
			sylog.Fatalf("%s", err)
		}
	},

	Use:     docs.ExecUse,
	Short:   docs.ExecShort,
	Long:    docs.ExecLong,
	Example: docs.ExecExamples,
}

// ShellCmd represents the shell command
var ShellCmd = &cobra.Command{
	DisableFlagsInUseLine: true,
	TraverseChildren:      true,
	Args:                  cobra.MinimumNArgs(1),
	PreRun:                actionPreRun,
	Run: func(cmd *cobra.Command, args []string) {
		// singularity shell <image>
		ep := launcher.ExecParams{
			Image:       args[0],
			PullTempDir: pullTempDirFromContext(cmd.Context()),
			Action:      "shell",
		}
		if err := launchContainer(cmd, ep); err != nil {
			sylog.Fatalf("%s", err)
		}
	},

	Use:     docs.ShellUse,
	Short:   docs.ShellShort,
	Long:    docs.ShellLong,
	Example: docs.ShellExamples,
}

// RunCmd represents the run command
var RunCmd = &cobra.Command{
	DisableFlagsInUseLine: true,
	TraverseChildren:      true,
	Args:                  cobra.MinimumNArgs(1),
	PreRun:                actionPreRun,
	Run: func(cmd *cobra.Command, args []string) {
		// singularity run <image> [args...]
		ep := launcher.ExecParams{
			Image:       args[0],
			PullTempDir: pullTempDirFromContext(cmd.Context()),
			Action:      "run",
			Args:        args[1:],
		}
		if err := launchContainer(cmd, ep); err != nil {
			sylog.Fatalf("%s", err)
		}
	},

	Use:     docs.RunUse,
	Short:   docs.RunShort,
	Long:    docs.RunLong,
	Example: docs.RunExamples,
}

// TestCmd represents the test command
var TestCmd = &cobra.Command{
	DisableFlagsInUseLine: true,
	TraverseChildren:      true,
	Args:                  cobra.MinimumNArgs(1),
	PreRun:                actionPreRun,
	Run: func(cmd *cobra.Command, args []string) {
		// singularity test <image> [args...]
		ep := launcher.ExecParams{
			Image:       args[0],
			PullTempDir: pullTempDirFromContext(cmd.Context()),
			Action:      "test",
			Args:        args[1:],
		}
		if err := launchContainer(cmd, ep); err != nil {
			sylog.Fatalf("%s", err)
		}
	},

	Use:     docs.RunTestUse,
	Short:   docs.RunTestShort,
	Long:    docs.RunTestLong,
	Example: docs.RunTestExample,
}

func launchContainer(cmd *cobra.Command, ep launcher.ExecParams) error {
	ns := launcher.Namespaces{
		User:  userNamespace,
		UTS:   utsNamespace,
		PID:   pidNamespace,
		IPC:   ipcNamespace,
		Net:   netNamespace,
		NoPID: noPidNamespace,
	}

	cgJSON, err := getCgroupsJSON()
	if err != nil {
		return err
	}
	if cgJSON != "" && strings.HasPrefix(ep.Image, "instance://") {
		cgJSON = ""
		sylog.Warningf("Resource limits & cgroups configuration are only applied to instances at instance start.")
	}

	ki, err := getEncryptionMaterial(cmd)
	if err != nil {
		return err
	}

	opts := []launcher.Option{
		launcher.OptWritable(isWritable),
		launcher.OptWritableTmpfs(isWritableTmpfs),
		launcher.OptOverlayPaths(overlayPath),
		launcher.OptScratchDirs(scratchPath),
		launcher.OptWorkDir(workdirPath),
		launcher.OptHome(
			homePath,
			cmd.Flag(actionHomeFlag.Name).Changed,
			noHome,
		),
		launcher.OptMounts(
			launcher.MountSpecs{
				Binds:      bindPaths,
				DataBinds:  dataPaths,
				Mounts:     mounts,
				FuseMounts: fuseMount,
			},
		),
		launcher.OptNoMount(noMount),
		launcher.OptNvidia(nvidia, nvCCLI),
		launcher.OptNoNvidia(noNvidia),
		launcher.OptRocm(rocm),
		launcher.OptNoRocm(noRocm),
		launcher.OptContainLibs(containLibsPath),
		launcher.OptProot(proot),
		launcher.OptEnv(singularityEnv, singularityEnvFiles, isCleanEnv),
		launcher.OptNoEval(noEval),
		launcher.OptNamespaces(ns),
		launcher.OptNetnsPath(netnsPath),
		launcher.OptNetwork(network, networkArgs),
		launcher.OptHostname(hostname),
		launcher.OptDNS(dns),
		launcher.OptCaps(addCaps, dropCaps),
		launcher.OptAllowSUID(allowSUID),
		launcher.OptKeepPrivs(keepPrivs),
		launcher.OptNoPrivs(noPrivs),
		launcher.OptSecurity(security),
		launcher.OptNoUmask(noUmask),
		launcher.OptCgroupsJSON(cgJSON),
		launcher.OptConfigFile(configurationFile),
		launcher.OptShellPath(shellPath),
		launcher.OptCwdPath(cwdPath),
		launcher.OptFakeroot(isFakeroot),
		launcher.OptNoSetgroups(noSetgroups),
		launcher.OptBoot(isBoot),
		launcher.OptNoInit(noInit),
		launcher.OptContain(isContained),
		launcher.OptContainAll(isContainAll),
		launcher.OptAppName(appName),
		launcher.OptKeyInfo(ki),
		launcher.OptSIFFuse(sifFUSE),
		launcher.OptCacheDisabled(disableCache),
		launcher.OptDevice(device),
		launcher.OptCdiDirs(cdiDirs),
		launcher.OptNoCompat(noCompat),
		launcher.OptTmpSandbox(tmpSandbox),
		launcher.OptNoTmpSandbox(noTmpSandbox),
		launcher.OptPullTempDir(ep.PullTempDir),
	}

	// Explicitly use the interface type here, as we will add alternative launchers later...
	var l launcher.Launcher

	if isOCI {
		sylog.Debugf("Using OCI runtime launcher.")

		tOpts := &ociimage.TransportOptions{
			Insecure:         noHTTPS,
			AuthConfig:       &authConfig,
			DockerDaemonHost: dockerHost,
			AuthFilePath:     ociauth.ChooseAuthFile(reqAuthFile),
			UserAgent:        useragent.Value(),
			Platform:         getOCIPlatform(),
		}
		opts = append(opts, launcher.OptTransportOptions(tOpts))

		l, err = ocilauncher.NewLauncher(opts...)
		if err != nil {
			return fmt.Errorf("while configuring container: %s", err)
		}
	} else {
		sylog.Debugf("Using native runtime launcher.")
		l, err = native.NewLauncher(opts...)
		if err != nil {
			return fmt.Errorf("while configuring container: %s", err)
		}
	}

	execErr := l.Exec(cmd.Context(), ep)

	// When the image is an OCI-SIF, the initial l.Exec above could fail in
	// OCI-Mode if required FUSE tools are not available. This is indicated by
	// execErr being an ocisif.UnavailableError.
	//
	// If the OCI-SIF image was created by replaceURIWithImage - i.e. the user
	// asked to run a docker:// or other URI, not an OCI-SIF file - then we can
	// try to exec again from the original URI. In this case, the OCI launcher
	// will construct a bundle based on a temporary sandbox rootfs, rather than
	// an OCI-SIF.

	// Fail if the execError wasn't a result of being unable to create a FUSE
	// mount bundle from an OCI-SIF.
	var mountErr bndocisif.UnavailableError
	if !(errors.As(execErr, &mountErr)) {
		return execErr
	}

	// Fail if the ImageURI is the same as the origImageURI ... i.e. if the
	// image was directly specified by the user, and is not a result of
	// replaceURIWIthImage.
	origImageURIPtr := cmd.Context().Value(keyOrigImageURI)
	if origImageURIPtr == nil {
		return fmt.Errorf("unable to recover original image URI from context")
	}
	origImageURI, ok := origImageURIPtr.(*string)
	if !ok {
		return fmt.Errorf("unable to recover original image URI (expected string, found: %T) from context", origImageURIPtr)
	}
	if ep.Image == *origImageURI {
		return execErr
	}

	// Fail if we are not permitted to try using a temporary sandbox.
	if !canUseTmpSandbox {
		sylog.Warningf("OCI-SIF could not be used, and fallback to temporary sandbox dir disallowed")
		return execErr
	}

	// Try to launch the original user-specified URI directly - which will use a
	// tmp sandbox rootfs bundle, rather than OCI-SIF.
	sylog.Warningf("%v", execErr)
	sylog.Warningf("OCI-SIF could not be used, falling back to unpacking OCI bundle in temporary sandbox dir")
	l, err = ocilauncher.NewLauncher(opts...)
	if err != nil {
		return fmt.Errorf("while configuring container: %s", err)
	}
	ep.Image = *origImageURI
	return l.Exec(cmd.Context(), ep)
}
