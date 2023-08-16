// Copyright (c) 2020, Control Command Inc. All rights reserved.
// Copyright (c) 2018-2023, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package cli

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/containers/image/v5/types"
	"github.com/spf13/cobra"
	"github.com/sylabs/singularity/v4/docs"
	"github.com/sylabs/singularity/v4/internal/pkg/cache"
	"github.com/sylabs/singularity/v4/internal/pkg/client/library"
	"github.com/sylabs/singularity/v4/internal/pkg/client/net"
	"github.com/sylabs/singularity/v4/internal/pkg/client/oci"
	"github.com/sylabs/singularity/v4/internal/pkg/client/oras"
	"github.com/sylabs/singularity/v4/internal/pkg/client/shub"
	"github.com/sylabs/singularity/v4/internal/pkg/runtime/launcher"
	"github.com/sylabs/singularity/v4/internal/pkg/runtime/launcher/native"
	ocilauncher "github.com/sylabs/singularity/v4/internal/pkg/runtime/launcher/oci"
	"github.com/sylabs/singularity/v4/internal/pkg/util/uri"
	"github.com/sylabs/singularity/v4/pkg/syfs"
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

// actionPreRun will:
//   - do the proper path unsetting;
//   - and implement flag inferences for:
//     --compat
//     --hostname
//   - run replaceURIWithImage;
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

	replaceURIWithImage(cmd.Context(), cmd, args)
}

func handleOCI(ctx context.Context, imgCache *cache.Handle, cmd *cobra.Command, pullFrom string) (string, error) {
	ociAuth, err := makeDockerCredentials(cmd)
	if err != nil {
		sylog.Fatalf("While creating Docker credentials: %v", err)
	}

	pullOpts := oci.PullOptions{
		TmpDir:     tmpDir,
		OciAuth:    ociAuth,
		DockerHost: dockerHost,
		NoHTTPS:    noHTTPS,
		OciSif:     isOCI,
	}

	return oci.Pull(ctx, imgCache, pullFrom, pullOpts)
}

func handleOras(ctx context.Context, imgCache *cache.Handle, cmd *cobra.Command, pullFrom string) (string, error) {
	ociAuth, err := makeDockerCredentials(cmd)
	if err != nil {
		return "", fmt.Errorf("while creating docker credentials: %v", err)
	}
	return oras.Pull(ctx, imgCache, pullFrom, tmpDir, ociAuth)
}

func handleLibrary(ctx context.Context, imgCache *cache.Handle, pullFrom string) (string, error) {
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
		TmpDir:        tmpDir,
		Platform:      getOCIPlatform(),
	}
	return library.Pull(ctx, imgCache, r, pullOpts)
}

func handleShub(ctx context.Context, imgCache *cache.Handle, pullFrom string) (string, error) {
	return shub.Pull(ctx, imgCache, pullFrom, tmpDir, noHTTPS)
}

func handleNet(ctx context.Context, imgCache *cache.Handle, pullFrom string) (string, error) {
	return net.Pull(ctx, imgCache, pullFrom, tmpDir)
}

func replaceURIWithImage(ctx context.Context, cmd *cobra.Command, args []string) {
	t, _ := uri.Split(args[0])
	// If joining an instance (instance://xxx), or we have a bare filename then
	// no retrieval / conversion is required.
	if t == "instance" || t == "" {
		return
	}

	var image string
	var err error

	// Create a cache handle only when we know we are using a URI
	imgCache := getCacheHandle(cache.Config{Disable: disableCache})
	if imgCache == nil {
		sylog.Fatalf("failed to create a new image cache handle")
	}

	switch t {
	case uri.Library:
		image, err = handleLibrary(ctx, imgCache, args[0])
	case uri.Oras:
		image, err = handleOras(ctx, imgCache, cmd, args[0])
	case uri.Shub:
		image, err = handleShub(ctx, imgCache, args[0])
	case oci.IsSupported(t):
		image, err = handleOCI(ctx, imgCache, cmd, args[0])
	case uri.HTTP:
		image, err = handleNet(ctx, imgCache, args[0])
	case uri.HTTPS:
		image, err = handleNet(ctx, imgCache, args[0])
	default:
		sylog.Fatalf("Unsupported transport type: %s", t)
	}

	if err != nil {
		sylog.Fatalf("Unable to handle %s uri: %v", args[0], err)
	}

	// TODO - drop once we have implemented prefix-less oci-sif vs native sif detection.
	if isOCI {
		image = "oci-sif:" + image
	}

	args[0] = image
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
			Image:   args[0],
			Action:  "exec",
			Process: args[1],
			Args:    args[2:],
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
			Image:  args[0],
			Action: "shell",
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
			Image:  args[0],
			Action: "run",
			Args:   args[1:],
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
			Image:  args[0],
			Action: "test",
			Args:   args[1:],
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
		launcher.OptMounts(bindPaths, mounts, fuseMount),
		launcher.OptNoMount(noMount),
		launcher.OptNvidia(nvidia, nvCCLI),
		launcher.OptNoNvidia(noNvidia),
		launcher.OptRocm(rocm),
		launcher.OptNoRocm(noRocm),
		launcher.OptContainLibs(containLibsPath),
		launcher.OptProot(proot),
		launcher.OptEnv(singularityEnv, singularityEnvFile, isCleanEnv),
		launcher.OptNoEval(noEval),
		launcher.OptNamespaces(ns),
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
	}

	// Explicitly use the interface type here, as we will add alternative launchers later...
	var l launcher.Launcher

	if isOCI {
		sylog.Debugf("Using OCI runtime launcher.")

		sysCtx := &types.SystemContext{
			OCIInsecureSkipTLSVerify: noHTTPS,
			DockerAuthConfig:         &dockerAuthConfig,
			DockerDaemonHost:         dockerHost,
			OSChoice:                 "linux",
			AuthFilePath:             syfs.DockerConf(),
			DockerRegistryUserAgent:  useragent.Value(),
		}
		if noHTTPS {
			sysCtx.DockerInsecureSkipTLSVerify = types.NewOptionalBool(true)
		}
		opts = append(opts, launcher.OptSysContext(sysCtx))

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

	return l.Exec(cmd.Context(), ep)
}
