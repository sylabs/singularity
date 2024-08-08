// Copyright (c) 2019-2024, Sylabs Inc. All rights reserved.
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
	"github.com/sylabs/singularity/v4/internal/pkg/ociplatform"
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

// LocalImage returns a ggrcv1.Image for imageURI that is guaranteed to be
// backed by a local file or directory. If the image is an OCI layout or docker
// tarball then it can be accessed directly. If the image is a tarball of an OCI
// layout then it is extracted to tmpDir. If the image is remote, or in the
// docker daemon, it will be pulled into the local cache - which is a
// multi-image OCI layout. If the cache is disabled, the image will be fetched
// into a subdirectory of the provided tmpDir. The caller is responsible for
// cleaning up tmpDir. The platform of the image will be checked against
// tOpts.Platform.
func LocalImage(ctx context.Context, tOpts *TransportOptions, imgCache *cache.Handle, imageURI, tmpDir string) (ggcrv1.Image, error) {
	// oci-archive tarball is a local file, but current ggcr cannot read
	// directly. Must always extract to a layoutdir .
	if strings.HasPrefix(imageURI, "oci-archive:") {
		return LocalImageLayout(ctx, tOpts, imgCache, imageURI, tmpDir)
	}

	srcType, srcRef, err := URItoSourceSinkRef(imageURI)
	if err != nil {
		return nil, err
	}
	// Docker tarballs and OCI layouts are already local
	if srcType == TarballSourceSink || srcType == OCISourceSink {
		img, err := srcType.Image(ctx, srcRef, tOpts, nil)
		if err != nil {
			return nil, err
		}
		// Verify against requested platform - ggcr doesn't filter on platform
		// when pulling a manifest directly, only on pulling from an image index.
		if err := ociplatform.CheckImagePlatform(tOpts.Platform, img); err != nil {
			return nil, fmt.Errorf("while checking OCI image: %w", err)
		}
		return img, nil
	}

	return LocalImageLayout(ctx, tOpts, imgCache, imageURI, tmpDir)
}

// LocalImage returns a ggrcv1.Image for imageURI that is guaranteed to be
// backed by a local OCI layout directory. If the image is a tarball of an OCI
// layout then it is extracted to tmpDir. If the image is remote, or in the
// docker daemon, it will be pulled into the local cache - which is a
// multi-image OCI layout. If the cache is disabled, the image will be fetched
// into a subdirectory of the provided tmpDir. The caller is responsible for
// cleaning up tmpDir. The platform of the image will be checked against
// tOpts.Platform.
func LocalImageLayout(ctx context.Context, tOpts *TransportOptions, imgCache *cache.Handle, imageURI, tmpDir string) (ggcrv1.Image, error) {
	if strings.HasPrefix(imageURI, "oci-archive:") {
		// oci-archive is a straight tar of an OCI layout, so extract to a tempDir
		layoutURI, _, err := extractOCIArchive(imageURI, tmpDir)
		if err != nil {
			return nil, err
		}
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

	// Verify against requested platform - ggcr doesn't filter on platform when
	// pulling a manifest directly, only on pulling from an image index.
	if err := ociplatform.CheckImagePlatform(tOpts.Platform, srcImg); err != nil {
		return nil, fmt.Errorf("while checking OCI image: %w", err)
	}

	// We might already have an OCI layout at this point - which is local.
	if srcType == OCISourceSink {
		rt.ProgressShutdown()
		return srcImg, nil
	}

	// Registry / Docker Daemon images need to be fetched

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
// layout that will be a subdirectory of tmpDir. The caller is responsible for
// calling cleanup to remove the layout, or otherwise cleaning up tmpDir.
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
			if _, err := io.Copy(f, tr); err != nil { //nolint:gosec
				return err
			}
		}
	}
}
