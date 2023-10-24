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

package cli

import (
	"bufio"
	"context"
	"io"
	"os"
	"path/filepath"

	"github.com/containerd/console"
	"github.com/moby/buildkit/client"
	dockerfile "github.com/moby/buildkit/frontend/dockerfile/builder"
	"github.com/moby/buildkit/util/progress/progressui"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/sylabs/singularity/v4/internal/pkg/client/ocisif"
	"github.com/sylabs/singularity/v4/pkg/sylog"
	"golang.org/x/sync/errgroup"
)

const (
	buildTag       = "tag"
	buildkitSocket = "unix:///run/buildkit/buildkitd.sock"
)

func buildImage(ctx context.Context, tarFile *os.File, spec string, clientsideFrontend bool) error {
	c, err := client.New(ctx, buildkitSocket, client.WithFailFast())
	if err != nil {
		return err
	}

	buildDir, err := os.MkdirTemp("", "singularity-buildkit-builddir-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(buildDir)

	pipeR, pipeW := io.Pipe()
	solveOpt, err := newSolveOpt(ctx, pipeW, buildDir, spec, clientsideFrontend)
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
		return err
	})
	eg.Go(func() error {
		var c console.Console
		if cn, err := console.ConsoleFromFile(os.Stderr); err == nil {
			c = cn
		}
		// not using shared context to not disrupt display but let is finish reporting errors
		_, err = progressui.DisplaySolveStatus(context.TODO(), c, os.Stdout, ch)
		return err
	})
	eg.Go(func() error {
		if err := writeDockerTar(pipeR, tarFile); err != nil {
			return err
		}
		err := pipeR.Close()
		return err
	})
	if err := eg.Wait(); err != nil {
		return err
	}

	return nil
}

func newSolveOpt(_ context.Context, w io.WriteCloser, buildDir, spec string, clientsideFrontend bool) (*client.SolveOpt, error) {
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

	// TODO: Propagate our registry auth info & use it here

	// TODO: Propagate our own build-args values into this code here

	// for _, buildArg := range clicontext.StringSlice("build-arg") {
	// 	kv := strings.SplitN(buildArg, "=", 2)
	// 	if len(kv) != 2 {
	// 		return nil, errors.Errorf("invalid build-arg value %s", buildArg)
	// 	}
	// 	frontendAttrs["build-arg:"+kv[0]] = kv[1]
	// }

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
	}, nil
}

func writeDockerTar(r io.Reader, outputFile *os.File) error {
	writer := bufio.NewWriter(outputFile)
	defer writer.Flush()
	_, err := writer.ReadFrom(r)
	if (err != nil) && (err != io.EOF) {
		return err
	}

	return nil
}

func runBuildOCI(ctx context.Context, _ *cobra.Command, dest, spec string) {
	tarFile, err := os.CreateTemp("", "singularity-buildkit-tar-")
	if err != nil {
		sylog.Fatalf("While trying to build tar image from dockerfile: %v", err)
	}
	defer tarFile.Close()
	defer os.Remove(tarFile.Name())

	if err := buildImage(ctx, tarFile, spec, false); err != nil {
		sylog.Fatalf("While building from dockerfile: %v", err)
	}
	sylog.Debugf("Saved OCI image as tar: %s", tarFile.Name())
	tarFile.Close()

	if _, err := ocisif.PullOCISIF(ctx, nil, dest, "oci-archive:"+tarFile.Name(), ocisif.PullOptions{}); err != nil {
		sylog.Fatalf("While converting OCI tar image to OCI-SIF: %v", err)
	}
}
