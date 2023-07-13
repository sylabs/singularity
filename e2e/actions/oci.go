// Copyright (c) 2022-2023, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package actions

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"text/template"

	cdispecs "github.com/container-orchestrated-devices/container-device-interface/specs-go"
	"github.com/sylabs/singularity/e2e/internal/e2e"
	"github.com/sylabs/singularity/internal/pkg/test/tool/dirs"
	"github.com/sylabs/singularity/internal/pkg/test/tool/require"
	"github.com/sylabs/singularity/internal/pkg/util/fs"
	"gotest.tools/v3/assert"
)

const (
	imgTestFilePath    string = "file-for-testing"
	squashfsTestString string = "squashfs-test-string"
	extfsTestString    string = "extfs-test-string"
)

var (
	imgsPath        = filepath.Join("..", "test", "images")
	squashfsImgPath = filepath.Join(imgsPath, "squashfs-for-overlay.img")
	extfsImgPath    = filepath.Join(imgsPath, "extfs-for-overlay.img")
)

func (c actionTests) actionOciRun(t *testing.T) {
	e2e.EnsureOCIArchive(t, c.env)
	e2e.EnsureOCISIF(t, c.env)
	e2e.EnsureDockerArchive(t, c.env)
	e2e.EnsureORASOCISIF(t, c.env)

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
			name:     "oci-sif",
			imageRef: "oci-sif:" + c.env.OCISIFPath,
			exit:     0,
		},
		{
			name:     "oci-sif-http",
			imageRef: "https://s3.amazonaws.com/singularity-ci-public/alpine-oci-sif-squashfs.sif",
			exit:     0,
		},
		{
			name:     "oci-sif-oras",
			imageRef: c.env.OrasTestOCISIF,
			exit:     0,
		},
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
	e2e.EnsureOCISIF(t, c.env)

	imageRef := "oci-sif:" + c.env.OCISIFPath

	// Create a temp testfile
	testdata, err := fs.MakeTmpDir(c.env.TestDir, "testdata", 0o755)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if !t.Failed() {
			os.RemoveAll(testdata)
		}
	})

	testdataTmp := filepath.Join(testdata, "tmp")
	if err := os.Mkdir(testdataTmp, 0o755); err != nil {
		t.Fatal(err)
	}

	// Create a temp testfile
	tmpfile, err := fs.MakeTmpFile(testdataTmp, "testSingularityExec.", 0o644)
	if err != nil {
		t.Fatal(err)
	}
	tmpfile.Close()

	basename := filepath.Base(tmpfile.Name())
	tmpfilePath := filepath.Join("/tmp", basename)
	homePath := filepath.Join("/home", basename)

	tests := []struct {
		name         string
		argv         []string
		exit         int
		wantOutputs  []e2e.SingularityCmdResultOp
		skipProfiles map[string]bool
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
			name: "Home",
			argv: []string{"--home", "/myhomeloc", imageRef, "sh", "-c", "env; mount"},
			wantOutputs: []e2e.SingularityCmdResultOp{
				e2e.ExpectOutput(e2e.RegexMatch, `\bHOME=/myhomeloc\b`),
				e2e.ExpectOutput(e2e.RegexMatch, `\btmpfs on /myhomeloc\b`),
			},
			exit: 0,
		},
		{
			name: "HomePath",
			argv: []string{"--home", testdataTmp + ":/home", imageRef, "test", "-f", homePath},
			exit: 0,
		},
		{
			name: "HomeTmp",
			argv: []string{"--home", "/tmp", imageRef, "true"},
			exit: 0,
		},
		{
			name: "HomeTmpExplicit",
			argv: []string{"--home", "/tmp:/home", imageRef, "true"},
			exit: 0,
		},
		{
			name: "NoHome",
			argv: []string{"--no-home", imageRef, "grep", e2e.OCIUserProfile.ContainerUser(t).Dir, "/proc/self/mountinfo"},
			exit: 1,
		},
		{
			name: "UTSNamespace",
			argv: []string{"--uts", imageRef, "true"},
			exit: 0,
		},
		{
			name: "Hostname",
			argv: []string{"--hostname", "whats-in-an-oci-name", imageRef, "hostname"},
			exit: 0,
			wantOutputs: []e2e.SingularityCmdResultOp{
				e2e.ExpectOutput(e2e.ExactMatch, "whats-in-an-oci-name"),
			},
		},
		{
			name: "Workdir",
			argv: []string{"--workdir", testdata, imageRef, "test", "-f", tmpfilePath},
			exit: 0,
		},
		{
			name: "Pwd",
			argv: []string{"--pwd", "/etc", imageRef, "pwd"},
			exit: 0,
			wantOutputs: []e2e.SingularityCmdResultOp{
				e2e.ExpectOutput(e2e.ExactMatch, "/etc"),
			},
		},
		{
			name: "Cwd",
			argv: []string{"--cwd", "/etc", imageRef, "pwd"},
			exit: 0,
			wantOutputs: []e2e.SingularityCmdResultOp{
				e2e.ExpectOutput(e2e.ExactMatch, "/etc"),
			},
		},
		{
			name: "ResolvConfGoogle",
			argv: []string{"--dns", "8.8.8.8,8.8.4.4", imageRef, "nslookup", "w3.org"},
			exit: 0,
			wantOutputs: []e2e.SingularityCmdResultOp{
				e2e.ExpectOutput(e2e.RegexMatch, `^(\s*)Server:(\s+)(8\.8\.8\.8|8\.8\.4\.4)(\s*)\n`),
			},
		},
		{
			name: "ResolvConfCloudflare",
			argv: []string{"--dns", "1.1.1.1", imageRef, "nslookup", "w3.org"},
			exit: 0,
			wantOutputs: []e2e.SingularityCmdResultOp{
				e2e.ExpectOutput(e2e.RegexMatch, `^(\s*)Server:(\s+)(1\.1\.1\.1)(\s*)\n`),
			},
		},
		{
			name: "CustomHomePreservesRootShell",
			argv: []string{"--home", "/tmp", imageRef, "cat", "/etc/passwd"},
			exit: 0,
			wantOutputs: []e2e.SingularityCmdResultOp{
				e2e.ExpectOutput(e2e.RegexMatch, `^root:x:0:0:root:[^:]*:/bin/ash\n`),
			},
		},
		{
			name: "Containlibs",
			argv: []string{"--containlibs", "/etc/hosts", imageRef, "ls", "/.singularity.d/libs"},
			exit: 0,
			wantOutputs: []e2e.SingularityCmdResultOp{
				e2e.ExpectOutput(e2e.ExactMatch, `hosts`),
			},
		},
	}
	for _, profile := range e2e.OCIProfiles {
		t.Run(profile.String(), func(t *testing.T) {
			for _, tt := range tests {
				skip, ok := tt.skipProfiles[profile.String()]
				if ok && skip {
					continue
				}

				c.env.RunSingularity(
					t,
					e2e.AsSubtest(tt.name),
					e2e.WithProfile(profile),
					e2e.WithCommand("exec"),
					e2e.WithDir("/tmp"),
					e2e.WithArgs(tt.argv...),
					e2e.ExpectExit(tt.exit, tt.wantOutputs...),
				)
			}
		})
	}
}

