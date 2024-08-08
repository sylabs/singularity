// Copyright (c) 2019-2023 Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

// The DOCKER E2E group tests functionality of actions, pulls / builds of
// Docker/OCI source images. These tests are separated from direct SIF build /
// pull / actions because they examine OCI specific image behavior. They are run
// ordered, rather than in parallel to avoid any concurrency issues with
// containers/image. Also, we can then maximally benefit from caching to avoid
// Docker Hub rate limiting.

package docker

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	dockerclient "github.com/docker/docker/client"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/pkg/errors"
	ocisif "github.com/sylabs/oci-tools/pkg/sif"
	"github.com/sylabs/sif/v2/pkg/sif"
	"github.com/sylabs/singularity/v4/e2e/internal/e2e"
	"github.com/sylabs/singularity/v4/e2e/internal/testhelper"
	"github.com/sylabs/singularity/v4/internal/pkg/test/tool/require"
	"github.com/sylabs/singularity/v4/internal/pkg/test/tool/tmpl"
	"github.com/sylabs/singularity/v4/internal/pkg/util/fs"
	"golang.org/x/sys/unix"
	"gotest.tools/assert"
)

type ctx struct {
	env e2e.TestEnv
}

func (c ctx) testDockerPulls(t *testing.T) {
	const tmpContainerFile = "test_container.sif"

	tmpPath, err := fs.MakeTmpDir(c.env.TestDir, "docker-", 0o755)
	err = errors.Wrapf(err, "creating temporary directory in %q for docker pull test", c.env.TestDir)
	if err != nil {
		t.Fatalf("failed to create temporary directory: %+v", err)
	}
	t.Cleanup(func() {
		if !t.Failed() {
			os.RemoveAll(tmpPath)
		}
	})

	tmpImage := filepath.Join(tmpPath, tmpContainerFile)

	tests := []struct {
		name    string
		options []string
		image   string
		uri     string
		exit    int
	}{
		{
			name:  "BusyboxLatestPull",
			image: tmpImage,
			uri:   "docker://busybox:latest",
			exit:  0,
		},
		{
			name:  "BusyboxLatestPullFail",
			image: tmpImage,
			uri:   "docker://busybox:latest",
			exit:  255,
		},
		{
			name:    "BusyboxLatestPullForce",
			options: []string{"--force"},
			image:   tmpImage,
			uri:     "docker://busybox:latest",
			exit:    0,
		},
		{
			name:    "Busybox1.28Pull",
			options: []string{"--force", "--dir", tmpPath},
			image:   tmpContainerFile,
			uri:     "docker://busybox:1.28",
			exit:    0,
		},
		{
			name:  "Busybox1.28PullFail",
			image: tmpImage,
			uri:   "docker://busybox:1.28",
			exit:  255,
		},
		{
			name:  "Busybox1.28PullDirFail",
			image: "/foo/sif.sif",
			uri:   "docker://busybox:1.28",
			exit:  255,
		},
	}

	for _, tt := range tests {
		c.env.RunSingularity(
			t,
			e2e.AsSubtest(tt.name),
			e2e.WithProfile(e2e.UserProfile),
			e2e.WithCommand("pull"),
			e2e.WithArgs(append(tt.options, tt.image, tt.uri)...),
			e2e.PostRun(func(t *testing.T) {
				if !t.Failed() && tt.exit == 0 {
					path := tt.image
					// handle the --dir case
					if path == tmpContainerFile {
						path = filepath.Join(tmpPath, tmpContainerFile)
					}
					c.env.ImageVerify(t, path)
				}
			}),
			e2e.ExpectExit(tt.exit),
		)
	}
}

// Testing DOCKER_ host support (only if docker available)
func (c ctx) testDockerHost(t *testing.T) {
	require.Command(t, "docker")

	// Temporary homedir for docker commands, so invoking docker doesn't create
	// a ~/.docker that may interfere elsewhere.
	tmpHome, cleanupHome := e2e.MakeTempDir(t, c.env.TestDir, "docker-", "")
	t.Cleanup(func() { e2e.Privileged(cleanupHome)(t) })

	// Create a Dockerfile for a small image we can build locally
	tmpPath, cleanup := e2e.MakeTempDir(t, c.env.TestDir, "docker-", "")
	t.Cleanup(func() { cleanup(t) })

	dockerfile := filepath.Join(tmpPath, "Dockerfile")
	dockerfileContent := []byte("FROM alpine:latest\n")
	err := os.WriteFile(dockerfile, dockerfileContent, 0o644)
	if err != nil {
		t.Fatalf("failed to create temporary Dockerfile: %+v", err)
	}

	dockerRef := "dinosaur/test-image:latest"
	dockerURI := "docker-daemon:" + dockerRef

	// Invoke docker build to build image in the docker daemon.
	// Use os/exec because easier to generate a command with a working directory
	e2e.Privileged(func(t *testing.T) {
		cmd := exec.Command("docker", "build", "-t", dockerRef, tmpPath)
		cmd.Dir = tmpPath
		cmd.Env = append(cmd.Env, "HOME="+tmpHome)
		out, err := cmd.CombinedOutput()
		t.Log(cmd.Args)
		if err != nil {
			t.Fatalf("Unexpected error while running command.\n%s: %s", err, string(out))
		}
	})(t)

	tests := []struct {
		name       string
		envarName  string
		envarValue string
		exit       int
	}{
		// Unset docker host should use default and succeed
		{
			name:       "singularityDockerHostEmpty",
			envarName:  "SINGULARITY_DOCKER_HOST",
			envarValue: "",
			exit:       0,
		},
		{
			name:       "dockerHostEmpty",
			envarName:  "DOCKER_HOST",
			envarValue: "",
			exit:       0,
		},

		// bad Docker host should fail
		{
			name:       "singularityDockerHostInvalid",
			envarName:  "SINGULARITY_DOCKER_HOST",
			envarValue: "tcp://192.168.59.103:oops",
			exit:       255,
		},
		{
			name:       "dockerHostInvalid",
			envarName:  "DOCKER_HOST",
			envarValue: "tcp://192.168.59.103:oops",
			exit:       255,
		},

		// Set to default should succeed
		// The default host varies based on OS, so we use dockerclient default
		{
			name:       "singularityDockerHostValid",
			envarName:  "SINGULARITY_DOCKER_HOST",
			envarValue: dockerclient.DefaultDockerHost,
			exit:       0,
		},
		{
			name:       "dockerHostValid",
			envarName:  "DOCKER_HOST",
			envarValue: dockerclient.DefaultDockerHost,
			exit:       0,
		},
	}

	for _, profile := range []e2e.Profile{e2e.RootProfile, e2e.OCIRootProfile} {
		t.Run(profile.String(), func(t *testing.T) {
			t.Run("exec", func(t *testing.T) {
				for _, tt := range tests {
					cmdOps := []e2e.SingularityCmdOp{
						e2e.WithProfile(profile),
						e2e.AsSubtest(profile.String() + "/" + tt.name),
						e2e.WithCommand("exec"),
						e2e.WithArgs("--disable-cache", dockerURI, "/bin/true"),
						e2e.WithEnv(append(os.Environ(), tt.envarName+"="+tt.envarValue)),
						e2e.ExpectExit(tt.exit),
					}
					c.env.RunSingularity(t, cmdOps...)
				}
			})

			t.Run("pull", func(t *testing.T) {
				for _, tt := range tests {
					cmdOps := []e2e.SingularityCmdOp{
						e2e.WithProfile(profile),
						e2e.AsSubtest(tt.name),
						e2e.WithCommand("pull"),
						e2e.WithArgs("--force", "--disable-cache", dockerURI),
						e2e.WithEnv(append(os.Environ(), tt.envarName+"="+tt.envarValue)),
						e2e.WithDir(tmpPath),
						e2e.ExpectExit(tt.exit),
					}
					c.env.RunSingularity(t, cmdOps...)
				}
			})
		})

		t.Run("build", func(t *testing.T) {
			for _, tt := range tests {
				cmdOps := []e2e.SingularityCmdOp{
					e2e.WithProfile(e2e.RootProfile),
					e2e.AsSubtest(tt.name),
					e2e.WithCommand("build"),
					e2e.WithArgs("--force", "--disable-cache", "test.sif", dockerURI),
					e2e.WithEnv(append(os.Environ(), tt.envarName+"="+tt.envarValue)),
					e2e.WithDir(tmpPath),
					e2e.ExpectExit(tt.exit),
				}
				c.env.RunSingularity(t, cmdOps...)
			}
		})
	}

	// Clean up docker image
	e2e.Privileged(func(t *testing.T) {
		cmd := exec.Command("docker", "rmi", dockerRef)
		cmd.Env = append(cmd.Env, "HOME="+tmpHome)
		_, err = cmd.Output()
		if err != nil {
			t.Fatalf("Unexpected error while cleaning up docker image.\n%s", err)
		}
	})(t)
}

