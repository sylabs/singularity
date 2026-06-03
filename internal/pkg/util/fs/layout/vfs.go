// Copyright (c) 2018-2026, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package layout

import (
	"fmt"
	iofs "io/fs"
	"os"
	"syscall"

	"github.com/sylabs/singularity/v4/internal/pkg/util/fs"
)

type DefaultVFS struct{}

func (v *DefaultVFS) Chown(name string, uid, gid int) error {
	return os.Chown(name, uid, gid)
}

func (v *DefaultVFS) EvalRelative(path, root string) string {
	return fs.EvalRelative(path, root)
}

func (v *DefaultVFS) Lchown(name string, uid, gid int) error {
	return os.Lchown(name, uid, gid)
}

func (v *DefaultVFS) Mkdir(name string, perm os.FileMode) error {
	return os.Mkdir(name, perm)
}

func (v *DefaultVFS) Readlink(name string) (string, error) {
	return os.Readlink(name)
}

func (v *DefaultVFS) ReadDir(dir string) ([]iofs.DirEntry, error) {
	return os.ReadDir(dir)
}

func (v *DefaultVFS) Stat(name string) (os.FileInfo, error) {
	return os.Stat(name)
}

func (v *DefaultVFS) Symlink(oldname, newname string) error {
	return os.Symlink(oldname, newname)
}

func (v *DefaultVFS) Umask(mask int) int {
	return syscall.Umask(mask)
}

func (v *DefaultVFS) WriteFile(filename string, data []byte, perm os.FileMode) error {
	f, err := os.OpenFile(filename, os.O_CREATE|os.O_WRONLY|os.O_EXCL, perm)
	if err != nil {
		if !os.IsExist(err) {
			return fmt.Errorf("failed to create file %s: %s", filename, err)
		}
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