// Shell interaction tests
func (c actionTests) actionOciShell(t *testing.T) {
	e2e.EnsureOCISIF(t, c.env)

	tests := []struct {
		name       string
		argv       []string
		consoleOps []e2e.SingularityConsoleOp
		exit       int
	}{
		{
			name: "ShellExit",
			argv: []string{"oci-sif:" + c.env.OCISIFPath},
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
			argv: []string{"oci-sif:" + c.env.OCISIFPath},
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
	e2e.EnsureOCISIF(t, c.env)
	imageRef := "oci-sif:" + c.env.OCISIFPath

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
	e2e.EnsureOCISIF(t, c.env)
	imageRef := "oci-sif:" + c.env.OCISIFPath

	workspace, cleanup := e2e.MakeTempDir(t, c.env.TestDir, "bind-workspace-", "")
	t.Cleanup(func() {
		if !t.Failed() {
			e2e.Privileged(cleanup)
		}
	})

	contCanaryDir := "/canary"
	hostCanaryDir := filepath.Join(workspace, "canary")

	contCanaryFile := "/canary/file"
	hostCanaryFile := filepath.Join(hostCanaryDir, "file")

	canaryFileBind := hostCanaryFile + ":" + contCanaryFile
	canaryFileMount := "type=bind,source=" + hostCanaryFile + ",destination=" + contCanaryFile
	canaryDirBind := hostCanaryDir + ":" + contCanaryDir
	canaryDirMount := "type=bind,source=" + hostCanaryDir + ",destination=" + contCanaryDir

	hostHomeDir := filepath.Join(workspace, "home")
	hostWorkDir := filepath.Join(workspace, "workdir")

	createWorkspaceDirs := func(t *testing.T) {
		mkWorkspaceDirs(t, hostCanaryDir, hostHomeDir, hostWorkDir, hostCanaryFile)
	}

	checkHostFn := func(path string, fn func(string) bool) func(*testing.T) {
		return func(t *testing.T) {
			if t.Failed() {
				return
			}
			if !fn(path) {
				t.Errorf("%s not found on host", path)
			}
			// When a nested bind is performed under workdir, the bind
			// destination will be created (if necessary) by runc/crun inside
			// workdir on the host. The bind destination will be created with
			// subuid:subgid ownership. This requires privilege, or a userns +
			// id mapping, to remove. (Relevant to tests like WorkdirTmpBind,
			// below.)
			e2e.Privileged(func(t *testing.T) {
				if err := os.RemoveAll(path); err != nil {
					t.Errorf("failed to delete %s: %s", path, err)
				}
			})(t)
		}
	}
	checkHostFile := func(path string) func(*testing.T) {
		return checkHostFn(path, fs.IsFile)
	}
	checkHostDir := func(path string) func(*testing.T) {
		return checkHostFn(path, fs.IsDir)
	}

	tests := []struct {
		name        string
		args        []string
		wantOutputs []e2e.SingularityCmdResultOp
		postRun     func(*testing.T)
		exit        int
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
		{
			name: "WorkdirTmpBind",
			args: []string{
				"--workdir", hostWorkDir,
				"--bind", hostCanaryDir + ":/tmp/canary/dir",
				imageRef,
				"test", "-f", "/tmp/canary/dir/file",
			},
			postRun: checkHostDir(filepath.Join(hostWorkDir, "tmp", "canary/dir")),
			exit:    0,
		},
		{
			name: "WorkdirVarTmpBind",
			args: []string{
				"--workdir", hostWorkDir,
				"--bind", hostCanaryDir + ":/var/tmp/canary/dir",
				imageRef,
				"test", "-f", "/var/tmp/canary/dir/file",
			},
			postRun: checkHostDir(filepath.Join(hostWorkDir, "var_tmp", "canary/dir")),
			exit:    0,
		},
		{
			name: "WorkdirVarTmpBindWritable",
			args: []string{
				"--workdir", hostWorkDir,
				"--bind", hostCanaryDir + ":/var/tmp/canary/dir",
				imageRef,
				"test", "-f", "/var/tmp/canary/dir/file",
			},
			postRun: checkHostDir(filepath.Join(hostWorkDir, "var_tmp", "canary/dir")),
			exit:    0,
		},
		{
			name: "IsScratchTmpfs",
			args: []string{
				"--scratch", "/name-of-a-scratch",
				imageRef,
				"mount",
			},
			wantOutputs: []e2e.SingularityCmdResultOp{
				e2e.ExpectOutput(e2e.RegexMatch, `\btmpfs on /name-of-a-scratch\b`),
			},
			exit: 0,
		},
		{
			name: "BindOverScratch",
			args: []string{
				"--scratch", "/name-of-a-scratch",
				"--bind", hostCanaryDir + ":/name-of-a-scratch",
				imageRef,
				"test", "-f", "/name-of-a-scratch/file",
			},
			exit: 0,
		},
		{
			name: "ScratchTmpfsBind",
			args: []string{
				"--scratch", "/scratch",
				"--bind", hostCanaryDir + ":/scratch/dir",
				imageRef,
				"test", "-f", "/scratch/dir/file",
			},
			exit: 0,
		},
		{
			name: "ScratchWorkdirBind",
			args: []string{
				"--workdir", hostWorkDir,
				"--scratch", "/scratch",
				"--bind", hostCanaryDir + ":/scratch/dir",
				imageRef,
				"test", "-f", "/scratch/dir/file",
			},
			postRun: checkHostDir(filepath.Join(hostWorkDir, "scratch/scratch", "dir")),
			exit:    0,
		},
		{
			name: "CustomHomeOneToOne",
			args: []string{
				"--home", hostHomeDir + ":" + hostHomeDir,
				"--bind", hostCanaryDir + ":" + filepath.Join(hostHomeDir, "canary121RO"),
				imageRef,
				"test", "-f", filepath.Join(hostHomeDir, "canary121RO/file"),
			},
			postRun: checkHostDir(filepath.Join(hostHomeDir, "canary121RO")),
			exit:    0,
		},
		{
			name: "CustomHomeBind",
			args: []string{
				"--home", hostHomeDir + ":/home/e2e",
				"--bind", hostCanaryDir + ":/home/e2e/canaryRO",
				imageRef,
				"test", "-f", "/home/e2e/canaryRO/file",
			},
			postRun: checkHostDir(filepath.Join(hostHomeDir, "canaryRO")),
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
					e2e.ExpectExit(tt.exit, tt.wantOutputs...),
				)
			}
		})
	}
}

