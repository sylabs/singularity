// Copyright (c) 2022, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package singularityenv

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/sylabs/singularity/v4/e2e/internal/e2e"
)

func (c ctx) ociSingularityEnv(t *testing.T) {
	e2e.EnsureOCISIF(t, c.env)
	e2e.EnsureImage(t, c.env)

	ociSIFDefaultPath := c.env.OCISIFPath
	nativeSIFDefaultPath := e2e.BusyboxSIF(t)
	nativeSIFCustomPath := c.env.ImagePath
	customPath := defaultPath + ":/go/bin:/usr/local/go/bin"

	// Append or prepend this path.
	partialPath := "/foo"

	// Overwrite the path with this one.
	overwrittenPath := "/usr/bin:/bin"

	// A path with a trailing comma
	trailingCommaPath := "/usr/bin:/bin,"

	tests := []struct {
		name   string
		images []string
		path   string
		env    []string
	}{
		{
			name:   "DefaultPath",
			images: []string{ociSIFDefaultPath, nativeSIFDefaultPath},
			path:   defaultPath,
			env:    []string{},
		},
		{
			name:   "CustomPath",
			images: []string{nativeSIFCustomPath},
			path:   customPath,
			env:    []string{},
		},
		{
			name:   "AppendToDefaultPath",
			images: []string{ociSIFDefaultPath, nativeSIFDefaultPath},
			path:   defaultPath + ":" + partialPath,
			env:    []string{"SINGULARITYENV_APPEND_PATH=" + partialPath},
		},
		{
			name:   "AppendToCustomPath",
			images: []string{nativeSIFCustomPath},
			path:   customPath + ":" + partialPath,
			env:    []string{"SINGULARITYENV_APPEND_PATH=" + partialPath},
		},
		{
			name:   "PrependToDefaultPath",
			images: []string{ociSIFDefaultPath, nativeSIFDefaultPath},
			path:   partialPath + ":" + defaultPath,
			env:    []string{"SINGULARITYENV_PREPEND_PATH=" + partialPath},
		},
		{
			name:   "PrependToCustomPath",
			images: []string{nativeSIFCustomPath},
			path:   partialPath + ":" + customPath,
			env:    []string{"SINGULARITYENV_PREPEND_PATH=" + partialPath},
		},
		{
			name:   "OverwriteDefaultPath",
			images: []string{ociSIFDefaultPath, nativeSIFDefaultPath},
			path:   overwrittenPath,
			env:    []string{"SINGULARITYENV_PATH=" + overwrittenPath},
		},
		{
			name:   "OverwriteCustomPath",
			images: []string{nativeSIFCustomPath},
			path:   overwrittenPath,
			env:    []string{"SINGULARITYENV_PATH=" + overwrittenPath},
		},
		{
			name:   "OverwriteTrailingCommaPath",
			images: []string{ociSIFDefaultPath, nativeSIFDefaultPath},
			path:   trailingCommaPath,
			env:    []string{"SINGULARITYENV_PATH=" + trailingCommaPath},
		},
	}

	for _, tt := range tests {
		testEnv := append(os.Environ(), tt.env...)
		for _, img := range tt.images {
			c.env.RunSingularity(
				t,
				e2e.AsSubtest(tt.name),
				e2e.WithProfile(e2e.OCIUserProfile),
				e2e.WithCommand("exec"),
				e2e.WithEnv(testEnv),
				e2e.WithRootlessEnv(),
				e2e.WithArgs(img, "/bin/sh", "-c", "echo $PATH"),
				e2e.ExpectExit(
					0,
					e2e.ExpectOutput(e2e.ExactMatch, tt.path),
				),
			)
		}
	}
}

