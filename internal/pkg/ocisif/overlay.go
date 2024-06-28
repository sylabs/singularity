// Copyright (c) 2024, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package ocisif

import (
	"fmt"
	"io"
	"os"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/types"
	ocitsif "github.com/sylabs/oci-tools/pkg/sif"
	"github.com/sylabs/sif/v2/pkg/sif"
	"github.com/sylabs/singularity/v4/pkg/image"
)

var ExtfsLayerMediaType types.MediaType = "application/vnd.sylabs.image.layer.v1.extfs"

// HasOverlay returns whether the OCI-SIF at imgPath has an extfs writable final
// layer - an 'overlay'.
func HasOverlay(imagePath string) (bool, error) {
	fi, err := sif.LoadContainerFromPath(imagePath,
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

// AddOverlay adds the provided extfs overlay file at overlayPath to the OCI-SIF
// at imagePath, as a new image layer.
func AddOverlay(imagePath string, overlayPath string) error {
	fi, err := sif.LoadContainerFromPath(imagePath)
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

	ol, err := overlayLayerFromFile(overlayPath)
	if err != nil {
		return err
	}

	img, err = mutate.AppendLayers(img, ol)
	if err != nil {
		return err
	}

	ii = mutate.AppendManifests(empty.Index, mutate.IndexAddendum{Add: img})

	return ocitsif.Update(fi, ii)
}

// OverlayOpener opens a source overlay image file to be added as a layer.
type OverlayOpener func() (io.ReadSeekCloser, error)

type overlayLayer struct {
	opener OverlayOpener
	digest v1.Hash
	diffID v1.Hash
	size   int64
}

var _ v1.Layer = (*overlayLayer)(nil)

// Descriptor returns the original descriptor from an image manifest. See partial.Descriptor.
func (l *overlayLayer) Descriptor() (*v1.Descriptor, error) {
	return &v1.Descriptor{
		Size:      l.size,
		Digest:    l.digest,
		MediaType: ExtfsLayerMediaType,
	}, nil
}

// Digest implements v1.Layer
func (l *overlayLayer) Digest() (v1.Hash, error) {
	return l.digest, nil
}

// DiffID returns the Hash of the uncompressed layer.
func (l *overlayLayer) DiffID() (v1.Hash, error) {
	return l.diffID, nil
}

// Compressed returns an io.ReadCloser for the compressed layer contents.
func (l *overlayLayer) Compressed() (io.ReadCloser, error) {
	return l.opener()
}

// Uncompressed returns an io.ReadCloser for the uncompressed layer contents.
func (l *overlayLayer) Uncompressed() (io.ReadCloser, error) {
	return l.opener()
}

// Size returns the compressed size of the Layer.
func (l *overlayLayer) Size() (int64, error) {
	return l.size, nil
}

// MediaType returns the media type of the Layer.
func (l *overlayLayer) MediaType() (types.MediaType, error) {
	return ExtfsLayerMediaType, nil
}

func overlayLayerFromFile(path string) (v1.Layer, error) {
	opener := func() (io.ReadSeekCloser, error) {
		return os.Open(path)
	}
	return overlayLayerFromOpener(opener)
}

const hdrBuffSize = 2048

func overlayLayerFromOpener(opener OverlayOpener) (v1.Layer, error) {
	rc, err := opener()
	if err != nil {
		return nil, err
	}
	defer rc.Close()

	// Ensure our source is an extfs image
	b := make([]byte, hdrBuffSize)
	if n, err := rc.Read(b); err != nil || n != hdrBuffSize {
		return nil, fmt.Errorf("while reading overlay file header: %w", err)
	}
	_, err = image.CheckExt3Header(b)
	if err != nil {
		return nil, fmt.Errorf("while checking overlay file header: %w", err)
	}

	_, err = rc.Seek(0, io.SeekStart)
	if err != nil {
		return nil, err
	}
	digest, size, err := v1.SHA256(rc)
	if err != nil {
		return nil, err
	}

	l := &overlayLayer{
		opener: opener,
		digest: digest,
		diffID: digest, // extfs is not compressed so we take digest == diffID for simplicity, though really it's over the filesystem instead of the content.
		size:   size,
	}
	return l, nil
}
