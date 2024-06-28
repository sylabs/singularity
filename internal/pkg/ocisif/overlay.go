// Copyright (c) 2024, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package ocisif

import (
	"fmt"
	"os"

	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/stream"
	"github.com/google/go-containerregistry/pkg/v1/types"
	ocitsif "github.com/sylabs/oci-tools/pkg/sif"
	"github.com/sylabs/sif/v2/pkg/sif"
)

var ExtfsLayerMediaType types.MediaType = "application/vnd.sylabs.image.layer.v1.extfs"

// HasOverlay returns whether the OCI-SIF has an extfs writable final layer - an
// 'overlay'.
func HasOverlay(rw sif.ReadWriter) (bool, error) {
	fi, err := sif.LoadContainer(rw,
		sif.OptLoadWithFlag(os.O_RDONLY),
		sif.OptLoadWithCloseOnUnload(false),
	)
	if err != nil {
		return false, err
	}
	defer fi.UnloadContainer()

	ii, err := ocitsif.ImageIndexFromFileImage(fi)
	if err != nil {
		return false, fmt.Errorf("while obtaining image index: %w", err)
	}
	ix, err := ii.IndexManifest()
	if err != nil {
		return false, fmt.Errorf("while obtaining index manifest: %w", err)
	}

	// One image only.
	if len(ix.Manifests) != 1 {
		return false, fmt.Errorf("only single image data containers are supported, found %d images", len(ix.Manifests))
	}
	imageDigest := ix.Manifests[0].Digest
	img, err := ii.Image(imageDigest)
	if err != nil {
		return false, fmt.Errorf("while initializing image: %w", err)
	}

	layers, err := img.Layers()
	if err != nil {
		return false, fmt.Errorf("while getting image layers: %w", err)
	}
	mt, err := layers[len(layers)-1].MediaType()
	if err != nil {
		return false, fmt.Errorf("while getting layer mediatype: %w", err)
	}
	return mt == ExtfsLayerMediaType, nil
}

// AddOverlay adds the provided overlay to the OCI-SIF as an image layer.
func AddOverlay(sifPath string, overlayPath string) error {
	fi, err := sif.LoadContainerFromPath(sifPath)
	if err != nil {
		return err
	}
	defer fi.UnloadContainer()

	ii, err := ocitsif.ImageIndexFromFileImage(fi)
	if err != nil {
		return fmt.Errorf("while obtaining image index: %w", err)
	}
	ix, err := ii.IndexManifest()
	if err != nil {
		return fmt.Errorf("while obtaining index manifest: %w", err)
	}

	// One image only.
	if len(ix.Manifests) != 1 {
		return fmt.Errorf("only single image data containers are supported, found %d images", len(ix.Manifests))
	}
	imageDigest := ix.Manifests[0].Digest
	img, err := ii.Image(imageDigest)
	if err != nil {
		return fmt.Errorf("while initializing image: %w", err)
	}

	oReader, err := os.Open(overlayPath)
	if err != nil {
		return err
	}
	defer oReader.Close()

	oLayer := stream.NewLayer(oReader, stream.WithMediaType(ExtfsLayerMediaType))
	img, err = mutate.AppendLayers(img, oLayer)
	if err != nil {
		return err
	}

	ii = mutate.AppendManifests(empty.Index, mutate.IndexAddendum{Add: img})

	return ocitsif.Update(fi, ii)
}