func (c ctx) ociEnvOption(t *testing.T) {
	e2e.EnsureOCISIF(t, c.env)
	e2e.EnsureImage(t, c.env)

	ociSIFDefaultPath := c.env.OCISIFPath
	nativeSIFDefaultPath := e2e.BusyboxSIF(t)
	nativeSIFCustomPath := c.env.ImagePath
	customPath := defaultPath + ":/go/bin:/usr/local/go/bin"

	tests := []struct {
		name     string
		images   []string
		envOpt   []string
		hostEnv  []string
		matchEnv string
		matchVal string
	}{
		{
			name:     "DefaultPath",
			images:   []string{ociSIFDefaultPath, nativeSIFDefaultPath},
			matchEnv: "PATH",
			matchVal: defaultPath,
		},
		{
			name:     "DefaultPathOverride",
			images:   []string{ociSIFDefaultPath, nativeSIFDefaultPath},
			envOpt:   []string{"PATH=/"},
			matchEnv: "PATH",
			matchVal: "/",
		},
		{
			name:     "AppendDefaultPath",
			images:   []string{ociSIFDefaultPath, nativeSIFDefaultPath},
			envOpt:   []string{"APPEND_PATH=/foo"},
			matchEnv: "PATH",
			matchVal: defaultPath + ":/foo",
		},
		{
			name:     "PrependDefaultPath",
			images:   []string{ociSIFDefaultPath, nativeSIFDefaultPath},
			envOpt:   []string{"PREPEND_PATH=/foo"},
			matchEnv: "PATH",
			matchVal: "/foo:" + defaultPath,
		},
		{
			name:     "DefaultPathImage",
			images:   []string{nativeSIFCustomPath},
			matchEnv: "PATH",
			matchVal: customPath,
		},
		{
			name:     "DefaultPathTestImageOverride",
			images:   []string{nativeSIFCustomPath},
			envOpt:   []string{"PATH=/"},
			matchEnv: "PATH",
			matchVal: "/",
		},
		{
			name:     "AppendDefaultPathTestImage",
			images:   []string{nativeSIFCustomPath},
			envOpt:   []string{"APPEND_PATH=/foo"},
			matchEnv: "PATH",
			matchVal: customPath + ":/foo",
		},
		{
			name:     "PrependDefaultPathTestImage",
			images:   []string{nativeSIFCustomPath},
			envOpt:   []string{"PREPEND_PATH=/foo"},
			matchEnv: "PATH",
			matchVal: "/foo:" + customPath,
		},
		{
			name:     "TestImageCgoEnabledDefault",
			images:   []string{nativeSIFCustomPath},
			matchEnv: "CGO_ENABLED",
			matchVal: "0",
		},
		{
			name:     "TestImageCgoEnabledOverride",
			images:   []string{nativeSIFCustomPath},
			envOpt:   []string{"CGO_ENABLED=1"},
			matchEnv: "CGO_ENABLED",
			matchVal: "1",
		},
		{
			name:     "TestImageCgoEnabledOverride_KO",
			images:   []string{nativeSIFCustomPath},
			hostEnv:  []string{"CGO_ENABLED=1"},
			matchEnv: "CGO_ENABLED",
			matchVal: "0",
		},
		{
			name:     "TestImageCgoEnabledOverrideFromEnv",
			images:   []string{nativeSIFCustomPath},
			hostEnv:  []string{"SINGULARITYENV_CGO_ENABLED=1"},
			matchEnv: "CGO_ENABLED",
			matchVal: "1",
		},
		{
			name:     "TestImageCgoEnabledOverrideEnvOptionPrecedence",
			images:   []string{nativeSIFCustomPath},
			hostEnv:  []string{"SINGULARITYENV_CGO_ENABLED=1"},
			envOpt:   []string{"CGO_ENABLED=2"},
			matchEnv: "CGO_ENABLED",
			matchVal: "2",
		},
		{
			name:     "TestImageCgoEnabledOverrideEmpty",
			images:   []string{nativeSIFCustomPath},
			envOpt:   []string{"CGO_ENABLED="},
			matchEnv: "CGO_ENABLED",
			matchVal: "",
		},
		{
			name:     "TestMultiLine",
			images:   []string{ociSIFDefaultPath, nativeSIFDefaultPath},
			envOpt:   []string{"MULTI=Hello\nWorld"},
			matchEnv: "MULTI",
			matchVal: "Hello\nWorld",
		},
		{
			name:     "TestEscapedNewline",
			images:   []string{ociSIFDefaultPath, nativeSIFDefaultPath},
			envOpt:   []string{"ESCAPED=Hello\\nWorld"},
			matchEnv: "ESCAPED",
			matchVal: "Hello\\nWorld",
		},
		{
			name:     "TestInvalidKey",
			images:   []string{ociSIFDefaultPath, nativeSIFDefaultPath},
			envOpt:   []string{"BASH_FUNC_ml%%=TEST"},
			matchEnv: "BASH_FUNC_ml%%",
			matchVal: "",
		},
		{
			name:     "TestDefaultLdLibraryPath",
			images:   []string{ociSIFDefaultPath, nativeSIFDefaultPath},
			matchEnv: "LD_LIBRARY_PATH",
			matchVal: singularityLibs,
		},
		{
			name:     "TestCustomTrailingCommaPath",
			images:   []string{ociSIFDefaultPath, nativeSIFDefaultPath},
			envOpt:   []string{"LD_LIBRARY_PATH=/foo,"},
			matchEnv: "LD_LIBRARY_PATH",
			matchVal: "/foo,:" + singularityLibs,
		},
		{
			name:     "TestCustomLdLibraryPath",
			images:   []string{ociSIFDefaultPath, nativeSIFDefaultPath},
			envOpt:   []string{"LD_LIBRARY_PATH=/foo"},
			matchEnv: "LD_LIBRARY_PATH",
			matchVal: "/foo:" + singularityLibs,
		},
		{
			name:     "SINGULARITY_NAME_OCI_SIF",
			images:   []string{ociSIFDefaultPath},
			matchEnv: "SINGULARITY_NAME",
			matchVal: ociSIFDefaultPath,
		},
		{
			name:     "SINGULARITY_NAME_NATIVE_SIF",
			images:   []string{nativeSIFDefaultPath},
			matchEnv: "SINGULARITY_NAME",
			matchVal: nativeSIFDefaultPath,
		},
	}

	for _, tt := range tests {
		testEnv := append(os.Environ(), tt.hostEnv...)
		args := make([]string, 0)
		if tt.envOpt != nil {
			args = append(args, "--env", strings.Join(tt.envOpt, ","))
		}
		for _, img := range tt.images {
			args = append(args, img, "/bin/sh", "-c", "echo \"${"+tt.matchEnv+"}\"")
			c.env.RunSingularity(
				t,
				e2e.AsSubtest(tt.name),
				e2e.WithProfile(e2e.OCIUserProfile),
				e2e.WithCommand("exec"),
				e2e.WithEnv(testEnv),
				e2e.WithRootlessEnv(),
				e2e.WithArgs(args...),
				e2e.ExpectExit(
					0,
					e2e.ExpectOutput(e2e.ExactMatch, tt.matchVal),
				),
			)
		}
	}
}

