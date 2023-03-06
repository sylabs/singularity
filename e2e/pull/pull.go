// Copyright (c) 2019-2022, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

// The E2E PULL group tests image pulls of SIF format images (library, oras
// sources). Docker / OCI image pull is tested as part of the DOCKER E2E group.

package pull

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/containerd/containerd/reference"
	"github.com/containerd/containerd/remotes/docker"
	"github.com/sylabs/singularity/e2e/internal/e2e"
	"github.com/sylabs/singularity/e2e/internal/testhelper"
	syoras "github.com/sylabs/singularity/internal/pkg/client/oras"
	"github.com/sylabs/singularity/internal/pkg/util/uri"
	"golang.org/x/sys/unix"
	"oras.land/oras-go/pkg/content"
	"oras.land/oras-go/pkg/oras"
)

type ctx struct {
	env e2e.TestEnv
}

type testStruct struct {
	desc             string // case description
	srcURI           string // source URI for image
	library          string // use specific library, XXX(mem): not tested yet
	arch             string // architecture to force, if any
	force            bool   // pass --force
	createDst        bool   // create destination file before pull
	unauthenticated  bool   // pass --allow-unauthenticated
	setImagePath     bool   // pass destination path
	setPullDir       bool   // pass --dir
	expectedExitCode int
	workDir          string
	pullDir          string
	imagePath        string
	expectedImage    string
	envVars          []string
}

func (c *ctx) imagePull(t *testing.T, tt testStruct) {
	// Use a one-time cache directory specific to this pull. This ensures we are always
	// testing an entire pull operation, performing the download into an empty cache.
	cacheDir, cleanup := e2e.MakeCacheDir(t, "")
	t.Cleanup(func() {
		if !t.Failed() {
			cleanup(t)
		}
	})
	c.env.UnprivCacheDir = cacheDir

	// We use a string rather than a slice of strings to avoid having an empty
	// element in the slice, which would cause the command to fail, without
	// over-complicating the code.
	argv := ""

	if tt.arch != "" {
		argv += "--arch " + tt.arch + " "
	}

	if tt.force {
		argv += "--force "
	}

	if tt.unauthenticated {
		argv += "--allow-unauthenticated "
	}

	if tt.pullDir != "" {
		argv += "--dir " + tt.pullDir + " "
	}

	if tt.library != "" {
		argv += "--library " + tt.library + " "
	}

	if tt.imagePath != "" {
		argv += tt.imagePath + " "
	}

	if tt.workDir == "" {
		wd, err := os.Getwd()
		if err != nil {
			t.Fatalf("unable to get working directory: %s", err)
		}
		tt.workDir = wd
	}

	argv += tt.srcURI

	c.env.RunSingularity(
		t,
		e2e.AsSubtest(tt.desc),
		e2e.WithProfile(e2e.UserProfile),
		e2e.WithEnv(tt.envVars),
		e2e.WithDir(tt.workDir),
		e2e.WithCommand("pull"),
		e2e.WithArgs(strings.Split(argv, " ")...),
		e2e.ExpectExit(tt.expectedExitCode))

	checkPullResult(t, tt)
}

func getImageNameFromURI(imgURI string) string {
	// XXX(mem): this function should be part of the code, not the test
	switch transport, ref := uri.Split(imgURI); {
	case ref == "":
		return "" // Invalid URI

	case transport == "":
		imgURI = "library://" + imgURI
	}

	return uri.GetName(imgURI)
}