// Test that DOCKER_xyz env vars take priority over other means of
// authenticating with Docker - in particular, the --authfile flag.
func (c ctx) testDockerCredsPriority(t *testing.T) {
	e2e.EnsureImage(t, c.env)

	privImgNoPrefix := strings.TrimPrefix(c.env.TestRegistryPrivImage, "docker://")
	simpleDef := e2e.PrepareDefFile(e2e.DefFileDetails{
		Bootstrap: "docker",
		From:      privImgNoPrefix,
	})
	t.Cleanup(func() {
		if !t.Failed() {
			os.Remove(simpleDef)
		}
	})

	tmpdir, tmpdirCleanup := e2e.MakeTempDir(t, c.env.TestDir, "build-auth", "")
	t.Cleanup(func() {
		if !t.Failed() {
			tmpdirCleanup(t)
		}
	})

	dockerfileContent := fmt.Sprintf(
		`
FROM %s
CMD /bin/true
`,
		privImgNoPrefix,
	)
	dockerfile, err := e2e.WriteTempFile(tmpdir, "Dockerfile", dockerfileContent)
	if err != nil {
		t.Fatalf("while trying to generate test dockerfile: %v", err)
	}
	ocisifPath := dockerfile + ".oci.sif"

	profiles := []e2e.Profile{
		e2e.UserProfile,
		e2e.RootProfile,
	}

	for _, p := range profiles {
		t.Run(p.String(), func(t *testing.T) {
			t.Run("def pull", func(t *testing.T) {
				c.dockerCredsPriorityTester(t, false, p, "pull", "--disable-cache", "--no-https", "-F", c.env.TestRegistryPrivImage)
			})
			t.Run("def exec", func(t *testing.T) {
				c.dockerCredsPriorityTester(t, false, p, "exec", "--disable-cache", "--no-https", c.env.TestRegistryPrivImage, "true")
			})
			t.Run("cstm pull", func(t *testing.T) {
				c.dockerCredsPriorityTester(t, true, p, "pull", "--disable-cache", "--no-https", "-F", c.env.TestRegistryPrivImage)
			})
			t.Run("cstm exec", func(t *testing.T) {
				c.dockerCredsPriorityTester(t, true, p, "exec", "--disable-cache", "--no-https", c.env.TestRegistryPrivImage, "true")
			})
		})
	}
	profiles = []e2e.Profile{
		e2e.OCIUserProfile,
		e2e.OCIRootProfile,
	}

	for _, p := range profiles {
		t.Run(p.String(), func(t *testing.T) {
			t.Run("def df build", func(t *testing.T) {
				c.dockerCredsPriorityTester(t, false, p, "build", "-F", ocisifPath, dockerfile)
			})
			t.Run("cstm df build", func(t *testing.T) {
				c.dockerCredsPriorityTester(t, true, p, "build", "-F", ocisifPath, dockerfile)
			})
		})
	}
}

func (c ctx) dockerCredsPriorityTester(t *testing.T, withCustomAuthFile bool, profile e2e.Profile, cmd string, args ...string) {
	tmpdir, tmpdirCleanup := e2e.MakeTempDir(t, c.env.TestDir, "docker-auth", "")
	t.Cleanup(func() {
		if !t.Failed() {
			tmpdirCleanup(t)
		}
	})

	prevCwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("could not get current working directory: %s", err)
	}
	defer os.Chdir(prevCwd)
	if err = os.Chdir(tmpdir); err != nil {
		t.Fatalf("could not change cwd to %q: %s", tmpdir, err)
	}

	localAuthFileName := ""
	if withCustomAuthFile {
		localAuthFileName = "./my_local_authfile"
	}

	authFileArgs := []string{}
	if withCustomAuthFile {
		authFileArgs = []string{"--authfile", localAuthFileName}
	}

	// Store the previous values of relevant env vars, and set up facilities for
	// wiping and restoring them.
	envVarSet := []string{
		"SINGULARITY_DOCKER_USERNAME",
		"SINGULARITY_DOCKER_PASSWORD",
		"DOCKER_USERNAME",
		"DOCKER_PASSWORD",
	}
	prevEnvVals := make(map[string]string)
	for _, varName := range envVarSet {
		if varVal, ok := os.LookupEnv(varName); ok {
			prevEnvVals[varName] = varVal
		}
	}
	wipeVars := func() {
		for _, varName := range envVarSet {
			os.Unsetenv(varName)
		}
	}
	restoreVars := func() {
		wipeVars()
		for _, varName := range envVarSet {
			if varVal, ok := prevEnvVals[varName]; ok {
				os.Setenv(varName, varVal)
			}
		}
	}

	t.Cleanup(func() {
		e2e.PrivateRepoLogout(t, c.env, profile, localAuthFileName)
		restoreVars()
	})

	tests := []struct {
		name            string
		pfxDockerUser   string
		pfxDockerPass   string
		nopfxDockerUser string
		nopfxDockerPass string
		authLoggedIn    bool
		expectExit      int
	}{
		{
			name:          "pfx denv wrong, no auth",
			pfxDockerUser: "wrong",
			pfxDockerPass: "wrong",
			authLoggedIn:  false,
			expectExit:    255,
		},
		{
			name:          "pfx denv wrong, auth",
			pfxDockerUser: "wrong",
			pfxDockerPass: "wrong",
			authLoggedIn:  true,
			expectExit:    255,
		},
		{
			name:          "pfx denv right, no auth",
			pfxDockerUser: e2e.DefaultUsername,
			pfxDockerPass: e2e.DefaultPassword,
			authLoggedIn:  false,
			expectExit:    0,
		},
		{
			name:          "pfx denv right, auth",
			pfxDockerUser: e2e.DefaultUsername,
			pfxDockerPass: e2e.DefaultPassword,
			authLoggedIn:  true,
			expectExit:    0,
		},
		{
			name:            "nopfx denv wrong, no auth",
			nopfxDockerUser: "wrong",
			nopfxDockerPass: "wrong",
			authLoggedIn:    false,
			expectExit:      255,
		},
		{
			name:            "nopfx denv wrong, auth",
			nopfxDockerUser: "wrong",
			nopfxDockerPass: "wrong",
			authLoggedIn:    true,
			expectExit:      255,
		},
		{
			name:            "nopfx denv right, no auth",
			nopfxDockerUser: e2e.DefaultUsername,
			nopfxDockerPass: e2e.DefaultPassword,
			authLoggedIn:    false,
			expectExit:      0,
		},
		{
			name:            "nopfx denv right, auth",
			nopfxDockerUser: e2e.DefaultUsername,
			nopfxDockerPass: e2e.DefaultPassword,
			authLoggedIn:    true,
			expectExit:      0,
		},
		{
			name:            "both denv (pfx right), auth",
			pfxDockerUser:   e2e.DefaultUsername,
			pfxDockerPass:   e2e.DefaultPassword,
			nopfxDockerUser: "wrong",
			nopfxDockerPass: "wrong",
			authLoggedIn:    true,
			expectExit:      0,
		},
		{
			name:            "both denv (pfx right), noauth",
			pfxDockerUser:   e2e.DefaultUsername,
			pfxDockerPass:   e2e.DefaultPassword,
			nopfxDockerUser: "wrong",
			nopfxDockerPass: "wrong",
			authLoggedIn:    false,
			expectExit:      0,
		},
		{
			name:            "both denv (nopfx right), auth",
			pfxDockerUser:   "wrong",
			pfxDockerPass:   "wrong",
			nopfxDockerUser: e2e.DefaultUsername,
			nopfxDockerPass: e2e.DefaultPassword,
			authLoggedIn:    true,
			expectExit:      255,
		},
		{
			name:            "both denv (nopfx right), noauth",
			pfxDockerUser:   "wrong",
			pfxDockerPass:   "wrong",
			nopfxDockerUser: e2e.DefaultUsername,
			nopfxDockerPass: e2e.DefaultPassword,
			authLoggedIn:    false,
			expectExit:      255,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wipeVars()
			if tt.pfxDockerUser != "" {
				os.Setenv("SINGULARITY_DOCKER_USERNAME", tt.pfxDockerUser)
			}
			if tt.pfxDockerPass != "" {
				os.Setenv("SINGULARITY_DOCKER_PASSWORD", tt.pfxDockerPass)
			}
			if tt.nopfxDockerUser != "" {
				os.Setenv("DOCKER_USERNAME", tt.nopfxDockerUser)
			}
			if tt.nopfxDockerPass != "" {
				os.Setenv("DOCKER_PASSWORD", tt.nopfxDockerPass)
			}
			if tt.authLoggedIn {
				e2e.PrivateRepoLogin(t, c.env, profile, localAuthFileName)
			} else {
				e2e.PrivateRepoLogout(t, c.env, profile, localAuthFileName)
			}
			c.env.RunSingularity(
				t,
				e2e.WithProfile(profile),
				e2e.WithCommand(cmd),
				e2e.WithArgs(append(authFileArgs, args...)...),
				e2e.ExpectExit(tt.expectExit),
			)
		})
	}
}

