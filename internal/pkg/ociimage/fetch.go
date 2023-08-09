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

	"github.com/containers/image/v5/copy"
	ocilayout "github.com/containers/image/v5/oci/layout"
	"github.com/containers/image/v5/types"
	"github.com/opencontainers/go-digest"
	"github.com/sylabs/singularity/v4/internal/pkg/cache"
	"github.com/sylabs/singularity/v4/pkg/sylog"
)

// FetchLayout will fetch the OCI image specified by imageRef to a containers/image OCI layout in layoutDir.
// An ImageReference to the image that was fetched into layoutDir is returned on success.
// If imgCache is non-nil, and enabled, the image will be pulled through the cache.
func FetchLayout(ctx context.Context, sysCtx *types.SystemContext, imgCache *cache.Handle, imageRef, layoutDir string) (types.ImageReference, digest.Digest, error) {
	policyCtx, err := defaultPolicy()
	if err != nil {
		return nil, "", err
	}

	srcRef, err := ParseImageRef(imageRef)
	if err != nil {
		return nil, "", fmt.Errorf("invalid image source: %v", err)
	}

	// oci-archive direct handling by containers/image can fail as non-root.
	// Perform a tar extraction first, and handle as an oci layout.
	if os.Geteuid() != 0 && srcRef.Transport().Name() == "oci-archive" {
		var tmpDir string
		tmpDir, err = os.MkdirTemp(sysCtx.BigFilesTemporaryDir, "temp-oci-")
		if err != nil {
			return nil, "", fmt.Errorf("could not create temporary oci directory: %v", err)
		}
		defer os.RemoveAll(tmpDir)

		archiveParts := strings.SplitN(srcRef.StringWithinTransport(), ":", 2)
		sylog.Debugf("Extracting oci-archive %q to %q", archiveParts[0], tmpDir)
		err = extractArchive(archiveParts[0], tmpDir)
		if err != nil {
			return nil, "", fmt.Errorf("error extracting the OCI archive file: %v", err)
		}
		// We may or may not have had a ':tag' in the source to handle
		if len(archiveParts) == 2 {
			srcRef, err = ocilayout.ParseReference(tmpDir + ":" + archiveParts[1])
		} else {
			srcRef, err = ocilayout.ParseReference(tmpDir)
		}
		if err != nil {
			return nil, "", err
		}
	}

	var imgDigest digest.Digest

	if imgCache != nil && !imgCache.IsDisabled() {
		// Grab the modified source ref from the cache
		srcRef, imgDigest, err = CacheReference(ctx, sysCtx, imgCache, srcRef)
		if err != nil {
			return nil, "", err
		}
	}

	lr, err := ocilayout.ParseReference(layoutDir + ":" + imgDigest.String())
	if err != nil {
		return nil, "", err
	}

	_, err = copy.Image(ctx, policyCtx, lr, srcRef, &copy.Options{
		ReportWriter: io.Discard,
		SourceCtx:    sysCtx,
	})
	if err != nil {
		return nil, "", err
	}

	return lr, imgDigest, nil
}

// Perform a dumb tar(gz) extraction with no chown, id remapping etc.
// This is needed for non-root handling of `oci-archive` as the extraction
// by containers/archive is failing when uid/gid don't match local machine
// and we're not root
func extractArchive(src string, dst string) error {
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
			if _, err := io.Copy(f, tr); err != nil {
				return err
			}
		}
	}
}
