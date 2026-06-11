// Copyright (c) 2018-2026, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package layout

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/sylabs/singularity/v4/internal/pkg/test"
	"github.com/sylabs/singularity/v4/internal/pkg/util/fs"
)

func TestLayout(t *testing.T) {
	test.DropPrivilege(t)
	defer test.ResetPrivilege(t)

	uid := os.Getuid()
	gid := os.Getgid()

	groups, err := os.Getgroups()
	if err != nil {
		t.Fatal(err)
	}
	for _, g := range groups {
		if g != gid {
			gid = g
			break
		}
	}

	dir := t.TempDir()

	// Uninitialized Manager.
	mgr := &Manager{}
	if err := mgr.AddDir("/etc"); err == nil {
		t.Errorf("should have failed with uninitialized root path")
	}
	if err := mgr.AddFile("/etc/passwd", nil); err == nil {
		t.Errorf("should have failed with uninitialized root path")
	}
	if err := mgr.AddSymlink("/etc/symlink", "/etc/passwd"); err == nil {
		t.Errorf("should have failed with uninitialized root path")
	}
	if err := mgr.Create(); err == nil {
		t.Errorf("should have failed with uninitialized root path")
	}

	// Initialize with root.
	_, err = NewManager("/fakedirectory")
	if err == nil {
		t.Error("should have failed with invalid root path directory")
	}
	mgr, err = NewManager(dir)
	if err != nil {
		t.Fatal(err)
	}

	if err := mgr.AddDir("etc"); err == nil {
		t.Errorf("should have failed with non absolute path")
	}
	if err := mgr.AddDir("/etc"); err != nil {
		t.Error(err)
	}
	if err := mgr.AddDir("/etc"); err == nil {
		t.Error("should have failed with existent path")
	}

	if _, err := mgr.GetPath("/etcd"); err == nil {
		t.Errorf("should have failed with non existent path")
	}

	if err := mgr.AddFile("/etc/passwd", []byte("hello")); err != nil {
		t.Error(err)
	}
	if err := mgr.AddSymlink("/etc/symlink", "/etc/passwd"); err != nil {
		t.Error(err)
	}

	if err := mgr.Chmod("/etc", 0o777); err != nil {
		t.Error(err)
	}
	if err := mgr.Chmod("/etcd", 0o777); err == nil {
		t.Error("should have failed with non existent path")
	}

	if err := mgr.Chown("/etc", uid, gid); err != nil {
		t.Error(err)
	}
	if err := mgr.Chown("/etcd", uid, gid); err == nil {
		t.Error("should have failed with non existent path")
	}

	if err := mgr.Chmod("/etc/passwd", 0o600); err != nil {
		t.Error(err)
	}
	if err := mgr.Chown("/etc/passwd", uid, gid); err != nil {
		t.Error(err)
	}
	if err := mgr.Chown("/etc/symlink", uid, gid); err != nil {
		t.Error(err)
	}

	if err := mgr.Create(); err != nil {
		t.Fatal(err)
	}
	if p, err := mgr.GetPath("/etc"); err == nil {
		if !fs.IsDir(p) {
			t.Errorf("failed to create directory %s", p)
		}
	} else {
		t.Error(err)
	}
	if p, err := mgr.GetPath("/etc/passwd"); err != nil {
		t.Error(err)
	} else {
		if !fs.IsFile(p) {
			t.Errorf("failed to create file %s", p)
		}
	}
	if p, err := mgr.GetPath("/etc/symlink"); err != nil {
		t.Error(err)
	} else {
		if !fs.IsLink(p) {
			t.Errorf("failed to create symlink %s", p)
		}
	}

	if err := mgr.AddSymlink("/etc/symlink2", "/etc/passwd"); err != nil {
		t.Error(err)
	}
	if err := mgr.Update(); err != nil {
		t.Fatal(err)
	}
	if p, err := mgr.GetPath("/etc/symlink2"); err != nil {
		t.Error(err)
	} else {
		if !fs.IsLink(p) {
			t.Errorf("failed to create symlink %s", p)
		}
	}
}

type nestedBindOverride struct {
	path     string
	realpath string
}

type nestedBindSetupResult struct {
	overrides   []nestedBindOverride
	wantDirs    []string
	wantFiles   []string
	wantNoFiles []string
	wantErr     string
}

type nestedBindTestCase struct {
	name     string
	setup    func(*testing.T) nestedBindSetupResult
	addDirs  []string
	addFiles []string
}