func (c actionTests) actionOciCdi(t *testing.T) {
	// Grab the reference OCI archive we're going to use
	e2e.EnsureOCISIF(t, c.env)
	imageRef := "oci-sif:" + c.env.OCISIFPath

	// Set up a custom subtestWorkspace object that will holds the collection of temporary directories (nested under the main temporary directory, mainDir) that each test will use.
	type subtestWorkspace struct {
		mainDir   string
		jsonsDir  string
		mountDirs []string
	}

	// Create a function to create a fresh subtestWorkspace, with distinct temporary directories, that each individual subtest will use
	setupIndivSubtestWorkspace := func(t *testing.T, numMountDirs int) *subtestWorkspace {
		stws := subtestWorkspace{}
		mainDir, cleanup := e2e.MakeTempDir(t, c.env.TestDir, "", "")
		t.Cleanup(func() {
			if !t.Failed() {
				e2e.Privileged(cleanup)
			}
		})
		stws.mainDir = mainDir

		// No need to do anything with the cleanup functions returned here, because the directories created are all going to be children of (tw.)mainDir, whose cleanup was already registered above.
		stws.jsonsDir, _ = e2e.MakeTempDir(t, stws.mainDir, "cdi-jsons-", "")
		stws.mountDirs = make([]string, 0, numMountDirs)
		for len(stws.mountDirs) < numMountDirs {
			dir, _ := e2e.MakeTempDir(t, stws.mainDir, fmt.Sprintf("mount-dir-%d-", len(stws.mountDirs)+1), "")
			// Make writable to all, due to current nested userns mapping restrictions.
			// Will work without this once crun-specific single mapping is present.
			os.Chmod(dir, 0o777)
			stws.mountDirs = append(stws.mountDirs, dir)
		}

		return &stws
	}

	// Set up the JSON template that we're going to populate on a per-subtest basis with particular CDI spec values
	e2eMountTemplateFilename := "cditemplate.json.tpl"
	cdiJSONTemplateFilePath := filepath.Join("..", "test", "cdi", e2eMountTemplateFilename)
	funcMap := template.FuncMap{
		// The name "title" is what the function will be called in the template text.
		"tojson": func(o any) string {
			s, _ := json.Marshal(o)
			return string(s)
		},
	}
	cdiJSONTemplate, err := template.New(e2eMountTemplateFilename).Funcs(funcMap).ParseFiles(cdiJSONTemplateFilePath)
	if err != nil {
		t.Errorf("Could not read JSON template for CDI e2e tests from file %#v", cdiJSONTemplateFilePath)
		return
	}

	// The set of actual subtests
	var wantUID uint32 = 1000
	var wantGID uint32 = 1000
	tests := []struct {
		name        string
		devices     []string
		wantExit    int
		postRun     func(t *testing.T)
		DeviceNodes []cdispecs.DeviceNode
		Mounts      []cdispecs.Mount
		Env         []string
	}{
		{
			name: "ValidMounts",
			devices: []string{
				"singularityCEtesting.sylabs.io/device=TesterDevice",
			},
			wantExit:    0,
			DeviceNodes: []cdispecs.DeviceNode{},
			Mounts: []cdispecs.Mount{
				{
					ContainerPath: "/tmp/mount1",
					Options:       []string{"rw", "bind", "users"},
				},
				{
					ContainerPath: "/tmp/mount3",
					Options:       []string{"rw", "bind", "users"},
				},
				{
					ContainerPath: "/tmp/mount13",
					Options:       []string{"rw", "bind", "users"},
				},
				{
					ContainerPath: "/tmp/mount17",
					Options:       []string{"rw", "bind", "users"},
				},
			},
			Env: []string{
				"ABCD=QWERTY",
				"EFGH=ASDFGH",
				"IJKL=ZXCVBN",
			},
		},
		{
			name: "InvalidDevice",
			devices: []string{
				"singularityCEtesting.sylabs.io/device=DoesNotExist",
			},
			wantExit:    255,
			DeviceNodes: []cdispecs.DeviceNode{},
			Mounts:      []cdispecs.Mount{},
			Env:         []string{},
		},
		{
			name: "KmsgDevice",
			devices: []string{
				"singularityCEtesting.sylabs.io/device=TesterDevice",
			},
			wantExit: 0,
			DeviceNodes: []cdispecs.DeviceNode{
				{
					HostPath:    "/dev/kmsg",
					Path:        "/dev/kmsg",
					Permissions: "rw",
					Type:        "c",
					UID:         &wantUID,
					GID:         &wantGID,
				},
			},
		},
	}

	for _, profile := range e2e.OCIProfiles {
		t.Run(profile.String(), func(t *testing.T) {
			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					stws := setupIndivSubtestWorkspace(t, len(tt.Mounts))

					// Populate the HostPath values we're going to feed into the CDI JSON template, based on the subtestWorkspace we just created
					for i, d := range stws.mountDirs {
						tt.Mounts[i].HostPath = d
					}

					// Inject this subtest's values into the template to create the CDI JSON file
					cdiJSONFilePath := filepath.Join(stws.jsonsDir, fmt.Sprintf("%s-cdi.json", tt.name))
					cdiJSONFile, err := os.OpenFile(cdiJSONFilePath, os.O_CREATE|os.O_WRONLY, 0o644)
					if err != nil {
						t.Errorf("could not create file %#v for writing CDI JSON: %v", cdiJSONFilePath, err)
					}
					if err = cdiJSONTemplate.Execute(cdiJSONFile, tt); err != nil {
						t.Errorf("error executing template %#v to create CDI JSON: %v", cdiJSONTemplateFilePath, err)
						return
					}
					cdiJSONFile.Close()

					// Create a list of test strings, each of which will be echoed into a separate file in a separate mount in the container.
					testfileStrings := make([]string, 0, len(tt.Mounts))
					for i := range tt.Mounts {
						testfileStrings = append(testfileStrings, fmt.Sprintf("test_string_for_mount_%d_in_test_%s", i, tt.name))
					}

					// Generate the command to be executed in the container
					// Start by printing all environment variables, to test using e2e.ContainMatch conditions later
					execCmd := "/usr/bin/env"

					// Add commands to test the presence of mapped devices.
					for _, d := range tt.DeviceNodes {
						testFlag := "-f"
						switch d.Type {
						case "c":
							testFlag = "-c"
						}
						execCmd += fmt.Sprintf(" && test %s %s", testFlag, d.Path)
					}

					// Add commands to test the presence, and functioning, of mounts.
					for i, m := range tt.Mounts {
						// Add a separate teststring echo statement for each mount
						execCmd += fmt.Sprintf(" && echo %s > %s/testfile_%d", testfileStrings[i], m.ContainerPath, i)
					}

					// Create a postRun function to check that the testfiles written to the container mounts made their way to the right host temporary directories
					testMountsAndEnv := func(t *testing.T) {
						for i, m := range tt.Mounts {
							testfileFilename := filepath.Join(m.HostPath, fmt.Sprintf("testfile_%d", i))
							b, err := os.ReadFile(testfileFilename)
							if err != nil {
								t.Errorf("could not read testfile %s", testfileFilename)
								return
							}

							s := string(b)
							if s != testfileStrings[i]+"\n" {
								t.Errorf("mismatched testfileString; expected %#v, got %#v (mount: %#v)", s, testfileStrings[i], m)
							}
						}
					}

					// Create a set of e2e.SingularityCmdResultOp objects to test that environment variables have been correctly injected into the container
					envExpects := make([]e2e.SingularityCmdResultOp, 0, len(tt.Env))
					for _, e := range tt.Env {
						envExpects = append(envExpects, e2e.ExpectOutput(e2e.ContainMatch, e))
					}

					// Run the subtest.
					c.env.RunSingularity(
						t,
						e2e.AsSubtest(tt.name),
						e2e.WithCommand("exec"),
						e2e.WithArgs(
							"--device",
							strings.Join(tt.devices, ","),
							"--cdi-dirs",
							stws.jsonsDir,
							imageRef,
							"/bin/sh", "-c", execCmd),
						e2e.WithProfile(profile),
						e2e.ExpectExit(tt.wantExit, envExpects...),
						e2e.PostRun(tt.postRun),
						e2e.PostRun(testMountsAndEnv),
					)
				})
			}
		})
	}
}