// AUFS whiteout tests
func (c ctx) testDockerAUFS(t *testing.T) {
	tests := []struct {
		name       string
		profile    e2e.Profile
		keepLayers bool
	}{
		// Native SIF - whiteouts applied to squashed image via umoci rootfs
		// extraction at creation.
		{
			name:       "NativeSIF",
			profile:    e2e.UserProfile,
			keepLayers: false,
		},
		// Single layer OCI-SIF - whiteouts applied to squashed image via
		// oci-tools.Squash at creation.
		{
			name:       "OCISIF",
			profile:    e2e.OCIUserProfile,
			keepLayers: false,
		},
		// Multi layer OCI-SIF - whiteouts translated AUFS -> OverlayFS by
		// oci-tools at creation and applied at runtime.
		{
			name:       "OCISIFKeepLayers",
			profile:    e2e.OCIUserProfile,
			keepLayers: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c.aufsTest(t, tt.profile, tt.keepLayers)
		})
	}
}

func (c ctx) aufsTest(t *testing.T, profile e2e.Profile, keepLayers bool) {
	imageDir, cleanup := e2e.MakeTempDir(t, c.env.TestDir, "aufs-", "")
	t.Cleanup(func() {
		if !t.Failed() {
			cleanup(t)
		}
	})
	imagePath := filepath.Join(imageDir, "container")

	args := []string{}
	if keepLayers {
		args = []string{"--keep-layers"}
	}
	args = append(args, imagePath, "docker://sylabsio/aufs-sanity")

	c.env.RunSingularity(
		t,
		e2e.WithProfile(profile),
		e2e.WithCommand("pull"),
		e2e.WithArgs(args...),
		e2e.ExpectExit(0),
	)

	if t.Failed() {
		return
	}

	fileTests := []struct {
		name string
		argv []string
		exit int
	}{
		// 'file2' should be present in three locations
		{
			name: "File 2",
			argv: []string{imagePath, "ls", "/test/whiteout-dir/file2", "/test/whiteout-file/file2", "/test/normal-dir/file2"},
			exit: 0,
		},
		// '/test/whiteout-file/file1' should be absent (via whiteout of the file)
		{
			name: "WhiteoutFileFile1",
			argv: []string{imagePath, "ls", "/test/whiteout-file/file1"},
			exit: 1,
		},
		// '/test/whiteout-dir/file1' should be absent (via whiteout of the dir)
		{
			name: "WhiteoutDirFile1",
			argv: []string{imagePath, "ls", "/test/whiteout-dir/file1"},
			exit: 1,
		},
	}

	for _, tt := range fileTests {
		c.env.RunSingularity(
			t,
			e2e.AsSubtest(tt.name),
			e2e.WithProfile(profile),
			e2e.WithCommand("exec"),
			e2e.WithArgs(tt.argv...),
			e2e.ExpectExit(tt.exit),
		)
	}
}

// Check force permissions for user builds #977
func (c ctx) testDockerPermissions(t *testing.T) {
	imageDir, cleanup := e2e.MakeTempDir(t, c.env.TestDir, "perm-", "")
	t.Cleanup(func() {
		if !t.Failed() {
			cleanup(t)
		}
	})
	imagePath := filepath.Join(imageDir, "container")

	c.env.RunSingularity(
		t,
		e2e.WithProfile(e2e.UserProfile),
		e2e.WithCommand("build"),
		e2e.WithArgs([]string{imagePath, "docker://sylabsio/userperms"}...),
		e2e.ExpectExit(0),
	)

	if t.Failed() {
		return
	}

	fileTests := []struct {
		name string
		argv []string
		exit int
	}{
		{
			name: "TestDir",
			argv: []string{imagePath, "ls", "/testdir/"},
			exit: 0,
		},
		{
			name: "TestDirFile",
			argv: []string{imagePath, "ls", "/testdir/testfile"},
			exit: 1,
		},
	}
	for _, tt := range fileTests {
		c.env.RunSingularity(
			t,
			e2e.AsSubtest(tt.name),
			e2e.WithProfile(e2e.UserProfile),
			e2e.WithCommand("exec"),
			e2e.WithArgs(tt.argv...),
			e2e.ExpectExit(tt.exit),
		)
	}
}

// Check whiteout of symbolic links #1592 #1576
func (c ctx) testDockerWhiteoutSymlink(t *testing.T) {
	imageDir, cleanup := e2e.MakeTempDir(t, c.env.TestDir, "whiteout-", "")
	t.Cleanup(func() {
		if !t.Failed() {
			cleanup(t)
		}
	})
	imagePath := filepath.Join(imageDir, "container")

	c.env.RunSingularity(
		t,
		e2e.WithProfile(e2e.UserProfile),
		e2e.WithCommand("build"),
		e2e.WithArgs([]string{imagePath, "docker://sylabsio/linkwh"}...),
		e2e.PostRun(func(t *testing.T) {
			if t.Failed() {
				return
			}
			c.env.ImageVerify(t, imagePath)
		}),
		e2e.ExpectExit(0),
	)
}

