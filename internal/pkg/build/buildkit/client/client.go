// Copyright 2015 The Linux Foundation.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Copyright (c) 2023, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.
//
// This file contains modified code originally taken from:
// github.com/moby/buildkit/blob/v0.12.3/examples/build-using-dockerfile/main.go

package client

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/blang/semver/v4"
	"github.com/containerd/console"
	ocitypes "github.com/containers/image/v5/types"
	moby_buildkit_v1 "github.com/moby/buildkit/api/services/control"
	"github.com/moby/buildkit/client"
	dockerfile "github.com/moby/buildkit/frontend/dockerfile/builder"
	"github.com/moby/buildkit/session"
	"github.com/moby/buildkit/util/progress/progressui"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/sylabs/singularity/v4/internal/pkg/build/args"
	bkdaemon "github.com/sylabs/singularity/v4/internal/pkg/build/buildkit/daemon"
	"github.com/sylabs/singularity/v4/internal/pkg/client/ocisif"
	"github.com/sylabs/singularity/v4/internal/pkg/ociplatform"
	"github.com/sylabs/singularity/v4/internal/pkg/remote/credential/ociauth"
	"github.com/sylabs/singularity/v4/pkg/sylog"
	"golang.org/x/sync/errgroup"
)

const (
	buildTag        = "tag"
	bkDefaultSocket = "unix:///run/buildkit/buildkitd.sock"
	bkLaunchTimeout = 120 * time.Second
	bkMinVersion    = "v0.12.3"
)

type Opts struct {
	// Optional Docker authentication config derived from interactive login or
	// environment variables
	AuthConf *ocitypes.DockerAuthConfig
	// Optional user requested authentication file for writing/reading OCI
	// registry credentials
	ReqAuthFile string
	// Variables passed to build procedure.
	BuildVarArgs []string
	// Variables file passed to build procedure.
	BuildVarArgFile string
	// Requested build architecture
	ReqArch string
	// Keep individual layers when creating OCI-SIF?
	KeepLayers bool
}

func Run(ctx context.Context, opts *Opts, dest, spec string) {
	sylog.Debugf("Requested build architecture is: %q", opts.ReqArch)
	bkSocket := os.Getenv("BUILDKIT_HOST")
	if bkSocket == "" {
		bkSocket = bkDefaultSocket
	}
	listenSocket := ensureBuildkitd(ctx, opts, bkSocket)
	if listenSocket == "" {
		sylog.Fatalf("Failed to launch buildkitd daemon within specified timeout (%v).", bkLaunchTimeout)
	}

	tarFile, err := os.CreateTemp("", "singularity-buildkit-tar-")
	if err != nil {
		sylog.Fatalf("While trying to build tar image from dockerfile: %v", err)
	}
	defer tarFile.Close()
	defer func() {
		tarFileName := tarFile.Name()
		if err := os.Remove(tarFileName); err != nil {
			sylog.Errorf("While trying to remove temporary tar file (%s): %v", tarFileName, err)
		}
	}()

	if err := buildImage(ctx, opts, tarFile, listenSocket, spec, false); err != nil {
		sylog.Fatalf("While building from dockerfile: %v", err)
	}
	sylog.Debugf("Saved OCI image as tar: %s", tarFile.Name())
	tarFile.Close()

	pullOpts := ocisif.PullOptions{
		KeepLayers: opts.KeepLayers,
	}
	if opts.ReqArch != "" {
		platform, err := ociplatform.PlatformFromArch(opts.ReqArch)
		if err != nil {
			sylog.Fatalf("could not determine OCI platform from architecture %q: %v", opts.ReqArch, err)
		}
		pullOpts.Platform = *platform
	}
	if _, err := ocisif.PullOCISIF(ctx, nil, dest, "oci-archive:"+tarFile.Name(), pullOpts); err != nil {
		sylog.Fatalf("While converting OCI tar image to OCI-SIF: %v", err)
	}
}

// ensureBuildkitd checks if a buildkitd daemon is already running, and if not,
// launches one. The bkSocket argument is the address at which to look for an
// already-running daemon.
func ensureBuildkitd(ctx context.Context, opts *Opts, bkSocket string) string {
	if isBuildkitdRunning(ctx, opts, bkSocket) {
		sylog.Infof("Found buildkitd already running at %q; will use that daemon.", bkSocket)
		return bkSocket
	}

	sylog.Infof("Did not find usable running buildkitd daemon; spawning our own.")
	socketChan := make(chan string, 1)
	go func() {
		daemonOpts := &bkdaemon.Opts{
			ReqArch: opts.ReqArch,
		}
		if err := bkdaemon.Run(ctx, daemonOpts, socketChan); err != nil {
			sylog.Fatalf("buildkitd returned error: %v", err)
		}
	}()
	go func() {
		time.Sleep(bkLaunchTimeout)
		socketChan <- ""
	}()

	return <-socketChan
}