func (c *ctx) setup(t *testing.T) {
	e2e.EnsureImage(t, c.env)

	// setup file and dir to use as invalid images
	orasInvalidDir, err := os.MkdirTemp(c.env.TestDir, "oras_push_dir-")
	if err != nil {
		t.Fatalf("unable to create src dir for push tests: %v", err)
	}

	orasInvalidFile, err := e2e.WriteTempFile(orasInvalidDir, "oras_invalid_image-", "Invalid Image Contents")
	if err != nil {
		t.Fatalf("unable to create src file for push tests: %v", err)
	}

	// prep local registry with oras generated artifacts
	// Note: the image name prevents collisions by using a package specific name
	// as the registry is shared between different test packages
	orasImages := []struct {
		srcPath        string
		uri            string
		layerMediaType string
	}{
		{
			srcPath:        c.env.ImagePath,
			uri:            fmt.Sprintf("%s/pull_test_sif:latest", c.env.TestRegistry),
			layerMediaType: syoras.SifLayerMediaTypeV1,
		},
		{
			srcPath:        c.env.ImagePath,
			uri:            fmt.Sprintf("%s/pull_test_sif_mediatypeproto:latest", c.env.TestRegistry),
			layerMediaType: syoras.SifLayerMediaTypeProto,
		},
		{
			srcPath:        orasInvalidDir,
			uri:            fmt.Sprintf("%s/pull_test_dir:latest", c.env.TestRegistry),
			layerMediaType: syoras.SifLayerMediaTypeV1,
		},
		{
			srcPath:        orasInvalidFile,
			uri:            fmt.Sprintf("%s/pull_test_invalid_file:latest", c.env.TestRegistry),
			layerMediaType: syoras.SifLayerMediaTypeV1,
		},
	}

	for _, i := range orasImages {
		err = orasPushNoCheck(i.srcPath, i.uri, i.layerMediaType)
		if err != nil {
			t.Fatalf("while prepping registry for oras tests: %v", err)
		}
	}
}