func runNestedBindTargetCase(t *testing.T, tt nestedBindTestCase) {
	rootDir := t.TempDir()
	mgr, err := NewManager(rootDir)
	if err != nil {
		t.Fatal(err)
	}

	result := tt.setup(t)
	for _, ov := range result.overrides {
		mgr.overrideDir(ov.path, ov.realpath)
	}
	for _, dir := range tt.addDirs {
		if err := mgr.AddDir(dir); err != nil {
			t.Fatal(err)
		}
	}
	for _, file := range tt.addFiles {
		if err := mgr.AddFile(file, nil); err != nil {
			t.Fatal(err)
		}
	}

	err = mgr.Create()

	if result.wantErr != "" {
		assert.Error(t, err)
		if err != nil {
			assert.Contains(t, err.Error(), result.wantErr)
		}
	} else if err != nil {
		t.Fatal(err)
	}

	for _, dir := range result.wantDirs {
		assert.DirExists(t, dir)
	}
	for _, file := range result.wantFiles {
		assert.FileExists(t, file)
	}
	for _, file := range result.wantNoFiles {
		assert.NoFileExists(t, file)
	}
}

func TestNestedBindTargets(t *testing.T) {
	tests := []nestedBindTestCase{
		{
			name: "file single level",
			setup: func(t *testing.T) nestedBindSetupResult {
				hostCanaryDir := t.TempDir()
				return nestedBindSetupResult{
					overrides: []nestedBindOverride{
						{path: "/canary", realpath: hostCanaryDir},
					},
					wantDirs: []string{
						filepath.Join(hostCanaryDir, "dir2"),
					},
					wantFiles: []string{
						filepath.Join(hostCanaryDir, "dir2", "nested"),
					},
				}
			},
			addDirs:  []string{"/canary/dir2"},
			addFiles: []string{"/canary/dir2/nested"},
		},
		{
			name: "dir single level",
			setup: func(t *testing.T) nestedBindSetupResult {
				hostCanaryDir := t.TempDir()
				return nestedBindSetupResult{
					overrides: []nestedBindOverride{
						{path: "/canary", realpath: hostCanaryDir},
					},
					wantDirs: []string{
						filepath.Join(hostCanaryDir, "dir2", "nested"),
					},
				}
			},
			addDirs: []string{"/canary/dir2/nested"},
		},
		{
			name: "file double level",
			setup: func(t *testing.T) nestedBindSetupResult {
				hostCanaryDir := t.TempDir()
				return nestedBindSetupResult{
					overrides: []nestedBindOverride{
						{path: "/canary", realpath: hostCanaryDir},
						{path: "/canary/dir2", realpath: hostCanaryDir},
					},
					wantDirs: []string{
						filepath.Join(hostCanaryDir, "dir2"),
					},
					wantFiles: []string{
						filepath.Join(hostCanaryDir, "nested"),
					},
				}
			},
			addDirs:  []string{"/canary/dir2"},
			addFiles: []string{"/canary/dir2/nested"},
		},
		{
			name: "dir double level",
			setup: func(t *testing.T) nestedBindSetupResult {
				hostCanaryDir := t.TempDir()
				return nestedBindSetupResult{
					overrides: []nestedBindOverride{
						{path: "/canary", realpath: hostCanaryDir},
						{path: "/canary/dir2", realpath: hostCanaryDir},
					},
					wantDirs: []string{
						filepath.Join(hostCanaryDir, "dir2"),
						filepath.Join(hostCanaryDir, "nested"),
					},
				}
			},
			addDirs: []string{"/canary/dir2/nested"},
		},
		{
			name: "dir below override",
			setup: func(t *testing.T) nestedBindSetupResult {
				workDir := t.TempDir()
				hostTmpDir := filepath.Join(workDir, "tmp")
				hostCanaryDir := t.TempDir()

				if err := os.Mkdir(hostTmpDir, 0o755); err != nil {
					t.Fatal(err)
				}

				return nestedBindSetupResult{
					overrides: []nestedBindOverride{
						{path: "/tmp", realpath: hostTmpDir},
						{path: "/tmp/canary/dir", realpath: hostCanaryDir},
					},
					wantDirs: []string{
						filepath.Join(hostTmpDir, "canary", "dir"),
					},
				}
			},
			addDirs: []string{"/tmp/canary/dir"},
		},
		{
			name: "file below override",
			setup: func(t *testing.T) nestedBindSetupResult {
				workDir := t.TempDir()
				hostTmpDir := filepath.Join(workDir, "tmp")
				hostCanaryDir := t.TempDir()

				if err := os.Mkdir(hostTmpDir, 0o755); err != nil {
					t.Fatal(err)
				}

				return nestedBindSetupResult{
					overrides: []nestedBindOverride{
						{path: "/tmp", realpath: hostTmpDir},
						{path: "/tmp/canary/dir", realpath: hostCanaryDir},
					},
					wantDirs: []string{
						filepath.Join(hostTmpDir, "canary", "dir"),
					},
					wantFiles: []string{
						filepath.Join(hostCanaryDir, "nested"),
					},
				}
			},
			addDirs:  []string{"/tmp/canary/dir"},
			addFiles: []string{"/tmp/canary/dir/nested"},
		},
	}

	test.DropPrivilege(t)
	defer test.ResetPrivilege(t)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runNestedBindTargetCase(t, tt)
		})
	}
}

