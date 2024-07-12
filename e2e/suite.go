// Copyright (c) 2020, Control Command Inc. All rights reserved.
// Copyright (c) 2019-2022 Sylabs Inc. All rights reserved.
// Copyright (c) Contributors to the Apptainer project, established as
//   Apptainer a Series of LF Projects LLC.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package e2e

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"

	// Tests imports
	"github.com/sylabs/singularity/v4/e2e/actions"
	"github.com/sylabs/singularity/v4/e2e/build"
	e2ebuildcfg "github.com/sylabs/singularity/v4/e2e/buildcfg"
	"github.com/sylabs/singularity/v4/e2e/cache"
	"github.com/sylabs/singularity/v4/e2e/cgroups"
	"github.com/sylabs/singularity/v4/e2e/cmdenvvars"
	"github.com/sylabs/singularity/v4/e2e/config"
	"github.com/sylabs/singularity/v4/e2e/data"
	"github.com/sylabs/singularity/v4/e2e/delete"
	"github.com/sylabs/singularity/v4/e2e/docker"
	"github.com/sylabs/singularity/v4/e2e/ecl"
	singularityenv "github.com/sylabs/singularity/v4/e2e/env"
	"github.com/sylabs/singularity/v4/e2e/gpu"
	"github.com/sylabs/singularity/v4/e2e/help"
	"github.com/sylabs/singularity/v4/e2e/inspect"
	"github.com/sylabs/singularity/v4/e2e/instance"
	"github.com/sylabs/singularity/v4/e2e/key"
	"github.com/sylabs/singularity/v4/e2e/keyserver"
	"github.com/sylabs/singularity/v4/e2e/oci"
	"github.com/sylabs/singularity/v4/e2e/overlay"
	"github.com/sylabs/singularity/v4/e2e/plugin"
	"github.com/sylabs/singularity/v4/e2e/pull"
	"github.com/sylabs/singularity/v4/e2e/push"
	"github.com/sylabs/singularity/v4/e2e/registry"
	"github.com/sylabs/singularity/v4/e2e/remote"
	"github.com/sylabs/singularity/v4/e2e/run"
	"github.com/sylabs/singularity/v4/e2e/runhelp"
	"github.com/sylabs/singularity/v4/e2e/security"
	"github.com/sylabs/singularity/v4/e2e/sign"
	"github.com/sylabs/singularity/v4/e2e/verify"
	"github.com/sylabs/singularity/v4/e2e/version"

	"github.com/sylabs/singularity/v4/e2e/internal/e2e"
	"github.com/sylabs/singularity/v4/e2e/internal/testhelper"
	"github.com/sylabs/singularity/v4/internal/pkg/buildcfg"
	"github.com/sylabs/singularity/v4/pkg/util/slice"
	useragent "github.com/sylabs/singularity/v4/pkg/util/user-agent"
)

var (
	runDisabled = flag.Bool("run_disabled", false, "run tests that have been temporarily disabled")
	runGroups   = flag.String("e2e_groups", "", "specify a comma separated list of e2e groups to run")
	runTests    = flag.String("e2e_tests", "", "specify a regex matching e2e tests to run")
)

var e2eGroups = map[string]testhelper.Group{
	"ACTIONS":        actions.E2ETests,
	"BUILD":          build.E2ETests,
	"CACHE":          cache.E2ETests,
	"CGROUPS":        cgroups.E2ETests,
	"CMDENVVARS":     cmdenvvars.E2ETests,
	"CONFIG":         config.E2ETests,
	"DATA":           data.E2ETests,
	"DELETE":         delete.E2ETests,
	"DOCKER":         docker.E2ETests,
	"E2EBUILDCFG":    e2ebuildcfg.E2ETests,
	"ECL":            ecl.E2ETests,
	"GPU":            gpu.E2ETests,
	"HELP":           help.E2ETests,
	"INSPECT":        inspect.E2ETests,
	"INSTANCE":       instance.E2ETests,
	"KEY":            key.E2ETests,
	"KEYSERVER":      keyserver.E2ETests,
	"OCI":            oci.E2ETests,
	"OVERLAY":        overlay.E2ETests,
	"PLUGIN":         plugin.E2ETests,
	"PULL":           pull.E2ETests,
	"PUSH":           push.E2ETests,
	"REGISTRY":       registry.E2ETests,
	"REMOTE":         remote.E2ETests,
	"RUN":            run.E2ETests,
	"RUNHELP":        runhelp.E2ETests,
	"SECURITY":       security.E2ETests,
	"SIGN":           sign.E2ETests,
	"SINGULARITYENV": singularityenv.E2ETests,
	"VERIFY":         verify.E2ETests,
	"VERSION":        version.E2ETests,
}

