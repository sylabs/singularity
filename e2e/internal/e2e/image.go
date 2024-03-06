// Copyright (c) 2019-2022, Sylabs Inc. All rights reserved.
// Copyright (c) Contributors to the Apptainer project, established as
//   Apptainer a Series of LF Projects LLC.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package e2e

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"

	"github.com/sylabs/singularity/v4/internal/pkg/ociimage"
	"github.com/sylabs/singularity/v4/internal/pkg/ociplatform"
	"github.com/sylabs/singularity/v4/internal/pkg/test/tool/exec"
	"github.com/sylabs/singularity/v4/pkg/syfs"
)

var (
	ensureMutex  sync.Mutex
	pullMutex    sync.Mutex
	ociCopyMutex sync.Mutex
)

// EnsureImage checks if e2e test image is already built or built
// it otherwise.
func EnsureImage(t *testing.T, env TestEnv) {
	ensureMutex.Lock()
	defer ensureMutex.Unlock()

	switch _, err := os.Stat(env.ImagePath); {
	case err == nil:
		// OK: file exists, return
		return

	case os.IsNotExist(err):
		// OK: file does not exist, continue

	default:
		// FATAL: something else is wrong
		t.Fatalf("Failed when checking image %q: %+v\n",
			env.ImagePath,
			err)
	}

	env.RunSingularity(
		t,
		WithProfile(RootProfile),
		WithCommand("build"),
		WithArgs("--force", env.ImagePath, "testdata/Singularity"),
		ExpectExit(0),
	)
}

// PullImage will pull a test image.
func PullImage(t *testing.T, env TestEnv, imageURL string, arch string, path string) {
	pullMutex.Lock()
	defer pullMutex.Unlock()

	if arch == "" {
		arch = runtime.GOARCH
	}

	switch _, err := os.Stat(path); {
	case err == nil:
		// OK: file exists, return
		return

	case os.IsNotExist(err):
		// OK: file does not exist, continue

	default:
		// FATAL: something else is wrong
		t.Fatalf("Failed when checking image %q: %+v\n", path, err)
	}

	env.RunSingularity(
		t,
		WithProfile(UserProfile),
		WithCommand("pull"),
		WithArgs("--force", "--allow-unsigned", "--arch", arch, path, imageURL),
		ExpectExit(0),
	)
}

// BusyboxImage will provide the path to a local busybox SIF image for the current architecture
func BusyboxSIF(t *testing.T) string {
	busyboxSIF := "testdata/busybox_" + runtime.GOARCH + ".sif"
	_, err := os.Stat(busyboxSIF)
	if os.IsNotExist(err) {
		t.Fatalf("busybox image not found for %s", runtime.GOARCH)
	}
	if err != nil {
		t.Error(err)
	}
	return busyboxSIF
}