// Check that both root via fakeroot and user without fakeroot are mapped to
// uid/gid on host, by writing a file out to host and checking ownership.
func (c actionTests) actionOciIDMaps(t *testing.T) {
	e2e.EnsureOCISIF(t, c.env)
	imageRef := "oci-sif:" + c.env.OCISIFPath

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

// actionOCICompat checks that the --oci mode has the behavior that the native mode gains from the --compat flag.
// Must be run in sequential section as it modifies host process umask.
func (c actionTests) actionOciCompat(t *testing.T) {
	e2e.EnsureOCISIF(t, c.env)
	imageRef := "oci-sif:" + c.env.OCISIFPath

	type test struct {
		name     string
		args     []string
		exitCode int
		expect   e2e.SingularityCmdResultOp
	}

	tests := []test{
		{
			name:     "containall",
			args:     []string{imageRef, "sh", "-c", "ls -lah $HOME"},
			exitCode: 0,
			expect:   e2e.ExpectOutput(e2e.ContainMatch, "total 0"),
		},
		{
			name:     "writable-tmpfs",
			args:     []string{imageRef, "sh", "-c", "touch /test"},
			exitCode: 0,
		},
		{
			name:     "no-init",
			args:     []string{imageRef, "sh", "-c", "ps"},
			exitCode: 0,
			expect:   e2e.ExpectOutput(e2e.UnwantedContainMatch, "sinit"),
		},
		{
			name:     "no-umask",
			args:     []string{imageRef, "sh", "-c", "umask"},
			exitCode: 0,
			expect:   e2e.ExpectOutput(e2e.ContainMatch, "0022"),
		},
	}

	oldUmask := syscall.Umask(0)
	defer syscall.Umask(oldUmask)

	for _, tt := range tests {
		c.env.RunSingularity(
			t,
			e2e.AsSubtest(tt.name),
			e2e.WithProfile(e2e.OCIUserProfile),
			e2e.WithCommand("exec"),
			e2e.WithArgs(tt.args...),
			e2e.ExpectExit(
				tt.exitCode,
				tt.expect,
			),
		)
	}
}

// actionOciOverlay checks that --overlay functions correctly in OCI mode.
//
//nolint:maintidx
func (c actionTests) actionOciOverlay(t *testing.T) {
	e2e.EnsureOCISIF(t, c.env)
	imageRef := "oci-sif:" + c.env.OCISIFPath

	for _, profile := range e2e.OCIProfiles {
		testDir, err := fs.MakeTmpDir(c.env.TestDir, "overlaytestdir", 0o755)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if !t.Failed() {
				os.RemoveAll(testDir)
			}
		})

		// Create a few writable overlay subdirs under testDir
		for i := 0; i < 3; i++ {
			dirName := fmt.Sprintf("my_rw_ol_dir%d", i)
			fullPath := filepath.Join(testDir, dirName)
			dirs.MkdirOrFatal(t, fullPath, 0o755)
			upperPath := filepath.Join(fullPath, "upper")
			dirs.MkdirOrFatal(t, upperPath, 0o777)
			dirs.MkdirOrFatal(t, filepath.Join(fullPath, "work"), 0o777)
			t.Cleanup(func() {
				if !t.Failed() {
					os.RemoveAll(fullPath)
				}
			})
		}

		// Create a few read-only overlay subdirs under testDir
		for i := 0; i < 3; i++ {
			dirName := fmt.Sprintf("my_ro_ol_dir%d", i)
			fullPath := filepath.Join(testDir, dirName)
			dirs.MkdirOrFatal(t, fullPath, 0o755)
			upperPath := filepath.Join(fullPath, "upper")
			dirs.MkdirOrFatal(t, upperPath, 0o777)
			dirs.MkdirOrFatal(t, filepath.Join(fullPath, "work"), 0o777)
			t.Cleanup(func() {
				if !t.Failed() {
					os.RemoveAll(fullPath)
				}
			})
			if err = os.WriteFile(
				filepath.Join(upperPath, fmt.Sprintf("testfile.%d", i)),
				[]byte(fmt.Sprintf("test_string_%d\n", i)),
				0o644); err != nil {
				t.Fatal(err)
			}
			if err = os.WriteFile(
				filepath.Join(upperPath, "maskable_testfile"),
				[]byte(fmt.Sprintf("maskable_string_%d\n", i)),
				0o644); err != nil {
				t.Fatal(err)
			}
		}

		// Create a copy of the extfs test image to be used for testing readonly
		// extfs image overlays
		readonlyExtfsImgPath := filepath.Join(testDir, "readonly-extfs.img")
		err = fs.CopyFile(extfsImgPath, readonlyExtfsImgPath, 0o444)
		if err != nil {
			t.Fatalf("could not copy %q to %q: %s", extfsImgPath, readonlyExtfsImgPath, err)
		}

		// Create a copy of the extfs test image to be used for testing writable
		// extfs image overlays
		writableExtfsImgPath := filepath.Join(testDir, "writable-extfs.img")
		err = fs.CopyFile(extfsImgPath, writableExtfsImgPath, 0o755)
		if err != nil {
			t.Fatalf("could not copy %q to %q: %s", extfsImgPath, writableExtfsImgPath, err)
		}

		tests := []struct {
			name         string
			args         []string
			exitCode     int
			requiredCmds []string
			wantOutputs  []e2e.SingularityCmdResultOp
		}{
			{
				name:     "ExistRWDir",
				args:     []string{"--overlay", filepath.Join(testDir, "my_rw_ol_dir0"), imageRef, "sh", "-c", "echo my_test_string > /my_test_file"},
				exitCode: 0,
			},
			{
				name:     "ExistRWDirRevisit",
				args:     []string{"--overlay", filepath.Join(testDir, "my_rw_ol_dir0"), imageRef, "cat", "/my_test_file"},
				exitCode: 0,
				wantOutputs: []e2e.SingularityCmdResultOp{
					e2e.ExpectOutput(e2e.ExactMatch, "my_test_string"),
				},
			},
			{
				name:     "ExistRWDirRevisitAsRO",
				args:     []string{"--overlay", filepath.Join(testDir, "my_rw_ol_dir0:ro"), imageRef, "cat", "/my_test_file"},
				exitCode: 0,
				wantOutputs: []e2e.SingularityCmdResultOp{
					e2e.ExpectOutput(e2e.ExactMatch, "my_test_string"),
				},
			},
			{
				name:     "RWOverlayMissing",
				args:     []string{"--overlay", filepath.Join(testDir, "something_nonexistent"), imageRef, "echo", "hi"},
				exitCode: 255,
			},
			{
				name:     "ROOverlayMissing",
				args:     []string{"--overlay", filepath.Join(testDir, "something_nonexistent:ro"), imageRef, "echo", "hi"},
				exitCode: 255,
			},
			{
				name:     "AutoAddTmpfs",
				args:     []string{"--overlay", filepath.Join(testDir, "my_ro_ol_dir1:ro"), imageRef, "sh", "-c", "echo this_should_disappear > /my_test_file"},
				exitCode: 0,
			},
			{
				name: "SeveralRODirs",
				args: []string{
					"--overlay", filepath.Join(testDir, "my_ro_ol_dir2:ro"),
					"--overlay", filepath.Join(testDir, "my_ro_ol_dir0:ro"),
					"--overlay", filepath.Join(testDir, "my_ro_ol_dir1:ro"),
					imageRef, "cat", "/testfile.1", "/maskable_testfile",
				},
				exitCode: 0,
				wantOutputs: []e2e.SingularityCmdResultOp{
					e2e.ExpectOutput(e2e.ContainMatch, "test_string_1"),
					e2e.ExpectOutput(e2e.ContainMatch, "maskable_string_2"),
				},
			},
			{
				name: "AllTypesAtOnce",
				args: []string{
					"--overlay", filepath.Join(testDir, "my_ro_ol_dir2:ro"),
					"--overlay", readonlyExtfsImgPath + ":ro",
					"--overlay", squashfsImgPath,
					"--overlay", filepath.Join(testDir, "my_ro_ol_dir1:ro"),
					"--overlay", filepath.Join(testDir, "my_rw_ol_dir0"),
					imageRef, "cat", "/testfile.1", "/maskable_testfile", filepath.Join("/", imgTestFilePath),
				},
				requiredCmds: []string{"squashfuse", "fuse2fs", "fusermount"},
				exitCode:     0,
				wantOutputs: []e2e.SingularityCmdResultOp{
					e2e.ExpectOutput(e2e.ContainMatch, "test_string_1"),
					e2e.ExpectOutput(e2e.ContainMatch, "maskable_string_2"),
					e2e.ExpectOutput(e2e.ContainMatch, extfsTestString),
				},
			},
			{
				name: "SquashfsAndDirs",
				args: []string{
					"--overlay", filepath.Join(testDir, "my_ro_ol_dir2:ro"),
					"--overlay", squashfsImgPath,
					"--overlay", filepath.Join(testDir, "my_ro_ol_dir1:ro"),
					"--overlay", filepath.Join(testDir, "my_rw_ol_dir0"),
					imageRef, "cat", "/testfile.1", "/maskable_testfile", filepath.Join("/", imgTestFilePath),
				},
				requiredCmds: []string{"squashfuse", "fusermount"},
				exitCode:     0,
				wantOutputs: []e2e.SingularityCmdResultOp{
					e2e.ExpectOutput(e2e.ContainMatch, "test_string_1"),
					e2e.ExpectOutput(e2e.ContainMatch, "maskable_string_2"),
					e2e.ExpectOutput(e2e.ContainMatch, squashfsTestString),
				},
			},
			{
				name: "ExtfsAndDirs",
				args: []string{
					"--overlay", filepath.Join(testDir, "my_ro_ol_dir2:ro"),
					"--overlay", readonlyExtfsImgPath + ":ro",
					"--overlay", filepath.Join(testDir, "my_ro_ol_dir1:ro"),
					"--overlay", filepath.Join(testDir, "my_rw_ol_dir0"),
					imageRef, "cat", "/testfile.1", "/maskable_testfile", filepath.Join("/", imgTestFilePath),
				},
				requiredCmds: []string{"fuse2fs", "fusermount"},
				exitCode:     0,
				wantOutputs: []e2e.SingularityCmdResultOp{
					e2e.ExpectOutput(e2e.ContainMatch, "test_string_1"),
					e2e.ExpectOutput(e2e.ContainMatch, "maskable_string_2"),
					e2e.ExpectOutput(e2e.ContainMatch, extfsTestString),
				},
			},
			{
				name: "SquashfsAndDirsAndMissingRO",
				args: []string{
					"--overlay", filepath.Join(testDir, "my_ro_ol_dir2:ro"),
					"--overlay", squashfsImgPath,
					"--overlay", filepath.Join(testDir, "my_ro_ol_dir1:ro"),
					"--overlay", filepath.Join(testDir, "something_nonexistent:ro"),
					"--overlay", filepath.Join(testDir, "my_rw_ol_dir0"),
					imageRef, "cat", "/testfile.1", "/maskable_testfile", filepath.Join("/", imgTestFilePath),
				},
				requiredCmds: []string{"squashfuse", "fusermount"},
				exitCode:     255,
			},
			{
				name: "SquashfsAndDirsAndMissingRW",
				args: []string{
					"--overlay", filepath.Join(testDir, "my_ro_ol_dir2:ro"),
					"--overlay", squashfsImgPath,
					"--overlay", filepath.Join(testDir, "my_ro_ol_dir1:ro"),
					"--overlay", filepath.Join(testDir, "something_nonexistent"),
					imageRef, "cat", "/testfile.1", "/maskable_testfile", filepath.Join("/", imgTestFilePath),
				},
				requiredCmds: []string{"squashfuse", "fusermount"},
				exitCode:     255,
			},
			{
				name: "TwoWritables",
				args: []string{
					"--overlay", filepath.Join(testDir, "my_ro_ol_dir2:ro"),
					"--overlay", filepath.Join(testDir, "my_ro_ol_dir1:ro"),
					"--overlay", filepath.Join(testDir, "my_rw_ol_dir1"),
					"--overlay", filepath.Join(testDir, "my_rw_ol_dir0"),
					imageRef, "cat", "/testfile.1", "/maskable_testfile", filepath.Join("/", imgTestFilePath),
				},
				exitCode: 255,
			},
			{
				name: "ThreeWritables",
				args: []string{
					"--overlay", filepath.Join(testDir, "my_ro_ol_dir2:ro"),
					"--overlay", filepath.Join(testDir, "my_ro_ol_dir1:ro"),
					"--overlay", filepath.Join(testDir, "my_rw_ol_dir1"),
					"--overlay", filepath.Join(testDir, "my_rw_ol_dir0"),
					"--overlay", filepath.Join(testDir, "my_rw_ol_dir2"),
					imageRef, "cat", "/testfile.1", "/maskable_testfile", filepath.Join("/", imgTestFilePath),
				},
				exitCode: 255,
			},
			{
				name:         "WritableExtfs",
				args:         []string{"--overlay", writableExtfsImgPath, imageRef, "sh", "-c", "echo my_test_string > /my_test_file"},
				requiredCmds: []string{"fuse2fs", "fuse-overlayfs", "fusermount"},
				exitCode:     0,
			},
			{
				name:         "WritableExtfsRevisit",
				args:         []string{"--overlay", writableExtfsImgPath, imageRef, "cat", "/my_test_file"},
				requiredCmds: []string{"fuse2fs", "fuse-overlayfs", "fusermount"},
				exitCode:     0,
				wantOutputs: []e2e.SingularityCmdResultOp{
					e2e.ExpectOutput(e2e.ExactMatch, "my_test_string"),
				},
			},
			{
				name:         "WritableExtfsRevisitAsRO",
				args:         []string{"--overlay", writableExtfsImgPath + ":ro", imageRef, "cat", "/my_test_file"},
				requiredCmds: []string{"fuse2fs", "fuse-overlayfs", "fusermount"},
				exitCode:     0,
				wantOutputs: []e2e.SingularityCmdResultOp{
					e2e.ExpectOutput(e2e.ExactMatch, "my_test_string"),
				},
			},
			{
				name: "WritableExtfsWithDirs",
				args: []string{
					"--overlay", filepath.Join(testDir, "my_ro_ol_dir0:ro"),
					"--overlay", filepath.Join(testDir, "my_ro_ol_dir1:ro"),
					"--overlay", writableExtfsImgPath,
					imageRef, "cat", "/my_test_file",
				},
				requiredCmds: []string{"fuse2fs", "fuse-overlayfs", "fusermount"},
				exitCode:     0,
				wantOutputs: []e2e.SingularityCmdResultOp{
					e2e.ExpectOutput(e2e.ExactMatch, "my_test_string"),
				},
			},
			{
				name: "WritableExtfsWithMix",
				args: []string{
					"--overlay", filepath.Join(testDir, "my_ro_ol_dir0:ro"),
					"--overlay", readonlyExtfsImgPath + ":ro",
					"--overlay", filepath.Join(testDir, "my_ro_ol_dir1:ro"),
					"--overlay", writableExtfsImgPath,
					imageRef, "cat", "/my_test_file",
				},
				exitCode:     0,
				requiredCmds: []string{"fuse2fs", "fuse-overlayfs", "fusermount"},
				wantOutputs: []e2e.SingularityCmdResultOp{
					e2e.ExpectOutput(e2e.ExactMatch, "my_test_string"),
				},
			},
			{
				name: "WritableExtfsWithAll",
				args: []string{
					"--overlay", filepath.Join(testDir, "my_ro_ol_dir0:ro"),
					"--overlay", readonlyExtfsImgPath + ":ro",
					"--overlay", filepath.Join(testDir, "my_ro_ol_dir1:ro"),
					"--overlay", writableExtfsImgPath,
					imageRef, "cat", "/my_test_file",
				},
				exitCode:     0,
				requiredCmds: []string{"squashfuse", "fuse2fs", "fuse-overlayfs", "fusermount"},
				wantOutputs: []e2e.SingularityCmdResultOp{
					e2e.ExpectOutput(e2e.ExactMatch, "my_test_string"),
				},
			},
		}

		t.Run(profile.String(), func(t *testing.T) {
			for _, tt := range tests {
				if !haveAllCommands(t, tt.requiredCmds) {
					continue
				}

				c.env.RunSingularity(
					t,
					e2e.AsSubtest(tt.name),
					e2e.WithProfile(profile),
					e2e.WithCommand("exec"),
					e2e.WithArgs(tt.args...),
					e2e.ExpectExit(
						tt.exitCode,
						tt.wantOutputs...,
					),
				)
			}
		})
	}
}

