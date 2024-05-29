// Copyright (c) 2023-2024 Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package ocisif

import (
	"errors"
	"fmt"
	"time"

	ggcrv1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	ggcrmutate "github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/types"
	"github.com/sylabs/oci-tools/pkg/mutate"
	ocitsif "github.com/sylabs/oci-tools/pkg/sif"
	"github.com/sylabs/singularity/v4/pkg/sylog"
	useragent "github.com/sylabs/singularity/v4/pkg/util/user-agent"
)

const (
	// TODO - Replace when exported from SIF / oci-tools
	SquashfsLayerMediaType types.MediaType = "application/vnd.sylabs.image.layer.v1.squashfs"

	// spareDescrptiorCapacity is the number of spare descriptors to allocate
	// when writing an image to an OCI-SIF file. This is to provide additional
	// descriptors, beyond those needed for the OCI image, to add e.g.
	// overlay(s) / signatures without re-writing the OCI-SIF.
	spareDescriptorCapacity = 8
)

var ErrFailedSquashfsConversion = errors.New("could not convert layer to squashfs")

// WriteImage will write an image to an oci-sif without applying any
// mutations.
func WriteImage(img ggcrv1.Image, imageDest string) error {
	ii := ggcrmutate.AppendManifests(empty.Index, ggcrmutate.IndexAddendum{
		Add: img,
	})
	sylog.Infof("Writing OCI-SIF image")
	return ocitsif.Write(imageDest, ii, ocitsif.OptWriteWithSpareDescriptorCapacity(spareDescriptorCapacity))
}

// WriteImageSquashfs will write an image to OCI-SIF, with layers in squashfs format.
func WriteImageSquashfs(img ggcrv1.Image, imageDest, workDir string) error {
	origDigest, err := img.Digest()
	if err != nil {
		return err
	}
	return writeImageSquashfs(img, origDigest, imageDest, workDir)
}

func writeImageSquashfs(img ggcrv1.Image, origDigest ggcrv1.Hash, imageDest, workDir string) error {
	// If all layers are SquashFS already, do not mutate.
	mf, err := img.Manifest()
	if err != nil {
		return err
	}
	allSquash := true
	for _, l := range mf.Layers {
		if l.MediaType != SquashfsLayerMediaType {
			allSquash = false
			break
		}
	}
	if allSquash {
		return WriteImage(img, imageDest)
	}

	sylog.Infof("Converting layers to SquashFS")
	squashFSImg, err := imgLayersToSquashfs(img, origDigest, workDir)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrFailedSquashfsConversion, err)
	}
	return WriteImage(squashFSImg, imageDest)
}

// WriteImageSquashedSquashfs will write an image to OCI-SIF, with a single squashed layer in squashfs format
func WriteImageSquashedSquashfs(img ggcrv1.Image, imageDest, workDir string) error {
	origDigest, err := img.Digest()
	if err != nil {
		return err
	}
	mf, err := img.Manifest()
	if err != nil {
		return err
	}
	// If the image has a single layer, do not mutate.Squash.
	if len(mf.Layers) == 1 {
		return writeImageSquashfs(img, origDigest, imageDest, workDir)
	}

	sylog.Infof("Squashing image to single layer")
	img, err = mutate.Squash(img)
	if err != nil {
		return fmt.Errorf("while squashing image: %w", err)
	}
	return writeImageSquashfs(img, origDigest, imageDest, workDir)
}

func imgLayersToSquashfs(img ggcrv1.Image, digest ggcrv1.Hash, workDir string) (sqfsImage ggcrv1.Image, err error) {
	ms := []mutate.Mutation{}

	layers, err := img.Layers()
	if err != nil {
		return nil, fmt.Errorf("while retrieving layers: %w", err)
	}

	var sqOpts []mutate.SquashfsConverterOpt
	if len(layers) == 1 {
		sqOpts = []mutate.SquashfsConverterOpt{
			mutate.OptSquashfsSkipWhiteoutConversion(true),
		}
	}

	for i, l := range layers {
		squashfsLayer, err := mutate.SquashfsLayer(l, workDir, sqOpts...)
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrFailedSquashfsConversion, err)
		}
		ms = append(ms, mutate.SetLayer(i, squashfsLayer))
	}

	ms = append(ms,
		mutate.SetHistory(ggcrv1.History{
			Created:    ggcrv1.Time{Time: time.Now()},
			CreatedBy:  useragent.Value(),
			Comment:    "oci-sif created from " + digest.Hex,
			EmptyLayer: false,
		}))

	sqfsImage, err = mutate.Apply(img, ms...)
	if err != nil {
		return nil, fmt.Errorf("while replacing layers: %w", err)
	}

	return sqfsImage, nil
}
