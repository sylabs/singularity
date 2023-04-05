// Copyright (c) 2022, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package actions

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/sylabs/singularity/e2e/internal/e2e"
	"github.com/sylabs/singularity/internal/pkg/util/fs"
)

func (c actionTests) actionOciRun(t *testing.T) {
	e2e.EnsureOCIArchive(t, c.env)
	e2e.EnsureDockerArchive(t, c.env)

	// Prepare oci source (oci directory layout)
	ociLayout := t.TempDir()
	cmd := exec.Command("tar", "-C", ociLayout, "-xf", c.env.OCIArchivePath)
	err := cmd.Run()
	if err != nil {
		t.Fatalf("Error extracting oci archive to layout: %v", err)
	}

	tests := []struct {
		name     string
		imageRef string
		argv     []string
		exit     int
	}{
		{
			name:     "docker-archive",
			imageRef: "docker-archive:" + c.env.DockerArchivePath,
			exit:     0,
		},
		{
			name:     "oci-archive",
			imageRef: "oci-archive:" + c.env.OCIArchivePath,
			exit:     0,
		},
		{
			name:     "oci",
			imageRef: "oci:" + ociLayout,
			exit:     0,
		},
		{
			name:     "true",
			imageRef: "oci:" + ociLayout,
			argv:     []string{"true"},
			exit:     0,
		},
		{
			name:     "false",
			imageRef: "oci:" + ociLayout,
			argv:     []string{"false"},
			exit:     1,
		},
	}

	for _, profile := range e2e.OCIProfiles {
		t.Run(profile.String(), func(t *testing.T) {
			for _, tt := range tests {
				cmdArgs := []string{tt.imageRef}
				cmdArgs = append(cmdArgs, tt.argv...)
				c.env.RunSingularity(
					t,
					e2e.AsSubtest(tt.name),
					e2e.WithProfile(profile),
					e2e.WithCommand("run"),
					// While we don't support args we are entering a /bin/sh interactively.
					e2e.ConsoleRun(e2e.ConsoleSendLine("exit")),
					e2e.WithArgs(cmdArgs...),
					e2e.ExpectExit(tt.exit),
				)
			}
		})
	}
}

// exec tests min fuctionality for singularity exec
func (c actionTests) actionOciExec(t *testing.T) {
	e2e.EnsureOCIArchive(t, c.env)

	imageRef := "oci-archive:" + c.env.OCIArchivePath

	tests := []struct {
		name string
		argv []string
		exit int
	}{
		{
			name: "NoCommand",
			argv: []string{imageRef},
			exit: 1,
		},
		{
			name: "True",
			argv: []string{imageRef, "true"},
			exit: 0,
		},
		{
			name: "TrueAbsPAth",
			argv: []string{imageRef, "/bin/true"},
			exit: 0,
		},
		{
			name: "False",
			argv: []string{imageRef, "false"},
			exit: 1,
		},
		{
			name: "FalseAbsPath",
			argv: []string{imageRef, "/bin/false"},
			exit: 1,
		},
		{
			name: "TouchTmp",
			argv: []string{imageRef, "/bin/touch", "/tmp/test"},
			exit: 0,
		},
		{
			name: "TouchVarTmp",
			argv: []string{imageRef, "/bin/touch", "/var/tmp/test"},
			exit: 0,
		},
		{
			name: "TouchHome",
			argv: []string{imageRef, "/bin/sh", "-c", "touch $HOME"},
			exit: 0,
		},
		{
			name: "UTSNamespace",
			argv: []string{"--uts", imageRef, "true"},
			exit: 0,
		},
	}
	for _, profile := range e2e.OCIProfiles {
		t.Run(profile.String(), func(t *testing.T) {
			for _, tt := range tests {
				c.env.RunSingularity(
					t,
					e2e.AsSubtest(tt.name),
					e2e.WithProfile(e2e.UserProfile),
					e2e.WithCommand("exec"),
					e2e.WithDir("/tmp"),
					e2e.WithArgs(tt.argv...),
					e2e.ExpectExit(tt.exit),
				)
			}
		})
	}
}