func newOverrideManager(t *testing.T, overrides []nestedBindOverride) *Manager {
	t.Helper()
	mgr, err := NewManager(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, ov := range overrides {
		mgr.overrideDir(ov.path, ov.realpath)
	}
	return mgr
}

func TestOverrideFor(t *testing.T) {
	tests := []struct {
		name               string
		overrides          []nestedBindOverride
		path               string
		wantLayoutOverride string
		wantOverrideSource string
		wantPathRel        string
	}{
		{
			name:               "no overrides",
			overrides:          nil,
			path:               "/canary/dir2",
			wantLayoutOverride: "",
			wantOverrideSource: "",
			wantPathRel:        "",
		},
		{
			name: "no match",
			overrides: []nestedBindOverride{
				{path: "/canary", realpath: "/host/canary"},
			},
			path:               "/other/dir2",
			wantLayoutOverride: "",
			wantOverrideSource: "",
			wantPathRel:        "",
		},
		{
			name: "no self override",
			overrides: []nestedBindOverride{
				{path: "/canary", realpath: "/host/canary"},
			},
			path:               "/canary",
			wantLayoutOverride: "",
			wantOverrideSource: "",
			wantPathRel:        "",
		},
		{
			name: "parent override",
			overrides: []nestedBindOverride{
				{path: "/canary", realpath: "/host/canary"},
			},
			path:               "/canary/dir2",
			wantLayoutOverride: "/canary",
			wantOverrideSource: "/host/canary",
			wantPathRel:        "dir2",
		},
		{
			name: "deeper override",
			overrides: []nestedBindOverride{
				{path: "/canary", realpath: "/host/canary"},
			},
			path:               "/canary/a/b/c",
			wantLayoutOverride: "/canary",
			wantOverrideSource: "/host/canary",
			wantPathRel:        "a/b/c",
		},
		{
			name: "nearest ancestor",
			overrides: []nestedBindOverride{
				{path: "/canary", realpath: "/host/canary"},
				{path: "/canary/dir2", realpath: "/host/dir2"},
			},
			path:               "/canary/dir2/nested",
			wantLayoutOverride: "/canary/dir2",
			wantOverrideSource: "/host/dir2",
			wantPathRel:        "nested",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mgr := newOverrideManager(t, tt.overrides)
			layoutPath, source, rel := mgr.overrideFor(tt.path)
			assert.Equal(t, tt.wantLayoutOverride, layoutPath, "layoutPath")
			assert.Equal(t, tt.wantOverrideSource, source, "source")
			assert.Equal(t, tt.wantPathRel, rel, "rel")
		})
	}
}

func TestNestedBindTarget(t *testing.T) {
	tests := []struct {
		name               string
		overrides          []nestedBindOverride
		path               string
		wantLayoutOverride string
		wantTarget         string
	}{
		{
			name:               "no overrides",
			overrides:          nil,
			path:               "/canary/dir2",
			wantLayoutOverride: "",
			wantTarget:         "",
		},
		{
			name: "no match",
			overrides: []nestedBindOverride{
				{path: "/canary", realpath: "/host/canary"},
			},
			path:               "/other/dir2",
			wantLayoutOverride: "",
			wantTarget:         "",
		},
		{
			name: "no self override",
			overrides: []nestedBindOverride{
				{path: "/canary", realpath: "/host/canary"},
			},
			path:               "/canary",
			wantLayoutOverride: "",
			wantTarget:         "",
		},
		{
			name: "parent override",
			overrides: []nestedBindOverride{
				{path: "/canary", realpath: "/host/canary"},
			},
			path:               "/canary/dir2",
			wantLayoutOverride: "/canary",
			wantTarget:         "/host/canary/dir2",
		},
		{
			name: "deeper override",
			overrides: []nestedBindOverride{
				{path: "/canary", realpath: "/host/canary"},
			},
			path:               "/canary/a/b/c",
			wantLayoutOverride: "/canary",
			wantTarget:         "/host/canary/a/b/c",
		},
		{
			name: "nearest ancestor",
			overrides: []nestedBindOverride{
				{path: "/canary", realpath: "/host/canary"},
				{path: "/canary/dir2", realpath: "/host/dir2"},
			},
			path:               "/canary/dir2/nested",
			wantLayoutOverride: "/canary/dir2",
			wantTarget:         "/host/dir2/nested",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mgr := newOverrideManager(t, tt.overrides)
			layoutPath, target := mgr.nestedBindTargetFor(tt.path)
			assert.Equal(t, tt.wantLayoutOverride, layoutPath, "layoutPath")
			assert.Equal(t, tt.wantTarget, target, "target")
		})
	}
}
