// Copyright (c) 2018-2026, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package layout

import (
	"fmt"
	iofs "io/fs"
	"os"
	"path/filepath"

	"github.com/sylabs/singularity/v4/internal/pkg/util/fs"
)

type RootedVFS struct {
	rootPath string
	root     *os.Root
}

func NewRootedVFS(path string) (*RootedVFS, error) {
	clean := filepath.Clean(path)
	if !filepath.IsAbs(clean) {
		return nil, fmt.Errorf("%s is not an absolute directory path", path)
	}
	if !fs.IsDir(clean) {
		return nil, fmt.Errorf("%s is not a directory or doesn't exist", clean)
	}
	return &RootedVFS{rootPath: clean}, nil
}

func (v *RootedVFS) ensureRoot() error {
	if v.root != nil {
		return nil
	}
	if v.rootPath == "" {
		return fmt.Errorf("root is not set")
	}
	root, err := os.OpenRoot(v.rootPath)
	if err != nil {
		return err
	}
	v.root = root
	return nil
}

func (v *RootedVFS) Close() error {
	if v.root == nil {
		return nil
	}
	err := v.root.Close()
	v.root = nil
	return err
}

func (v *RootedVFS) Chown(name string, uid, gid int) error {
	if err := v.ensureRoot(); err != nil {
		return err
	}
	return v.root.Chown(name, uid, gid)
}

func (v *RootedVFS) Lchown(name string, uid, gid int) error {
	if err := v.ensureRoot(); err != nil {
		return err
	}
	return v.root.Lchown(name, uid, gid)
}

func (v *RootedVFS) Mkdir(name string, perm os.FileMode) error {
	if err := v.ensureRoot(); err != nil {
		return err
	}

	if err := v.root.Mkdir(name, perm&0o777); err != nil {
		return err
	}
	if perm&0o777 != perm {
		return v.root.Chmod(name, perm)
	}
	return nil
}

func (v *RootedVFS) Readlink(name string) (string, error) {
	if err := v.ensureRoot(); err != nil {
		return "", err
	}
	return v.root.Readlink(name)
}

func (v *RootedVFS) ReadDir(dir string) ([]iofs.DirEntry, error) {
	if err := v.ensureRoot(); err != nil {
		return nil, err
	}
	f, err := v.root.Open(dir)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return f.ReadDir(-1)
}

func (v *RootedVFS) Stat(name string) (os.FileInfo, error) {
	if err := v.ensureRoot(); err != nil {
		return nil, err
	}
	return v.root.Stat(name)
}

func (v *RootedVFS) Symlink(oldname, newname string) error {
	if err := v.ensureRoot(); err != nil {
		return err
	}
	return v.root.Symlink(oldname, newname)
}

func (v *RootedVFS) WriteFile(filename string, data []byte, perm os.FileMode) error {
	if err := v.ensureRoot(); err != nil {
		return err
	}

	f, err := v.root.OpenFile(filename, os.O_CREATE|os.O_WRONLY|os.O_EXCL, perm)
	if err != nil {
		return err
	}
	if len(data) > 0 {
		_, err = f.Write(data)
	}
	if err1 := f.Close(); err == nil {
		err = err1
	}
	return err
}