// Shell interaction tests
func (c actionTests) actionOciShell(t *testing.T) {
	e2e.EnsureOCIArchive(t, c.env)

	tests := []struct {
		name       string
		argv       []string
		consoleOps []e2e.SingularityConsoleOp
		exit       int
	}{
		{
			name: "ShellExit",
			argv: []string{"oci-archive:" + c.env.OCIArchivePath},
			consoleOps: []e2e.SingularityConsoleOp{
				// "cd /" to work around issue where a long
				// working directory name causes the test
				// to fail because the "Singularity" that
				// we are looking for is chopped from the
				// front.
				// TODO(mem): This test was added back in 491a71716013654acb2276e4b37c2e015d2dfe09
				e2e.ConsoleSendLine("cd /"),
				e2e.ConsoleExpect("Singularity"),
				e2e.ConsoleSendLine("exit"),
			},
			exit: 0,
		},
		{
			name: "ShellBadCommand",
			argv: []string{"oci-archive:" + c.env.OCIArchivePath},
			consoleOps: []e2e.SingularityConsoleOp{
				e2e.ConsoleSendLine("_a_fake_command"),
				e2e.ConsoleSendLine("exit"),
			},
			exit: 127,
		},
	}

	for _, profile := range e2e.OCIProfiles {
		t.Run(profile.String(), func(t *testing.T) {
			for _, tt := range tests {
				c.env.RunSingularity(
					t,
					e2e.AsSubtest(tt.name),
					e2e.WithProfile(profile),
					e2e.WithCommand("shell"),
					e2e.WithArgs(tt.argv...),
					e2e.ConsoleRun(tt.consoleOps...),
					e2e.ExpectExit(tt.exit),
				)
			}
		})
	}
}

func (c actionTests) actionOciNetwork(t *testing.T) {
	e2e.EnsureOCIArchive(t, c.env)
	imageRef := "oci-archive:" + c.env.OCIArchivePath

	tests := []struct {
		name       string
		profile    e2e.Profile
		netType    string
		expectExit int
	}{
		{
			name:       "InvalidNetworkRoot",
			profile:    e2e.OCIRootProfile,
			netType:    "bridge",
			expectExit: 255,
		},
		{
			name:       "InvalidNetworkUser",
			profile:    e2e.OCIUserProfile,
			netType:    "bridge",
			expectExit: 255,
		},
		{
			name:       "InvalidNetworkFakeroot",
			profile:    e2e.OCIFakerootProfile,
			netType:    "bridge",
			expectExit: 255,
		},
		{
			name:       "NoneNetworkRoot",
			profile:    e2e.OCIRootProfile,
			netType:    "none",
			expectExit: 0,
		},
		{
			name:       "NoneNetworkUser",
			profile:    e2e.OCIUserProfile,
			netType:    "none",
			expectExit: 0,
		},
		{
			name:       "NoneNetworkFakeRoot",
			profile:    e2e.OCIFakerootProfile,
			netType:    "none",
			expectExit: 0,
		},
	}

	for _, tt := range tests {
		c.env.RunSingularity(
			t,
			e2e.AsSubtest(tt.name),
			e2e.WithProfile(tt.profile),
			e2e.WithCommand("exec"),
			e2e.WithArgs("--net", "--network", tt.netType, imageRef, "id"),
			e2e.ExpectExit(tt.expectExit),
		)
	}
}