func haveAllCommands(t *testing.T, cmds []string) bool {
	for _, c := range cmds {
		if _, err := exec.LookPath(c); err != nil {
			return false
		}
	}

	return true
}

// actionOciOverlayTeardown checks that OCI-mode overlays are correctly
// unmounted even in root mode (i.e., when user namespaces are not involved).
func (c actionTests) actionOciOverlayTeardown(t *testing.T) {
	e2e.EnsureOCISIF(t, c.env)
	imageRef := "oci-sif:" + c.env.OCISIFPath

	const mountInfoPath string = "/proc/self/mountinfo"
	mountsPre, err := os.ReadFile(mountInfoPath)
	if err != nil {
		t.Fatal(err)
	}

	tmpDir, cleanup := e2e.MakeTempDir(t, c.env.TestDir, "oci_overlay_teardown-", "")
	t.Cleanup(func() {
		if !t.Failed() {
			cleanup(t)
		}
	})

	dirs.MkdirOrFatal(t, filepath.Join(tmpDir, "upper"), 0o777)
	dirs.MkdirOrFatal(t, filepath.Join(tmpDir, "work"), 0o777)

	c.env.RunSingularity(
		t,
		e2e.WithProfile(e2e.OCIRootProfile),
		e2e.WithCommand("exec"),
		e2e.WithArgs("--overlay", tmpDir+":ro", imageRef, "/bin/true"),
		e2e.ExpectExit(0),
	)

	mountsPost, err := os.ReadFile(mountInfoPath)
	if err != nil {
		t.Fatal(err)
	}

	assert.Equal(
		t, string(mountsPre), string(mountsPost),
		"/proc/self/mountinfo table differs after running OCI container")
}

