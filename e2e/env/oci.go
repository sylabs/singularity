// Copyright (c) 2022, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package singularityenv

import (
	"os"
	"path/filepath"
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
		envFile  string
		envOpt   []string
		hostEnv  []string
		matchEnv string
		matchVal string
	}{
		{
			name:   "DefaultPathOverride",
			images: []string{ociSIFDefaultPath, nativeSIFDefaultPath}, envFile: "PATH=/",
			matchEnv: "PATH",
			matchVal: "/",
		},
		{
			name:   "DefaultPathOverrideEnvOptionPrecedence",
			images: []string{ociSIFDefaultPath, nativeSIFDefaultPath}, envOpt: []string{"PATH=/etc"},
			envFile:  "PATH=/",
			matchEnv: "PATH",
			matchVal: "/etc",
		},
		{
			name:   "DefaultPathOverrideEnvOptionPrecedence",
			images: []string{ociSIFDefaultPath, nativeSIFDefaultPath}, envOpt: []string{"PATH=/etc"},
			envFile:  "PATH=/",
			matchEnv: "PATH",
			matchVal: "/etc",
		},
		{
			name:   "AppendDefaultPath",
			images: []string{ociSIFDefaultPath, nativeSIFDefaultPath}, envFile: "APPEND_PATH=/",
			matchEnv: "PATH",
			matchVal: defaultPath + ":/",
		},
		{
			name:   "PrependDefaultPath",
			images: []string{ociSIFDefaultPath, nativeSIFDefaultPath}, envFile: "PREPEND_PATH=/",
			matchEnv: "PATH",
			matchVal: "/:" + defaultPath,
		},
		{
			name:   "DefaultLdLibraryPath",
			images: []string{ociSIFDefaultPath, nativeSIFDefaultPath}, matchEnv: "LD_LIBRARY_PATH",
			matchVal: singularityLibs,
		},
		{
			name:   "CustomLdLibraryPath",
			images: []string{ociSIFDefaultPath, nativeSIFDefaultPath}, envFile: "LD_LIBRARY_PATH=/foo",
			matchEnv: "LD_LIBRARY_PATH",
			matchVal: "/foo:" + singularityLibs,
		},
		{
			name:   "CustomTrailingCommaPath",
			images: []string{ociSIFDefaultPath, nativeSIFDefaultPath}, envFile: "LD_LIBRARY_PATH=/foo,",
			matchEnv: "LD_LIBRARY_PATH",
			matchVal: "/foo,:" + singularityLibs,
		},
	}

	for _, tt := range tests {
		for _, img := range tt.images {
			testEnv := append(os.Environ(), tt.hostEnv...)
			args := make([]string, 0)
			if tt.envOpt != nil {
				args = append(args, "--env", strings.Join(tt.envOpt, ","))
			}
			if tt.envFile != "" {
				os.WriteFile(p, []byte(tt.envFile), 0o644)
				args = append(args, "--env-file", p)
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