func (c ctx) ociEnvFile(t *testing.T) {
	e2e.EnsureOCISIF(t, c.env)
	e2e.EnsureImage(t, c.env)

	ociSIFDefaultPath := c.env.OCISIFPath
	nativeSIFDefaultPath := e2e.BusyboxSIF(t)

	dir, cleanup := e2e.MakeTempDir(t, c.env.TestDir, "envfile-", "")
	defer cleanup(t)
	p := filepath.Join(dir, "env.file")

	tests := []struct {
		name     string
		images   []string
		envFiles []string
		envOpt   []string
		hostEnv  []string
		matchEnv string
		matchVal string
	}{
		{
			name:     "DefaultPathOverride",
			images:   []string{ociSIFDefaultPath, nativeSIFDefaultPath},
			envFiles: []string{"PATH=/"},
			matchEnv: "PATH",
			matchVal: "/",
		},
		{
			name:     "DefaultPathOverrideEnvOptionPrecedence",
			images:   []string{ociSIFDefaultPath, nativeSIFDefaultPath},
			envOpt:   []string{"PATH=/etc"},
			envFiles: []string{"PATH=/"},
			matchEnv: "PATH",
			matchVal: "/etc",
		},
		{
			name:     "DefaultPathOverrideEnvFileOptionPrecedence",
			images:   []string{ociSIFDefaultPath, nativeSIFDefaultPath},
			envFiles: []string{"PATH=/", "PATH=/etc"},
			matchEnv: "PATH",
			matchVal: "/etc",
		},
		{
			name:     "DefaultPathOverrideEnvAndEnvFileOptionPrecedence",
			images:   []string{ociSIFDefaultPath, nativeSIFDefaultPath},
			envOpt:   []string{"PATH=/etc"},
			envFiles: []string{"PATH=/", "PATH=/foo"},
			matchEnv: "PATH",
			matchVal: "/etc",
		},
		{
			name:     "AppendDefaultPath",
			images:   []string{ociSIFDefaultPath, nativeSIFDefaultPath},
			envFiles: []string{"APPEND_PATH=/"},
			matchEnv: "PATH",
			matchVal: defaultPath + ":/",
		},
		{
			name:     "PrependDefaultPath",
			images:   []string{ociSIFDefaultPath, nativeSIFDefaultPath},
			envFiles: []string{"PREPEND_PATH=/"},
			matchEnv: "PATH",
			matchVal: "/:" + defaultPath,
		},
		{
			name:     "DefaultLdLibraryPath",
			images:   []string{ociSIFDefaultPath, nativeSIFDefaultPath},
			matchEnv: "LD_LIBRARY_PATH",
			matchVal: singularityLibs,
		},
		{
			name:     "CustomLdLibraryPath",
			images:   []string{ociSIFDefaultPath, nativeSIFDefaultPath},
			envFiles: []string{"LD_LIBRARY_PATH=/foo"},
			matchEnv: "LD_LIBRARY_PATH",
			matchVal: "/foo:" + singularityLibs,
		},
		{
			name:     "CustomTrailingCommaPath",
			images:   []string{ociSIFDefaultPath, nativeSIFDefaultPath},
			envFiles: []string{"LD_LIBRARY_PATH=/foo,"},
			matchEnv: "LD_LIBRARY_PATH",
			matchVal: "/foo,:" + singularityLibs,
		},
		{
			name:     "HostEnvUnset",
			images:   []string{ociSIFDefaultPath, nativeSIFDefaultPath},
			envFiles: []string{"HELLO=$YOU"},
			matchEnv: "HELLO",
			matchVal: "",
		},
		{
			name:     "HostEnvSet",
			images:   []string{ociSIFDefaultPath, nativeSIFDefaultPath},
			envFiles: []string{"HELLO=$YOU"},
			hostEnv:  []string{"YOU=YOU"},
			matchEnv: "HELLO",
			matchVal: "YOU",
		},
	}

	for _, tt := range tests {
		for _, img := range tt.images {
			testEnv := append(os.Environ(), tt.hostEnv...)
			args := make([]string, 0)
			if tt.envOpt != nil {
				args = append(args, "--env", strings.Join(tt.envOpt, ","))
			}
			if len(tt.envFiles) > 0 {
				for i, envFile := range tt.envFiles {
					filename := fmt.Sprint(p, i)
					os.WriteFile(filename, []byte(envFile), 0o644)
					args = append(args, "--env-file", filename)
				}
			}
			args = append(args, img, "/bin/sh", "-c", "echo $"+tt.matchEnv)

			c.env.RunSingularity(
				t,
				e2e.AsSubtest(tt.name),
				e2e.WithProfile(e2e.OCIUserProfile),
				e2e.WithCommand("exec"),
				e2e.WithEnv(testEnv),
				e2e.WithRootlessEnv(),
				e2e.WithArgs(args...),
				e2e.ExpectExit(
					0,
					e2e.ExpectOutput(e2e.ExactMatch, tt.matchVal),
				),
			)
		}
	}
}

