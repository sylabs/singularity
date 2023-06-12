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
	"runtime"
	"strings"

	"github.com/containers/image/v5/types"
	"github.com/spf13/cobra"
	"github.com/sylabs/singularity/docs"
	"github.com/sylabs/singularity/internal/pkg/cache"
	"github.com/sylabs/singularity/internal/pkg/client/library"
	"github.com/sylabs/singularity/internal/pkg/client/net"
	"github.com/sylabs/singularity/internal/pkg/client/oci"
	"github.com/sylabs/singularity/internal/pkg/client/oras"
	"github.com/sylabs/singularity/internal/pkg/client/shub"
	"github.com/sylabs/singularity/internal/pkg/runtime/launcher"
	"github.com/sylabs/singularity/internal/pkg/runtime/launcher/native"
	ocilauncher "github.com/sylabs/singularity/internal/pkg/runtime/launcher/oci"
	"github.com/sylabs/singularity/internal/pkg/util/uri"
	"github.com/sylabs/singularity/pkg/syfs"
	"github.com/sylabs/singularity/pkg/sylog"
	useragent "github.com/sylabs/singularity/pkg/util/user-agent"
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
//   - run replaceURIWithImage;
//   - do the proper path unsetting;
//   - and implement flag inferences for:
//     --compat
//     --hostname
func actionPreRun(cmd *cobra.Command, args []string) {
	// For compatibility - we still set USER_PATH so it will be visible in the
	// container, and can be used there if needed. USER_PATH is not used by
	// singularity itself in 3.9+
	userPath := strings.Join([]string{os.Getenv("PATH"), defaultPath}, ":")
	os.Setenv("USER_PATH", userPath)

	replaceURIWithImage(cmd.Context(), cmd, args)

	// --compat infers other options that give increased OCI / Docker compatibility
	// Excludes uts/user/net namespaces as these are restrictive for many Singularity
	// installs.
	if isCompat {
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
	return library.Pull(ctx, imgCache, r, runtime.GOARCH, tmpDir, c)
}

func handleShub(ctx context.Context, imgCache *cache.Handle, pullFrom string) (string, error) {
	return shub.Pull(ctx, imgCache, pullFrom, tmpDir, noHTTPS)
}

func handleNet(ctx context.Context, imgCache *cache.Handle, pullFrom string) (string, error) {
	return net.Pull(ctx, imgCache, pullFrom, tmpDir)
}

func replaceURIWithImage(ctx context.Context, cmd *cobra.Command, args []string) {
	// If args[0] is not transport:ref (ex. instance://...) formatted return, not a URI
	t, _ := uri.Split(args[0])
	if t == "instance" || t == "" {
		return
	}

	var image string
	var err error

	// Create a cache handle only when we know we are are using a URI
	imgCache := getCacheHandle(cache.Config{Disable: disableCache})
	if imgCache == nil {
		sylog.Fatalf("failed to create a new image cache handle")
	}

	// The OCI runtime launcher will handle OCI image sources directly.
	if ociRuntime {
		if oci.IsSupported(t) != t {
			sylog.Fatalf("OCI runtime only supports OCI image sources. %s is not supported.", t)
		}
		return
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

	args[0] = image
}

// setVM will set the --vm option if needed by other options
func setVM(cmd *cobra.Command) {
	// check if --vm-ram or --vm-cpu changed from default value
	for _, flagName := range []string{"vm-ram", "vm-cpu"} {
		if flag := cmd.Flag(flagName); flag != nil && flag.Changed {
			// this option requires the VM setting to be enabled
			cmd.Flags().Set("vm", "true")
			return
		}
	}

	// since --syos is a boolean, it cannot be added to the above list
	if isSyOS && !vm {
		// let the user know that passing --syos implicitly enables --vm
		sylog.Warningf("The --syos option requires a virtual machine, automatically enabling --vm option.")
		cmd.Flags().Set("vm", "true")
	}
}

// ExecCmd represents the exec command
var ExecCmd = &cobra.Command{
	DisableFlagsInUseLine: true,
	TraverseChildren:      true,
	Args:                  cobra.MinimumNArgs(2),
	PreRun:                actionPreRun,
	Run: func(cmd *cobra.Command, args []string) {
		// singularity exec <image> <command> [args...]
		image := args[0]
		containerCmd := "/.singularity.d/actions/exec"
		containerArgs := args[1:]
		// OCI runtime does not use an action script
		if ociRuntime {
			containerCmd = args[1]
			containerArgs = args[2:]
		}
		setVM(cmd)
		if vm {
			execVM(cmd, image, containerCmd, containerArgs)
			return
		}
		if err := launchContainer(cmd, image, containerCmd, containerArgs, ""); err != nil {
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
		image := args[0]
		containerCmd := "/.singularity.d/actions/shell"
		containerArgs := []string{}
		// OCI runtime does not use an action script, but must match behavior.
		// See - internal/pkg/util/fs/files/action_scripts.go (case shell).
		if ociRuntime {
			// SINGULARITY_SHELL or --shell has priority
			if shellPath != "" {
				containerCmd = shellPath
				// Clear the shellPath - not handled internally by the OCI runtime, as we exec directly without an action script.
				shellPath = ""
			} else {
				// Otherwise try to exec /bin/bash --norc, falling back to /bin/sh
				containerCmd = "/bin/sh"
				containerArgs = []string{"-c", "test -x /bin/bash && PS1='Singularity> ' exec /bin/bash --norc || PS1='Singularity> ' exec /bin/sh"}
			}
		}
		setVM(cmd)
		if vm {
			execVM(cmd, image, containerCmd, containerArgs)
			return
		}
		if err := launchContainer(cmd, image, containerCmd, containerArgs, ""); err != nil {
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
		image := args[0]
		containerCmd := "/.singularity.d/actions/run"
		containerArgs := args[1:]
		// OCI runtime does not use an action script
		if ociRuntime {
			containerCmd = ""
		}
		setVM(cmd)
		if vm {
			execVM(cmd, args[0], containerCmd, containerArgs)
			return
		}
		if err := launchContainer(cmd, image, containerCmd, containerArgs, ""); err != nil {
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
		image := args[0]
		containerCmd := "/.singularity.d/actions/test"
		containerArgs := args[1:]
		if vm {
			execVM(cmd, image, containerCmd, containerArgs)
			return
		}
		if err := launchContainer(cmd, image, containerCmd, containerArgs, ""); err != nil {
			sylog.Fatalf("%s", err)
		}
	},

	Use:     docs.RunTestUse,
	Short:   docs.RunTestShort,
	Long:    docs.RunTestLong,
	Example: docs.RunTestExample,
}

func launchContainer(cmd *cobra.Command, image string, containerCmd string, containerArgs []string, instanceName string) error {
	ns := launcher.Namespaces{
		User: userNamespace,
		UTS:  utsNamespace,
		PID:  pidNamespace,
		IPC:  ipcNamespace,
		Net:  netNamespace,
	}

	cgJSON, err := getCgroupsJSON()
	if err != nil {
		return err
	}
	if cgJSON != "" && strings.HasPrefix(image, "instance://") {
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
		launcher.OptPwdPath(pwdPath),
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
	}

	// Explicitly use the interface type here, as we will add alternative launchers later...
	var l launcher.Launcher

	if ociRuntime {
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

	return l.Exec(cmd.Context(), image, containerCmd, containerArgs, instanceName)
}