func (c ctx) testPullCmd(t *testing.T) {
	tests := []testStruct{
		{
			desc:             "non existent image",
			srcURI:           "library://sylabs/tests/does_not_exist:0",
			expectedExitCode: 255,
		},

		// --allow-unauthenticated tests
		{
			desc:             "unsigned image allow unauthenticated",
			srcURI:           "library://sylabs/tests/unsigned:1.0.0",
			unauthenticated:  true,
			expectedExitCode: 0,
		},

		// --force tests
		{
			desc:             "force existing file",
			srcURI:           "library://alpine:3.11.5",
			force:            true,
			createDst:        true,
			unauthenticated:  true,
			expectedExitCode: 0,
		},
		{
			desc:             "force non-existing file",
			srcURI:           "library://alpine:3.11.5",
			force:            true,
			createDst:        false,
			unauthenticated:  true,
			expectedExitCode: 0,
		},
		{
			// --force should not have an effect on --allow-unauthenticated=false
			desc:             "unsigned image force require authenticated",
			srcURI:           "library://sylabs/tests/unsigned:1.0.0",
			force:            true,
			unauthenticated:  false,
			expectedExitCode: 0,
		},

		// test version specifications
		{
			desc:             "image with specific hash",
			srcURI:           "library://alpine:sha256.03883ca565b32e58fa0a496316d69de35741f2ef34b5b4658a6fec04ed8149a8",
			arch:             "amd64",
			unauthenticated:  true,
			expectedExitCode: 0,
		},
		{
			desc:             "latest tag",
			srcURI:           "library://alpine:latest",
			unauthenticated:  true,
			expectedExitCode: 0,
		},

		// --dir tests
		{
			desc:             "dir no image path",
			srcURI:           "library://alpine:3.11.5",
			unauthenticated:  true,
			setPullDir:       true,
			setImagePath:     false,
			expectedExitCode: 0,
		},
		{
			// XXX(mem): this specific test is passing both --path and an image path to
			// singularity pull. The current behavior is that the code is joining both paths and
			// failing to find the image in the expected location indicated by image path
			// because image path is absolute, so after joining /tmp/a/b/c and
			// /tmp/a/b/image.sif, the code expects to find /tmp/a/b/c/tmp/a/b/image.sif. Since
			// the directory /tmp/a/b/c/tmp/a/b does not exist, it fails to create the file
			// image.sif in there.
			desc:             "dir image path",
			srcURI:           "library://alpine:3.11.5",
			unauthenticated:  true,
			setPullDir:       true,
			setImagePath:     true,
			expectedExitCode: 255,
		},

		// transport tests
		{
			desc:             "bare image name",
			srcURI:           "alpine:3.11.5",
			force:            true,
			unauthenticated:  true,
			expectedExitCode: 0,
		},
		// TODO(mem): reenable this; disabled while shub is down
		// {
		// 	desc:            "image from shub",
		// 	srcURI:          "shub://GodloveD/busybox",
		// 	force:           true,
		// 	unauthenticated: false,
		// 	expectSuccess:   true,
		// },
		// Finalized v1 layer mediaType (3.7 and onward)
		{
			desc:             "oras transport for SIF from registry",
			srcURI:           "oras://" + c.env.TestRegistry + "/pull_test_sif:latest", // TODO(mem): obtain registry from context
			force:            true,
			unauthenticated:  false,
			expectedExitCode: 0,
		},
		// Original/prototype layer mediaType (<3.7)
		{
			desc:             "oras transport for SIF from registry (SifLayerMediaTypeProto)",
			srcURI:           "oras://" + c.env.TestRegistry + "/pull_test_sif_mediatypeproto:latest", // TODO(mem): obtain registry from context
			force:            true,
			unauthenticated:  false,
			expectedExitCode: 0,
		},

		// pulling of invalid images with oras
		{
			desc:             "oras pull of non SIF file",
			srcURI:           "oras://" + c.env.TestRegistry + "/pull_test_:latest", // TODO(mem): obtain registry from context
			force:            true,
			expectedExitCode: 255,
		},
		{
			desc:             "oras pull of packed dir",
			srcURI:           "oras://" + c.env.TestRegistry + "/pull_test_invalid_file:latest", // TODO(mem): obtain registry from context
			force:            true,
			expectedExitCode: 255,
		},

		// pulling with library URI argument
		{
			desc:             "bad library URI",
			srcURI:           "library://busybox:1.31.1",
			library:          "https://bad-library.sylabs.io",
			expectedExitCode: 255,
		},
		{
			desc:             "default library URI",
			srcURI:           "library://busybox:1.31.1",
			library:          "https://library.sylabs.io",
			force:            true,
			expectedExitCode: 0,
		},

		// pulling with library URI containing host name and library argument
		{
			desc:             "library URI containing host name and library argument",
			srcURI:           "library://notlibrary.sylabs.io/library/default/busybox:1.31.1",
			library:          "https://notlibrary.sylabs.io",
			expectedExitCode: 255,
		},

		// pulling with library URI containing host name
		{
			desc:             "library URI containing bad host name",
			srcURI:           "library://notlibrary.sylabs.io/library/default/busybox:1.31.1",
			expectedExitCode: 255,
		},
		{
			desc:             "library URI containing host name",
			srcURI:           "library://library.sylabs.io/library/default/busybox:1.31.1",
			force:            true,
			expectedExitCode: 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			tmpdir, err := os.MkdirTemp(c.env.TestDir, "pull_test.")
			if err != nil {
				t.Fatalf("Failed to create temporary directory for pull test: %+v", err)
			}
			t.Cleanup(func() {
				if !t.Failed() {
					os.RemoveAll(tmpdir)
				}
			})

			if tt.setPullDir {
				tt.pullDir, err = os.MkdirTemp(tmpdir, "pull_dir.")
				if err != nil {
					t.Fatalf("Failed to create temporary directory for pull dir: %+v", err)
				}
			}

			if tt.setImagePath {
				tt.imagePath = filepath.Join(tmpdir, "image.sif")
				tt.expectedImage = tt.imagePath
			} else {
				// No explicit image path specified. Will use temp dir as working directory,
				// so we pull into a clean location.
				tt.workDir = tmpdir
				imageName := getImageNameFromURI(tt.srcURI)
				tt.expectedImage = filepath.Join(tmpdir, imageName)

				// if there's a pullDir, that's where we expect to find the image
				if tt.pullDir != "" {
					tt.expectedImage = filepath.Join(tt.pullDir, imageName)
				}

			}

			// In order to actually test force, there must already be a file present in
			// the expected location
			if tt.createDst {
				fh, err := os.Create(tt.expectedImage)
				if err != nil {
					t.Fatalf("failed to create file %q: %+v\n", tt.expectedImage, err)
				}
				fh.Close()
			}

			c.imagePull(t, tt)
		})
	}
}

func checkPullResult(t *testing.T, tt testStruct) {
	if tt.expectedExitCode == 0 {
		_, err := os.Stat(tt.expectedImage)
		switch err {
		case nil:
			// PASS
			return

		case os.ErrNotExist:
			// FAIL
			t.Errorf("expecting image at %q, not found: %+v\n", tt.expectedImage, err)

		default:
			// FAIL
			t.Errorf("unable to stat image at %q: %+v\n", tt.expectedImage, err)
		}

		// XXX(mem): This is running a bunch of commands in the downloaded
		// images. Do we really want this here? If yes, we need to have a
		// way to do this in a generic fashion, as it's going to be shared
		// with build as well.

		// imageVerify(t, tt.imagePath, false)
	}
}