//nolint:maintidx
func (c actionTests) actionOciBinds(t *testing.T) {
	e2e.EnsureOCIArchive(t, c.env)
	imageRef := "oci-archive:" + c.env.OCIArchivePath

	workspace, cleanup := e2e.MakeTempDir(t, c.env.TestDir, "bind-workspace-", "")
	defer e2e.Privileged(cleanup)

	contCanaryDir := "/canary"
	hostCanaryDir := filepath.Join(workspace, "canary")

	contCanaryFile := "/canary/file"
	hostCanaryFile := filepath.Join(hostCanaryDir, "file")

	canaryFileBind := hostCanaryFile + ":" + contCanaryFile
	canaryFileMount := "type=bind,source=" + hostCanaryFile + ",destination=" + contCanaryFile
	canaryDirBind := hostCanaryDir + ":" + contCanaryDir
	canaryDirMount := "type=bind,source=" + hostCanaryDir + ",destination=" + contCanaryDir

	createWorkspaceDirs := func(t *testing.T) {
		e2e.Privileged(func(t *testing.T) {
			if err := os.RemoveAll(hostCanaryDir); err != nil && !os.IsNotExist(err) {
				t.Fatalf("failed to delete canary_dir: %s", err)
			}
		})(t)

		if err := fs.Mkdir(hostCanaryDir, 0o777); err != nil {
			t.Fatalf("failed to create canary_dir: %s", err)
		}
		if err := fs.Touch(hostCanaryFile); err != nil {
			t.Fatalf("failed to create canary_file: %s", err)
		}
		if err := os.Chmod(hostCanaryFile, 0o777); err != nil {
			t.Fatalf("failed to apply permissions on canary_file: %s", err)
		}
	}

	checkHostFn := func(path string, fn func(string) bool) func(*testing.T) {
		return func(t *testing.T) {
			if t.Failed() {
				return
			}
			if !fn(path) {
				t.Errorf("%s not found on host", path)
			}
			if err := os.RemoveAll(path); err != nil {
				t.Errorf("failed to delete %s: %s", path, err)
			}
		}
	}
	checkHostFile := func(path string) func(*testing.T) {
		return checkHostFn(path, fs.IsFile)
	}
	checkHostDir := func(path string) func(*testing.T) {
		return checkHostFn(path, fs.IsDir)
	}

	tests := []struct {
		name    string
		args    []string
		postRun func(*testing.T)
		exit    int
	}{
		{
			name: "NonExistentSource",
			args: []string{
				"--bind", "/non/existent/source/path",
				imageRef,
				"true",
			},
			exit: 255,
		},
		{
			name: "RelativeBindDestination",
			args: []string{
				"--bind", hostCanaryFile + ":relative",
				imageRef,
				"true",
			},
			exit: 255,
		},
		{
			name: "SimpleFile",
			args: []string{
				"--bind", canaryFileBind,
				imageRef,
				"test", "-f", contCanaryFile,
			},
			exit: 0,
		},
		{
			name: "SimpleDir",
			args: []string{
				"--bind", canaryDirBind,
				imageRef,
				"test", "-f", contCanaryFile,
			},
			exit: 0,
		},
		{
			name: "HomeOverride",
			args: []string{
				"--bind", hostCanaryDir + ":/home",
				imageRef,
				"test", "-f", "/home/file",
			},
			exit: 0,
		},
		{
			name: "TmpOverride",
			args: []string{
				"--bind", hostCanaryDir + ":/tmp",
				imageRef,
				"test", "-f", "/tmp/file",
			},
			exit: 0,
		},
		{
			name: "VarTmpOverride",
			args: []string{
				"--bind", hostCanaryDir + ":/var/tmp",
				imageRef,
				"test", "-f", "/var/tmp/file",
			},
			exit: 0,
		},
		{
			name: "NestedBindFile",
			args: []string{
				"--bind", canaryDirBind,
				"--bind", hostCanaryFile + ":" + filepath.Join(contCanaryDir, "file2"),
				imageRef,
				"test", "-f", "/canary/file2",
			},
			postRun: checkHostFile(filepath.Join(hostCanaryDir, "file2")),
			exit:    0,
		},
		{
			name: "NestedBindDir",
			args: []string{
				"--bind", canaryDirBind,
				"--bind", hostCanaryDir + ":" + filepath.Join(contCanaryDir, "dir2"),
				imageRef,
				"test", "-d", "/canary/dir2",
			},
			postRun: checkHostDir(filepath.Join(hostCanaryDir, "dir2")),
			exit:    0,
		},
		{
			name: "MultipleNestedBindDir",
			args: []string{
				"--bind", canaryDirBind,
				"--bind", hostCanaryDir + ":" + filepath.Join(contCanaryDir, "dir2"),
				"--bind", hostCanaryFile + ":" + filepath.Join(filepath.Join(contCanaryDir, "dir2"), "nested"),
				imageRef,
				"test", "-f", "/canary/dir2/nested",
			},
			postRun: checkHostFile(filepath.Join(hostCanaryDir, "nested")),
			exit:    0,
		},
		// For the --mount variants we are really just verifying the CLI
		// acceptance of one or more --mount flags. Translation from --mount
		// strings to BindPath structs is checked in unit tests. The
		// functionality of bind mounts of various kinds is already checked
		// above, with --bind flags. No need to duplicate all of these.
		{
			name: "MountSingle",
			args: []string{
				"--mount", canaryFileMount,
				imageRef,
				"test", "-f", contCanaryFile,
			},
			exit: 0,
		},
		{
			name: "MountNested",
			args: []string{
				"--mount", canaryDirMount,
				"--mount", "source=" + hostCanaryFile + ",destination=" + filepath.Join(contCanaryDir, "file3"),
				imageRef,
				"test", "-f", "/canary/file3",
			},
			postRun: checkHostFile(filepath.Join(hostCanaryDir, "file3")),
			exit:    0,
		},
	}

	for _, profile := range e2e.OCIProfiles {
		profile := profile
		createWorkspaceDirs(t)

		t.Run(profile.String(), func(t *testing.T) {
			for _, tt := range tests {
				c.env.RunSingularity(
					t,
					e2e.AsSubtest(tt.name),
					e2e.WithProfile(profile),
					e2e.WithCommand("exec"),
					e2e.WithArgs(tt.args...),
					e2e.PostRun(tt.postRun),
					e2e.ExpectExit(tt.exit),
				)
			}
		})
	}
}

