// Copyright (c) 2024, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package datacontainer

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sync"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	ocimutate "github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
	"github.com/google/go-containerregistry/pkg/v1/types"
	"github.com/sylabs/oci-tools/pkg/mutate"
	ocisif "github.com/sylabs/oci-tools/pkg/sif"
)

// ConfigMediaType custom media type.
const (
	ConfigMediaType types.MediaType = "application/vnd.sylabs.data-container.config.v1+json"
)

// Placeholder config - will become empty JSON in written image
type Config struct{}

// WriteSIFFromPath takes a path to a directory or regular file, and writes
// a data container image populated with the directory/file to dest, as an OCI-SIF.
func WriteOCISIFFromPath(path string, dst string, workDir string) error {
	img, err := newImageFromFSPath(os.DirFS(filepath.Dir(path)), filepath.Base(path), Config{}, workDir)
	if err != nil {
		return err
	}

	ii := ocimutate.AppendManifests(empty.Index,
		ocimutate.IndexAddendum{
			Add: img,
		})
	return ocisif.Write(dst, ii)
}

// newImageFromFSPath takes a path to a directory or regular file within fsys, and returns
// a data container image populated with the directory/file.
func newImageFromFSPath(fsys fs.FS, path string, cfg Config, workDir string) (v1.Image, error) {
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

	sqOpts := []mutate.SquashfsConverterOpt{
		mutate.OptSquashfsSkipWhiteoutConversion(true),
	}
	squashfsLayer, err := mutate.SquashfsLayer(l, workDir, sqOpts...)
	if err != nil {
		return nil, err
	}

	return createDataContainerFromLayer(squashfsLayer, cfg)
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
func createDataContainerFromLayer(layer v1.Layer, cfg Config) (v1.Image, error) {
	img := ocimutate.MediaType(empty.Image, types.OCIManifestSchema1)

	img, err := ocimutate.AppendLayers(img, layer)
	if err != nil {
		return nil, err
	}

	return mutate.Apply(img,
		mutate.SetConfig(cfg, ConfigMediaType),
	)
}
