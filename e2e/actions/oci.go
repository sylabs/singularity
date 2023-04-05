// Copyright (c) 2022, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package actions

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"text/template"

	cdispecs "github.com/container-orchestrated-devices/container-device-interface/specs-go"
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
	homePath := filepath.Join("/home", basename)

	tests := []struct {
		name        string
		argv        []string
		exit        int
		wantOutputs []e2e.SingularityCmdResultOp
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
			name: "Pwd",
			argv: []string{"--pwd", "/tmp", imageRef, "pwd"},
			exit: 0,
			wantOutputs: []e2e.SingularityCmdResultOp{
				e2e.ExpectOutput(e2e.ExactMatch, "/tmp"),
			},
		},
		{
			name: "Cwd",
			argv: []string{"--cwd", "/tmp", imageRef, "pwd"},
			exit: 0,
			wantOutputs: []e2e.SingularityCmdResultOp{
				e2e.ExpectOutput(e2e.ExactMatch, "/tmp"),
			},
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
					e2e.ExpectExit(tt.exit, tt.wantOutputs...),
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

	hostHomeDir := filepath.Join(workspace, "home")

	createWorkspaceDirs := func(t *testing.T) {
		e2e.Privileged(func(t *testing.T) {
			if err := os.RemoveAll(hostCanaryDir); err != nil && !os.IsNotExist(err) {
				t.Fatalf("failed to delete canary_dir: %s", err)
			}
			if err := os.RemoveAll(hostHomeDir); err != nil && !os.IsNotExist(err) {
				t.Fatalf("failed to delete workspace home: %s", err)
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
		if err := fs.Mkdir(hostHomeDir, 0o777); err != nil {
			t.Fatalf("failed to create workspace home directory: %s", err)
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
	e2e.EnsureOCIArchive(t, c.env)
	imageRef := "oci-archive:" + c.env.OCIArchivePath

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
					execCmd := "/bin/env"

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