func (c ctx) testDockerDefFile(t *testing.T) {
	imageDir, cleanup := e2e.MakeTempDir(t, c.env.TestDir, "def-", "")
	t.Cleanup(func() {
		if !t.Failed() {
			cleanup(t)
		}
	})
	imagePath := filepath.Join(imageDir, "container")

	getKernelMajor := func(t *testing.T) (major int) {
		var buf unix.Utsname
		if err := unix.Uname(&buf); err != nil {
			err = errors.Wrap(err, "getting current kernel information")
			t.Fatalf("uname failed: %+v", err)
		}
		n, err := fmt.Sscanf(string(buf.Release[:]), "%d.", &major)
		err = errors.Wrap(err, "getting current kernel release")
		if err != nil {
			t.Fatalf("Sscanf failed, n=%d: %+v", n, err)
		}
		if n != 1 {
			t.Fatalf("Unexpected result while getting major release number: n=%d", n)
		}
		return
	}

	tests := []struct {
		name                string
		kernelMajorRequired int
		archRequired        string
		from                string
	}{
		{
			name:                "Alpine",
			kernelMajorRequired: 0,
			from:                "alpine:latest",
		},
		{
			name:                "AlmaLinux_9",
			kernelMajorRequired: 3,
			from:                "almalinux:9",
		},
		{
			name:                "Ubuntu_2204",
			kernelMajorRequired: 3,
			from:                "ubuntu:22.04",
		},
	}

	for _, tt := range tests {
		defFile := e2e.PrepareDefFile(e2e.DefFileDetails{
			Bootstrap: "docker",
			From:      tt.from,
		})

		c.env.RunSingularity(
			t,
			e2e.AsSubtest(tt.name),
			e2e.WithProfile(e2e.RootProfile),
			e2e.WithCommand("build"),
			e2e.WithArgs([]string{imagePath, defFile}...),
			e2e.PreRun(func(t *testing.T) {
				require.Arch(t, tt.archRequired)
				if getKernelMajor(t) < tt.kernelMajorRequired {
					t.Skipf("kernel >=%v.x required", tt.kernelMajorRequired)
				}
			}),
			e2e.PostRun(func(t *testing.T) {
				if t.Failed() {
					return
				}

				c.env.ImageVerify(t, imagePath)

				if !t.Failed() {
					os.Remove(imagePath)
					os.Remove(defFile)
				}
			}),
			e2e.ExpectExit(0),
		)
	}
}

func (c ctx) testDockerRegistry(t *testing.T) {
	imageDir, cleanup := e2e.MakeTempDir(t, c.env.TestDir, "registry-", "")
	t.Cleanup(func() {
		if !t.Failed() {
			cleanup(t)
		}
	})
	imagePath := filepath.Join(imageDir, "container")

	tests := []struct {
		name string
		exit int
		dfd  e2e.DefFileDetails
	}{
		{
			name: "Alpine",
			exit: 0,
			dfd: e2e.DefFileDetails{
				Bootstrap: "docker",
				From:      c.env.TestRegistry + "/my-alpine:3.18",
			},
		},
		{
			name: "AlpineRegistry",
			exit: 0,
			dfd: e2e.DefFileDetails{
				Bootstrap: "docker",
				From:      "my-alpine:3.18",
				Registry:  c.env.TestRegistry,
			},
		},
		{
			name: "AlpineNamespace",
			exit: 255,
			dfd: e2e.DefFileDetails{
				Bootstrap: "docker",
				From:      "my-alpine:3.18",
				Registry:  c.env.TestRegistry,
				Namespace: "not-a-namespace",
			},
		},
	}

	for _, tt := range tests {
		defFile := e2e.PrepareDefFile(tt.dfd)

		c.env.RunSingularity(
			t,
			e2e.WithProfile(e2e.RootProfile),
			e2e.WithCommand("build"),
			e2e.WithArgs("--disable-cache", "--no-https", imagePath, defFile),
			e2e.PostRun(func(t *testing.T) {
				if t.Failed() || tt.exit != 0 {
					return
				}

				c.env.ImageVerify(t, imagePath)

				if !t.Failed() {
					os.Remove(imagePath)
					os.Remove(defFile)
				}
			}),
			e2e.ExpectExit(tt.exit),
		)
	}
}

func (c ctx) testDockerLabels(t *testing.T) {
	imageDir, cleanup := e2e.MakeTempDir(t, c.env.TestDir, "labels-", "")
	t.Cleanup(func() {
		if !t.Failed() {
			cleanup(t)
		}
	})
	imagePath := filepath.Join(imageDir, "container")

	// Test container & set labels
	// See: https://github.com/sylabs/singularity-test-containers/pull/1
	imgSrc := "docker://sylabsio/labels"
	label1 := "LABEL1: 1"
	label2 := "LABEL2: TWO"

	c.env.RunSingularity(
		t,
		e2e.AsSubtest("build"),
		e2e.WithProfile(e2e.RootProfile),
		e2e.WithCommand("build"),
		e2e.WithArgs(imagePath, imgSrc),
		e2e.ExpectExit(0),
	)

	verifyOutput := func(t *testing.T, r *e2e.SingularityCmdResult) {
		output := string(r.Stdout)
		for _, l := range []string{label1, label2} {
			if !strings.Contains(output, l) {
				t.Errorf("Did not find expected label %s in inspect output", l)
			}
		}
	}

	c.env.RunSingularity(
		t,
		e2e.AsSubtest("inspect"),
		e2e.WithProfile(e2e.UserProfile),
		e2e.WithCommand("inspect"),
		e2e.WithArgs([]string{"--labels", imagePath}...),
		e2e.ExpectExit(0, verifyOutput),
	)
}

func (c ctx) testDockerCMD(t *testing.T) {
	imageDir, cleanup := e2e.MakeTempDir(t, c.env.TestDir, "docker-", "")
	t.Cleanup(func() {
		if !t.Failed() {
			cleanup(t)
		}
	})
	imagePath := filepath.Join(imageDir, "container")

	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("while getting $HOME: %s", err)
	}

	c.env.RunSingularity(
		t,
		e2e.WithProfile(e2e.UserProfile),
		e2e.WithCommand("pull"),
		e2e.WithArgs(imagePath, "docker://sylabsio/docker-cmd"),
		e2e.ExpectExit(0),
	)

	tests := []struct {
		name         string
		args         []string
		noeval       bool
		expectOutput string
	}{
		// Singularity historic behavior (without --no-eval)
		// These do not all match Docker, due to evaluation, consumption of quoting.
		{
			name:         "default",
			args:         []string{},
			noeval:       false,
			expectOutput: `CMD 'quotes' "quotes" $DOLLAR s p a c e s`,
		},
		{
			name:         "override",
			args:         []string{"echo", "test"},
			noeval:       false,
			expectOutput: `test`,
		},
		{
			name:         "override env var",
			args:         []string{"echo", "$HOME"},
			noeval:       false,
			expectOutput: home,
		},
		// This looks very wrong, but is historic behavior
		{
			name:         "override sh echo",
			args:         []string{"sh", "-c", `echo "hello there"`},
			noeval:       false,
			expectOutput: "hello",
		},
		// Docker/OCI behavior (with --no-eval)
		{
			name:         "no-eval/default",
			args:         []string{},
			noeval:       true,
			expectOutput: `CMD 'quotes' "quotes" $DOLLAR s p a c e s`,
		},
		{
			name:         "no-eval/override",
			args:         []string{"echo", "test"},
			noeval:       true,
			expectOutput: `test`,
		},
		{
			name:         "no-eval/override env var",
			noeval:       true,
			args:         []string{"echo", "$HOME"},
			expectOutput: "$HOME",
		},
		{
			name:         "no-eval/override sh echo",
			noeval:       true,
			args:         []string{"sh", "-c", `echo "hello there"`},
			expectOutput: "hello there",
		},
	}

	for _, tt := range tests {
		cmdArgs := []string{}
		if tt.noeval {
			cmdArgs = append(cmdArgs, "--no-eval")
		}
		cmdArgs = append(cmdArgs, imagePath)
		cmdArgs = append(cmdArgs, tt.args...)
		c.env.RunSingularity(
			t,
			e2e.AsSubtest(tt.name),
			e2e.WithProfile(e2e.UserProfile),
			e2e.WithCommand("run"),
			e2e.WithArgs(cmdArgs...),
			e2e.ExpectExit(0,
				e2e.ExpectOutput(e2e.ExactMatch, tt.expectOutput),
			),
		)
	}
}