// Check that write permissions are indeed available for writable FUSE-mounted
// extfs image overlays.
func (c actionTests) actionOciOverlayExtfsPerms(t *testing.T) {
	require.Command(t, "fuse2fs")
	require.Command(t, "fuse-overlayfs")
	require.Command(t, "fusermount")

	for _, profile := range e2e.OCIProfiles {
		// First, create a writable extfs overlay with `singularity overlay create`.
		tmpDir, cleanup := e2e.MakeTempDir(t, c.env.TestDir, "oci_overlay_extfs_perms-", "")
		t.Cleanup(func() {
			if !t.Failed() {
				cleanup(t)
			}
		})

		imgPath := filepath.Join(tmpDir, "extfs-perms-test.img")

		c.env.RunSingularity(
			t,
			e2e.WithProfile(e2e.UserProfile),
			e2e.WithCommand("overlay"),
			e2e.WithArgs("create", "--size", "64", imgPath),
			e2e.ExpectExit(0),
		)

		// Now test whether we can write to, and subsequently read from, the image
		// we created.
		e2e.EnsureOCISIF(t, c.env)
		imageRef := "oci-sif:" + c.env.OCISIFPath

		tests := []struct {
			name        string
			args        []string
			exitCode    int
			wantOutputs []e2e.SingularityCmdResultOp
		}{
			{
				name:     "FirstWrite",
				args:     []string{"--overlay", imgPath, imageRef, "sh", "-c", "echo my_test_string > /my_test_file"},
				exitCode: 0,
			},
			{
				name:     "ThenRead",
				args:     []string{"--overlay", imgPath, imageRef, "cat", "/my_test_file"},
				exitCode: 0,
				wantOutputs: []e2e.SingularityCmdResultOp{
					e2e.ExpectOutput(e2e.ExactMatch, "my_test_string"),
				},
			},
		}
		t.Run(profile.String(), func(t *testing.T) {
			for _, tt := range tests {
				c.env.RunSingularity(
					t,
					e2e.AsSubtest(tt.name),
					e2e.WithProfile(profile),
					e2e.WithCommand("exec"),
					e2e.WithArgs(tt.args...),
					e2e.ExpectExit(
						tt.exitCode,
						tt.wantOutputs...,
					),
				)
			}
		})
	}
}

