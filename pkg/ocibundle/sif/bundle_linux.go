// Copyright (c) 2019-2023, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package sifbundle

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	imgspecv1 "github.com/opencontainers/image-spec/specs-go/v1"
	specs "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/sylabs/singularity/v4/internal/pkg/runtime/engine/config/oci/generate"
	"github.com/sylabs/singularity/v4/internal/pkg/util/env"
	"github.com/sylabs/singularity/v4/internal/pkg/util/fs"
	"github.com/sylabs/singularity/v4/internal/pkg/util/fs/squashfs"

	"github.com/sylabs/singularity/v4/pkg/image"
	"github.com/sylabs/singularity/v4/pkg/ocibundle"
	"github.com/sylabs/singularity/v4/pkg/ocibundle/tools"
	useragent "github.com/sylabs/singularity/v4/pkg/util/user-agent"
)

type sifBundle struct {
	image      string
	bundlePath string
	writable   bool
	ocibundle.Bundle
	arch string
	// imageSpec is the OCI image information, CMD, ENTRYPOINT, etc.
	imageSpec *imgspecv1.Image
}

func (s *sifBundle) writeConfig(g *generate.Generator) error {
	if s.imageSpec == nil {
		return fmt.Errorf("cannot write bundle config with nil image spec")
	}
	imgConfig := s.imageSpec.Config

	if len(g.Config.Process.Args) == 1 && g.Config.Process.Args[0] == tools.RunScript {
		args := imgConfig.Entrypoint
		args = append(args, imgConfig.Cmd...)
		if len(args) > 0 {
			g.SetProcessArgs(args)
		}
	}

	if g.Config.Process.Cwd == "" && imgConfig.WorkingDir != "" {
		g.SetProcessCwd(imgConfig.WorkingDir)
	}
	for _, e := range imgConfig.Env {
		found := false
		k := strings.SplitN(e, "=", 2)
		for _, pe := range g.Config.Process.Env {
			if strings.HasPrefix(pe, k[0]+"=") {
				found = true
				break
			}
		}
		if !found {
			g.AddProcessEnv(k[0], k[1])
		}
	}

	volumes := tools.Volumes(s.bundlePath).Path()
	for dst := range imgConfig.Volumes {
		replacer := strings.NewReplacer(string(os.PathSeparator), "_")
		src := filepath.Join(volumes, replacer.Replace(dst))
		if err := os.MkdirAll(src, 0o755); err != nil {
			return fmt.Errorf("failed to create volume directory %s: %s", src, err)
		}
		g.AddMount(specs.Mount{
			Source:      src,
			Destination: dst,
			Type:        "none",
			Options:     []string{"bind", "rw"},
		})
	}

	return tools.SaveBundleConfig(s.bundlePath, g)
}

// Create creates an OCI bundle from a SIF image
func (s *sifBundle) Create(ctx context.Context, ociConfig *specs.Spec) error {
	if s.image == "" {
		return fmt.Errorf("image wasn't set, need one to create bundle")
	}

	img, err := image.Init(s.image, s.writable)
	if err != nil {
		return fmt.Errorf("failed to load SIF image %s: %s", s.image, err)
	}
	defer img.File.Close()

	if img.Type != image.SIF {
		return fmt.Errorf("%s is not a SIF image", s.image)
	}

	part, err := img.GetRootFsPartition()
	if err != nil {
		return fmt.Errorf("while getting root filesystem in SIF %s: %s", s.image, err)
	}

	if part.Type != image.SQUASHFS {
		return fmt.Errorf("unsupported image fs type: %v", part.Type)
	}
	offset := part.Offset
	s.arch = part.Architecture

	if err := s.setImageSpec(img); err != nil {
		return fmt.Errorf("failed to set image spec: %w", err)
	}

	// generate OCI bundle directory and config
	g, err := tools.GenerateBundleConfig(s.bundlePath, ociConfig)
	if err != nil {
		return fmt.Errorf("failed to generate OCI bundle/config: %s", err)
	}

	rootFs := tools.RootFs(s.bundlePath).Path()
	if _, err := squashfs.FUSEMount(ctx, offset, s.image, rootFs, false); err != nil {
		tools.DeleteBundle(s.bundlePath)
		return fmt.Errorf("failed to mount SIF partition: %s", err)
	}

	if err := s.writeConfig(g); err != nil {
		// best effort to release FUSE mount
		squashfs.FUSEUnmount(ctx, rootFs)
		tools.DeleteBundle(s.bundlePath)
		return fmt.Errorf("failed to write OCI configuration: %s", err)
	}

	if s.writable {
		if err := tools.CreateOverlay(ctx, s.bundlePath); err != nil {
			// best effort to release FUSE mount
			squashfs.FUSEUnmount(ctx, rootFs)
			tools.DeleteBundle(s.bundlePath)
			return fmt.Errorf("failed to create overlay: %s", err)
		}
	}
	return nil
}