// In OCI mode, default emulates compat which implies --no-eval.
// OCI-SIF images will never evaluate env vars, however for native SIF
// we should see evaluation when we use --no-compat, so test that!
func (c ctx) ociNativeEnvEval(t *testing.T) {
	e2e.EnsureImage(t, c.env)

	testArgs := []string{"/bin/sh", "-c", "echo $WHO"}

	tests := []struct {
		name         string
		env          []string
		args         []string
		noCompat     bool
		noEval       bool
		expectOutput string
	}{
		// Docker/OCI behavior (default for OCI mode)
		{
			name:         "no env",
			args:         testArgs,
			env:          []string{},
			noCompat:     false,
			expectOutput: "",
		},
		{
			name:         "string env",
			args:         testArgs,
			env:          []string{"SINGULARITYENV_WHO=ME"},
			noCompat:     false,
			expectOutput: "ME",
		},
		{
			name:         "env var",
			args:         testArgs,
			env:          []string{"SINGULARITYENV_WHO=$HOME"},
			noCompat:     false,
			expectOutput: "$HOME",
		},
		{
			name:         "double quoted env var",
			args:         testArgs,
			env:          []string{"SINGULARITYENV_WHO=\"$HOME\""},
			noCompat:     false,
			expectOutput: "\"$HOME\"",
		},
		{
			name:         "single quoted env var",
			args:         testArgs,
			env:          []string{"SINGULARITYENV_WHO='$HOME'"},
			noCompat:     false,
			expectOutput: "'$HOME'",
		},
		{
			name:         "escaped env var",
			args:         testArgs,
			env:          []string{"SINGULARITYENV_WHO=\\$HOME"},
			noCompat:     false,
			expectOutput: "\\$HOME",
		},
		{
			name:         "subshell env",
			args:         testArgs,
			env:          []string{"SINGULARITYENV_WHO=$(id -u)"},
			noCompat:     false,
			expectOutput: "$(id -u)",
		},
		{
			name:         "double quoted subshell env",
			args:         testArgs,
			env:          []string{"SINGULARITYENV_WHO=\"$(id -u)\""},
			noCompat:     false,
			expectOutput: "\"$(id -u)\"",
		},
		{
			name:         "single quoted subshell env",
			args:         testArgs,
			env:          []string{"SINGULARITYENV_WHO='$(id -u)'"},
			noCompat:     false,
			expectOutput: "'$(id -u)'",
		},
		{
			name:         "escaped subshell env",
			args:         testArgs,
			env:          []string{"SINGULARITYENV_WHO=\\$(id -u)"},
			noCompat:     false,
			expectOutput: "\\$(id -u)",
		},
		// Singularity historic behavior (native SIF with --no-compat)
		{
			name:         "no-compat/no env",
			args:         testArgs,
			env:          []string{},
			noCompat:     true,
			expectOutput: "",
		},
		{
			name:         "no-compat/string env",
			args:         testArgs,
			env:          []string{"SINGULARITYENV_WHO=ME"},
			noCompat:     true,
			expectOutput: "ME",
		},
		{
			name:         "no-compat/env var",
			args:         testArgs,
			env:          []string{"SINGULARITYENV_WHO=$HOME"},
			noCompat:     true,
			expectOutput: e2e.OCIUserProfile.ContainerUser(t).Dir,
		},
		{
			name:         "no-compat/double quoted env var",
			args:         testArgs,
			env:          []string{"SINGULARITYENV_WHO=\"$HOME\""},
			noCompat:     true,
			expectOutput: "\"" + e2e.OCIUserProfile.ContainerUser(t).Dir + "\"",
		},
		{
			name:         "no-compat/single quoted env var",
			args:         testArgs,
			env:          []string{"SINGULARITYENV_WHO='$HOME'"},
			noCompat:     true,
			expectOutput: "'" + e2e.OCIUserProfile.ContainerUser(t).Dir + "'",
		},
		{
			name:         "no-compat/escaped env var",
			args:         testArgs,
			env:          []string{"SINGULARITYENV_WHO=\\$HOME"},
			noCompat:     true,
			expectOutput: "$HOME",
		},
		{
			name:         "no-compat/subshell env",
			args:         testArgs,
			env:          []string{"SINGULARITYENV_WHO=$(id -u)"},
			noCompat:     true,
			expectOutput: strconv.Itoa(os.Getuid()),
		},
		{
			name:         "no-compat/double quoted subshell env",
			args:         testArgs,
			env:          []string{"SINGULARITYENV_WHO=\"$(id -u)\""},
			noCompat:     true,
			expectOutput: "\"" + strconv.Itoa(os.Getuid()) + "\"",
		},
		{
			name:         "no-compat/single quoted subshell env",
			args:         testArgs,
			env:          []string{"SINGULARITYENV_WHO='$(id -u)'"},
			noCompat:     true,
			expectOutput: "'" + strconv.Itoa(os.Getuid()) + "'",
		},
		{
			name:         "no-compat/escaped subshell env",
			args:         testArgs,
			env:          []string{"SINGULARITYENV_WHO=\\$(id -u)"},
			noCompat:     true,
			expectOutput: "$(id -u)",
		},
		// Finally check using --no-eval with --no-compat turns evaluation back off.
		{
			name:         "no-compat/noeval env var",
			args:         testArgs,
			env:          []string{"SINGULARITYENV_WHO=$HOME"},
			noCompat:     true,
			noEval:       true,
			expectOutput: "$HOME",
		},
	}

	for _, tt := range tests {
		cmdArgs := []string{}
		if tt.noCompat {
			cmdArgs = append(cmdArgs, "--no-compat")
		}
		if tt.noEval {
			cmdArgs = append(cmdArgs, "--no-eval")
		}
		cmdArgs = append(cmdArgs, c.env.ImagePath)
		cmdArgs = append(cmdArgs, tt.args...)
		testEnv := append(os.Environ(), tt.env...)
		c.env.RunSingularity(
			t,
			e2e.AsSubtest(tt.name),
			e2e.WithEnv(testEnv),
			e2e.WithProfile(e2e.OCIUserProfile),
			e2e.WithCommand("exec"),
			e2e.WithArgs(cmdArgs...),
			e2e.ExpectExit(0,
				e2e.ExpectOutput(e2e.ExactMatch, tt.expectOutput),
			),
		)
	}
}

