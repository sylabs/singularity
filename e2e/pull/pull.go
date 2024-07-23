// Copyright (c) 2019-2023, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

// The E2E PULL group tests image pulls of SIF format images (library, oras
// sources). Docker / OCI image pull is tested as part of the DOCKER E2E group.

package pull

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/types"
	ocitsif "github.com/sylabs/oci-tools/pkg/sif"
	"github.com/sylabs/sif/v2/pkg/sif"
	"github.com/sylabs/singularity/v4/e2e/internal/e2e"
	"github.com/sylabs/singularity/v4/e2e/internal/testhelper"
	"github.com/sylabs/singularity/v4/internal/pkg/client/oras"
	syoras "github.com/sylabs/singularity/v4/internal/pkg/client/oras"
	"github.com/sylabs/singularity/v4/internal/pkg/ocisif"
	"github.com/sylabs/singularity/v4/internal/pkg/test/tool/require"
	"github.com/sylabs/singularity/v4/internal/pkg/util/fs"
	"github.com/sylabs/singularity/v4/internal/pkg/util/uri"
	"github.com/sylabs/singularity/v4/pkg/image"
	"golang.org/x/sys/unix"
)

type ctx struct {
	env e2e.TestEnv
}

type testStruct struct {
	desc              string // case description
	srcURI            string // source URI for image
	library           string // use specific library server URI
	arch              string // architecture to force, if any
	platform          string // platform to force, if any
	force             bool   // pass --force
	createDst         bool   // create destination file before pull
	unauthenticated   bool   // pass --allow-unauthenticated
	setImagePath      bool   // pass destination path (singularity pull <image path> <source>)
	setPullDir        bool   // pass --dir
	oci               bool   // pass --oci
	keepLayers        bool   // pass --keep-layers
	noHTTPS           bool   // pass --no-https
	expectedExitCode  int
	workDir           string
	pullDir           string
	imagePath         string
	expectedImage     string
	expectedOCI       bool
	expectedOCILayers int // number of squashfs layers if OCI-SIF
	envVars           []string
	requirements      func(t *testing.T)
}