//nolint:dupl
func (c ctx) testDockerENTRYPOINT(t *testing.T) {
	imageDir, cleanup := e2e.MakeTempDir(t, c.env.TestDir, "docker-", "")
	t.Cleanup(func() {
		if !t.Failed() {
			cleanup(t)
		}
	})
	imagePath := filepath.Join(imageDir, "container")

	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("while getting $HOME: %s", err)
	}

	c.env.RunSingularity(
		t,
		e2e.WithProfile(e2e.UserProfile),
		e2e.WithCommand("pull"),
		e2e.WithArgs(imagePath, "docker://sylabsio/docker-entrypoint"),
		e2e.ExpectExit(0),
	)

	tests := []struct {
		name         string
		args         []string
		noeval       bool
		expectOutput string
	}{
		// Singularity historic behavior (without --no-eval)
		// These do not all match Docker, due to evaluation, consumption of quoting.
		{
			name:         "default",
			args:         []string{},
			noeval:       false,
			expectOutput: `ENTRYPOINT 'quotes' "quotes" $DOLLAR s p a c e s`,
		},
		{
			name:         "override",
			args:         []string{"echo", "test"},
			noeval:       false,
			expectOutput: `ENTRYPOINT 'quotes' "quotes" $DOLLAR s p a c e s echo test`,
		},
		{
			name:         "override env var",
			args:         []string{"echo", "$HOME"},
			noeval:       false,
			expectOutput: `ENTRYPOINT 'quotes' "quotes" $DOLLAR s p a c e s echo ` + home,
		},
		// Docker/OCI behavior (with --no-eval)
		{
			name:         "no-eval/default",
			args:         []string{},
			noeval:       true,
			expectOutput: `ENTRYPOINT 'quotes' "quotes" $DOLLAR s p a c e s`,
		},
		{
			name:         "no-eval/override",
			args:         []string{"echo", "test"},
			noeval:       true,
			expectOutput: `ENTRYPOINT 'quotes' "quotes" $DOLLAR s p a c e s echo test`,
		},
		{
			name:         "no-eval/override env var",
			noeval:       true,
			args:         []string{"echo", "$HOME"},
			expectOutput: `ENTRYPOINT 'quotes' "quotes" $DOLLAR s p a c e s echo $HOME`,
		},
	}

	for _, tt := range tests {
		cmdArgs := []string{}
		if tt.noeval {
			cmdArgs = append(cmdArgs, "--no-eval")
		}
		cmdArgs = append(cmdArgs, imagePath)
		cmdArgs = append(cmdArgs, tt.args...)
		c.env.RunSingularity(
			t,
			e2e.AsSubtest(tt.name),
			e2e.WithProfile(e2e.UserProfile),
			e2e.WithCommand("run"),
			e2e.WithArgs(cmdArgs...),
			e2e.ExpectExit(0,
				e2e.ExpectOutput(e2e.ExactMatch, tt.expectOutput),
			),
		)
	}
}

//nolint:dupl
func (c ctx) testDockerCMDENTRYPOINT(t *testing.T) {
	imageDir, cleanup := e2e.MakeTempDir(t, c.env.TestDir, "docker-", "")
	t.Cleanup(func() {
		if !t.Failed() {
			cleanup(t)
		}
	})
	imagePath := filepath.Join(imageDir, "container")

	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("while getting $HOME: %s", err)
	}

	c.env.RunSingularity(
		t,
		e2e.WithProfile(e2e.UserProfile),
		e2e.WithCommand("pull"),
		e2e.WithArgs(imagePath, "docker://sylabsio/docker-cmd-entrypoint"),
		e2e.ExpectExit(0),
	)

	tests := []struct {
		name         string
		args         []string
		noeval       bool
		expectOutput string
	}{
		// Singularity historic behavior (without --no-eval)
		// These do not all match Docker, due to evaluation, consumption of quoting.
		{
			name:         "default",
			args:         []string{},
			noeval:       false,
			expectOutput: `ENTRYPOINT 'quotes' "quotes" $DOLLAR s p a c e s CMD 'quotes' "quotes" $DOLLAR s p a c e s`,
		},
		{
			name:         "override",
			args:         []string{"echo", "test"},
			noeval:       false,
			expectOutput: `ENTRYPOINT 'quotes' "quotes" $DOLLAR s p a c e s echo test`,
		},
		{
			name:         "override env var",
			args:         []string{"echo", "$HOME"},
			noeval:       false,
			expectOutput: `ENTRYPOINT 'quotes' "quotes" $DOLLAR s p a c e s echo ` + home,
		},
		// Docker/OCI behavior (with --no-eval)
		{
			name:         "no-eval/default",
			args:         []string{},
			noeval:       true,
			expectOutput: `ENTRYPOINT 'quotes' "quotes" $DOLLAR s p a c e s CMD 'quotes' "quotes" $DOLLAR s p a c e s`,
		},
		{
			name:         "no-eval/override",
			args:         []string{"echo", "test"},
			noeval:       true,
			expectOutput: `ENTRYPOINT 'quotes' "quotes" $DOLLAR s p a c e s echo test`,
		},
		{
			name:         "no-eval/override env var",
			noeval:       true,
			args:         []string{"echo", "$HOME"},
			expectOutput: `ENTRYPOINT 'quotes' "quotes" $DOLLAR s p a c e s echo $HOME`,
		},
	}

	for _, tt := range tests {
		cmdArgs := []string{}
		if tt.noeval {
			cmdArgs = append(cmdArgs, "--no-eval")
		}
		cmdArgs = append(cmdArgs, imagePath)
		cmdArgs = append(cmdArgs, tt.args...)
		c.env.RunSingularity(
			t,
			e2e.AsSubtest(tt.name),
			e2e.WithProfile(e2e.UserProfile),
			e2e.WithCommand("run"),
			e2e.WithArgs(cmdArgs...),
			e2e.ExpectExit(0,
				e2e.ExpectOutput(e2e.ExactMatch, tt.expectOutput),
			),
		)
	}
}

// https://github.com/sylabs/singularity/issues/233
// This tests quotes in the CMD shell form, not the [ .. ] exec form.
func (c ctx) testDockerCMDQuotes(t *testing.T) {
	c.env.RunSingularity(
		t,
		e2e.WithProfile(e2e.UserProfile),
		e2e.WithCommand("run"),
		e2e.WithArgs("docker://sylabsio/issue233"),
		e2e.ExpectExit(0,
			e2e.ExpectOutput(e2e.ContainMatch, "Test run"),
		),
	)
}