// CopyImage will copy an OCI image from source to destination
func CopyOCIImage(t *testing.T, source, dest string, insecureSource, insecureDest bool) {
	// Mutex required due to https://github.com/google/go-containerregistry/issues/1849
	ociCopyMutex.Lock()
	defer ociCopyMutex.Unlock()
	// Use the auth config written out in dockerhub_auth.go - only if
	// source/dest are not insecure, or are the localhost. We don't want to
	// inadvertently send out credentials over http (!)
	u := CurrentUser(t)
	configPath := filepath.Join(u.Dir, ".singularity", syfs.DockerConfFile)

	srcType, srcRef, err := ociimage.URItoSourceSinkRef(source)
	if err != nil {
		t.Fatalf("failed to parse %s reference: %s", source, err)
	}

	platform, err := ociplatform.DefaultPlatform()
	if err != nil {
		t.Fatalf("failed to obtain platform: %s", err)
	}

	srcOpts := ociimage.TransportOptions{
		Insecure: insecureSource,
		Platform: *platform,
	}
	if !insecureSource || isLocalHost(source) {
		srcOpts.AuthFilePath = configPath
	}

	srcImage, err := srcType.Image(context.Background(), srcRef, &srcOpts, nil)
	if err != nil {
		t.Fatalf("failed to initialize source: %v", err)
	}

	// Must copy through a temp layout due to https://github.com/google/go-containerregistry/issues/1849
	tmpDir, cleanup := MakeTempDir(t, "", "copy-oci-image-", "")
	defer cleanup(t)
	if err := ociimage.OCISourceSink.WriteImage(srcImage, tmpDir, nil); err != nil {
		t.Fatalf("failed to write temporary layout: %s", err)
	}
	tmpImg, err := ociimage.OCISourceSink.Image(context.Background(), tmpDir, nil, nil)
	if err != nil {
		t.Fatalf("failed to initialize temporary layout source: %v", err)
	}

	dstType, dstRef, err := ociimage.URItoSourceSinkRef(dest)
	if err != nil {
		t.Fatalf("failed to parse %s reference: %s", dest, err)
	}
	dstOpts := ociimage.TransportOptions{
		Insecure: insecureSource,
	}
	if !insecureDest || isLocalHost(dest) {
		dstOpts.AuthFilePath = configPath
	}

	if err := dstType.WriteImage(tmpImg, dstRef, &dstOpts); err != nil {
		t.Fatalf("failed to copy %s to %s: %s", source, dest, err)
	}
}

// isLocalHost checks if the host component of a given URI points to the
// localhost. Note that this function returns a boolean: a malformed URI is
// considered a URI whose host does not point to localhost.
func isLocalHost(uri string) bool {
	u, err := url.Parse(uri)
	if err != nil {
		return false
	}

	switch u.Hostname() {
	case "localhost", "127.0.0.1":
		return true
	}

	return false
}

var orasImageOnce sync.Once

func EnsureORASImage(t *testing.T, env TestEnv) {
	EnsureImage(t, env)

	ensureMutex.Lock()
	defer ensureMutex.Unlock()

	orasImageOnce.Do(func() {
		t.Logf("Pushing %s to %s", env.ImagePath, env.OrasTestImage)
		env.RunSingularity(
			t,
			WithProfile(UserProfile),
			WithCommand("push"),
			WithArgs(env.ImagePath, env.OrasTestImage),
			ExpectExit(0),
		)
		if t.Failed() {
			t.Fatalf("failed to push ORAS image to local registry")
		}
	})
}

var orasOCISIFOnce sync.Once

func EnsureORASOCISIF(t *testing.T, env TestEnv) {
	EnsureOCISIF(t, env)

	ensureMutex.Lock()
	defer ensureMutex.Unlock()

	orasOCISIFOnce.Do(func() {
		t.Logf("Pushing %s to %s", env.OCISIFPath, env.OrasTestOCISIF)
		env.RunSingularity(
			t,
			WithProfile(UserProfile),
			WithCommand("push"),
			WithArgs(env.OCISIFPath, env.OrasTestOCISIF),
			ExpectExit(0),
		)
		if t.Failed() {
			t.Fatalf("failed to push ORAS oci-sif image to local registry")
		}
	})
}

var registryOCISIFOnce sync.Once

func EnsureRegistryOCISIF(t *testing.T, env TestEnv) {
	EnsureOCISIF(t, env)

	ensureMutex.Lock()
	defer ensureMutex.Unlock()

	registryOCISIFOnce.Do(func() {
		t.Logf("Pushing %s to %s", env.OCISIFPath, env.TestRegistryOCISIF)
		env.RunSingularity(
			t,
			WithProfile(UserProfile),
			WithCommand("push"),
			WithArgs(env.OCISIFPath, env.TestRegistryOCISIF),
			ExpectExit(0),
		)
		if t.Failed() {
			t.Fatalf("failed to push oci-sif image to local registry %q", env.TestRegistryOCISIF)
		}
	})
}

