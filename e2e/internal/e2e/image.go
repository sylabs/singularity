// Copyright (c) 2019-2022, Sylabs Inc. All rights reserved.
// Copyright (c) Contributors to the Apptainer project, established as
//   Apptainer a Series of LF Projects LLC.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package e2e

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/containers/image/v5/copy"
	"github.com/containers/image/v5/docker"
	dockerarchive "github.com/containers/image/v5/docker/archive"
	ociarchive "github.com/containers/image/v5/oci/archive"
	ocilayout "github.com/containers/image/v5/oci/layout"
	"github.com/containers/image/v5/signature"
	"github.com/containers/image/v5/types"
	"github.com/sylabs/singularity/pkg/syfs"
	useragent "github.com/sylabs/singularity/pkg/util/user-agent"
)

var (
	ensureMutex sync.Mutex
	pullMutex   sync.Mutex
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
	policy := &signature.Policy{Default: []signature.PolicyRequirement{signature.NewPRInsecureAcceptAnything()}}
	policyCtx, err := signature.NewPolicyContext(policy)
	if err != nil {
		t.Fatalf("failed to copy %s to %s: %s", source, dest, err)
	}

	srcCtx := &types.SystemContext{
		OCIInsecureSkipTLSVerify:    insecureSource,
		DockerInsecureSkipTLSVerify: types.NewOptionalBool(insecureSource),
		DockerRegistryUserAgent:     useragent.Value(),
	}
	dstCtx := &types.SystemContext{
		OCIInsecureSkipTLSVerify:    insecureDest,
		DockerInsecureSkipTLSVerify: types.NewOptionalBool(insecureDest),
		DockerRegistryUserAgent:     useragent.Value(),
	}

	// Use the auth config written out in dockerhub_auth.go - only if source/dest are not insecure.
	// We don't want to inadvertently send out credentials over http (!)
	u := CurrentUser(t)
	configPath := filepath.Join(u.Dir, ".singularity", syfs.DockerConfFile)
	if !insecureSource {
		srcCtx.AuthFilePath = configPath
	}
	if !insecureDest {
		dstCtx.AuthFilePath = configPath
	}

	srcRef, err := parseRef(source)
	if err != nil {
		t.Fatalf("failed to parse %s reference: %s", source, err)
	}
	dstRef, err := parseRef(dest)
	if err != nil {
		t.Fatalf("failed to parse %s reference: %s", dest, err)
	}

	_, err = copy.Image(context.Background(), policyCtx, dstRef, srcRef, &copy.Options{
		ReportWriter:   io.Discard,
		SourceCtx:      srcCtx,
		DestinationCtx: dstCtx,
	})
	if err != nil {
		t.Fatalf("failed to copy %s to %s: %s", source, dest, err)
	}
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

// EnsureImage checks if e2e OCI test archive is available, and fetches
// it otherwise.
func EnsureOCIArchive(t *testing.T, env TestEnv) {
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

	// Prepare oci-archive source
	t.Logf("Copying %s to %s", env.TestRegistryImage, "oci-archive:"+env.OCIArchivePath)
	CopyOCIImage(t, env.TestRegistryImage, "oci-archive:"+env.OCIArchivePath, true, false)
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

func parseRef(refString string) (ref types.ImageReference, err error) {
	parts := strings.SplitN(refString, ":", 2)
	if len(parts) < 2 {
		return nil, fmt.Errorf("could not parse image ref: %s", refString)
	}

	switch parts[0] {
	case "docker":
		ref, err = docker.ParseReference(parts[1])
	case "docker-archive":
		ref, err = dockerarchive.ParseReference(parts[1])
	case "oci":
		ref, err = ocilayout.ParseReference(parts[1])
	case "oci-archive":
		ref, err = ociarchive.ParseReference(parts[1])
	default:
		return nil, fmt.Errorf("cannot create an OCI container from %s source", parts[0])
	}

	return ref, err
}