// Run is the main func for the test framework, initializes the required vars
// and sets the environment for the RunE2ETests framework
func Run(t *testing.T) {
	flag.Parse()

	var testenv e2e.TestEnv

	if *runDisabled {
		testenv.RunDisabled = true
	}
	// init buildcfg values
	useragent.InitValue(buildcfg.PACKAGE_NAME, buildcfg.PACKAGE_VERSION)

	// Ensure binary is in $PATH
	cmdPath := filepath.Join(buildcfg.BINDIR, "singularity")
	if _, err := exec.LookPath(cmdPath); err != nil {
		log.Fatalf("singularity is not installed on this system: %v", err)
	}

	testenv.CmdPath = cmdPath

	sysconfdir := func(fn string) string {
		return filepath.Join(buildcfg.SYSCONFDIR, "singularity", fn)
	}

	// Make temp dir for tests
	name, err := os.MkdirTemp("", "stest.")
	if err != nil {
		log.Fatalf("failed to create temporary directory: %v", err)
	}
	defer e2e.Privileged(func(t *testing.T) {
		if t.Failed() {
			t.Logf("Test failed, not removing %s", name)
			return
		}

		os.RemoveAll(name)
	})(t)

	if err := os.Chmod(name, 0o755); err != nil {
		log.Fatalf("failed to chmod temporary directory: %v", err)
	}
	testenv.TestDir = name

	// Make shared cache dirs for privileged and unpriviliged E2E tests.
	// Individual tests that depend on specific ordered cache behavior, or
	// directly test the cache, should override the TestEnv values within the
	// specific test.
	privCacheDir, cleanPrivCache := e2e.MakeCacheDir(t, testenv.TestDir)
	testenv.PrivCacheDir = privCacheDir
	defer e2e.Privileged(func(t *testing.T) {
		cleanPrivCache(t)
	})(t)

	unprivCacheDir, cleanUnprivCache := e2e.MakeCacheDir(t, testenv.TestDir)
	testenv.UnprivCacheDir = unprivCacheDir
	defer cleanUnprivCache(t)

	// e2e tests need to run in a somehow agnostic environment, so we
	// don't use environment of user executing tests in order to not
	// wrongly interfering with cache stuff, sylabs library tokens,
	// PGP keys
	e2e.SetupHomeDirectories(t)

	// generate singularity.conf with default values
	e2e.SetupDefaultConfig(t, filepath.Join(testenv.TestDir, "singularity.conf"))

	// create an empty plugin directory
	e2e.SetupPluginDir(t, testenv.TestDir)

	// duplicate system remote.yaml and create a temporary one on top of original
	e2e.SetupSystemRemoteFile(t, testenv.TestDir)

	// create an empty ECL configuration and empty global keyring
	e2e.SetupSystemECLAndGlobalKeyRing(t, testenv.TestDir)

	// Creates '$HOME/.singularity/docker-config.json' with credentials
	e2e.SetupDockerHubCredentials(t)

	// Ensure config files are installed
	configFiles := []string{
		sysconfdir("singularity.conf"),
		sysconfdir("ecl.toml"),
		sysconfdir("capability.json"),
		sysconfdir("nvliblist.conf"),
	}

	for _, cf := range configFiles {
		//nolint:forcetypeassert
		if fi, err := os.Stat(cf); err != nil {
			t.Fatalf("%s is not installed on this system: %v", cf, err)
		} else if !fi.Mode().IsRegular() {
			t.Fatalf("%s is not a regular file", cf)
		} else if fi.Sys().(*syscall.Stat_t).Uid != 0 {
			t.Fatalf("%s must be owned by root", cf)
		}
	}

	// Provision local registry
	testenv.TestRegistry = e2e.StartRegistry(t, testenv)
	testenv.TestRegistryImage = fmt.Sprintf("docker://%s/my-alpine:3.18", testenv.TestRegistry)
	testenv.TestRegistryLayeredImage = fmt.Sprintf("docker://%s/aufs-sanity:latest", testenv.TestRegistry)
	testenv.TestRegistryPrivURI = fmt.Sprintf("docker://%s", testenv.TestRegistry)
	testenv.TestRegistryPrivPath = fmt.Sprintf("%s/private/e2eprivrepo", testenv.TestRegistry)
	testenv.TestRegistryPrivImage = fmt.Sprintf("docker://%s/my-alpine:3.18", testenv.TestRegistryPrivPath)

	// Copy small test image (alpine:3.18) into local registry from DockerHub
	insecureSource := false
	insecureValue := os.Getenv("E2E_DOCKER_MIRROR_INSECURE")
	if insecureValue != "" {
		insecureSource, err = strconv.ParseBool(insecureValue)
		if err != nil {
			t.Fatalf("could not convert E2E_DOCKER_MIRROR_INSECURE=%s: %s", insecureValue, err)
		}
	}
	e2e.CopyOCIImage(t, "docker://alpine:3.18", testenv.TestRegistryImage, insecureSource, true)

	// This image has many (8) small layers, constructed to test overlay behavior.
	// https://github.com/sylabs/singularity-test-containers/tree/master/docker-aufs-sanity
	e2e.CopyOCIImage(t, "docker://sylabsio/aufs-sanity:latest", testenv.TestRegistryLayeredImage, insecureSource, true)

	// Copy same test image into private location in test registry
	e2e.PrivateRepoLogin(t, testenv, e2e.UserProfile, "")
	e2e.CopyOCIImage(t, "docker://alpine:3.18", testenv.TestRegistryPrivImage, insecureSource, true)
	e2e.PrivateRepoLogout(t, testenv, e2e.UserProfile, "")

	// SIF base test path, built on demand by e2e.EnsureImage
	imagePath := path.Join(name, "test.sif")
	t.Log("Path to test image:", imagePath)
	testenv.ImagePath = imagePath

	// OCI Layout test directory path, built on demand by e2e.EnsureOCILayout
	ociLayoutPath := path.Join(name, "oci-layout")
	t.Log("Path to test OCI layout:", ociLayoutPath)
	testenv.OCILayoutPath = ociLayoutPath

	// OCI Archive test image path, built on demand by e2e.EnsureOCIArchive
	ociArchivePath := path.Join(name, "oci-archive.tar")
	t.Log("Path to test OCI archive:", ociArchivePath)
	testenv.OCIArchivePath = ociArchivePath

	// OCI-SIF Image, retrieved on demand by e2e.EnsureOCISIF
	ociSifPath := path.Join(name, "oci-sif.sif")
	t.Log("Path to test OCI-SIF image:", ociSifPath)
	testenv.OCISIFPath = ociSifPath

	// Docker Archive test image path, built on demand by e2e.EnsureDockerArchive
	dockerArchivePath := path.Join(name, "docker.tar")
	t.Log("Path to test Docker archive:", dockerArchivePath)
	testenv.DockerArchivePath = dockerArchivePath

	// Local registry ORAS SIF image, built on demand by e2e.EnsureORASImage
	testenv.OrasTestImage = fmt.Sprintf("oras://%s/oras_test_sif:latest", testenv.TestRegistry)

	// Local registry ORAS OCI-SIF image, built on demand by e2e.EnsureORASOCISIF
	testenv.OrasTestOCISIF = fmt.Sprintf("oras://%s/oras_test_oci-sif:latest", testenv.TestRegistry)

	// OCI-SIF image pushed as OCI image to local registry, built on demand by e2e.EnsureRegistryOCISIF
	testenv.TestRegistryOCISIF = fmt.Sprintf("docker://%s/registry_test_oci-sif:latest", testenv.TestRegistry)

	t.Cleanup(func() {
		if !t.Failed() {
			os.Remove(imagePath)
			os.Remove(ociArchivePath)
			os.Remove(dockerArchivePath)
		}
	})

	suite := testhelper.NewSuite(t, testenv)

	groups := []string{}
	if runGroups != nil && *runGroups != "" {
		groups = strings.Split(*runGroups, ",")
	}

	for key, val := range e2eGroups {
		if len(groups) == 0 || slice.ContainsString(groups, key) {
			suite.AddGroup(key, val)
		}
	}

	suite.Run(runTests)
}