// Update will update the OCI config for the OCI bundle, so that it is ready for execution.
func (s *sifBundle) Update(_ context.Context, ociConfig *specs.Spec) error {
	// generate OCI bundle directory and config
	g, err := tools.GenerateBundleConfig(s.bundlePath, ociConfig)
	if err != nil {
		return fmt.Errorf("failed to generate OCI bundle/config: %s", err)
	}
	return tools.SaveBundleConfig(s.bundlePath, g)
}

// Delete erases OCI bundle create from SIF image
func (s *sifBundle) Delete(ctx context.Context) error {
	overlayExists := fs.IsDir(filepath.Join(s.bundlePath, "overlay"))
	if s.writable && overlayExists {
		if err := tools.DeleteOverlay(ctx, s.bundlePath); err != nil {
			return fmt.Errorf("delete error: %s", err)
		}
	}
	// Umount rootfs
	rootFsDir := tools.RootFs(s.bundlePath).Path()
	if err := squashfs.FUSEUnmount(ctx, rootFsDir); err != nil {
		return fmt.Errorf("failed to unmount %s: %s", rootFsDir, err)
	}
	// delete bundle directory
	return tools.DeleteBundle(s.bundlePath)
}

// ImageSpec returns an OCI Image Spec for the container.
func (s *sifBundle) ImageSpec() *imgspecv1.Image {
	return s.imageSpec
}

// setImageSpec will generate an imageSpec, using the OCI image config embedded
// in the SIF file if present.
func (s *sifBundle) setImageSpec(img *image.Image) error {
	now := time.Now()
	p := imgspecv1.Platform{
		Architecture: s.arch,
		OS:           "linux",
	}

	// Singularity images have a runscript and/or startscript. These do not
	// translate well to the OCI Entrypoint/Cmd specification. The OCI config
	// embedded into a SIF at build time doesn't try to resolve the issue, and
	// just sets Cmd to /bin/sh. Use the same default here, so it's up to the
	// launcher to resolve runscript/startscript handling.
	c := imgspecv1.ImageConfig{
		Cmd: []string{"/bin/sh"},
		Env: []string{"PATH=" + env.DefaultPath},
	}

	reader, err := image.NewSectionReader(img, image.SIFDescOCIConfigJSON, -1)
	if err != nil && err != image.ErrNoSection {
		return fmt.Errorf("failed to read %s section: %s", image.SIFDescOCIConfigJSON, err)
	}
	// We have an image config from the SIF, so overwrite default
	if err == nil {
		if err := json.NewDecoder(reader).Decode(&c); err != nil {
			return fmt.Errorf("failed to decode %s: %s", image.SIFDescOCIConfigJSON, err)
		}
	}

	s.imageSpec = &imgspecv1.Image{
		Created:  &now,
		Author:   useragent.Value(),
		Platform: p,
		Config:   c,
	}
	return nil
}

func (s *sifBundle) Path() string {
	return s.bundlePath
}

// FromSif returns a bundle interface to create/delete OCI bundle from SIF image
func FromSif(image, bundle string, writable bool) (ocibundle.Bundle, error) {
	var err error

	s := &sifBundle{
		writable: writable,
	}
	s.bundlePath, err = filepath.Abs(bundle)
	if err != nil {
		return nil, fmt.Errorf("failed to determine bundle path: %s", err)
	}
	if image != "" {
		s.image, err = filepath.Abs(image)
		if err != nil {
			return nil, fmt.Errorf("failed to determine image path: %s", err)
		}
	}
	return s, nil
}
