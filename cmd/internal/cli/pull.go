// Copyright (c) 2020, Control Command Inc. All rights reserved.
// Copyright (c) 2018-2023, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package cli

import (
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/sylabs/singularity/v4/docs"
	"github.com/sylabs/singularity/v4/internal/pkg/cache"
	"github.com/sylabs/singularity/v4/internal/pkg/client/library"
	"github.com/sylabs/singularity/v4/internal/pkg/client/net"
	"github.com/sylabs/singularity/v4/internal/pkg/client/oci"
	"github.com/sylabs/singularity/v4/internal/pkg/client/oras"
	"github.com/sylabs/singularity/v4/internal/pkg/client/shub"
	"github.com/sylabs/singularity/v4/internal/pkg/ociimage"
	"github.com/sylabs/singularity/v4/internal/pkg/remote/endpoint"
	"github.com/sylabs/singularity/v4/internal/pkg/util/uri"
	"github.com/sylabs/singularity/v4/pkg/cmdline"
	"github.com/sylabs/singularity/v4/pkg/sylog"
)

const (
	// LibraryProtocol holds the sylabs cloud library base URI,
	// for more info refer to https://cloud.sylabs.io/library.
	LibraryProtocol = "library"
	// ShubProtocol holds singularity hub base URI,
	// for more info refer to https://singularity-hub.org/
	ShubProtocol = "shub"
	// HTTPProtocol holds the remote http base URI.
	HTTPProtocol = "http"
	// HTTPSProtocol holds the remote https base URI.
	HTTPSProtocol = "https"
	// OrasProtocol holds the oras URI.
	OrasProtocol = "oras"
	// Docker Registry protocol
	DockerProtocol = "docker"
)

var (
	// pullLibraryURI holds the base URI to a Sylabs library API instance.
	pullLibraryURI string
	// pullImageName holds the name to be given to the pulled image.
	pullImageName string
	// unauthenticatedPull when true; won't ask to keep a unsigned container after pulling it.
	unauthenticatedPull bool
	// pullDir is the path that the containers will be pulled to, if set.
	pullDir string
)

// --library
var pullLibraryURIFlag = cmdline.Flag{
	ID:           "pullLibraryURIFlag",
	Value:        &pullLibraryURI,
	DefaultValue: "",
	Name:         "library",
	Usage:        "download images from the provided library",
	EnvKeys:      []string{"LIBRARY"},
}

// --name
var pullNameFlag = cmdline.Flag{
	ID:           "pullNameFlag",
	Value:        &pullImageName,
	DefaultValue: "",
	Name:         "name",
	Hidden:       true,
	Usage:        "specify a custom image name",
	EnvKeys:      []string{"PULL_NAME"},
}

// --dir
var pullDirFlag = cmdline.Flag{
	ID:           "pullDirFlag",
	Value:        &pullDir,
	DefaultValue: "",
	Name:         "dir",
	Usage:        "download images to the specific directory",
	EnvKeys:      []string{"PULLDIR", "PULLFOLDER"},
}

// --disable-cache
var pullDisableCacheFlag = cmdline.Flag{
	ID:           "pullDisableCacheFlag",
	Value:        &disableCache,
	DefaultValue: false,
	Name:         "disable-cache",
	Usage:        "dont use cached images/blobs and dont create them",
	EnvKeys:      []string{"DISABLE_CACHE"},
}

// -U|--allow-unsigned
var pullAllowUnsignedFlag = cmdline.Flag{
	ID:           "pullAllowUnauthenticatedFlag",
	Value:        &unauthenticatedPull,
	DefaultValue: false,
	Name:         "allow-unsigned",
	ShortHand:    "U",
	Usage:        "do not require a signed container",
	EnvKeys:      []string{"ALLOW_UNSIGNED"},
	Deprecated:   `pull no longer exits with an error code in case of unsigned image. Now the flag only suppress warning message.`,
}

// --allow-unauthenticated
var pullAllowUnauthenticatedFlag = cmdline.Flag{
	ID:           "pullAllowUnauthenticatedFlag",
	Value:        &unauthenticatedPull,
	DefaultValue: false,
	Name:         "allow-unauthenticated",
	ShortHand:    "",
	Usage:        "do not require a signed container",
	EnvKeys:      []string{"ALLOW_UNAUTHENTICATED"},
	Hidden:       true,
}