// Check that the USER & WORKDIR in a docker container are honored under --oci mode
func (c ctx) testDockerUSERWORKDIR(t *testing.T) {
	dockerURI := "docker://sylabsio/docker-user"
	dockerfile := filepath.Join("..", "test", "defs", "Dockerfile.customuser")
	tmpdir, tmpdirCleanup := e2e.MakeTempDir(t, "", "dockerfile-build-USER-", "temp dir for OCI-SIF images")
	t.Cleanup(func() {
		if !t.Failed() {
			tmpdirCleanup(t)
		}
	})
	userBuiltOCISIF := filepath.Join(tmpdir, "docker-user.oci.sif")
	c.env.RunSingularity(
		t,
		e2e.AsSubtest("user df build"),
		e2e.WithProfile(e2e.OCIUserProfile),
		e2e.WithCommand("build"),
		e2e.WithArgs(userBuiltOCISIF, dockerfile),
		e2e.ExpectExit(0),
	)
	rootBuiltOCISIF := filepath.Join(tmpdir, "rootbuilt-docker-user.oci.sif")
	c.env.RunSingularity(
		t,
		e2e.AsSubtest("root df build"),
		e2e.WithProfile(e2e.OCIRootProfile),
		e2e.WithCommand("build"),
		e2e.WithArgs(rootBuiltOCISIF, dockerfile),
		e2e.ExpectExit(0),
	)

	// Sanity check singularity native engine... no support for USER
	c.env.RunSingularity(
		t,
		e2e.AsSubtest("default"),
		e2e.WithProfile(e2e.UserProfile),
		e2e.WithCommand("run"),
		e2e.WithArgs(dockerURI),
		e2e.ExpectExit(0, e2e.ExpectOutput(e2e.ContainMatch, fmt.Sprintf("uid=%d(%s) gid=%d",
			e2e.UserProfile.ContainerUser(t).UID,
			e2e.UserProfile.ContainerUser(t).Name,
			e2e.UserProfile.ContainerUser(t).GID,
		))),
	)

	metaTests := map[string]string{
		"uri":          dockerURI,
		"user oci-sif": userBuiltOCISIF,
		"root oci-sif": rootBuiltOCISIF,
	}

	for subtestName, container := range metaTests {
		t.Run(subtestName, func(t *testing.T) {
			c.testDockerUSERWorker(t, container)
		})
	}
}

func (c ctx) testDockerUSERWorker(t *testing.T, container string) {
	tests := []struct {
		name          string
		cmd           string
		args          []string
		wd            string
		expectOutputs []e2e.SingularityCmdResultOp
		profiles      []e2e.Profile
		expectExit    int
	}{
		// `--oci` should honor container USER by default
		{
			name:     "OCIImageUser",
			cmd:      "run",
			profiles: []e2e.Profile{e2e.OCIUserProfile, e2e.OCIRootProfile},
			args:     []string{container},
			expectOutputs: []e2e.SingularityCmdResultOp{
				e2e.ExpectOutput(e2e.ContainMatch, `uid=2000(testuser) gid=2000(testgroup)`),
			},
		},
		// `--fakeroot` is an explicit request for root in the container
		{
			name:     "OCIFakerootUser",
			profiles: []e2e.Profile{e2e.OCIFakerootProfile},
			args:     []string{container},
			expectOutputs: []e2e.SingularityCmdResultOp{
				e2e.ExpectOutput(e2e.ContainMatch, `uid=0(root) gid=0(root)`),
			},
		},

		// At present, we don't support specifying `--home` when container declares a USER.
		{
			name:       "WithHomeOCIUser",
			cmd:        "run",
			profiles:   []e2e.Profile{e2e.OCIUserProfile, e2e.OCIRootProfile, e2e.OCIFakerootProfile},
			args:       []string{"--home", "/tmp", container},
			expectExit: 255,
		},
		// $HOME env var should match the container USER's home dir, by default.
		{
			name:     "OCIImageHomeEnv",
			cmd:      "exec",
			profiles: []e2e.Profile{e2e.OCIUserProfile, e2e.OCIRootProfile},
			args:     []string{container, "env"},
			expectOutputs: []e2e.SingularityCmdResultOp{
				e2e.ExpectOutput(e2e.RegexMatch, `\bHOME=/home/testuser\b`),
			},
			expectExit: 0,
		},
		// `--fakeroot` is an explicit request for root in the container, so verify home dir.
		{
			name:     "OCIFakerootHomeEnv",
			cmd:      "exec",
			profiles: []e2e.Profile{e2e.OCIFakerootProfile},
			args:     []string{container, "env"},
			expectOutputs: []e2e.SingularityCmdResultOp{
				e2e.ExpectOutput(e2e.RegexMatch, `\bHOME=/root\b`),
			},
			expectExit: 0,
		},
		// USER's home directory should always be owned by USER
		{
			name:     "OCIImageHomePerms",
			cmd:      "exec",
			profiles: []e2e.Profile{e2e.OCIUserProfile, e2e.OCIRootProfile, e2e.OCIFakerootProfile},
			args:     []string{container, "stat", "-c", "%U(%u):%G(%g)", "/home/testuser"},
			expectOutputs: []e2e.SingularityCmdResultOp{
				e2e.ExpectOutput(e2e.ExactMatch, "testuser(2000):testgroup(2000)"),
			},
			expectExit: 0,
		},
		// WORKDIR should be honored, by default.
		{
			name:     "OCIImageWorkdir",
			cmd:      "exec",
			profiles: []e2e.Profile{e2e.OCIUserProfile, e2e.OCIRootProfile, e2e.OCIFakerootProfile},
			args:     []string{container, "pwd"},
			expectOutputs: []e2e.SingularityCmdResultOp{
				e2e.ExpectOutput(e2e.ExactMatch, "/home/testuser"),
			},
			expectExit: 0,
		},
		// --no-compat emulates native mode, so WORKDIR is ignored and container is entered at host CWD.
		{
			name:     "OCINoCompatWorkdir",
			cmd:      "exec",
			profiles: []e2e.Profile{e2e.OCIUserProfile, e2e.OCIRootProfile, e2e.OCIFakerootProfile},
			args:     []string{"--no-compat", container, "pwd"},
			wd:       "/tmp",
			expectOutputs: []e2e.SingularityCmdResultOp{
				e2e.ExpectOutput(e2e.ExactMatch, "/tmp"),
			},
			expectExit: 0,
		},
	}

	for _, tt := range tests {
		for _, profile := range tt.profiles {
			cmd := "run"
			if tt.cmd != "" {
				cmd = tt.cmd
			}
			cmdOps := []e2e.SingularityCmdOp{
				e2e.AsSubtest(tt.name + "/" + profile.String()),
				e2e.WithProfile(profile),
				e2e.WithCommand(cmd),
				e2e.WithArgs(tt.args...),
				e2e.ExpectExit(tt.expectExit, tt.expectOutputs...),
			}
			if tt.wd != "" {
				cmdOps = append(cmdOps, e2e.WithDir(tt.wd))
			}
			c.env.RunSingularity(
				t,
				cmdOps...,
			)
		}
	}
}