// isBuildkitdRunning tries to determine whether there's already an instance of
// buildkitd running. The bkSocket argument is the address at which to look for
// an already-running daemon.
func isBuildkitdRunning(ctx context.Context, opts *Opts, bkSocket string) bool {
	if opts.ReqArch != "" {
		return false
	}
	c, err := client.New(ctx, bkSocket, client.WithFailFast())
	if err != nil {
		return false
	}
	defer c.Close()

	cc := c.ControlClient()
	ir := moby_buildkit_v1.InfoRequest{}
	bkInfo, err := cc.Info(ctx, &ir)
	found := (err == nil)
	if found {
		sylog.Infof("Found running buildkit, version: %s", bkInfo.BuildkitVersion.Version)
		minVer, err := semver.Make(strings.TrimPrefix(bkMinVersion, "v"))
		if err != nil {
			sylog.Fatalf("While trying to parse minimal version cutoff for buildkit daemon (%q): %v", bkMinVersion, err)
		}
		foundVer, err := semver.Make(strings.TrimPrefix(bkInfo.BuildkitVersion.Version, "v"))
		if err != nil {
			sylog.Fatalf("While trying to parse version of running buildkit daemon (%q): %v", bkInfo.BuildkitVersion.Version, err)
		}
		if foundVer.Compare(minVer) < 0 {
			sylog.Infof("Running buildkitd daemon version is older than minimal version required (%s)", bkMinVersion)
			return false
		}
	}

	return found
}

func buildImage(ctx context.Context, opts *Opts, tarFile *os.File, listenSocket, spec string, clientsideFrontend bool) error {
	c, err := client.New(ctx, listenSocket, client.WithFailFast())
	if err != nil {
		return err
	}

	buildDir, err := os.MkdirTemp("", "singularity-buildkit-builddir-")
	if err != nil {
		return err
	}
	defer func() {
		if err := os.RemoveAll(buildDir); err != nil {
			sylog.Errorf("While trying to remove temporary build dir (%s): %v", buildDir, err)
		}
	}()

	pipeR, pipeW := io.Pipe()
	solveOpt, err := newSolveOpt(ctx, opts, pipeW, buildDir, spec, clientsideFrontend)
	if err != nil {
		return err
	}

	ch := make(chan *client.SolveStatus)
	eg, ctx := errgroup.WithContext(ctx)
	eg.Go(func() error {
		var err error
		if clientsideFrontend {
			_, err = c.Build(ctx, *solveOpt, "", dockerfile.Build, ch)
		} else {
			_, err = c.Solve(ctx, nil, *solveOpt, ch)
		}
		if err != nil {
			pipeR.Close()
		}
		return err
	})
	eg.Go(func() error {
		var c console.Console
		progressWriter := io.Discard
		if sylog.GetLevel() >= 0 {
			progressWriter = os.Stdout
			if cn, err := console.ConsoleFromFile(os.Stderr); err == nil {
				c = cn
			}
		} else {
			logrus.SetLevel(logrus.ErrorLevel)
		}
		// not using shared context to not disrupt display but let is finish reporting errors
		_, err := progressui.DisplaySolveStatus(context.Background(), c, progressWriter, ch)
		if err != nil {
			pipeR.Close()
		}
		return err
	})
	eg.Go(func() error {
		if err := writeDockerTar(pipeR, tarFile); err != nil {
			return err
		}
		err := pipeR.Close()
		return err
	})

	return eg.Wait()
}

func newSolveOpt(_ context.Context, opts *Opts, w io.WriteCloser, buildDir, spec string, clientsideFrontend bool) (*client.SolveOpt, error) {
	if buildDir == "" {
		return nil, errors.New("please specify build context (e.g. \".\" for the current directory)")
	} else if buildDir == "-" {
		return nil, errors.New("stdin not supported yet")
	}

	if spec == "" {
		spec = filepath.Join(buildDir, "Dockerfile")
	}
	localDirs := map[string]string{
		"context":    buildDir,
		"dockerfile": filepath.Dir(spec),
	}

	frontend := "dockerfile.v0" // TODO: use gateway
	if clientsideFrontend {
		frontend = ""
	}
	frontendAttrs := map[string]string{
		"filename": filepath.Base(spec),
	}

	frontendAttrs["no-cache"] = ""

	attachable := []session.Attachable{bkdaemon.NewAuthProvider(opts.AuthConf, ociauth.ChooseAuthFile(opts.ReqAuthFile))}

	buildArgsMap, err := args.ReadBuildArgs(opts.BuildVarArgs, opts.BuildVarArgFile)
	if err != nil {
		return nil, err
	}
	for k, v := range buildArgsMap {
		frontendAttrs["build-arg:"+k] = v
	}

	return &client.SolveOpt{
		Exports: []client.ExportEntry{
			{
				Type: "docker", // TODO: use containerd image store when it is integrated to Docker
				Attrs: map[string]string{
					"name": buildTag,
				},
				Output: func(_ map[string]string) (io.WriteCloser, error) {
					return w, nil
				},
			},
		},
		LocalDirs:     localDirs,
		Frontend:      frontend,
		FrontendAttrs: frontendAttrs,
		Session:       attachable,
	}, nil
}

func writeDockerTar(r io.Reader, outputFile *os.File) error {
	_, err := io.Copy(outputFile, r)

	return err
}