// Check that both root via fakeroot and user without fakeroot are mapped to
// uid/gid on host, by writing a file out to host and checking ownership.
func (c actionTests) actionOciIDMaps(t *testing.T) {
	e2e.EnsureOCIArchive(t, c.env)
	imageRef := "oci-archive:" + c.env.OCIArchivePath

	bindDir, cleanup := e2e.MakeTempDir(t, c.env.TestDir, "usermap", "")
	t.Cleanup(func() {
		if !t.Failed() {
			cleanup(t)
		}
	})

	for _, profile := range []e2e.Profile{e2e.OCIUserProfile, e2e.OCIFakerootProfile} {
		t.Run(profile.String(), func(t *testing.T) {
			cmdArgs := []string{
				"-B", fmt.Sprintf("%s:/test", bindDir),
				imageRef,
				"/bin/touch", fmt.Sprintf("/test/%s", profile.String()),
			}
			c.env.RunSingularity(
				t,
				e2e.AsSubtest(profile.String()),
				e2e.WithProfile(profile),
				e2e.WithCommand("exec"),
				e2e.WithArgs(cmdArgs...),
				e2e.ExpectExit(0),
				e2e.PostRun(func(t *testing.T) {
					fp := filepath.Join(bindDir, profile.String())
					expectUID := profile.HostUser(t).UID
					expectGID := profile.HostUser(t).GID
					if !fs.IsOwner(fp, expectUID) {
						t.Errorf("%s not owned by uid %d", fp, expectUID)
					}
					if !fs.IsGroup(fp, expectGID) {
						t.Errorf("%s not owned by gid %d", fp, expectGID)
					}
				}),
			)
		})
	}
}