// this is a version of the oras push functionality that does not check that given the
// file is a valid SIF, this allows us to push arbitrary objects to the local registry
// to test the pull validation
// We can also set the layer mediaType - so we can push images with older media types
// to verify that they can still be pulled.
func orasPushNoCheck(path, ref, layerMediaType string) error {
	ref = strings.TrimPrefix(ref, "//")

	spec, err := reference.Parse(ref)
	if err != nil {
		return fmt.Errorf("unable to parse oci reference: %w", err)
	}

	// Hostname() will panic if there is no '/' in the locator
	// explicitly check for this and fail in order to prevent panic
	// this case will only occur for incorrect uris
	if !strings.Contains(spec.Locator, "/") {
		return fmt.Errorf("not a valid oci object uri: %s", ref)
	}

	// append default tag if no object exists
	if spec.Object == "" {
		spec.Object = "latest"
	}

	resolver := docker.NewResolver(docker.ResolverOptions{})

	store := content.NewFile("")
	defer store.Close()

	// Get the filename from path and use it as the name in the file store
	name := filepath.Base(path)

	desc, err := store.Add(name, layerMediaType, path)
	if err != nil {
		return fmt.Errorf("unable to add SIF to store: %w", err)
	}

	manifest, manifestDesc, config, configDesc, err := content.GenerateManifestAndConfig(nil, nil, desc)
	if err != nil {
		return fmt.Errorf("unable to generate manifest and config: %w", err)
	}

	if err := store.Load(configDesc, config); err != nil {
		return fmt.Errorf("unable to load config: %w", err)
	}

	if err := store.StoreManifest(spec.String(), manifestDesc, manifest); err != nil {
		return fmt.Errorf("unable to store manifest: %w", err)
	}

	if _, err := oras.Copy(context.Background(), store, spec.String(), resolver, ""); err != nil {
		return fmt.Errorf("unable to push: %w", err)
	}

	return nil
}

func (c ctx) testPullDisableCacheCmd(t *testing.T) {
	cacheDir, err := os.MkdirTemp("", "e2e-imgcache-")
	if err != nil {
		t.Fatalf("failed to create temporary directory: %s", err)
	}
	t.Cleanup(func() {
		if !t.Failed() {
			err := os.RemoveAll(cacheDir)
			if err != nil {
				t.Fatalf("failed to delete temporary directory %s: %s", cacheDir, err)
			}
		}
	})

	c.env.UnprivCacheDir = cacheDir

	disableCacheTests := []struct {
		name      string
		imagePath string
		imageSrc  string
	}{
		{
			name:      "library",
			imagePath: filepath.Join(c.env.TestDir, "library.sif"),
			imageSrc:  "library://alpine:latest",
		},
		{
			name:      "oras",
			imagePath: filepath.Join(c.env.TestDir, "oras.sif"),
			imageSrc:  "oras://" + c.env.TestRegistry + "/pull_test_sif:latest",
		},
	}

	for _, tt := range disableCacheTests {
		cmdArgs := []string{"--disable-cache", tt.imagePath, tt.imageSrc}
		c.env.RunSingularity(
			t,
			e2e.AsSubtest(tt.name),
			e2e.WithProfile(e2e.UserProfile),
			e2e.WithCommand("pull"),
			e2e.WithArgs(cmdArgs...),
			e2e.ExpectExit(0),
			e2e.PostRun(func(t *testing.T) {
				// Cache entry must not have been created
				cacheEntryPath := filepath.Join(cacheDir, "cache")
				if _, err := os.Stat(cacheEntryPath); !os.IsNotExist(err) {
					t.Errorf("cache created while disabled (%s exists)", cacheEntryPath)
				}
				// We also need to check the image pulled is in the correct place!
				// Issue #5628s
				_, err := os.Stat(tt.imagePath)
				if os.IsNotExist(err) {
					t.Errorf("image does not exist at %s", tt.imagePath)
				}
			}),
		)
	}
}