func init() {
	addCmdInit(func(cmdManager *cmdline.CommandManager) {
		cmdManager.RegisterCmd(PullCmd)

		cmdManager.RegisterFlagForCmd(&commonForceFlag, PullCmd)
		cmdManager.RegisterFlagForCmd(&pullLibraryURIFlag, PullCmd)
		cmdManager.RegisterFlagForCmd(&pullNameFlag, PullCmd)
		cmdManager.RegisterFlagForCmd(&commonNoHTTPSFlag, PullCmd)
		cmdManager.RegisterFlagForCmd(&commonTmpDirFlag, PullCmd)
		cmdManager.RegisterFlagForCmd(&pullDisableCacheFlag, PullCmd)
		cmdManager.RegisterFlagForCmd(&pullDirFlag, PullCmd)

		cmdManager.RegisterFlagForCmd(&dockerHostFlag, PullCmd)
		cmdManager.RegisterFlagForCmd(&dockerUsernameFlag, PullCmd)
		cmdManager.RegisterFlagForCmd(&dockerPasswordFlag, PullCmd)
		cmdManager.RegisterFlagForCmd(&dockerLoginFlag, PullCmd)

		cmdManager.RegisterFlagForCmd(&buildNoCleanupFlag, PullCmd)
		cmdManager.RegisterFlagForCmd(&pullAllowUnsignedFlag, PullCmd)
		cmdManager.RegisterFlagForCmd(&pullAllowUnauthenticatedFlag, PullCmd)

		cmdManager.RegisterFlagForCmd(&commonOCIFlag, PullCmd)
		cmdManager.RegisterFlagForCmd(&commonNoOCIFlag, PullCmd)
		cmdManager.RegisterFlagForCmd(&commonKeepLayersFlag, PullCmd)

		cmdManager.RegisterFlagForCmd(&commonArchFlag, PullCmd)
		cmdManager.RegisterFlagForCmd(&commonPlatformFlag, PullCmd)

		cmdManager.RegisterFlagForCmd(&commonAuthFileFlag, PullCmd)
	})
}

// PullCmd singularity pull
var PullCmd = &cobra.Command{
	DisableFlagsInUseLine: true,
	Args:                  cobra.RangeArgs(1, 2),
	Run:                   pullRun,
	Use:                   docs.PullUse,
	Short:                 docs.PullShort,
	Long:                  docs.PullLong,
	Example:               docs.PullExample,
}

