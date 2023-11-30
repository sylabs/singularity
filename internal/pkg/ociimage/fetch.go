// Copyright (c) 2019-2023, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package ociimage

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	ggcrv1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/sylabs/singularity/v4/internal/pkg/cache"
	"github.com/sylabs/singularity/v4/internal/pkg/client/progress"
	"github.com/sylabs/singularity/v4/pkg/sylog"
)

// cachedImage will ensure that the provided v1.Image is present in the Singularity
// OCI cache layout dir, and return a new v1.Image pointing to the cached copy.
func cachedImage(ctx context.Context, imgCache *cache.Handle, srcImg ggcrv1.Image) (ggcrv1.Image, error) {
	if imgCache == nil || imgCache.IsDisabled() {
		return nil, fmt.Errorf("undefined image cache")
	}

	digest, err := srcImg.Digest()
	if err != nil {
		return nil, err
	}

	layoutDir, err := imgCache.GetOciCacheDir(cache.OciBlobCacheType)
	if err != nil {
		return nil, err
	}

	cachedRef := layoutDir + "@" + digest.String()
	sylog.Debugf("Caching image to %s", cachedRef)
	if err := OCISourceSink.WriteImage(srcImg, layoutDir, nil); err != nil {
		return nil, err
	}

	return OCISourceSink.Image(ctx, cachedRef, nil, nil)
}

// FetchToLayout will fetch the OCI image specified by imageRef to an OCI layout
// and return a v1.Image referencing it. If imgCache is non-nil, and enabled,
// the image will be fetched into Singularity's cache - which is a multi-image
// OCI layout. If the cache is disabled, the image will be fetched into a
// subdirectory of the provided tmpDir. The caller is responsible for cleaning
// up tmpDir.
func FetchToLayout(ctx context.Context, tOpts *TransportOptions, imgCache *cache.Handle, imageURI, tmpDir string) (ggcrv1.Image, error) {
	// oci-archive - Perform a tar extraction first, and handle as an oci layout.
	if strings.HasPrefix(imageURI, "oci-archive:") {
		layoutURI, cleanup, err := extractOCIArchive(imageURI, tmpDir)
		if err != nil {
			return nil, err
		}
		defer cleanup()
		imageURI = layoutURI
	}

	srcType, srcRef, err := URItoSourceSinkRef(imageURI)
	if err != nil {
		return nil, err
	}

	rt := progress.NewRoundTripper(ctx, nil)

	srcImg, err := srcType.Image(ctx, srcRef, tOpts, rt)
	if err != nil {
		rt.ProgressShutdown()
		return nil, err
	}

	if imgCache != nil && !imgCache.IsDisabled() {
		// Ensure the image is cached, and return reference to the cached image.
		cachedImg, err := cachedImage(ctx, imgCache, srcImg)
		if err != nil {
			rt.ProgressShutdown()
			return nil, err
		}
		rt.ProgressComplete()
		rt.ProgressWait()
		return cachedImg, nil
	}

	// No cache - write to layout directory provided
	tmpLayout, err := os.MkdirTemp(tmpDir, "layout-")
	if err != nil {
		return nil, err
	}
	sylog.Debugf("Copying %q to temporary layout at %q", srcRef, tmpLayout)
	if err = OCISourceSink.WriteImage(srcImg, tmpLayout, nil); err != nil {
		rt.ProgressShutdown()
		return nil, err
	}
	rt.ProgressComplete()
	rt.ProgressWait()

	return OCISourceSink.Image(ctx, tmpLayout, tOpts, nil)
}

// extractOCIArchive will extract a tar `oci-archive:` image into a temporary
// layout dir. The caller is responsible for calling cleanup() to remove the
// temporary layout.
func extractOCIArchive(archiveURI, tmpDir string) (layoutURI string, cleanup func(), err error) {
	layoutDir, err := os.MkdirTemp(tmpDir, "temp-oci-")
	if err != nil {
		return "", nil, fmt.Errorf("could not create temporary oci directory: %v", err)
	}
	// oci-archive:<path>[:tag]
	refParts := strings.SplitN(archiveURI, ":", 3)
	sylog.Debugf("Extracting oci-archive %q to %q", refParts[1], layoutDir)
	err = extractTarNaive(refParts[1], layoutDir)
	if err != nil {
		os.RemoveAll(layoutDir)
		return "", nil, fmt.Errorf("error extracting the OCI archive file: %v", err)
	}
	// We may or may not have had a ':tag' in the source to handle
	layoutURI = "oci:" + layoutDir
	if len(refParts) == 3 {
		layoutURI = layoutURI + ":" + refParts[2]
	}
	cleanup = func() {
		os.RemoveAll(layoutDir)
	}
	return layoutURI, cleanup, nil
}

// extractTarNaive will extract a tar with no chown, id remapping etc. It only
// writes directories and regular files. This naive extraction avoids any
// permissions / xattr issues when extracting a tarred OCI layout.
func extractTarNaive(src string, dst string) error {
	f, err := os.Open(src)
	if err != nil {
		return err
	}
	defer f.Close()

	r := bufio.NewReader(f)
	header, err := r.Peek(10) // read a few bytes without consuming
	if err != nil {
		return err
	}
	gzipped := strings.Contains(http.DetectContentType(header), "x-gzip")

	if gzipped {
		r, err := gzip.NewReader(f)
		if err != nil {
			return err
		}
		defer r.Close()
	}

	tr := tar.NewReader(r)

	for {
		header, err := tr.Next()
		switch {
		// if no more files are found return
		case err == io.EOF:
			return nil

		// return any other error
		case err != nil:
			return err

		// if the header is nil, just skip it (not sure how this happens)
		case header == nil:
			continue
		}

		// ZipSlip protection - don't escape from dst
		//#nosec G305
		target := filepath.Join(dst, header.Name)
		if !strings.HasPrefix(target, filepath.Clean(dst)+string(os.PathSeparator)) {
			return fmt.Errorf("%s: illegal extraction path", target)
		}

		// check the file type
		switch header.Typeflag {
		// if its a dir and it doesn't exist create it
		case tar.TypeDir:
			if _, err := os.Stat(target); err != nil {
				if err := os.MkdirAll(target, 0o755); err != nil {
					return err
				}
			}
		// if it's a file create it
		case tar.TypeReg:
			f, err := os.OpenFile(target, os.O_CREATE|os.O_RDWR, os.FileMode(header.Mode))
			if err != nil {
				return err
			}
			defer f.Close()

			// copy over contents
			//#nosec G110
			if _, err := io.Copy(f, tr); err != nil {
				return err
			}
		}
	}
}