// testPullUmask will run some pull tests with different umasks, and
// ensure the output file has the correct permissions.
func (c ctx) testPullUmask(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, c.env.ImagePath)
	}))
	defer srv.Close()

	umask22Image := "0022-umask-pull"
	umask77Image := "0077-umask-pull"
	umask27Image := "0027-umask-pull"

	umaskTests := []struct {
		name       string
		imagePath  string
		umask      int
		expectPerm uint32
		force      bool
	}{
		{
			name:       "0022 umask pull",
			imagePath:  filepath.Join(c.env.TestDir, umask22Image),
			umask:      0o022,
			expectPerm: 0o755,
		},
		{
			name:       "0077 umask pull",
			imagePath:  filepath.Join(c.env.TestDir, umask77Image),
			umask:      0o077,
			expectPerm: 0o700,
		},
		{
			name:       "0027 umask pull",
			imagePath:  filepath.Join(c.env.TestDir, umask27Image),
			umask:      0o027,
			expectPerm: 0o750,
		},

		// With the force flag, and override the image. The permission will
		// reset to 0666 after every test.
		{
			name:       "0022 umask pull override",
			imagePath:  filepath.Join(c.env.TestDir, umask22Image),
			umask:      0o022,
			expectPerm: 0o755,
			force:      true,
		},
		{
			name:       "0077 umask pull override",
			imagePath:  filepath.Join(c.env.TestDir, umask77Image),
			umask:      0o077,
			expectPerm: 0o700,
			force:      true,
		},
		{
			name:       "0027 umask pull override",
			imagePath:  filepath.Join(c.env.TestDir, umask27Image),
			umask:      0o027,
			expectPerm: 0o750,
			force:      true,
		},
	}

	// Helper function to get the file mode for a file.
	getFilePerm := func(t *testing.T, path string) uint32 {
		finfo, err := os.Stat(path)
		if err != nil {
			t.Fatalf("failed while getting file permission: %s", err)
		}
		return uint32(finfo.Mode().Perm())
	}

	// Set a common umask, then reset it back later.
	oldUmask := unix.Umask(0o022)
	defer unix.Umask(oldUmask)

	// TODO: should also check the cache umask.
	for _, tc := range umaskTests {
		var cmdArgs []string
		if tc.force {
			cmdArgs = append(cmdArgs, "--force")
		}
		cmdArgs = append(cmdArgs, tc.imagePath, srv.URL)

		c.env.RunSingularity(
			t,
			e2e.WithProfile(e2e.UserProfile),
			e2e.PreRun(func(t *testing.T) {
				// Reset the file permission after every pull.
				err := os.Chmod(tc.imagePath, 0o666)
				if !os.IsNotExist(err) && err != nil {
					t.Fatalf("failed chmod-ing file: %s", err)
				}

				// Set the test umask.
				unix.Umask(tc.umask)
			}),
			e2e.PostRun(func(t *testing.T) {
				// Check the file permission.
				permOut := getFilePerm(t, tc.imagePath)
				if tc.expectPerm != permOut {
					t.Fatalf("Unexpected failure: expecting file perm: %o, got: %o", tc.expectPerm, permOut)
				}
			}),
			e2e.WithCommand("pull"),
			e2e.WithArgs(cmdArgs...),
			e2e.ExpectExit(0),
		)
	}
}

// E2ETests is the main func to trigger the test suite
func E2ETests(env e2e.TestEnv) testhelper.Tests {
	c := ctx{
		env: env,
	}

	np := testhelper.NoParallel

	return testhelper.Tests{
		// Run pull tests sequentially among themselves, as they perform a lot
		// of un-cached pulls which could otherwise lead to hitting rate limits.
		"ordered": func(t *testing.T) {
			// Setup a test registry to pull from (for oras).
			c.setup(t)
			t.Run("pull", c.testPullCmd)
			t.Run("pullDisableCache", c.testPullDisableCacheCmd)
			t.Run("concurrencyConfig", c.testConcurrencyConfig)
			t.Run("concurrentPulls", c.testConcurrentPulls)
		},
		"issue1087": c.issue1087,
		// Manipulates umask for the process, so must be run alone to avoid
		// causing permission issues for other tests.
		"pullUmaskCheck": np(c.testPullUmask),
		// Regressions
		// Manipulates remotes, so must run alone
		"issue5808": np(c.issue5808),
	}
}
