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
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/blang/semver/v4"
	"github.com/google/go-containerregistry/pkg/authn"
	moby_buildkit_v1 "github.com/moby/buildkit/api/services/control"
	"github.com/moby/buildkit/client"
	dockerfile "github.com/moby/buildkit/frontend/dockerfile/builder"
	"github.com/moby/buildkit/session"
	"github.com/moby/buildkit/util/progress/progressui"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/sylabs/singularity/v4/internal/pkg/build/args"
	bkauth "github.com/sylabs/singularity/v4/internal/pkg/build/buildkit/auth"
	"github.com/sylabs/singularity/v4/internal/pkg/client/ocisif"
	"github.com/sylabs/singularity/v4/internal/pkg/ociplatform"
	"github.com/sylabs/singularity/v4/internal/pkg/remote/credential/ociauth"
	"github.com/sylabs/singularity/v4/internal/pkg/util/bin"
	"github.com/sylabs/singularity/v4/pkg/sylog"
	"golang.org/x/sync/errgroup"
)

const (
	buildTag          = "tag"
	bkDefaultSocket   = "unix:///run/buildkit/buildkitd.sock"
	bkLaunchTimeout   = 120 * time.Second
	bkShutdownTimeout = 10 * time.Second
	bkMinVersion      = "v0.12.3"
)

type Opts struct {
	// Optional Docker authentication config derived from interactive login or
	// environment variables
	AuthConf *authn.AuthConfig
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
	// Context dir in which to perform build (relevant for ADD statements, etc.)
	ContextDir string
	// Disable buildkitd's internal caching mechanism
	DisableCache bool
}

func Run(ctx context.Context, opts *Opts, dest, spec string) error {
	sylog.Debugf("Requested build architecture is: %q", opts.ReqArch)
	bkSocket := os.Getenv("BUILDKIT_HOST")
	if bkSocket == "" {
		bkSocket = bkDefaultSocket
	}

	listenSocket, bkCleanup, err := ensureBuildkitd(ctx, opts, bkSocket)
	if err != nil {
		return fmt.Errorf("failed to launch / connect to buildkitd daemon: %w", err)
	}
	if bkCleanup != nil {
		defer bkCleanup()
	}

	tarFile, err := os.CreateTemp("", "singularity-buildkit-tar-")
	if err != nil {
		return fmt.Errorf("while creating temporary tar file: %w", err)
	}
	defer tarFile.Close()
	defer func() {
		tarFileName := tarFile.Name()
		if err := os.Remove(tarFileName); err != nil {
			sylog.Errorf("While trying to remove temporary tar file (%s): %v", tarFileName, err)
		}
	}()

	if err := buildImage(ctx, opts, tarFile, listenSocket, spec, false); err != nil {
		return fmt.Errorf("while building from dockerfile: %w", err)
	}
	sylog.Debugf("Saved OCI image as tar: %s", tarFile.Name())
	tarFile.Close()

	pullOpts := ocisif.PullOptions{
		KeepLayers: opts.KeepLayers,
	}
	if opts.ReqArch != "" {
		platform, err := ociplatform.PlatformFromArch(opts.ReqArch)
		if err != nil {
			return fmt.Errorf("could not determine OCI platform from architecture %q: %w", opts.ReqArch, err)
		}
		pullOpts.Platform = *platform
	}
	if _, err := ocisif.PullOCISIF(ctx, nil, dest, "docker-archive:"+tarFile.Name(), pullOpts); err != nil {
		return fmt.Errorf("while converting OCI tar image to OCI-SIF: %w", err)
	}

	return nil
}

// ensureBuildkitd checks if a buildkitd daemon is already running, and if not,
// launches one. The trySocket argument is the address at which to look for an
// already-running daemon. The bkSocket returned is the address of the running
// buildkitd, which may have been started by us. The cleanup function, if
// non-nil, will cleanly shutdown a daemon started by us.
func ensureBuildkitd(ctx context.Context, opts *Opts, trySocket string) (bkSocket string, cleanup func(), err error) {
	if opts.ReqArch != "" {
		sylog.Infof("Specific architecture requested. Starting built-in singularity-buildkitd.")
		return startBuildkitd(ctx, opts)
	}

	var ok bool
	if ok, err = isBuildkitdRunning(ctx, trySocket, bkMinVersion); ok {
		sylog.Infof("Found system buildkitd already running at %q; will use that daemon.", bkSocket)
		return trySocket, nil, nil
	}
	sylog.Debugf("while checking for existing buildkitd: %v", err)

	sylog.Infof("Did not find usable system buildkitd daemon. Starting built-in singularity-buildkitd.")
	return startBuildkitd(ctx, opts)
}

