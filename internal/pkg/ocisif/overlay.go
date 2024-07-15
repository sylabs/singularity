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
	ocitmutate "github.com/sylabs/oci-tools/pkg/mutate"
	ocitsif "github.com/sylabs/oci-tools/pkg/sif"
	"github.com/sylabs/sif/v2/pkg/sif"
	"github.com/sylabs/singularity/v4/pkg/image"
	"github.com/sylabs/singularity/v4/pkg/sylog"
)

var Ext3LayerMediaType types.MediaType = "application/vnd.sylabs.image.layer.v1.ext3"

func getSingleImage(fi *sif.FileImage) (v1.Image, error) {
	ii, err := ocitsif.ImageIndexFromFileImage(fi)
	if err != nil {
		return nil, fmt.Errorf("while obtaining image index: %w", err)
	}
	ix, err := ii.IndexManifest()
	if err != nil {
		return nil, fmt.Errorf("while obtaining index manifest: %w", err)
	}

	// One image only.
	if len(ix.Manifests) != 1 {
		return nil, fmt.Errorf("only single image data containers are supported, found %d images", len(ix.Manifests))
	}
	imageDigest := ix.Manifests[0].Digest
	return ii.Image(imageDigest)
}

// HasOverlay returns whether the OCI-SIF at imgPath has an ext3 writable final
// layer - an 'overlay'. If present, the offset of the overlay data in the
// OCI-SIF file is also returned.
func HasOverlay(imagePath string) (bool, int64, error) {
	fi, err := sif.LoadContainerFromPath(imagePath,
		sif.OptLoadWithFlag(os.O_RDONLY),
	)
	if err != nil {
		return false, 0, err
	}
	defer fi.UnloadContainer()

	img, err := getSingleImage(fi)
	if err != nil {
		return false, 0, fmt.Errorf("while getting image: %w", err)
	}
	layers, err := img.Layers()
	if err != nil {
		return false, 0, fmt.Errorf("while getting image layers: %w", err)
	}
	if len(layers) < 1 {
		return false, 0, fmt.Errorf("image has no layers")
	}
	mt, err := layers[len(layers)-1].MediaType()
	if err != nil {
		return false, 0, fmt.Errorf("while getting layer mediatype: %w", err)
	}
	// Not an overlay as last layer
	if mt != Ext3LayerMediaType {
		return false, 0, nil
	}

	// Overlay as last layer, get offset
	ld, err := layers[len(layers)-1].Digest()
	if err != nil {
		return false, 0, fmt.Errorf("while getting layer digest: %w", err)
	}
	desc, err := fi.GetDescriptor(sif.WithOCIBlobDigest(ld))
	if err != nil {
		return false, 0, fmt.Errorf("while getting layer descriptor: %w", err)
	}
	return true, desc.Offset(), nil
}

// AddOverlay adds the provided ext3 overlay file at overlayPath to the OCI-SIF
// at imagePath, as a new image layer.
func AddOverlay(imagePath string, overlayPath string) error {
	fi, err := sif.LoadContainerFromPath(imagePath)
	if err != nil {
		return err
	}
	defer fi.UnloadContainer()

	img, err := getSingleImage(fi)
	if err != nil {
		return fmt.Errorf("while getting image: %w", err)
	}

	ol, err := ext3LayerFromFile(overlayPath)
	if err != nil {
		return err
	}

	img, err = mutate.AppendLayers(img, ol)
	if err != nil {
		return err
	}

	ii := mutate.AppendManifests(empty.Index, mutate.IndexAddendum{Add: img})

	return ocitsif.Update(fi, ii)
}