// Test that we can pull for different --platforms
func (c ctx) testDockerPlatform(t *testing.T) {
	tmpPath, err := fs.MakeTmpDir(c.env.TestDir, "docker-platform-", 0o755)
	if err != nil {
		t.Fatalf("failed to create temporary directory: %v", err)
	}
	t.Cleanup(func() {
		if !t.Failed() {
			os.RemoveAll(tmpPath)
		}
	})
	tmpSIF := filepath.Join(tmpPath, "test.sif")

	tests := []struct {
		name     string
		platform string
		uri      string
		exit     int
	}{
		{
			name:     "MultiArchArm64",
			platform: "linux/arm64/v8",
			uri:      "docker://alpine:latest",
			exit:     0,
		},
		{
			name:     "MultiArchPpc64le",
			platform: "linux/ppc64le",
			uri:      "docker://alpine:latest",
			exit:     0,
		},
		{
			name:     "MultiArchInvalidPlatform",
			platform: "windows/m68k",
			uri:      "docker://alpine:latest",
			exit:     255,
		},
		{
			name:     "SingleArchArm64",
			platform: "linux/arm64/v8",
			uri:      "docker://arm64v8/alpine:latest",
			exit:     0,
		},
		{
			name:     "SingleArchPpc64le",
			platform: "linux/ppc64le",
			uri:      "docker://ppc64le/alpine:latest",
			exit:     0,
		},
		{
			name:     "SingleArchInvalidPlatform",
			platform: "windows/m68k",
			uri:      "docker://ppc64le/alpine:latest",
			exit:     255,
		},
		{
			name:     "SingleArchMissingPlatform",
			platform: "linux/arm64",
			uri:      "docker://ppc64le/alpine:latest",
			exit:     255,
		},
	}

	for _, p := range []e2e.Profile{e2e.UserProfile, e2e.OCIUserProfile} {
		for _, tt := range tests {
			c.env.RunSingularity(
				t,
				e2e.AsSubtest(p.String()+"/"+tt.name),
				e2e.WithProfile(p),
				e2e.WithCommand("pull"),
				e2e.WithArgs("--force", "--platform", tt.platform, tmpSIF, tt.uri),
				e2e.ExpectExit(tt.exit),
				e2e.PostRun(func(t *testing.T) {
					if t.Failed() || tt.exit != 0 {
						return
					}
					if p.OCI() {
						checkOCISIFPlatform(t, tmpSIF, tt.platform)
					} else {
						checkNativeSIFPlatform(t, tmpSIF, tt.platform)
					}
				}),
			)
		}
	}
}

func checkOCISIFPlatform(t *testing.T, imgPath, platform string) {
	wantPlatform, err := v1.ParsePlatform(platform)
	if err != nil {
		t.Errorf("while parsing platform %v", err)
	}

	fi, err := sif.LoadContainerFromPath(imgPath, sif.OptLoadWithFlag(os.O_RDONLY))
	defer fi.UnloadContainer()
	if err != nil {
		t.Errorf("while loading SIF: %v", err)
	}

	ix, err := ocisif.ImageIndexFromFileImage(fi)
	if err != nil {
		t.Errorf("while obtaining image index: %v", err)
	}
	idxManifest, err := ix.IndexManifest()
	if err != nil {
		t.Errorf("while obtaining index manifest: %v", err)
	}
	if len(idxManifest.Manifests) != 1 {
		t.Errorf("image has multiple manifests")
	}
	imageDigest := idxManifest.Manifests[0].Digest
	img, err := ix.Image(imageDigest)
	if err != nil {
		t.Errorf("while initializing image: %v", err)
	}
	cfg, err := img.ConfigFile()
	if err != nil {
		t.Errorf("while fetching image config: %v", err)
	}
	if !cfg.Platform().Equals(*wantPlatform) {
		t.Errorf("wrong platform - wanted %q, got %q", wantPlatform.String(), cfg.Platform().String())
	}
}

func checkNativeSIFPlatform(t *testing.T, imgPath, platform string) {
	wantPlatform, err := v1.ParsePlatform(platform)
	if err != nil {
		t.Errorf("while parsing platform %v", err)
	}

	fi, err := sif.LoadContainerFromPath(imgPath, sif.OptLoadWithFlag(os.O_RDONLY))
	defer fi.UnloadContainer()
	if err != nil {
		t.Errorf("while loading SIF: %v", err)
	}
	d, err := fi.GetDescriptor(sif.WithPartitionType(sif.PartPrimSys))
	if err != nil {
		t.Errorf("while getting primary partition: %v", err)
	}
	_, _, arch, _ := d.PartitionMetadata() //nolint:dogsled
	if arch != wantPlatform.Architecture {
		t.Errorf("wrong architecture - wanted %q, got %q", wantPlatform.Architecture, arch)
	}
}

// Test that we can perform cross-architecture builds from Dockerfile using buildkit
func (c ctx) testDockerCrossArchBk(t *testing.T) {
	tmpdir, tmpdirCleanup := e2e.MakeTempDir(t, "", "dockerfile_crossarch_", "dir")
	t.Cleanup(func() {
		if !t.Failed() {
			tmpdirCleanup(t)
		}
	})

	dockerfile, err := e2e.WriteTempFile(tmpdir, "Dockerfile", `
FROM alpine
CMD /bin/true
`)
	if err != nil {
		t.Fatalf("While trying to create temporary Dockerfile: %v", err)
	}

	arch := getNonNativeArch()
	profiles := []e2e.Profile{e2e.OCIUserProfile, e2e.OCIRootProfile}
	for _, profile := range profiles {
		imgPath := filepath.Join(tmpdir, "image."+profile.String()+".oci.sif")
		c.env.RunSingularity(
			t,
			e2e.AsSubtest(profile.String()),
			e2e.WithProfile(profile),
			e2e.WithCommand("build"),
			e2e.WithArgs("--arch", arch, imgPath, dockerfile),
			e2e.ExpectExit(0),
			e2e.PostRun(func(t *testing.T) {
				verifyImgArch(t, imgPath, arch)
			}),
		)
	}
}

func getNonNativeArch() string {
	nativeArch := runtime.GOARCH
	switch nativeArch {
	case "amd64":
		return "arm64"
	default:
		return "amd64"
	}
}

func verifyImgArch(t *testing.T, imgPath, arch string) {
	fi, err := sif.LoadContainerFromPath(imgPath, sif.OptLoadWithFlag(os.O_RDONLY))
	if err != nil {
		t.Fatalf("while loading SIF (%s): %v", imgPath, err)
	}
	defer fi.UnloadContainer()

	ix, err := ocisif.ImageIndexFromFileImage(fi)
	if err != nil {
		t.Fatalf("while obtaining image index from %s: %v", imgPath, err)
	}
	idxManifest, err := ix.IndexManifest()
	if err != nil {
		t.Fatalf("while obtaining index manifest from %s: %v", imgPath, err)
	}
	if len(idxManifest.Manifests) != 1 {
		t.Fatalf("while reading %s: single manifest expected, found %d manifests", imgPath, len(idxManifest.Manifests))
	}
	imageDigest := idxManifest.Manifests[0].Digest

	img, err := ix.Image(imageDigest)
	if err != nil {
		t.Fatalf("while initializing image from %s: %v", imgPath, err)
	}

	cg, err := img.ConfigFile()
	if err != nil {
		t.Fatalf("while accessing config for %s: %v", imgPath, err)
	}

	assert.Equal(t, arch, cg.Architecture)
}