// startBuildkitd starts a singularity-buildkitd process. On success it returns
// the address of the socket on which the daemon is listening. The daemon will
// be shutdown cleanly when the context is canceled.
func startBuildkitd(ctx context.Context, opts *Opts) (bkSocket string, cleanup func(), err error) {
	bkCmd, err := bin.FindSingularityBuildkitd()
	if err != nil {
		return "", nil, err
	}

	bkSocket = generateSocketAddress()

	// singularity-buildkitd <socket-uri> [architecture]
	args := []string{bkSocket}
	if opts.ReqArch != "" {
		args = append(args, opts.ReqArch)
	}
	cmd := exec.CommandContext(ctx, bkCmd, args...)
	cmd.WaitDelay = bkShutdownTimeout
	cmd.Cancel = func() error {
		sylog.Infof("Terminating singularity-buildkitd (PID %d)", cmd.Process.Pid)
		return cmd.Process.Signal(syscall.SIGTERM)
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	cleanup = func() {
		if err := cmd.Cancel(); err != nil {
			sylog.Errorf("while canceling buildkit daemon process: %v", err)
		}
		cmd.Wait()
	}

	if err := cmd.Start(); err != nil {
		return "", nil, err
	}

	timeout := time.After(bkLaunchTimeout)
	tick := time.NewTicker(time.Second)
	for {
		select {
		case <-ctx.Done():
			cleanup()
			return "", nil, fmt.Errorf("%v", ctx.Err().Error())
		case <-timeout:
			cleanup()
			return "", nil, fmt.Errorf("%s", "singularity-buildkitd failed to start")
		case <-tick.C:
			if ok, err := isBuildkitdRunning(ctx, bkSocket, ""); ok {
				return bkSocket, cleanup, nil
			} else {
				sylog.Debugf("singularity-buildkitd not ready, waiting 1s to retry... %v", err)
			}
		}
	}
}

// isBuildkitdRunning tries to determine whether there's already an instance of
// buildkitd running. The bkSocket argument is the address at which to look for
// an already-running daemon. The reqVersion argument is an optional string
// specifcying a minimum buildkitd version that must be satisfied.
func isBuildkitdRunning(ctx context.Context, bkSocket, reqVersion string) (bool, error) {
	c, err := client.New(ctx, bkSocket)
	if err != nil {
		return false, err
	}
	defer c.Close()

	cc := c.ControlClient()
	ir := moby_buildkit_v1.InfoRequest{}
	bkInfo, err := cc.Info(ctx, &ir)
	if err != nil {
		return false, err
	}

	if reqVersion == "" {
		return true, nil
	}

	sylog.Infof("Found running buildkit, version: %s", bkInfo.BuildkitVersion.Version)
	minVer, err := semver.Make(strings.TrimPrefix(bkMinVersion, "v"))
	if err != nil {
		return false, fmt.Errorf("while trying to parse minimal version cutoff for buildkit daemon (%q): %v", bkMinVersion, err)
	}
	foundVer, err := semver.Make(strings.TrimPrefix(bkInfo.BuildkitVersion.Version, "v"))
	if err != nil {
		return false, fmt.Errorf("while trying to parse version of running buildkit daemon (%q): %v", bkInfo.BuildkitVersion.Version, err)
	}
	if foundVer.Compare(minVer) < 0 {
		return false, fmt.Errorf("running buildkitd daemon version is older than minimum version required (%s)", bkMinVersion)
	}

	return true, nil
}

func buildImage(ctx context.Context, opts *Opts, tarFile *os.File, listenSocket, spec string, clientsideFrontend bool) error {
	c, err := client.New(ctx, listenSocket)
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
		var d progressui.Display
		var err error
		if sylog.GetLevel() >= 0 {
			d, err = progressui.NewDisplay(os.Stderr, progressui.TtyMode)
			if err != nil {
				// If an error occurs while attempting to create the tty display,
				// fallback to using plain mode on stdout (in contrast to stderr).
				d, err = progressui.NewDisplay(os.Stdout, progressui.PlainMode)
				if err != nil {
					sylog.Errorf("while initializing progress display: %v", err)
				}
			}
		} else {
			d, err = progressui.NewDisplay(io.Discard, progressui.PlainMode)
			if err != nil {
				sylog.Errorf("while initializing dummy progress display:%v", err)
			}
			logrus.SetLevel(logrus.ErrorLevel)
		}
		_, err = d.UpdateFrom(ctx, ch)
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

	localDirs := map[string]string{
		"context":    opts.ContextDir,
		"dockerfile": filepath.Dir(spec),
	}

	frontend := "dockerfile.v0" // TODO: use gateway
	if clientsideFrontend {
		frontend = ""
	}
	frontendAttrs := map[string]string{
		"filename": filepath.Base(spec),
	}

	if opts.DisableCache {
		frontendAttrs["no-cache"] = ""
	}

	attachable := []session.Attachable{bkauth.NewAuthProvider(opts.AuthConf, ociauth.ChooseAuthFile(opts.ReqAuthFile))}

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

func generateSocketAddress() string {
	socketPath := "/run/singularity-buildkitd"

	//  pam_systemd sets XDG_RUNTIME_DIR but not other dirs.
	xdgRuntimeDir := os.Getenv("XDG_RUNTIME_DIR")
	if xdgRuntimeDir != "" {
		dirs := strings.Split(xdgRuntimeDir, ":")
		socketPath = filepath.Join(dirs[0], "singularity-buildkitd")
	}

	return "unix://" + filepath.Join(socketPath, fmt.Sprintf("singularity-buildkitd-%d.sock", os.Getpid()))
}
