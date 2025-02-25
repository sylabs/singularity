// Copyright (c) 2025, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package nested

import (
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/sylabs/singularity/v4/e2e/internal/e2e"
	"github.com/sylabs/singularity/v4/e2e/internal/testhelper"
	"github.com/sylabs/singularity/v4/internal/pkg/buildcfg"
	"github.com/sylabs/singularity/v4/internal/pkg/test/tool/require"
)

type ctx struct {
	env e2e.TestEnv
}

//nolint:dupl
func (c ctx) docker(t *testing.T) {
	require.Command(t, "docker")
	e2e.EnsureORASImage(t, c.env)
	e2e.EnsureRegistryOCISIF(t, c.env)

	// Temporary homedir for docker commands, so invoking docker doesn't create
	// a ~/.docker that may interfere elsewhere.
	tmpHome, cleanupHome := e2e.MakeTempDir(t, c.env.TestDir, "nested-docker-", "")
	t.Cleanup(func() { e2e.Privileged(cleanupHome)(t) })

	dockerFile := "testdata/Dockerfile.nested"
	dockerRef := "singularity-e2e-docker-nested"
	dockerBuild(t, dockerFile, dockerRef, "../", tmpHome)
	defer dockerRMI(t, dockerRef, tmpHome)

	// Execution using privileged Docker. Root in outer Docker container.
	dockerRunPrivileged(t, "version", dockerRef, tmpHome)
	dockerRunPrivileged(t, "exec", dockerRef, tmpHome, "exec", c.env.OrasTestImage, "/bin/true")
	dockerRunPrivileged(t, "execUserNS", dockerRef, tmpHome, "exec", "-u", c.env.OrasTestImage, "/bin/true")
	dockerRunPrivileged(t, "execOCI", dockerRef, tmpHome, "exec", "--oci", c.env.TestRegistryOCISIF, "/bin/true")
	dockerRunPrivileged(t, "buildSIF", dockerRef, tmpHome, "build", "test.sif", "singularity-src/examples/library/Singularity")
}

func dockerBuild(t *testing.T, dockerFile, dockerRef, contextPath, homeDir string) {
	t.Run("build/"+dockerRef, e2e.Privileged(func(t *testing.T) {
		cmd := exec.Command("docker", "build",
			"--build-arg", "GOVERSION="+runtime.Version(),
			"--build-arg", "GOOS="+runtime.GOOS,
			"--build-arg", "GOARCH="+runtime.GOARCH,
			"-t", dockerRef, "-f", dockerFile, contextPath)
		cmd.Env = append(cmd.Env, "HOME="+homeDir)
		out, err := cmd.CombinedOutput()
		t.Log(cmd.Args)
		if err != nil {
			t.Fatalf("Failed building docker container.\n%s: %s", err, string(out))
		}
	}))
}

func dockerRMI(t *testing.T, dockerRef, homeDir string) {
	t.Run("rmi/"+dockerRef, e2e.Privileged(func(t *testing.T) {
		cmd := exec.Command("docker", "rmi", dockerRef)
		cmd.Env = append(cmd.Env, "HOME="+homeDir)
		out, err := cmd.CombinedOutput()
		t.Log(cmd.Args)
		if err != nil {
			t.Fatalf("Failed removing docker container.\n%s: %s", err, string(out))
		}
	}))
}

func dockerRunPrivileged(t *testing.T, name, dockerRef, homeDir string, args ...string) { //nolint:unparam
	t.Run(name, e2e.Privileged(func(t *testing.T) {
		cmdArgs := []string{"run", "-i", "--rm", "--privileged", "--network=host", dockerRef}
		cmdArgs = append(cmdArgs, args...)
		cmd := exec.Command("docker", cmdArgs...)
		cmd.Env = append(cmd.Env, "HOME="+homeDir)
		out, err := cmd.CombinedOutput()
		t.Log(cmd.Args)
		if err != nil {
			t.Fatalf("Failed running docker container.\n%s: %s", err, string(out))
		}
	}))
}

//nolint:dupl
func (c ctx) podman(t *testing.T) {
	require.Command(t, "podman")
	e2e.EnsureORASImage(t, c.env)
	e2e.EnsureRegistryOCISIF(t, c.env)

	// Temporary homedir for docker commands, so invoking docker doesn't create
	// a ~/.docker that may interfere elsewhere.
	tmpHome, cleanupHome := e2e.MakeTempDir(t, c.env.TestDir, "nested-docker-", "")
	t.Cleanup(func() { e2e.Privileged(cleanupHome)(t) })

	dockerFile := "testdata/Dockerfile.nested"
	dockerRef := "localhost/singularity-e2e-podman-nested"
	podmanBuild(t, dockerFile, dockerRef, "../", tmpHome)
	defer podmanRMI(t, dockerRef, tmpHome)

	// Rootless podman - fake userns root in outer podman container.
	podmanRun(t, "version", dockerRef, tmpHome)
	podmanRun(t, "exec", dockerRef, tmpHome, "exec", c.env.OrasTestImage, "/bin/true")
	podmanRun(t, "execUserNS", dockerRef, tmpHome, "exec", "-u", c.env.OrasTestImage, "/bin/true")
	podmanRun(t, "execOCI", dockerRef, tmpHome, "exec", "--oci", c.env.TestRegistryOCISIF, "/bin/true")
	podmanRun(t, "buildSIF", dockerRef, tmpHome, "build", "test.sif", "singularity-src/examples/library/Singularity")
}

