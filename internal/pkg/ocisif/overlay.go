// Copyright (c) 2024, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package ocisif

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/types"
	ocitmutate "github.com/sylabs/oci-tools/pkg/mutate"
	ocitsif "github.com/sylabs/oci-tools/pkg/sif"
	"github.com/sylabs/sif/v2/pkg/sif"
	"github.com/sylabs/singularity/v4/internal/pkg/util/fs/fuse"
	"github.com/sylabs/singularity/v4/internal/pkg/util/fs/squashfs"
	"github.com/sylabs/singularity/v4/pkg/image"
	"github.com/sylabs/singularity/v4/pkg/sylog"
)

var Ext3LayerMediaType types.MediaType = "application/vnd.sylabs.image.layer.v1.ext3"

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

	img, err := GetSingleImage(fi)
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

	img, err := GetSingleImage(fi)
	if err != nil {
		return fmt.Errorf("while getting image: %w", err)
	}

	ol, err := imageLayerFromFile(overlayPath, image.EXT3)
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

	img, err := GetSingleImage(fi)
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
	newLayer, err := imageLayerFromOpener(o, image.EXT3)
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

// SealOverlay converts an ext3 overlay into a r/o squashfs layer. If `tmpDir`
// is specified, then the temporary squashfs image will be created inside it. If
// tmpDir is the empty string, then temporary files will be created at the
// location returned by os.TempDir.
func SealOverlay(imagePath, tmpDir string) error {
	fi, err := sif.LoadContainerFromPath(imagePath)
	if err != nil {
		return err
	}
	defer fi.UnloadContainer()

	img, err := GetSingleImage(fi)
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
	l := layers[len(layers)-1]
	mt, err := l.MediaType()
	if err != nil {
		return fmt.Errorf("while getting layer mediatype: %w", err)
	}
	if mt != Ext3LayerMediaType {
		return fmt.Errorf("image does not contain a writable overlay")
	}

	d, err := l.Digest()
	if err != nil {
		return err
	}
	desc, err := fi.GetDescriptor(sif.WithOCIBlobDigest(d))
	if err != nil {
		return err
	}

	// Mount extfs inside tmpDir. We use a subdirectory so that the permissions
	// applied from the root of image cannot open tmpDir to other users.
	tmpDir, err = os.MkdirTemp(tmpDir, "overlay-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)
	mntDir := filepath.Join(tmpDir, "overlay")
	if err := os.Mkdir(mntDir, 0o755); err != nil {
		return err
	}
	im := fuse.ImageMount{
		Type:       image.EXT3,
		Readonly:   true,
		SourcePath: imagePath,
		ExtraOpts:  []string{fmt.Sprintf("offset=%d", desc.Offset())},
	}
	im.SetMountPoint(mntDir)
	if err := im.Mount(context.TODO()); err != nil {
		return err
	}
	// Create squashfs from upper directory inside mounted extfs dir.
	upperDir := filepath.Join(mntDir, "upper")
	sqfs := filepath.Join(tmpDir, "overlay.sqfs")

	// Exclude the dangling `.wh..opq` 0:0 char device whiteout marker created
	// by fuse-overlayfs inside opaque directories. This is neither a valid AUFS
	// or native OverlayFS whiteout.
	// See: https://github.com/sylabs/singularity/issues/3176
	sqfsOpts := []squashfs.MksquashfsOpt{
		squashfs.OptWildcards(true),
		// '...' indicates an non-rooted match, i.e. match .wh..opq in any directory.
		squashfs.OptExcludes([]string{"... .wh..opq"}),
	}

	if err := squashfs.Mksquashfs([]string{upperDir}, sqfs, sqfsOpts...); err != nil {
		return err
	}
	// Unmount the extfs.
	if err := im.Unmount(context.TODO()); err != nil {
		return err
	}
	// Replace extfs overlay layer with the squashfs.
	newLayer, err := imageLayerFromFile(sqfs, image.SQUASHFS)
	if err != nil {
		return err
	}
	img, err = ocitmutate.Apply(img, ocitmutate.SetLayer(len(layers)-1, newLayer))
	if err != nil {
		return err
	}
	ii := mutate.AppendManifests(empty.Index, mutate.IndexAddendum{Add: img})
	return ocitsif.Update(fi, ii)
}

// imageOpener opens an ext3 or squashfs filesystem image file to be added as a
// layer.
type imageOpener func() (io.ReadCloser, error)

type imageLayer struct {
	imageType int // image.EXT3 and image.SQUASHFS are currently implemented
	opener    imageOpener
	digest    v1.Hash
	diffID    v1.Hash
	size      int64
}

var _ v1.Layer = (*imageLayer)(nil)

// Descriptor returns the original descriptor from an image manifest. See partial.Descriptor.
func (l *imageLayer) Descriptor() (*v1.Descriptor, error) {
	mt, err := l.MediaType()
	if err != nil {
		return nil, err
	}
	return &v1.Descriptor{
		Size:      l.size,
		Digest:    l.digest,
		MediaType: mt,
	}, nil
}

// Digest implements v1.Layer
func (l *imageLayer) Digest() (v1.Hash, error) {
	return l.digest, nil
}

// DiffID returns the Hash of the uncompressed layer.
func (l *imageLayer) DiffID() (v1.Hash, error) {
	return l.diffID, nil
}

// Compressed returns an io.ReadCloser for the compressed layer contents.
func (l *imageLayer) Compressed() (io.ReadCloser, error) {
	return l.opener()
}

// Uncompressed returns an io.ReadCloser for the uncompressed layer contents.
func (l *imageLayer) Uncompressed() (io.ReadCloser, error) {
	return l.opener()
}

// Size returns the compressed size of the Layer.
func (l *imageLayer) Size() (int64, error) {
	return l.size, nil
}

// MediaType returns the media type of the Layer.
func (l *imageLayer) MediaType() (types.MediaType, error) {
	switch l.imageType {
	case image.EXT3:
		return Ext3LayerMediaType, nil
	case image.SQUASHFS:
		return SquashfsLayerMediaType, nil
	default:
		return "", errUnsupportedType
	}
}

func imageLayerFromFile(path string, imageType int) (v1.Layer, error) {
	opener := func() (io.ReadCloser, error) {
		return os.Open(path)
	}
	return imageLayerFromOpener(opener, imageType)
}

const hdrBuffSize = 2048

func imageLayerFromOpener(opener imageOpener, imageType int) (v1.Layer, error) {
	rc, err := opener()
	if err != nil {
		return nil, err
	}
	defer rc.Close()

	// Ensure our source is really the specified image type
	b := make([]byte, hdrBuffSize)
	if n, err := rc.Read(b); err != nil || n != hdrBuffSize {
		return nil, fmt.Errorf("while reading overlay file header: %w", err)
	}
	switch imageType {
	case image.EXT3:
		_, err = image.CheckExt3Header(b)
	case image.SQUASHFS:
		_, err = image.CheckSquashfsHeader(b)
	default:
		return nil, errUnsupportedType
	}
	if err != nil {
		return nil, fmt.Errorf("while checking image file header: %w", err)
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

	l := &imageLayer{
		imageType: imageType,
		opener:    opener,
		digest:    digest,
		diffID:    digest, // no compression - diffID = digest
		size:      size,
	}
	return l, nil
}