// SyncOverlay synchronizes the digests of the overlay, stored in the OCI
// structures, with its true content.
func SyncOverlay(imagePath string) error {
	fi, err := sif.LoadContainerFromPath(imagePath)
	if err != nil {
		return err
	}
	defer fi.UnloadContainer()

	img, err := getSingleImage(fi)
	if err != nil {
		return fmt.Errorf("while getting image: %w", err)
	}
	layers, err := img.Layers()
	if err != nil {
		return fmt.Errorf("while getting image layers: %w", err)
	}
	if len(layers) < 1 {
		return fmt.Errorf("image has no layers")
	}
	oldLayer := layers[len(layers)-1]
	mt, err := oldLayer.MediaType()
	if err != nil {
		return fmt.Errorf("while getting layer mediatype: %w", err)
	}
	if mt != Ext3LayerMediaType {
		return fmt.Errorf("image does not contain a writable overlay")
	}

	// Existing descriptor and digest
	oldDigest, err := oldLayer.Digest()
	if err != nil {
		return err
	}
	desc, err := fi.GetDescriptor(sif.WithOCIBlobDigest(oldDigest))
	if err != nil {
		return err
	}
	// Updated layer and digest
	o := func() (io.ReadCloser, error) {
		return io.NopCloser(desc.GetReader()), nil
	}
	newLayer, err := ext3LayerFromOpener(o)
	if err != nil {
		return err
	}
	newDigest, err := newLayer.Digest()
	if err != nil {
		return err
	}

	if newDigest == oldDigest {
		sylog.Infof("OCI digest matches overlay, no update required.")
		return nil
	}

	// Update overlay OCI.Blob digest in SIF. This must be done before the
	// oci-tools Update is called, so that it re-uses the existing overlay
	// descriptor, with the updated digest.
	if err := fi.SetOCIBlobDigest(desc.ID(), newDigest); err != nil {
		return fmt.Errorf("while updating descriptor digest: %v", err)
	}

	img, err = ocitmutate.Apply(img, ocitmutate.SetLayer(len(layers)-1, newLayer))
	if err != nil {
		return err
	}
	ii := mutate.AppendManifests(empty.Index, mutate.IndexAddendum{Add: img})
	return ocitsif.Update(fi, ii)
}

// ext3ImageOpener opens a source ext3 overlay image file to be added as a layer.
type ext3ImageOpener func() (io.ReadCloser, error)

type ext3ImageLayer struct {
	opener ext3ImageOpener
	digest v1.Hash
	diffID v1.Hash
	size   int64
}

var _ v1.Layer = (*ext3ImageLayer)(nil)

// Descriptor returns the original descriptor from an image manifest. See partial.Descriptor.
func (l *ext3ImageLayer) Descriptor() (*v1.Descriptor, error) {
	return &v1.Descriptor{
		Size:      l.size,
		Digest:    l.digest,
		MediaType: Ext3LayerMediaType,
	}, nil
}

// Digest implements v1.Layer
func (l *ext3ImageLayer) Digest() (v1.Hash, error) {
	return l.digest, nil
}

// DiffID returns the Hash of the uncompressed layer.
func (l *ext3ImageLayer) DiffID() (v1.Hash, error) {
	return l.diffID, nil
}

// Compressed returns an io.ReadCloser for the compressed layer contents.
func (l *ext3ImageLayer) Compressed() (io.ReadCloser, error) {
	return l.opener()
}

// Uncompressed returns an io.ReadCloser for the uncompressed layer contents.
func (l *ext3ImageLayer) Uncompressed() (io.ReadCloser, error) {
	return l.opener()
}

// Size returns the compressed size of the Layer.
func (l *ext3ImageLayer) Size() (int64, error) {
	return l.size, nil
}

// MediaType returns the media type of the Layer.
func (l *ext3ImageLayer) MediaType() (types.MediaType, error) {
	return Ext3LayerMediaType, nil
}

func ext3LayerFromFile(path string) (v1.Layer, error) {
	opener := func() (io.ReadCloser, error) {
		return os.Open(path)
	}
	return ext3LayerFromOpener(opener)
}

const hdrBuffSize = 2048

func ext3LayerFromOpener(opener ext3ImageOpener) (v1.Layer, error) {
	rc, err := opener()
	if err != nil {
		return nil, err
	}
	defer rc.Close()

	// Ensure our source is really an ext3 image
	b := make([]byte, hdrBuffSize)
	if n, err := rc.Read(b); err != nil || n != hdrBuffSize {
		return nil, fmt.Errorf("while reading overlay file header: %w", err)
	}
	_, err = image.CheckExt3Header(b)
	if err != nil {
		return nil, fmt.Errorf("while checking overlay file header: %w", err)
	}

	// Re-open rather than seek, so we can use the SIF GetReader API which
	// returns an io.Reader only.
	if err := rc.Close(); err != nil {
		return nil, err
	}
	rc, err = opener()
	if err != nil {
		return nil, err
	}
	defer rc.Close()

	digest, size, err := v1.SHA256(rc)
	if err != nil {
		return nil, err
	}

	l := &ext3ImageLayer{
		opener: opener,
		digest: digest,
		diffID: digest, // no compression - diffID = digest
		size:   size,
	}
	return l, nil
}
