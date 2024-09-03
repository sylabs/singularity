// Copyright (c) 2024, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package ocisif

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sync"

	ggcrv1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	ocimutate "github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
	"github.com/google/go-containerregistry/pkg/v1/types"
	"github.com/sylabs/oci-tools/pkg/mutate"
	ocitsif "github.com/sylabs/oci-tools/pkg/sif"
	"github.com/sylabs/sif/v2/pkg/sif"
)

// ConfigMediaType custom media type.
const (
	DataContainerArtifactType string          = "application/vnd.sylabs.data-container.v1"
	EmptyConfigMediaType      types.MediaType = "application/vnd.oci.empty.v1+json"
)

// WriteDataContainerFromPath takes a path to a directory or regular file, and writes
// a data container image populated with the directory/file to dest, as an OCI-SIF.
func WriteDataContainerFromPath(path string, dst string, workDir string) error {
	img, err := newDataContainerFromFSPath(os.DirFS(filepath.Dir(path)), filepath.Base(path))
	if err != nil {
		return err
	}
	w, err := NewImageWriter(img, dst, workDir,
		WithSquashFSLayers(true),
		WithArtifactType(DataContainerArtifactType),
	)
	if err != nil {
		return err
	}
	return w.Write()
}

// newDataContainerFromFSPath takes a path to a directory or regular file within fsys, and returns
// a data container image populated with the directory/file.
func newDataContainerFromFSPath(fsys fs.FS, path string) (ggcrv1.Image, error) {
	fi, err := fs.Stat(fsys, path)
	if err != nil {
		return nil, err
	}

	var fn tarWriterFunc

	switch t := fi.Mode().Type(); {
	case t.IsRegular():
		fn = fileTARWriter(fsys, path)

	case t.IsDir():
		fsys, err := fs.Sub(fsys, path)
		if err != nil {
			return nil, err
		}
		fn = fsTARWriter(fsys, ".")

	default:
		return nil, fmt.Errorf("%v: %w (%v)", path, errUnsupportedType, t)
	}

	l, err := tarball.LayerFromOpener(tarOpener(fn), tarball.WithMediaType(types.OCILayer))
	if err != nil {
		return nil, err
	}

	return createDataContainerFromLayer(l)
}

// tarOpener adapts a tarWriter to a tarball.Opener, in a way that is safe for concurrent use, as
// is common by go-containerregsitry.
func tarOpener(fn tarWriterFunc) tarball.Opener {
	var m sync.Mutex

	return func() (io.ReadCloser, error) {
		pr, pw := io.Pipe()
		go func() {
			m.Lock()
			defer m.Unlock()

			pw.CloseWithError(fn(pw))
		}()
		return pr, nil
	}
}

// createDataContainerFromLayer create OCI datacontainer from the supplied v1.Layer.
func createDataContainerFromLayer(layer ggcrv1.Layer) (ggcrv1.Image, error) {
	img := ocimutate.MediaType(empty.Image, types.OCIManifestSchema1)

	img, err := ocimutate.AppendLayers(img, layer)
	if err != nil {
		return nil, err
	}

	return mutate.Apply(img,
		mutate.SetConfig(struct{}{}, types.MediaType(EmptyConfigMediaType)),
	)
}

func DataContainerLayerOffset(f *os.File) (int64, error) {
	fimg, err := sif.LoadContainer(f,
		sif.OptLoadWithFlag(os.O_RDONLY),
		sif.OptLoadWithCloseOnUnload(false),
	)
	if err != nil {
		return 0, err
	}
	defer fimg.UnloadContainer()

	ix, err := ocitsif.ImageIndexFromFileImage(fimg)
	if err != nil {
		return 0, fmt.Errorf("while obtaining image index: %w", err)
	}
	idxManifest, err := ix.IndexManifest()
	if err != nil {
		return 0, fmt.Errorf("while obtaining index manifest: %w", err)
	}

	// One image only.
	if len(idxManifest.Manifests) != 1 {
		return 0, fmt.Errorf("only single image data containers are supported, found %d images", len(idxManifest.Manifests))
	}
	imageDigest := idxManifest.Manifests[0].Digest

	img, err := ix.Image(imageDigest)
	if err != nil {
		return 0, fmt.Errorf("while initializing image: %w", err)
	}

	// One SquashFS layer only.
	layers, err := img.Layers()
	if err != nil {
		return 0, fmt.Errorf("while getting image layers: %w", err)
	}
	if len(layers) != 1 {
		return 0, fmt.Errorf("only single layer data containers are supported, found %d layers", len(layers))
	}
	mt, err := layers[0].MediaType()
	if err != nil {
		return 0, fmt.Errorf("while getting layer mediatype: %w", err)
	}
	if mt != SquashfsLayerMediaType {
		return 0, fmt.Errorf("unsupported layer mediaType: %v", mt)
	}

	offset, err := layers[0].(*ocitsif.Layer).Offset()
	return offset, err
}
