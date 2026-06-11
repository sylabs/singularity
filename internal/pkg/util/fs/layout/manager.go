// Copyright (c) 2018-2026, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package layout

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/sylabs/singularity/v4/internal/pkg/util/fs"
	"github.com/sylabs/singularity/v4/pkg/sylog"
)

const (
	dirMode  os.FileMode = 0o755
	fileMode os.FileMode = 0o644
)

type file struct {
	created bool
	mode    os.FileMode
	uid     int
	gid     int
	content []byte
}

type dir struct {
	created bool
	mode    os.FileMode
	uid     int
	gid     int
}

type symlink struct {
	created bool
	uid     int
	gid     int
	target  string
}

type dirOverride struct {
	source            string
	nestedBindTargets []string
}

// Manager manages a filesystem layout in a given path
type Manager struct {
	// VFS is the virtual filesystem used to create the layout.
	VFS DefaultVFS

	// DirMode and FileMode are the default permissions for directories and
	// files created by the manager.
	DirMode  os.FileMode
	FileMode os.FileMode

	rootPath string
	entries  map[string]any
	dirs     []*dir

	// dirOverrides accumulates binds / nested bind information. The key of the map is
	// the session directory that will be overridden by the bind. The value is a
	// struct containing the source of the bind and any nested bind targets that
	// need to be created inside this bind so that additional nested binds have a valid
	// mount target.
	//
	// For example, with `--bind /data:/foo --bind /tmp:/foo/bar` and an overlay
	// layout the map will contain:
	//
	// "/overlay-lowerdir/foo": {
	//    source: "/data",
	//    nestedBindTargets: ["/data/bar"],
	// }, "/overlay-lowerdir/foo/bar": {
	//    source: "/tmp",
	//    nestedBindTargets: <nil>,
	// }
	//
	// With an underlay layout the map will contain:
	//
	// "/underlay/foo": {
	//    source: "/data",
	//    nestedBindTargets: ["/data/bar"],
	// }, "/underlay/foo/bar": {
	//    source: "/tmp",
	//    nestedBindTargets: <nil>,
	// }
	//
	// In both cases, the manager will ensure that /data/bar is created before
	// the second bind is mounted, so that the nested bind has a valid target to
	// mount on.
	dirOverrides map[string]*dirOverride
}

func NewManager(path string) (*Manager, error) {
	m := &Manager{VFS: DefaultVFS{}}
	if err := m.setRootPath(path); err != nil {
		return nil, err
	}
	return m, nil
}

func (m *Manager) checkPath(path string, checkExist bool) (string, error) {
	if m.entries == nil {
		return "", fmt.Errorf("root path is not set")
	}
	p := filepath.Clean(path)
	if !filepath.IsAbs(p) {
		return "", fmt.Errorf("path %s is not an absolute path", p)
	}
	if checkExist {
		if _, ok := m.entries[p]; ok {
			return "", fmt.Errorf("%s already exists in layout", p)
		}
	} else {
		if _, ok := m.entries[p]; !ok {
			return "", fmt.Errorf("%s doesn't exist in layout", p)
		}
	}
	return p, nil
}

// addDirs adds path and all of its parent directories to the layout if
// they don't exist. It also tracks nested bind targets for any directories that
// are overridden for a bind.
func (m *Manager) addDirs(path string) error {
	uid := os.Getuid()
	gid := os.Getgid()

	parts := strings.Split(path, string(os.PathSeparator))
	l := len(parts)
	p := ""
	for i := 1; i < l; i++ {
		s := parts[i : i+1][0]
		p += "/" + s
		if s != "" {
			if _, ok := m.entries[p]; !ok {
				d := &dir{mode: m.DirMode, uid: uid, gid: gid}
				m.entries[p] = d
				m.dirs = append(m.dirs, d)
				// If this directory is under an overridden directory, then ensure
				// we track the corresponding nested bind target.
				if layoutPath, target := m.nestedBindTargetFor(p); target != "" {
					sylog.Debugf("Adding nested bind target %s for overridden directory %s", target, layoutPath)
					if err := m.addNestedBindTarget(layoutPath, target); err != nil {
						return err
					}
				}
			}
		}
	}
	return nil
}

// setRootPath sets layout root path
func (m *Manager) setRootPath(path string) error {
	if !fs.IsDir(path) {
		return fmt.Errorf("%s is not a directory or doesn't exists", path)
	}
	m.rootPath = filepath.Clean(path)
	if m.entries == nil {
		m.entries = make(map[string]any)
	} else {
		return fmt.Errorf("root path is already set")
	}
	if m.dirOverrides == nil {
		m.dirOverrides = map[string]*dirOverride{}
	}
	if m.dirs == nil {
		m.dirs = make([]*dir, 0)
	}
	if m.DirMode == 0o000 {
		m.DirMode = dirMode
	}
	if m.FileMode == 0o000 {
		m.FileMode = fileMode
	}
	d := &dir{mode: m.DirMode, uid: os.Getuid(), gid: os.Getgid()}
	m.entries["/"] = d
	m.dirs = append(m.dirs, d)
	return nil
}