func DownloadFile(url string, path string) error {
	dl, err := os.Create(path)
	if err != nil {
		return err
	}
	defer dl.Close()

	r, err := http.Get(url)
	if err != nil {
		return err
	}
	defer r.Body.Close()

	_, err = io.Copy(dl, r.Body)
	if err != nil {
		return err
	}
	return nil
}

func EnsureOCILayout(t *testing.T, env TestEnv) {
	ensureMutex.Lock()
	defer ensureMutex.Unlock()

	switch _, err := os.Stat(env.OCILayoutPath); {
	case err == nil:
		// OK: dir exists, return
		return

	case os.IsNotExist(err):
		// OK: dir does not exist, continue

	default:
		// FATAL: something else is wrong
		t.Fatalf("Failed when checking image %q: %+v\n",
			env.OCILayoutPath,
			err)
	}

	// Prepare oci-archive source
	t.Logf("Copying %s to %s", env.TestRegistryImage, "oci:"+env.OCILayoutPath)
	CopyOCIImage(t, env.TestRegistryImage, "oci:"+env.OCILayoutPath, true, false)
}

// EnsureImage checks if e2e OCI test archive is available, and fetches
// it otherwise.
func EnsureOCIArchive(t *testing.T, env TestEnv) {
	EnsureOCILayout(t, env)
	ensureMutex.Lock()
	defer ensureMutex.Unlock()

	switch _, err := os.Stat(env.OCIArchivePath); {
	case err == nil:
		// OK: file exists, return
		return

	case os.IsNotExist(err):
		// OK: file does not exist, continue

	default:
		// FATAL: something else is wrong
		t.Fatalf("Failed when checking image %q: %+v\n",
			env.OCIArchivePath,
			err)
	}

	t.Logf("Tarring %s to %s", env.OCILayoutPath, "oci-archive:"+env.OCIArchivePath)
	cmd := exec.Command("tar", "-cf", env.OCIArchivePath, "-C", env.OCILayoutPath, "index.json", "oci-layout", "blobs")
	err := cmd.Run(t)
	if err.ExitCode != 0 {
		t.Fatalf("Error tarring oci layout to archive: %v", err)
	}
}

// EnsureImage checks if e2e OCI-SIF file is available, and fetches it
// otherwise.
func EnsureOCISIF(t *testing.T, env TestEnv) {
	ensureMutex.Lock()
	defer ensureMutex.Unlock()

	switch _, err := os.Stat(env.OCISIFPath); {
	case err == nil:
		// OK: file exists, return
		return

	case os.IsNotExist(err):
		// OK: file does not exist, continue

	default:
		// FATAL: something else is wrong
		t.Fatalf("Failed when checking image %q: %+v\n",
			env.OCISIFPath,
			err)
	}

	env.RunSingularity(
		t,
		WithProfile(OCIUserProfile),
		WithCommand("pull"),
		WithArgs("--no-https", env.OCISIFPath, env.TestRegistryImage),
		ExpectExit(0),
	)
}

// EnsureDockerArchive checks if e2e Docker test archive is available, and fetches
// it otherwise.
func EnsureDockerArchive(t *testing.T, env TestEnv) {
	ensureMutex.Lock()
	defer ensureMutex.Unlock()

	switch _, err := os.Stat(env.DockerArchivePath); {
	case err == nil:
		// OK: file exists, return
		return

	case os.IsNotExist(err):
		// OK: file does not exist, continue

	default:
		// FATAL: something else is wrong
		t.Fatalf("Failed when checking image %q: %+v\n",
			env.DockerArchivePath,
			err)
	}

	// Prepare oci-archive source
	t.Logf("Copying %s to %s", env.TestRegistryImage, "docker-archive:"+env.DockerArchivePath)
	CopyOCIImage(t, env.TestRegistryImage, "docker-archive:"+env.DockerArchivePath, true, false)
}
