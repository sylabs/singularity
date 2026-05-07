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
	"syscall"

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

// Manager constructs a container filesystem layout in the session directory.
type Manager struct {
	// VFS is the virtual filesystem used to create the layout. It is rooted at
	// the session directory. Note that nested bind targets are created using
	// an os.Root rooted at the parent of the bind source target.
	VFS *RootedVFS

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

// NewManager returns a new layout manager with the provided path as its root.
func NewManager(path string) (*Manager, error) {
	vfs, err := NewRootedVFS(path)
	if err != nil {
		return nil, err
	}
	root := &dir{mode: dirMode, uid: os.Getuid(), gid: os.Getgid()}
	return &Manager{
		VFS:          vfs,
		DirMode:      dirMode,
		FileMode:     fileMode,
		rootPath:     filepath.Clean(path),
		entries:      map[string]any{"/": root},
		dirs:         []*dir{root},
		dirOverrides: map[string]*dirOverride{},
	}, nil
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

func validateNestedBindTarget(path string, fi os.FileInfo) error {
	if fi.Mode()&os.ModeSymlink != 0 {
		// Must accept symlinks to a directory, e.g. for `/home/<user>` which is
		// symlinked to shared storage location.
		// Ref: https://github.com/apptainer/singularity/issues/4836
		resolved, err := filepath.EvalSymlinks(path)
		if err != nil {
			return fmt.Errorf("failed to resolve nested bind target %s: %s", path, err)
		}
		fi, err = os.Stat(resolved)
		if err != nil {
			return fmt.Errorf("failed to stat resolved nested bind target %s: %s", resolved, err)
		}
	}
	if !fi.IsDir() {
		return fmt.Errorf("nested bind target %s exists but is not a directory", path)
	}
	return nil
}

// ensureNestedBindTarget ensures that the nested bind target directory exists,
// creating it where necessary, using os.Root on the parent directory to avoid
// escaping the parent bind.
func ensureNestedBindTarget(path string, mode os.FileMode) error {
	dir, base := filepath.Dir(path), filepath.Base(path)
	parent, err := os.OpenRoot(dir)
	if err != nil {
		return err
	}
	defer parent.Close()

	// If override path exists, it must be a directory.
	fi, err := parent.Lstat(base)
	if err == nil {
		return validateNestedBindTarget(path, fi)
	}
	if !os.IsNotExist(err) {
		return fmt.Errorf("failed to stat %s: %s", path, err)
	}

	// If nested bind target doesn't exist, create it. We must accommodate another
	// process creating the same location, where singularity has been launched
	// in parallel with the same bind configuration.
	sylog.Infof("Creating empty target directory for nested bind at %s", path)
	if err := parent.Mkdir(base, mode&0o777); err != nil {
		if !os.IsExist(err) {
			return fmt.Errorf("failed to create %s directory: %s", path, err)
		}
		fi, err := parent.Lstat(base)
		if err != nil {
			return fmt.Errorf("failed to stat %s after concurrent creation: %s", path, err)
		}
		return validateNestedBindTarget(path, fi)
	}

	// Check for / apply high mode bits (sticky/setuid/setgid) that Mkdir does not honor.
	if mode&0o777 == mode {
		return nil
	}
	f, err := parent.OpenFile(base, os.O_RDONLY|syscall.O_DIRECTORY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return fmt.Errorf("failed to open %s directory: %s", path, err)
	}
	defer f.Close()
	if err := f.Chmod(mode); err != nil {
		return fmt.Errorf("failed to set mode on %s directory: %s", path, err)
	}
	return nil
}

// diskPath returns the absolute path where p will be materialized on disk:
// inside the session root, or inside the bind source if p falls under an
// overridden directory. Used for diagnostic messages.
func (m *Manager) diskPath(p string) string {
	if _, source, rel := m.overrideFor(p); source != "" {
		return filepath.Join(source, rel)
	}
	return filepath.Join(m.rootPath, p)
}

// vfsFor returns an appropriately rooted VFS for operations on p. If p is not
// within an override directory, the session-rooted VFS is returned. If p is
// within an override directory, a new RootedVFS rooted at the bind source is
// returned.
func (m *Manager) vfsFor(p string) (*RootedVFS, string, error) {
	layoutPath, source, rel := m.overrideFor(p)
	if layoutPath == "" {
		rel := strings.TrimPrefix(filepath.Clean(p), string(os.PathSeparator))
		if rel == "" {
			rel = "."
		}
		return m.VFS, rel, nil
	}
	v, err := NewRootedVFS(source)
	if err != nil {
		return nil, "", err
	}
	return v, rel, nil
}

// HasOverride returns true if the provided layout path is overridden by a bind, false otherwise.
func (m *Manager) HasOverride(layoutPath string) bool {
	_, ok := m.dirOverrides[layoutPath]
	return ok
}

// PathResolvesOutsideOverride reports whether resolvedPath falls outside the
// deepest bind source that overrides path. This catches symlinked paths that
// look like they are below a bind in the container layout, but resolve to a
// different host tree that cannot be reached through that bind target.
func (m *Manager) PathResolvesOutsideOverride(path, resolvedPath string) bool {
	_, source, _ := m.overrideFor(path)
	if source == "" {
		return false
	}

	resolvedSource, err := filepath.EvalSymlinks(source)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(resolvedSource, resolvedPath)
	if err != nil {
		return false
	}
	return rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator))
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

// sync materializes the layout in the session directory. Directories (including
// nested bind targets) are created first, followed by files and symlinks.
func (m *Manager) sync() error {
	uid := os.Getuid()
	gid := os.Getgid()

	if m.entries == nil {
		return fmt.Errorf("root path is not set")
	}

	oldmask := syscall.Umask(0)
	defer syscall.Umask(oldmask)

	for _, d := range m.dirs[1:] {
		if d.created {
			continue
		}
		path := ""
		for p, e := range m.entries {
			if e == d {
				path = p
				if ovDir, ok := m.dirOverrides[p]; ok {
					for _, nbt := range ovDir.nestedBindTargets {
						if err := ensureNestedBindTarget(nbt, m.DirMode); err != nil {
							return err
						}
					}
				}
				break
			}
		}
		if path == "" {
			continue
		}
		// Directories are always created in the session, even when path is
		// under an overridden directory: they may serve as bind-mount targets
		// for other mount points (e.g. the home staging dir). Override targets
		// outside the session are handled separately by ensureNestedBindTarget.
		fullPath := filepath.Join(m.rootPath, path)
		name := strings.TrimPrefix(filepath.Clean(path), string(os.PathSeparator))
		mode := m.DirMode
		if d.mode != m.DirMode {
			mode = d.mode
		}
		if err := m.VFS.Mkdir(name, mode); err != nil {
			if !os.IsExist(err) {
				return fmt.Errorf("failed to create %s directory: %s", fullPath, err)
			}
			// skip owner change, not created by us
			d.created = true
			continue
		}
		if d.uid != uid || d.gid != gid {
			if err := m.VFS.Chown(name, d.uid, d.gid); err != nil {
				return fmt.Errorf("failed to change owner of %s: %s", fullPath, err)
			}
		}
		d.created = true
	}

	for p, e := range m.entries {
		if err := m.syncEntry(p, e, uid, gid); err != nil {
			return err
		}
	}
	return nil
}

// syncEntry materializes a single file or symlink entry. It opens the
// appropriate VFS for p (session-rooted, or freshly rooted at the override
// target) and closes the override VFS, if any, before returning so per-entry
// fds don't accumulate across the whole sync.
func (m *Manager) syncEntry(p string, e any, uid, gid int) error {
	switch entry := e.(type) {
	case *file:
		if entry.created {
			return nil
		}
		ops, name, err := m.vfsFor(p)
		if err != nil {
			return err
		}
		if ops != m.VFS {
			defer ops.Close()
		}
		if err := ops.WriteFile(name, entry.content, entry.mode); err != nil {
			if !os.IsExist(err) {
				return fmt.Errorf("failed to create file %s: %s", m.diskPath(p), err)
			}
			// skip content write or owner change, not created by us
			entry.created = true
			return nil
		}
		if entry.uid != uid || entry.gid != gid {
			if err := ops.Chown(name, entry.uid, entry.gid); err != nil {
				return fmt.Errorf("failed to change %s ownership: %s", m.diskPath(p), err)
			}
		}
		entry.created = true
	case *symlink:
		if entry.created {
			return nil
		}
		ops, name, err := m.vfsFor(p)
		if err != nil {
			return err
		}
		if ops != m.VFS {
			defer ops.Close()
		}
		if err := ops.Symlink(entry.target, name); err != nil {
			if !os.IsExist(err) {
				return fmt.Errorf("failed to create symlink %s: %s", m.diskPath(p), err)
			}
			// check that current symlink point to the right target if it's a symlink
			// otherwise we consider the entry as already created no matter if it's a
			// file, a directory or something else
			target, err := ops.Readlink(name)
			if err == nil && target != entry.target {
				return fmt.Errorf("symlink %s point to %s instead of %s", m.diskPath(p), target, entry.target)
			}
			// skip symlink owner change, not created by us
			entry.created = true
			return nil
		}
		if entry.uid != uid || entry.gid != gid {
			if err := ops.Lchown(name, entry.uid, entry.gid); err != nil {
				return fmt.Errorf("failed to change %s ownership: %s", m.diskPath(p), err)
			}
		}
		entry.created = true
	}
	return nil
}