func podmanBuild(t *testing.T, dockerFile, dockerRef, contextPath, homeDir string) {
	t.Run("build/"+dockerRef, func(t *testing.T) {
		cmd := exec.Command("podman", "build",
			"--build-arg", "GOVERSION="+runtime.Version(),
			"--build-arg", "GOOS="+runtime.GOOS,
			"--build-arg", "GOARCH="+runtime.GOARCH,
			"-t", dockerRef, "-f", dockerFile, contextPath)
		cmd.Env = append(cmd.Env, "HOME="+homeDir)
		out, err := cmd.CombinedOutput()
		t.Log(cmd.Args)
		if err != nil {
			t.Fatalf("Failed building podman container.\n%s: %s", err, string(out))
		}
	})
}

func podmanRMI(t *testing.T, dockerRef, homeDir string) {
	t.Run("rmi/"+dockerRef, func(t *testing.T) {
		cmd := exec.Command("podman", "rmi", dockerRef)
		cmd.Env = append(cmd.Env, "HOME="+homeDir)
		out, err := cmd.CombinedOutput()
		t.Log(cmd.Args)
		if err != nil {
			t.Fatalf("Failed removing podman container.\n%s: %s", err, string(out))
		}
	})
}

func podmanRun(t *testing.T, name, dockerRef, homeDir string, args ...string) { //nolint:unparam
	t.Run(name, func(t *testing.T) {
		cmdArgs := []string{"run", "-i", "--rm", "--privileged", "--network=host", dockerRef}
		cmdArgs = append(cmdArgs, args...)
		cmd := exec.Command("podman", cmdArgs...)
		cmd.Env = append(cmd.Env, "HOME="+homeDir)
		out, err := cmd.CombinedOutput()
		t.Log(cmd.Args)
		if err != nil {
			t.Fatalf("Failed running podman container.\n%s: %s", err, string(out))
		}
	})
}

func (c ctx) singularity(t *testing.T) {
	e2e.EnsureORASImage(t, c.env)
	e2e.EnsureRegistryOCISIF(t, c.env)

	nestedDef := "e2e/testdata/nested.def"
	nestedSIF := filepath.Join(t.TempDir(), "nested.sif")
	c.env.RunSingularity(
		t,
		e2e.AsSubtest("build"),
		e2e.WithDir(buildcfg.SOURCEDIR),
		e2e.WithProfile(e2e.RootProfile),
		e2e.WithCommand("build"),
		e2e.WithArgs(
			"--build-arg", "GOVERSION="+runtime.Version(),
			"--build-arg", "GOOS="+runtime.GOOS,
			"--build-arg", "GOARCH="+runtime.GOARCH,
			nestedSIF, nestedDef,
		),
		e2e.ExpectExit(0),
	)

	tmpBuildSIF := filepath.Join(t.TempDir(), "build.sif")

	tests := []struct {
		name         string
		outerProfile e2e.Profile
		outerArgs    []string
		innerCommand string
		innerArgs    []string
	}{
		// Root host -> root outer -> root inner
		{
			name:         "root/exec",
			outerProfile: e2e.RootProfile,
			outerArgs:    []string{},
			innerCommand: "exec",
			innerArgs:    []string{c.env.OrasTestImage, "/bin/true"},
		},
		{
			name:         "root/build",
			outerProfile: e2e.FakerootProfile,
			outerArgs:    []string{},
			innerCommand: "build",
			innerArgs:    []string{"--force", tmpBuildSIF, "examples/library/Singularity"},
		},
		// User host -> fakeroot outer -> fakeroot inner
		{
			name:         "fakeroot/exec",
			outerProfile: e2e.FakerootProfile,
			outerArgs:    []string{},
			innerCommand: "exec",
			innerArgs:    []string{c.env.OrasTestImage, "/bin/true"},
		},
		{
			name:         "fakeroot/build",
			outerProfile: e2e.FakerootProfile,
			outerArgs:    []string{},
			innerCommand: "build",
			innerArgs:    []string{"--force", tmpBuildSIF, "examples/library/Singularity"},
		},
		// User outside -> user (userns) outer -> user (userns) inner
		{
			name:         "userns/exec",
			outerProfile: e2e.UserNamespaceProfile,
			outerArgs:    []string{},
			innerCommand: "exec",
			innerArgs:    []string{"-u", c.env.OrasTestImage, "/bin/true"},
		},
		// OCI-Mode: Root host -> root outer -> root inner
		{
			name:         "rootOCI/exec",
			outerProfile: e2e.OCIRootProfile,
			outerArgs:    []string{"--keep-privs"},
			innerCommand: "exec",
			innerArgs:    []string{c.env.OrasTestImage, "/bin/true"},
		},
		// OCI-Mode: User host -> fakeroot outer -> fakeroot inner
		{
			name:         "fakerootOCI/exec",
			outerProfile: e2e.OCIFakerootProfile,
			outerArgs:    []string{"--keep-privs"},
			innerCommand: "exec",
			innerArgs:    []string{c.env.OrasTestImage, "/bin/true"},
		},
	}
	for _, tt := range tests {
		cmdArgs := tt.outerArgs
		cmdArgs = append(cmdArgs, nestedSIF)
		cmdArgs = append(cmdArgs, tt.innerCommand)
		cmdArgs = append(cmdArgs, tt.innerArgs...)
		c.env.RunSingularity(
			t,
			e2e.AsSubtest(tt.name),
			e2e.WithDir(buildcfg.SOURCEDIR),
			e2e.WithProfile(e2e.RootProfile),
			e2e.WithCommand("run"),
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

	return testhelper.Tests{
		"Docker":      c.docker,
		"Podman":      c.podman,
		"Singularity": c.singularity,
	}
}