func pullRun(cmd *cobra.Command, args []string) {
	ctx := cmd.Context()

	imgCache := getCacheHandle(cache.Config{Disable: disableCache})
	if imgCache == nil {
		sylog.Fatalf("Failed to create an image cache handle")
	}

	pullFrom := args[len(args)-1]
	transport, ref := uri.Split(pullFrom)
	if ref == "" {
		sylog.Fatalf("Bad URI %s", pullFrom)
	}

	suffix := "sif"
	if isOCI {
		suffix = "oci.sif"
	}

	pullTo := pullImageName
	if pullTo == "" {
		pullTo = args[0]
		if len(args) == 1 {
			if transport == "" {
				pullTo = uri.Filename("library://"+pullFrom, suffix)
			} else {
				pullTo = uri.Filename(pullFrom, suffix) // TODO: If not library/shub & no name specified, simply put to cache
			}
		}
	}

	if pullDir != "" {
		pullTo = filepath.Join(pullDir, pullTo)
	}

	_, err := os.Stat(pullTo)
	if !os.IsNotExist(err) {
		// image already exists
		if !forceOverwrite {
			sylog.Fatalf("Image file already exists: %q - will not overwrite", pullTo)
		}
	}

	switch transport {
	case LibraryProtocol, "":
		ref, err := library.NormalizeLibraryRef(pullFrom)
		if err != nil {
			sylog.Fatalf("Malformed library reference: %v", err)
		}

		if pullLibraryURI != "" && ref.Host != "" {
			sylog.Fatalf("Conflicting arguments; do not use --library with a library URI containing host name")
		}

		var libraryURI string
		if pullLibraryURI != "" {
			libraryURI = pullLibraryURI
		} else if ref.Host != "" {
			// override libraryURI if ref contains host name
			if noHTTPS {
				libraryURI = "http://" + ref.Host
			} else {
				libraryURI = "https://" + ref.Host
			}
		}

		lc, err := getLibraryClientConfig(libraryURI)
		if err != nil {
			sylog.Fatalf("Unable to get library client configuration: %v", err)
		}
		co, err := getKeyserverClientOpts("", endpoint.KeyserverVerifyOp)
		if err != nil {
			sylog.Fatalf("Unable to get keyserver client configuration: %v", err)
		}

		pullOpts := library.PullOptions{
			Endpoint:      currentRemoteEndpoint,
			KeyClientOpts: co,
			LibraryConfig: lc,
			RequireOciSif: isOCI,
			KeepLayers:    keepLayers,
			TmpDir:        tmpDir,
			Platform:      getOCIPlatform(),
		}
		_, err = library.PullToFile(ctx, imgCache, pullTo, ref, pullOpts)
		if err != nil && err != library.ErrLibraryPullUnsigned {
			sylog.Fatalf("While pulling library image: %v", err)
		}
		if err == library.ErrLibraryPullUnsigned {
			sylog.Warningf("Skipping container verification")
		}
	case ShubProtocol:
		if isOCI {
			sylog.Fatalf("Pull from shub:// to OCI-SIF (--oci) is not supported. Omit --oci, or use --no-oci, to pull a non-OCI Singularity container.")
		}
		if platform != "" || arch != "" {
			sylog.Warningf("Pull from shub:// is a direct download, --arch and --platform have no effect.")
		}

		_, err := shub.PullToFile(ctx, imgCache, pullTo, pullFrom, noHTTPS)
		if err != nil {
			sylog.Fatalf("While pulling shub image: %v\n", err)
		}
	case OrasProtocol:
		if isOCI {
			sylog.Warningf("Pull from oras:// URIs is a direct download, --oci has no effect.")
		}
		if platform != "" || arch != "" {
			sylog.Warningf("Pull from oras:// is a direct download, --arch and --platform have no effect.")
		}

		ociAuth, err := makeOCICredentials(cmd)
		if err != nil {
			sylog.Fatalf("Unable to make docker oci credentials: %s", err)
		}

		_, err = oras.PullToFile(ctx, imgCache, pullTo, pullFrom, ociAuth, reqAuthFile)
		if err != nil {
			sylog.Fatalf("While pulling image from oci registry: %v", err)
		}
	case HTTPProtocol, HTTPSProtocol:
		if isOCI {
			sylog.Warningf("Pull from http[s]:// URIs is a direct download, --oci has no effect.")
		}
		if platform != "" || arch != "" {
			sylog.Warningf("Pull from http[s]:// is a direct download, --arch and --platform have no effect.")
		}

		_, err := net.PullToFile(ctx, imgCache, pullTo, pullFrom)
		if err != nil {
			sylog.Fatalf("While pulling from image from http(s): %v\n", err)
		}
	case ociimage.SupportedTransport(transport):
		ociAuth, err := makeOCICredentials(cmd)
		if err != nil {
			sylog.Fatalf("While creating Docker credentials: %v", err)
		}

		pullOpts := oci.PullOptions{
			TmpDir:      tmpDir,
			OciAuth:     ociAuth,
			DockerHost:  dockerHost,
			NoHTTPS:     noHTTPS,
			NoCleanUp:   buildArgs.noCleanUp,
			OciSif:      isOCI,
			KeepLayers:  keepLayers,
			Platform:    getOCIPlatform(),
			ReqAuthFile: reqAuthFile,
		}

		_, err = oci.PullToFile(ctx, imgCache, pullTo, pullFrom, pullOpts)
		if err != nil {
			sylog.Fatalf("While making image from oci registry: %v", err)
		}
	default:
		sylog.Fatalf("Unsupported transport type: %s", transport)
	}
}