// In OCI mode, default emulates compat which implies --cleanenv.
// Ensure we see host env vars with --no-compat
func (c ctx) ociNoCompatHost(t *testing.T) {
	e2e.EnsureImage(t, c.env)

	tests := []struct {
		name         string
		env          []string
		args         []string
		noCompat     bool
		cleanenv     bool
		expectOutput string
	}{
		{
			name:         "no host env",
			args:         []string{"/bin/sh", "-c", "echo $WHO"},
			env:          []string{},
			cleanenv:     false,
			expectOutput: "",
		},
		{
			name:         "set host env",
			args:         []string{"/bin/sh", "-c", "echo $WHO"},
			env:          []string{"WHO=ME"},
			cleanenv:     false,
			expectOutput: "ME",
		},
		{
			name:         "override host env",
			args:         []string{"/bin/sh", "-c", "echo $WHO"},
			env:          []string{"WHO=ME", "SINGULARITYENV_WHO=YOU"},
			cleanenv:     false,
			expectOutput: "YOU",
		},
		{
			name:         "no override container",
			args:         []string{"/bin/sh", "-c", "echo $CGO_ENABLED"},
			env:          []string{"CGO_ENABLED=2"},
			cleanenv:     false,
			expectOutput: "0",
		},

		// Finally check using --no-eval with --cleanenv turns host envs back off.
		{
			name:         "cleanenv",
			args:         []string{"/bin/sh", "-c", "echo $WHO"},
			env:          []string{"WHO=ME"},
			cleanenv:     true,
			expectOutput: "",
		},
	}

	for _, tt := range tests {
		cmdArgs := []string{"--no-compat"}
		if tt.cleanenv {
			cmdArgs = append(cmdArgs, "--cleanenv")
		}
		cmdArgs = append(cmdArgs, c.env.ImagePath)
		cmdArgs = append(cmdArgs, tt.args...)
		testEnv := append(os.Environ(), tt.env...)
		c.env.RunSingularity(
			t,
			e2e.AsSubtest(tt.name),
			e2e.WithEnv(testEnv),
			e2e.WithProfile(e2e.OCIUserProfile),
			e2e.WithCommand("exec"),
			e2e.WithArgs(cmdArgs...),
			e2e.ExpectExit(0,
				e2e.ExpectOutput(e2e.ExactMatch, tt.expectOutput),
			),
		)
	}
}