// Make sure --workdir and --scratch work together nicely even when workdir is a
// relative path. Test needs to be run in non-parallel mode, because it changes
// the current working directory of the host.
func (c actionTests) ociRelWorkdirScratch(t *testing.T) {
	e2e.EnsureOCISIF(t, c.env)
	imageRef := "oci-sif:" + c.env.OCISIFPath

	testdir, cleanup := e2e.MakeTempDir(t, c.env.TestDir, "persistent-overlay-", "")
	t.Cleanup(func() {
		if !t.Failed() {
			e2e.Privileged(cleanup)
		}
	})

	const subdirName string = "mysubdir"
	if err := os.Mkdir(filepath.Join(testdir, subdirName), 0o777); err != nil {
		t.Fatalf("could not create subdirectory %q in %q: %s", subdirName, testdir, err)
	}

	// Change current working directory, with deferred undoing of change.
	prevCwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("could not get current working directory: %s", err)
	}
	defer os.Chdir(prevCwd)
	if err = os.Chdir(testdir); err != nil {
		t.Fatalf("could not change cwd to %q: %s", testdir, err)
	}

	profiles := e2e.OCIProfiles

	for _, p := range profiles {
		c.env.RunSingularity(
			t,
			e2e.AsSubtest(p.String()),
			e2e.WithProfile(p),
			e2e.WithCommand("exec"),
			e2e.WithArgs("--workdir", "./"+subdirName, "--scratch", "/myscratch", imageRef, "true"),
			e2e.ExpectExit(0),
		)
	}
}

// ociSTDPipe tests pipe stdin/stdout to singularity actions cmd
func (c actionTests) ociSTDPipe(t *testing.T) {
	e2e.EnsureOCISIF(t, c.env)
	imageRef := "oci-sif:" + c.env.OCISIFPath

	stdinTests := []struct {
		name    string
		command string
		argv    []string
		input   string
		exit    int
	}{
		{
			name:    "TrueSTDIN",
			command: "exec",
			argv:    []string{imageRef, "grep", "hi"},
			input:   "hi",
			exit:    0,
		},
		{
			name:    "FalseSTDIN",
			command: "exec",
			argv:    []string{imageRef, "grep", "hi"},
			input:   "bye",
			exit:    1,
		},
	}

	var input bytes.Buffer

	for _, tt := range stdinTests {
		c.env.RunSingularity(
			t,
			e2e.AsSubtest(tt.name),
			e2e.WithProfile(e2e.OCIUserProfile),
			e2e.WithCommand(tt.command),
			e2e.WithArgs(tt.argv...),
			e2e.WithStdin(&input),
			e2e.PreRun(func(t *testing.T) {
				input.WriteString(tt.input)
			}),
			e2e.ExpectExit(tt.exit),
		)
		input.Reset()
	}

	user := e2e.CurrentUser(t)
	stdoutTests := []struct {
		name    string
		command string
		argv    []string
		output  string
		exit    int
	}{
		{
			name:    "CwdPath",
			command: "exec",
			argv:    []string{"--cwd", "/etc", imageRef, "pwd"},
			output:  "/etc",
			exit:    0,
		},
		{
			name:    "PwdPath",
			command: "exec",
			argv:    []string{"--pwd", "/etc", imageRef, "pwd"},
			output:  "/etc",
			exit:    0,
		},
		{
			name:    "id",
			command: "exec",
			argv:    []string{imageRef, "id", "-un"},
			output:  user.Name,
			exit:    0,
		},
	}
	for _, tt := range stdoutTests {
		c.env.RunSingularity(
			t,
			e2e.AsSubtest(tt.name),
			e2e.WithProfile(e2e.OCIUserProfile),
			e2e.WithCommand(tt.command),
			e2e.WithArgs(tt.argv...),
			e2e.ExpectExit(
				tt.exit,
				e2e.ExpectOutput(e2e.ExactMatch, tt.output),
			),
		)
	}
}