// AddDir adds a directory in layout, will recursively add parent
// directories if they don't exist
func (m *Manager) AddDir(path string) error {
	p, err := m.checkPath(path, true)
	if err != nil {
		return err
	}
	return m.addDirs(p)
}

// AddFile adds a file in layout, will recursively add parent
// directories if they don't exist
func (m *Manager) AddFile(path string, content []byte) error {
	p, err := m.checkPath(path, true)
	if err != nil {
		return err
	}
	if err := m.addDirs(filepath.Dir(p)); err != nil {
		return err
	}
	m.entries[p] = &file{mode: m.FileMode, uid: os.Getuid(), gid: os.Getgid(), content: content}
	return nil
}

// AddSymlink adds a symlink in layout, will recursively add parent
// directories if they don't exist
func (m *Manager) AddSymlink(path string, target string) error {
	p, err := m.checkPath(path, true)
	if err != nil {
		return err
	}
	if err := m.addDirs(filepath.Dir(p)); err != nil {
		return err
	}
	m.entries[p] = &symlink{uid: os.Getuid(), gid: os.Getgid(), target: target}
	return nil
}

// overrideDir marks layoutPath in the session as being overridden by a bind from realPath.
func (m *Manager) overrideDir(layoutPath string, realPath string) {
	if existing, ok := m.dirOverrides[layoutPath]; ok {
		if existing.source == realPath {
			return
		}
		sylog.Warningf("path %s is already overridden by %s, replacing with %s", layoutPath, existing.source, realPath)
	}
	m.dirOverrides[layoutPath] = &dirOverride{source: realPath}
	sylog.Debugf("Overriding layout directory %s with bind from %s", layoutPath, realPath)
}

// addNestedBindTarget adds target as a nested bind target for the overridden directory layoutPath.
func (m *Manager) addNestedBindTarget(layoutPath string, target string) error {
	ov, ok := m.dirOverrides[layoutPath]
	if !ok {
		return fmt.Errorf("no override has been set for %s", layoutPath)
	}
	if slices.Contains(ov.nestedBindTargets, target) {
		return nil
	}
	ov.nestedBindTargets = append(ov.nestedBindTargets, target)
	sylog.Debugf("Adding nested bind target %s for overridden directory %s", target, layoutPath)
	return nil
}

// nestedBindTargetFor checks whether the layout path p is under an overridden
// directory. If it is, returns the overridden ancestor's path in the layout and
// the path in the bind source on which p will be mounted.
//
// For example, with `/canary` overridden by a bind from `/host/canary`, p =
// `/canary/dir2` returns layoutPath = `/canary` and target =
// `/host/canary/dir2`. Both return values are empty when p is not under any
// override.
func (m *Manager) nestedBindTargetFor(p string) (layoutOverride, target string) {
	layoutOverride, source, rel := m.overrideFor(p)
	if layoutOverride == "" {
		return "", ""
	}
	return layoutOverride, filepath.Join(source, rel)
}

// overrideFor finds the nearest ancestor of p that is overridden by a bind. It
// walks up p's parent directories, until it finds an entry in dirOverrides. If
// an override is found, it returns the overridden ancestor's path in the layout, the
// source of the bind that overrides it, and the path of p relative to the
// overridden ancestor. If p is not under any override, it returns three empty
// strings.
func (m *Manager) overrideFor(p string) (layoutOverride, overrideSource, pRel string) {
	for baseDir := filepath.Dir(p); baseDir != "/"; baseDir = filepath.Dir(baseDir) {
		ovDir, ok := m.dirOverrides[baseDir]
		if !ok {
			continue
		}
		rel, err := filepath.Rel(baseDir, p)
		if err != nil {
			return "", "", ""
		}
		return baseDir, ovDir.source, rel
	}
	return "", "", ""
}

// HasOverride returns true if the provided layout path is overridden by a bind, false otherwise.
func (m *Manager) HasOverride(layoutPath string) bool {
	_, ok := m.dirOverrides[layoutPath]
	return ok
}

// GetPath returns the full host path, for path in the layout.
func (m *Manager) GetPath(path string) (string, error) {
	_, err := m.checkPath(path, false)
	if err != nil {
		return "", err
	}
	return filepath.Join(m.rootPath, path), nil
}