// Test support for SCIF containers in OCI mode
func (c ctx) testDockerSCIF(t *testing.T) {
	tmpdir, tmpdirCleanup := e2e.MakeTempDir(t, "", "docker-scif-", "dir")
	t.Cleanup(func() {
		if !t.Failed() {
			tmpdirCleanup(t)
		}
	})

	scifRecipeFilename := "local_scif_recipe"
	scifRecipeFullpath := filepath.Join(tmpdir, scifRecipeFilename)
	scifRecipeSource := filepath.Join("..", "test", "defs", "scif_recipe")
	if err := fs.CopyFile(scifRecipeSource, scifRecipeFullpath, 0o755); err != nil {
		t.Fatalf("While trying to copy %q to %q: %v", scifRecipeSource, scifRecipeFullpath, err)
	}

	tmplValues := struct{ SCIFRecipeFilename string }{SCIFRecipeFilename: scifRecipeFilename}
	scifDockerfile := tmpl.Execute(t, tmpdir, "Dockerfile-", filepath.Join("..", "test", "defs", "Dockerfile.scif.tmpl"), tmplValues)
	scifImageFilename := "scif-image.oci.sif"
	scifImageFullpath := filepath.Join(tmpdir, scifImageFilename)

	// Uncomment when `singularity inspect --oci` for Docker-style SCIF
	// containers is enabled.
	// See: https://github.com/sylabs/singularity/pull/2360
	// scifInspectOutAllPath := filepath.Join("..", "test", "defs", "scif_recipe.inspect_output.all")
	// scifInspectOutAllBytes, err := os.ReadFile(scifInspectOutAllPath)
	// if err != nil {
	// 	t.Fatalf("While trying to read contents of %s: %v", scifInspectOutAllPath, err)
	// }
	// scifInspectOutOnePath := filepath.Join("..", "test", "defs", "scif_recipe.inspect_output.one")
	// scifInspectOutOneBytes, err := os.ReadFile(scifInspectOutOnePath)
	// if err != nil {
	// 	t.Fatalf("While trying to read contents of %s: %v", scifInspectOutOnePath, err)
	// }

	// testInspectOutput := func(bytes []byte) func(t *testing.T, r *e2e.SingularityCmdResult) {
	// 	return func(t *testing.T, r *e2e.SingularityCmdResult) {
	// 		got := string(r.Stdout)
	// 		assert.Equal(t, got, string(bytes))
	// 	}
	// }

	c.env.RunSingularity(
		t,
		e2e.AsSubtest("build"),
		e2e.WithProfile(e2e.OCIUserProfile),
		e2e.WithCommand("build"),
		e2e.WithDir(tmpdir),
		e2e.WithArgs(scifImageFilename, scifDockerfile),
		e2e.ExpectExit(0),
	)

	tests := []struct {
		name       string
		cmd        string
		app        string
		preArgs    []string
		args       []string
		expects    []e2e.SingularityCmdResultOp
		expectExit int
	}{
		{
			name: "run echo",
			cmd:  "run",
			app:  "hello-world-echo",
			expects: []e2e.SingularityCmdResultOp{
				e2e.ExpectOutput(e2e.ContainMatch, "The best app is hello-world-echo"),
			},
			expectExit: 0,
		},
		{
			name: "exec echo",
			cmd:  "exec",
			app:  "hello-world-echo",
			args: []string{"echo", "This is different text that should still include [e]SCIF_APPNAME"},
			expects: []e2e.SingularityCmdResultOp{
				e2e.ExpectOutput(e2e.ContainMatch, "This is different text that should still include hello-world-echo"),
			},
			expectExit: 0,
		},
		{
			name: "run script",
			cmd:  "run",
			app:  "hello-world-script",
			expects: []e2e.SingularityCmdResultOp{
				e2e.ExpectOutput(e2e.ContainMatch, "Hello World!"),
			},
			expectExit: 0,
		},
		{
			name: "exec script",
			cmd:  "exec",
			app:  "hello-world-script",
			args: []string{"echo", "This is different text that should still include [e]SCIF_APPNAME"},
			expects: []e2e.SingularityCmdResultOp{
				e2e.ExpectOutput(e2e.ContainMatch, "This is different text that should still include hello-world-script"),
			},
			expectExit: 0,
		},
		{
			name: "exec script2",
			cmd:  "exec",
			app:  "hello-world-script",
			args: []string{"/bin/bash hello-world.sh"},
			expects: []e2e.SingularityCmdResultOp{
				e2e.ExpectOutput(e2e.ContainMatch, "Hello World!"),
			},
			expectExit: 0,
		},
		// Uncomment when `singularity inspect --oci` for Docker-style SCIF
		// containers is enabled.
		// See: https://github.com/sylabs/singularity/pull/2360
		// {
		//  name:    "insp all",
		//  cmd:     "inspect",
		//  preArgs: []string{"--oci", "--list-apps"},
		//  expects: []e2e.SingularityCmdResultOp{
		//      testInspectOutput(scifInspectOutAllBytes),
		//  },
		//  expectExit: 0,
		// }, {
		//  name:    "insp one",
		//  cmd:     "inspect",
		//  app:     "hello-world-script",
		//  preArgs: []string{"--oci"},
		//  expects: []e2e.SingularityCmdResultOp{
		//      testInspectOutput(scifInspectOutOneBytes),
		//  },
		//  expectExit: 0,
		// },
	}

	for _, tt := range tests {
		args := tt.preArgs[:]
		if tt.app != "" {
			args = append(args, "--app", tt.app)
		}
		args = append(args, scifImageFullpath)
		if len(tt.args) > 0 {
			args = append(args, tt.args...)
		}

		c.env.RunSingularity(
			t,
			e2e.AsSubtest(tt.name),
			e2e.WithProfile(e2e.OCIUserProfile),
			e2e.WithCommand(tt.cmd),
			e2e.WithArgs(args...),
			e2e.ExpectExit(0, tt.expects...),
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
		// Run most docker:// source tests sequentially amongst themselves, so we
		// don't hit DockerHub massively in parallel, and we benefit from
		// caching as the same images are used frequently.
		"ordered": func(t *testing.T) {
			t.Run("AUFS", c.testDockerAUFS)
			t.Run("def file", c.testDockerDefFile)
			t.Run("permissions", c.testDockerPermissions)
			t.Run("pulls", c.testDockerPulls)
			t.Run("whiteout symlink", c.testDockerWhiteoutSymlink)
			t.Run("labels", c.testDockerLabels)
			t.Run("cmd", c.testDockerCMD)
			t.Run("entrypoint", c.testDockerENTRYPOINT)
			t.Run("cmdentrypoint", c.testDockerCMDENTRYPOINT)
			t.Run("cmd quotes", c.testDockerCMDQuotes)
			t.Run("user workdir", c.testDockerUSERWORKDIR)
			t.Run("platform", c.testDockerPlatform)
			t.Run("crossarch buildkit", c.testDockerCrossArchBk)
			t.Run("scif", c.testDockerSCIF)
			// Regressions
			t.Run("issue 4524", c.issue4524)
			t.Run("issue 1286", c.issue1286)
			t.Run("issue 1528", c.issue1528)
			t.Run("issue 1586", c.issue1586)
			t.Run("issue 1670", c.issue1670)
		},
		// Tests that are especially slow, or run against a local docker
		// registry, can be run in parallel, with `--disable-cache` used within
		// them to avoid docker caching concurrency issues.
		"docker host": c.testDockerHost,
		"cred prio":   np(c.testDockerCredsPriority),
		"registry":    c.testDockerRegistry,
		// Regressions
		"issue 4943": c.issue4943,
		"issue 5172": c.issue5172,
		"issue 274":  c.issue274,  // https://github.com/sylabs/singularity/issues/274
		"issue 1704": c.issue1704, // https://github.com/sylabs/singularity/issues/1704
	}
}