func (c *ctx) imagePull(t *testing.T, tt testStruct) {
	if tt.requirements != nil {
		tt.requirements(t)
	}

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

	if tt.platform != "" {
		argv += "--platform " + tt.platform + " "
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

	if tt.keepLayers {
		argv += "--keep-layers "
	}

	if tt.noHTTPS {
		argv += "--no-https "
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

	profile := e2e.UserProfile
	if tt.oci {
		profile = e2e.OCIUserProfile
	}

	c.env.RunSingularity(
		t,
		e2e.WithProfile(profile),
		e2e.WithEnv(tt.envVars),
		e2e.WithDir(tt.workDir),
		e2e.WithCommand("pull"),
		e2e.WithArgs(strings.Split(argv, " ")...),
		e2e.ExpectExit(tt.expectedExitCode))

	checkPullResult(t, tt)
}

func getImageNameFromURI(imgURI string, oci bool) string {
	// XXX(mem): this function should be part of the code, not the test
	switch transport, ref := uri.Split(imgURI); {
	case ref == "":
		return "" // Invalid URI

	case transport == "":
		imgURI = "library://" + imgURI
	}

	suffix := "sif"
	if oci {
		suffix = "oci.sif"
	}

	return uri.Filename(imgURI, suffix)
}

func (c *ctx) setup(t *testing.T) {
	e2e.EnsureImage(t, c.env)
	e2e.EnsureOCISIF(t, c.env)
	e2e.EnsureORASOCISIF(t, c.env)
	e2e.EnsureRegistryOCISIF(t, c.env)

	orasInvalidFile, err := e2e.WriteTempFile(c.env.TestDir, "oras_invalid_image-", "Invalid Image Contents")
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
			srcPath:        orasInvalidFile,
			uri:            fmt.Sprintf("%s/pull_test_invalid_file:latest", c.env.TestRegistry),
			layerMediaType: syoras.SifLayerMediaTypeV1,
		},
		{
			srcPath:        c.env.OCISIFPath,
			uri:            fmt.Sprintf("%s/pull_test_oci-sif:latest", c.env.TestRegistry),
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

//nolint:maintidx
func (c ctx) testPullCmd(t *testing.T) {
	tests := []testStruct{
		//
		// library:// URIs
		// SCS / Singularity Enterprise & compatible.
		//
		{
			desc:             "library non-existent",
			srcURI:           "library://sylabs/tests/does_not_exist:0",
			expectedExitCode: 255,
		},
		// --allow-unauthenticated tests
		{
			desc:             "library allow-unauthenticated",
			srcURI:           "library://sylabs/tests/unsigned:1.0.0",
			unauthenticated:  true,
			expectedExitCode: 0,
		},
		// --force tests
		{
			desc:             "library force existing",
			srcURI:           "library://alpine:3.11.5",
			force:            true,
			createDst:        true,
			unauthenticated:  true,
			expectedExitCode: 0,
		},
		{
			desc:             "library force non-existing",
			srcURI:           "library://alpine:3.11.5",
			force:            true,
			createDst:        false,
			unauthenticated:  true,
			expectedExitCode: 0,
		},
		{
			// --force should not have an effect on --allow-unauthenticated=false
			desc:             "library force allow-unauthenticated",
			srcURI:           "library://sylabs/tests/unsigned:1.0.0",
			force:            true,
			unauthenticated:  false,
			expectedExitCode: 0,
		},
		// test version specifications
		{
			desc:             "library hash",
			srcURI:           "library://alpine:sha256.03883ca565b32e58fa0a496316d69de35741f2ef34b5b4658a6fec04ed8149a8",
			arch:             "amd64",
			unauthenticated:  true,
			expectedExitCode: 0,
		},
		{
			desc:             "library tag",
			srcURI:           "library://alpine:latest",
			unauthenticated:  true,
			expectedExitCode: 0,
		},
		// forced arch and platform equivalent
		{
			desc:             "library non-oci arch",
			srcURI:           "library://alpine:3.11.5",
			arch:             "ppc64le",
			expectedExitCode: 0,
		},
		{
			desc:             "library non-oci platform",
			srcURI:           "library://alpine:3.11.5",
			platform:         "linux/ppc64le",
			expectedExitCode: 0,
		},
		// --dir tests
		{
			desc:             "library dir",
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
			desc:             "library dir with image path",
			srcURI:           "library://alpine:3.11.5",
			unauthenticated:  true,
			setPullDir:       true,
			setImagePath:     true,
			expectedExitCode: 255,
		},
		// default transport should be library
		{
			desc:             "library default transport",
			srcURI:           "alpine:3.11.5",
			force:            true,
			unauthenticated:  true,
			expectedExitCode: 0,
		},
		// pulling with library URI argument
		{
			desc:             "library bad library flag",
			srcURI:           "library://busybox:1.31.1",
			library:          "https://bad-library.sylabs.io",
			expectedExitCode: 255,
		},
		{
			desc:             "library default library flag",
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
		// pulling an OCI-SIF image from library backing registry
		{
			desc:   "library oci-sif fallback",
			srcURI: "library://sylabs/tests/alpine-oci-sif:latest",
			// will try library protocol first, should then attempt oci pull
			oci:               false,
			expectedOCI:       true,
			expectedOCILayers: 1,
			expectedExitCode:  0,
			requirements: func(t *testing.T) {
				require.Arch(t, "amd64")
			},
		},
		{
			desc:   "library oci-sif direct",
			srcURI: "library://sylabs/tests/alpine-oci-sif:latest",
			// direct oci pull
			oci:               true,
			expectedOCI:       true,
			expectedOCILayers: 1,
			expectedExitCode:  0,
			requirements: func(t *testing.T) {
				require.Arch(t, "amd64")
			},
		},
		{
			desc:   "library oci-sif platform",
			srcURI: "library://sylabs/tests/alpine-ppc64le-oci-sif:latest",
			// direct oci pull
			platform:          "linux/ppc64le",
			oci:               true,
			expectedOCI:       true,
			expectedOCILayers: 1,
			expectedExitCode:  0,
		},
		{
			desc:   "library oci-sif arch",
			srcURI: "library://sylabs/tests/alpine-ppc64le-oci-sif:latest",
			// direct oci pull
			arch:              "ppc64le",
			oci:               true,
			expectedOCI:       true,
			expectedOCILayers: 1,
			expectedExitCode:  0,
		},
		//
		// shub:// URIs
		// Singularity Hub (retired) and compatible.
		//
		// TODO(mem): reenable this; disabled while shub is down
		// {
		// 	desc:            "image from shub",
		// 	srcURI:          "shub://GodloveD/busybox",
		// 	force:           true,
		// 	unauthenticated: false,
		// 	expectSuccess:   true,
		// },

		//
		// oras:// URIs
		// SIF file as ORAS / OCI artifact.
		//
		// Finalized v1 layer mediaType (3.7 and onward)
		{
			desc:             "oras transport for SIF from registry",
			srcURI:           "oras://" + c.env.TestRegistry + "/pull_test_sif:latest",
			force:            true,
			unauthenticated:  false,
			expectedExitCode: 0,
		},
		// Original/prototype layer mediaType (<3.7)
		{
			desc:             "oras transport for SIF from registry (SifLayerMediaTypeProto)",
			srcURI:           "oras://" + c.env.TestRegistry + "/pull_test_sif_mediatypeproto:latest",
			force:            true,
			unauthenticated:  false,
			expectedExitCode: 0,
		},
		// OCI-SIF
		{
			desc:              "oras pull of oci-sif",
			srcURI:            "oras://" + c.env.TestRegistry + "/pull_test_oci-sif:latest",
			force:             true,
			expectedOCI:       true,
			expectedOCILayers: 1,
			expectedExitCode:  0,
		},
		// Invalid (non-SIF) artifacts
		{
			desc:             "oras pull of non SIF file",
			srcURI:           "oras://" + c.env.TestRegistry + "/pull_test_:latest",
			force:            true,
			expectedExitCode: 255,
		},
		{
			desc:             "oras pull of packed dir",
			srcURI:           "oras://" + c.env.TestRegistry + "/pull_test_invalid_file:latest",
			force:            true,
			expectedExitCode: 255,
		},

		//
		// docker:// URIs
		// Standard OCI images, and OCI-SIF single layer squashfs images, in an OCI distribution-spec registry.
		//
		// pulling a standard OCI image from local registry to a native SIF
		{
			desc:              "docker oci to sif",
			srcURI:            c.env.TestRegistryImage,
			oci:               false,
			expectedOCI:       false,
			expectedOCILayers: 1,
			noHTTPS:           true,
			force:             true,
			expectedExitCode:  0,
		},
		// pulling a standard OCI image from local registry to an OCI-SIF
		{
			desc:              "docker oci to oci-sif",
			srcURI:            c.env.TestRegistryImage,
			oci:               true,
			expectedOCI:       true,
			expectedOCILayers: 1,
			noHTTPS:           true,
			force:             true,
			expectedExitCode:  0,
		},
		// pulling an OCI-SIF image from local registry to an OCI-SIF
		{
			desc:              "docker oci-sif to oci-sif",
			srcURI:            c.env.TestRegistryOCISIF,
			oci:               true,
			expectedOCI:       true,
			expectedOCILayers: 1,
			noHTTPS:           true,
			force:             true,
			expectedExitCode:  0,
		},
		// pulling an OCI-SIF image from local registry to a native SIF (not implemented)
		{
			desc:             "docker oci-sif to sif",
			srcURI:           c.env.TestRegistryOCISIF,
			oci:              false,
			expectedOCI:      false,
			noHTTPS:          true,
			force:            true,
			expectedExitCode: 255,
		},
		// pulling an OCI-SIF image from local registry to a multi-layer OCI-SIF
		{
			desc:              "docker oci-sif to multi-layer oci-sif",
			srcURI:            c.env.TestRegistryLayeredImage,
			oci:               true,
			keepLayers:        true,
			expectedOCI:       true,
			expectedOCILayers: 8,
			noHTTPS:           true,
			force:             true,
			expectedExitCode:  0,
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
				imageName := getImageNameFromURI(tt.srcURI, tt.oci)
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
	if tt.expectedExitCode != 0 {
		return
	}
	_, err := os.Stat(tt.expectedImage)
	switch err {
	case nil:
		// PASS

	case os.ErrNotExist:
		// FAIL
		t.Errorf("expecting image at %q, not found: %+v\n", tt.expectedImage, err)

	default:
		// FAIL
		t.Errorf("unable to stat image at %q: %+v\n", tt.expectedImage, err)
	}

	// image.Init does an architecture check... so we can't call it if we are
	// pulling a foreign arch image on purpose.
	if tt.arch != "" || tt.platform != "" {
		return
	}

	img, err := image.Init(tt.expectedImage, false)
	if err != nil {
		t.Fatalf("while checking image: %v", err)
	}
	defer img.File.Close()

	switch img.Type {
	case image.SIF:
		if tt.expectedOCI {
			t.Errorf("Native SIF pulled, but --oci specified")
		}
	case image.OCISIF:
		if !tt.expectedOCI {
			t.Errorf("OCI-SIF pulled, but --oci not specified")
		}
		checkOCISIF(t, tt.expectedImage, tt.expectedOCILayers)
	default:
		t.Errorf("Unexpected image type %d", img.Type)
	}
}

func checkOCISIF(t *testing.T, imgFile string, expectLayers int) {
	fi, err := sif.LoadContainerFromPath(imgFile, sif.OptLoadWithFlag(os.O_RDONLY))
	if err != nil {
		t.Fatalf("while loading SIF: %v", err)
	}
	defer fi.UnloadContainer()

	ix, err := ocitsif.ImageIndexFromFileImage(fi)
	if err != nil {
		t.Fatalf("while obtaining image index: %v", err)
	}
	idxManifest, err := ix.IndexManifest()
	if err != nil {
		t.Fatalf("while obtaining index manifest: %v", err)
	}
	if len(idxManifest.Manifests) != 1 {
		t.Fatalf("single manifest expected, found %d manifests", len(idxManifest.Manifests))
	}
	imageDigest := idxManifest.Manifests[0].Digest

	img, err := ix.Image(imageDigest)
	if err != nil {
		t.Fatalf("while initializing image: %v", err)
	}

	layers, err := img.Layers()
	if err != nil {
		t.Fatalf("while fetching image layers: %v", err)
	}

	if len(layers) != expectLayers {
		t.Errorf("expected %d layers, found %d", expectLayers, len(layers))
	}

	for i, l := range layers {
		mt, err := l.MediaType()
		if err != nil {
			t.Fatalf("while examining layer: %v", err)
		}
		if mt != "application/vnd.sylabs.image.layer.v1.squashfs" {
			t.Errorf("layer %d: unsupported layer mediaType %q", i, mt)
		}
	}
}

// this is a version of the oras push functionality that does not check that given the
// file is a valid SIF, this allows us to push arbitrary objects to the local registry
// to test the pull validation
// We can also set the layer mediaType - so we can push images with older media types
// to verify that they can still be pulled.
func orasPushNoCheck(path, ref, layerMediaType string) error {
	ref = strings.TrimPrefix(ref, "oras://")
	ref = strings.TrimPrefix(ref, "//")

	// Get reference to image in the remote
	ir, err := name.ParseReference(ref,
		name.WithDefaultTag(name.DefaultTag),
		name.WithDefaultRegistry(name.DefaultRegistry),
	)
	if err != nil {
		return err
	}

	im, err := oras.NewImageFromSIF(path, types.MediaType(layerMediaType))
	if err != nil {
		return err
	}

	return remote.Write(ir, im, remote.WithUserAgent("singularity e2e-test"))
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
		platform  string
		oci       bool
		noHTTPS   bool
	}{
		{
			name:      "library native",
			imagePath: filepath.Join(c.env.TestDir, "nocache-library.sif"),
			imageSrc:  "library://alpine:latest",
		},
		{
			name:      "library oci-sif",
			imagePath: filepath.Join(c.env.TestDir, "nocache-library.oci.sif"),
			imageSrc:  "library://sylabs/tests/alpine-oci-sif:latest",
			platform:  "linux/amd64",
			oci:       true,
		},
		{
			name:      "oras",
			imagePath: filepath.Join(c.env.TestDir, "nocache-oras.sif"),
			imageSrc:  "oras://" + c.env.TestRegistry + "/pull_test_sif:latest",
		},
		{
			name:      "docker oci-sif",
			imagePath: filepath.Join(c.env.TestDir, "nocache-docker.oci.sif"),
			imageSrc:  c.env.TestRegistryImage,
			oci:       true,
			noHTTPS:   true,
		},
	}

	for _, tt := range disableCacheTests {
		cmdArgs := []string{"--disable-cache"}
		if tt.oci {
			cmdArgs = append(cmdArgs, "--oci")
		}
		if tt.noHTTPS {
			cmdArgs = append(cmdArgs, "--no-https")
		}
		if tt.platform != "" {
			cmdArgs = append(cmdArgs, "--platform", tt.platform)
		}
		cmdArgs = append(cmdArgs, tt.imagePath, tt.imageSrc)
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

func (c ctx) testPullOCIOverlay(t *testing.T) {
	require.MkfsExt3(t)
	e2e.EnsureOCISIF(t, c.env)

	// Create OCI-SIF image with overlay
	tmpDir, cleanup := e2e.MakeTempDir(t, c.env.TestDir, "pull-oci-overlay-", "")
	defer cleanup(t)
	overlaySIF := filepath.Join(tmpDir, "overlay.sif")
	if err := fs.CopyFile(c.env.OCISIFPath, overlaySIF, 0o755); err != nil {
		t.Fatal(err)
	}
	c.env.RunSingularity(
		t,
		e2e.AsSubtest("overlay create"),
		e2e.WithProfile(e2e.UserProfile),
		e2e.WithCommand("overlay"),
		e2e.WithArgs("create", overlaySIF),
		e2e.ExpectExit(0),
	)

	// Push up to local registry
	imgRef := fmt.Sprintf("docker://%s/oci-sif-overlay:test", c.env.TestRegistry)
	c.env.RunSingularity(
		t,
		e2e.AsSubtest("push"),
		e2e.WithProfile(e2e.UserProfile),
		e2e.WithCommand("push"),
		e2e.WithArgs(overlaySIF, imgRef),
		e2e.ExpectExit(0),
	)

	tests := []struct {
		name       string
		keepLayers bool
		expectExit int
	}{
		{
			name:       "pull",
			keepLayers: false,
			expectExit: 0,
		},
		{
			name:       "pull keep-layers",
			keepLayers: true,
			expectExit: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dest := filepath.Join(tmpDir, "pull.sif")
			defer os.Remove(dest)

			args := []string{}
			if tt.keepLayers {
				args = []string{"--keep-layers"}
			}
			args = append(args, dest, imgRef)

			c.env.RunSingularity(
				t,
				e2e.WithDir(t.TempDir()),
				e2e.WithProfile(e2e.OCIUserProfile),
				e2e.WithCommand("pull"),
				e2e.WithArgs(args...),
				e2e.ExpectExit(tt.expectExit),
			)

			if tt.expectExit == 0 {
				hasOverlay, _, err := ocisif.HasOverlay(dest)
				if err != nil {
					t.Error(err)
				}
				if !hasOverlay {
					t.Errorf("Pulled image %s does not have an overlay", dest)
				}
			}
		})
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
			t.Run("oci overlay", c.testPullOCIOverlay)
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
