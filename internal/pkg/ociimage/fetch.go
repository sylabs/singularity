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
	"github.com/containers/image/v5/docker"
	dockerarchive "github.com/containers/image/v5/docker/archive"
	dockerdaemon "github.com/containers/image/v5/docker/daemon"
	ociarchive "github.com/containers/image/v5/oci/archive"
	ocilayout "github.com/containers/image/v5/oci/layout"
	"github.com/containers/image/v5/signature"
	"github.com/containers/image/v5/types"
	"github.com/sylabs/singularity/internal/pkg/cache"
	"github.com/sylabs/singularity/pkg/sylog"
)

// FetchLayout will fetch the OCI image specified by imageRef to a containers/image OCI layout in layoutDir.
// An ImageReference to the image that was fetched into layoutDir is returned on success.
// If imgCache is non-nil, and enabled, the image will be pulled through the cache.
func FetchLayout(ctx context.Context, sysCtx *types.SystemContext, imgCache *cache.Handle, imageRef, layoutDir string) (layoutRef types.ImageReference, err error) {
	policy := &signature.Policy{Default: []signature.PolicyRequirement{signature.NewPRInsecureAcceptAnything()}}
	policyCtx, err := signature.NewPolicyContext(policy)
	if err != nil {
		return nil, err
	}

	parts := strings.SplitN(imageRef, ":", 2)
	if len(parts) < 2 {
		return nil, fmt.Errorf("could not parse image ref: %s", imageRef)
	}
	var srcRef types.ImageReference

	switch parts[0] {
	case "docker":
		srcRef, err = docker.ParseReference(parts[1])
	case "docker-archive":
		srcRef, err = dockerarchive.ParseReference(parts[1])
	case "docker-daemon":
		srcRef, err = dockerdaemon.ParseReference(parts[1])
	case "oci":
		srcRef, err = ocilayout.ParseReference(parts[1])
	case "oci-archive":
		if os.Geteuid() == 0 {
			// As root, the direct oci-archive handling will work
			srcRef, err = ociarchive.ParseReference(parts[1])
		} else {
			// As non-root we need to do a dumb tar extraction first
			var tmpDir string
			tmpDir, err = os.MkdirTemp(sysCtx.BigFilesTemporaryDir, "temp-oci-")
			if err != nil {
				return nil, fmt.Errorf("could not create temporary oci directory: %v", err)
			}
			defer os.RemoveAll(tmpDir)

			archiveParts := strings.SplitN(parts[1], ":", 2)
			sylog.Debugf("Extracting oci-archive %q to %q", archiveParts[0], tmpDir)
			err = extractArchive(archiveParts[0], tmpDir)
			if err != nil {
				return nil, fmt.Errorf("error extracting the OCI archive file: %v", err)
			}
			// We may or may not have had a ':tag' in the source to handle
			if len(archiveParts) == 2 {
				srcRef, err = ocilayout.ParseReference(tmpDir + ":" + archiveParts[1])
			} else {
				srcRef, err = ocilayout.ParseReference(tmpDir)
			}
		}
	default:
		return nil, fmt.Errorf("cannot create an OCI container from %s source", parts[0])
	}

	if err != nil {
		return nil, fmt.Errorf("invalid image source: %v", err)
	}

	if imgCache != nil && !imgCache.IsDisabled() {
		// Grab the modified source ref from the cache
		srcRef, err = ConvertReference(ctx, imgCache, srcRef, sysCtx)
		if err != nil {
			return nil, err
		}
	}

	lr, err := ocilayout.ParseReference(layoutDir + ":singularity")
	if err != nil {
		return nil, err
	}

	_, err = copy.Image(ctx, policyCtx, lr, srcRef, &copy.Options{
		ReportWriter: io.Discard,
		SourceCtx:    sysCtx,
	})
	if err != nil {
		return nil, err
	}

	return lr, nil
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