func (c actionTests) actionOciNoMount(t *testing.T) {
	e2e.EnsureOCISIF(t, c.env)
	imageRef := "oci-sif:" + c.env.OCISIFPath

	tests := []struct {
		name      string
		noMount   string
		noMatch   string
		warnMatch string
		exit      int
	}{
		{
			name:    "proc",
			noMount: "proc",
			noMatch: "on /proc",
			exit:    1, // mount fails with exit code 1 when there is no `/proc`
		},
		{
			name:    "sys",
			noMount: "sys",
			noMatch: "on /sys",
			exit:    0,
		},
		{
			name:      "dev",
			noMount:   "dev",
			warnMatch: "--no-mount dev is not supported in OCI mode, ignoring.",
			exit:      0,
		},
		{
			name:    "devpts",
			noMount: "devpts",
			noMatch: "on /dev/pts",
			exit:    0,
		},
		{
			name:    "tmp",
			noMount: "tmp",
			noMatch: "on /tmp",
			exit:    0,
		},
		{
			name:    "home",
			noMount: "home",
			noMatch: "on /home",
			exit:    0,
		},
		{
			name:      "cwd",
			noMount:   "cwd",
			warnMatch: "--no-mount cwd is not supported in OCI mode, ignoring.",
			exit:      0,
		},
		//
		// TODO - singularity.conf bind path handling is not implemented yet in OCI mode.
		//        If/when it is, consider how to handle below.
		//
		// /etc/hosts & /etc/localtime are default 'bind path' entries we should
		// be able to disable by abs path. Although other 'bind path' entries
		// are ignored under '--contain' these two are handled specially in
		// addBindsMount(), so make sure that `--no-mount` applies properly
		// under contain also.
		{
			name:    "/etc/hosts",
			noMount: "/etc/hosts",
			// noMatch: "on /etc/hosts",
			warnMatch: "--no-mount /etc/hosts is not supported in OCI mode, ignoring.",
			exit:      0,
		},
		{
			name:    "/etc/localtime",
			noMount: "/etc/localtime",
			// noMatch: "on /etc/localtime",
			warnMatch: "--no-mount /etc/localtime is not supported in OCI mode, ignoring.",
			exit:      0,
		},
		// bind-paths should disable all of the bind path mounts - including both defaults
		{
			name:    "binds-paths-hosts",
			noMount: "bind-paths",
			// noMatch: "on /etc/hosts",
			warnMatch: "--no-mount bind-paths is not supported in OCI mode, ignoring.",
			exit:      0,
		},
		{
			name:    "bind-paths-localtime",
			noMount: "bind-paths",
			// noMatch: "on /etc/localtime",
			warnMatch: "--no-mount bind-paths is not supported in OCI mode, ignoring.",
			exit:      0,
		},
	}

	for _, tt := range tests {

		expectOp := e2e.ExpectOutput(e2e.UnwantedContainMatch, tt.noMatch)
		if tt.warnMatch != "" {
			expectOp = e2e.ExpectError(e2e.ContainMatch, tt.warnMatch)
		}

		c.env.RunSingularity(
			t,
			e2e.AsSubtest(tt.name),
			e2e.WithProfile(e2e.OCIUserProfile),
			e2e.WithCommand("exec"),
			e2e.WithArgs("--no-mount", tt.noMount, imageRef, "mount"),
			e2e.ExpectExit(tt.exit, expectOp),
		)
	}
}

// actionOciNoSetgoups checks that supplementary groups are visible, mapped to
// nobody, in an unprivileged OCI mode container that runs in a user namespace.
// Requires crun as runtime.
func (c actionTests) actionOciNoSetgroups(t *testing.T) {
	require.Command(t, "crun")
	e2e.EnsureOCISIF(t, c.env)
	imageRef := "oci-sif:" + c.env.OCISIFPath

	gid := int(e2e.OCIUserProfile.ContainerUser(t).GID)
	containerGroup, err := user.LookupGroupId(strconv.Itoa(gid))
	if err != nil {
		t.Fatal(err)
	}

	// Inside the e2e-tests we will be a member of our user group + single supplementary group.
	// With `--fakeroot --no-setgroups` we should see these map to:
	//    <username> nobody
	c.env.RunSingularity(
		t,
		e2e.WithProfile(e2e.OCIUserProfile),
		e2e.WithCommand("exec"),
		e2e.WithArgs("--no-setgroups", imageRef, "sh", "-c", "groups"),
		e2e.ExpectExit(
			0,
			e2e.ExpectOutput(e2e.ExactMatch, containerGroup.Name+" nobody"),
		),
	)
}

// Check that by default, the container is entered at the correct $HOME for the
// user, and $HOME in their passwd entry is correct.
// https://github.com/sylabs/singularity/issues/1791
func (c actionTests) actionOciHomeCwdPasswd(t *testing.T) {
	e2e.EnsureOCISIF(t, c.env)
	imageRef := "oci-sif:" + c.env.OCISIFPath
	for _, p := range e2e.OCIProfiles {
		cu := p.ContainerUser(t)
		// Ignore shell field as we use preserve container value. Tested previously.
		passwdLine := fmt.Sprintf("^%s:x:%d:%d:%s:%s:",
			cu.Name,
			cu.UID,
			cu.GID,
			cu.Gecos,
			cu.Dir,
		)

		c.env.RunSingularity(
			t,
			e2e.AsSubtest(p.String()+"/cwd"),
			e2e.WithProfile(p),
			e2e.WithCommand("exec"),
			e2e.WithArgs(imageRef, "pwd"),
			e2e.ExpectExit(
				0,
				e2e.ExpectOutput(e2e.ExactMatch, cu.Dir),
			),
		)

		c.env.RunSingularity(
			t,
			e2e.AsSubtest(p.String()+"/passwd"),
			e2e.WithProfile(p),
			e2e.WithCommand("exec"),
			e2e.WithArgs(imageRef, "grep", "^"+cu.Name, "/etc/passwd"),
			e2e.ExpectExit(
				0,
				e2e.ExpectOutput(e2e.RegexMatch, passwdLine),
			),
		)

	}
}