// Chmod sets permission mode for path
func (m *Manager) Chmod(path string, mode os.FileMode) error {
	_, err := m.checkPath(path, false)
	if err != nil {
		return err
	}
	//nolint:forcetypeassert
	switch m.entries[path].(type) {
	case *file:
		m.entries[path].(*file).mode = mode
	case *dir:
		m.entries[path].(*dir).mode = mode
	}
	return nil
}

// Chown sets ownership for path
func (m *Manager) Chown(path string, uid, gid int) error {
	_, err := m.checkPath(path, false)
	if err != nil {
		return err
	}
	//nolint:forcetypeassert
	switch m.entries[path].(type) {
	case *file:
		m.entries[path].(*file).uid = uid
		m.entries[path].(*file).gid = gid
	case *dir:
		m.entries[path].(*dir).uid = uid
		m.entries[path].(*dir).gid = gid
	case *symlink:
		m.entries[path].(*symlink).uid = uid
		m.entries[path].(*symlink).gid = gid
	}
	return nil
}

// Create creates the filesystem layout
func (m *Manager) Create() error {
	return m.sync()
}

// Update updates the filesystem layout
func (m *Manager) Update() error {
	return m.sync()
}

func (m *Manager) sync() error {
	uid := os.Getuid()
	gid := os.Getgid()

	if m.entries == nil {
		return fmt.Errorf("root path is not set")
	}

	oldmask := m.VFS.Umask(0)
	defer m.VFS.Umask(oldmask)

	for _, d := range m.dirs[1:] {
		if d.created {
			continue
		}
		path := ""
		for p, e := range m.entries {
			if e == d {
				path = m.rootPath + p
				if ovDir, ok := m.dirOverrides[p]; ok {
					for _, nbt := range ovDir.nestedBindTargets {
						// Already exists - maybe created by a previous bind.
						if _, err := m.VFS.Stat(nbt); err == nil {
							continue
						}
						sylog.Debugf("Creating nested bind target %s for overridden directory %s", nbt, p)
						if err := m.VFS.Mkdir(nbt, m.DirMode); err != nil && !os.IsExist(err) {
							return fmt.Errorf("failed to create nested bind target %s: %s", nbt, err)
						}
					}
				}
				break
			}
		}
		if path == "" {
			continue
		}
		if d.mode != m.DirMode {
			if err := m.VFS.Mkdir(path, d.mode); err != nil {
				if !os.IsExist(err) {
					return fmt.Errorf("failed to create %s directory: %s", path, err)
				}
				// skip owner change, not created by us
				d.created = true
				continue
			}
		} else {
			if err := m.VFS.Mkdir(path, m.DirMode); err != nil {
				if !os.IsExist(err) {
					return fmt.Errorf("failed to create %s directory: %s", path, err)
				}
				// skip owner change, not created by us
				d.created = true
				continue
			}
		}
		if d.uid != uid || d.gid != gid {
			if err := m.VFS.Chown(path, d.uid, d.gid); err != nil {
				return fmt.Errorf("failed to change owner of %s: %s", path, err)
			}
		}
		d.created = true
	}

	for p, e := range m.entries {
		path := m.rootPath + p
		if _, target := m.nestedBindTargetFor(p); target != "" {
			path = target
		}
		switch entry := e.(type) {
		case *file:
			if entry.created {
				continue
			}
			if err := m.VFS.WriteFile(path, entry.content, entry.mode); err != nil {
				if !os.IsExist(err) {
					return fmt.Errorf("failed to create file %s: %s", path, err)
				}
				// skip content write or owner change, not created by us
				entry.created = true
				continue
			}
			if entry.uid != uid || entry.gid != gid {
				if err := m.VFS.Chown(path, entry.uid, entry.gid); err != nil {
					return fmt.Errorf("failed to change %s ownership: %s", path, err)
				}
			}
			entry.created = true
		case *symlink:
			if entry.created {
				continue
			}
			if err := m.VFS.Symlink(entry.target, path); err != nil {
				if !os.IsExist(err) {
					return fmt.Errorf("failed to create symlink %s: %s", path, err)
				}
				// check that current symlink point to the right target if it's a symlink
				// otherwise we consider the entry as already created no matter if it's a
				// file, a directory or something else
				target, err := m.VFS.Readlink(path)
				if err == nil && target != entry.target {
					return fmt.Errorf("symlink %s point to %s instead of %s", path, target, entry.target)
				}
				// skip symlink owner change, not created by us
				entry.created = true
				continue
			}
			if entry.uid != uid || entry.gid != gid {
				if err := m.VFS.Lchown(path, entry.uid, entry.gid); err != nil {
					return fmt.Errorf("failed to change %s ownership: %s", path, err)
				}
			}
			entry.created = true
		}
	}
	return nil
}
