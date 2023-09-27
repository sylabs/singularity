// Copyright (c) 2019-2022, Sylabs Inc. All rights reserved.
// Copyright (c) Contributors to the Apptainer project, established as
//   Apptainer a Series of LF Projects LLC.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/pkg/errors"
	"github.com/sylabs/singularity/v4/internal/pkg/buildcfg"
	"github.com/sylabs/singularity/v4/internal/pkg/util/user"
)

// SetupHomeDirectories creates temporary home directories for
// privileged and unprivileged users and bind mount those directories
// on top of real ones. It's possible because e2e tests are executed
// in a dedicated mount namespace.
func SetupHomeDirectories(t *testing.T) {
	var unprivUser, privUser *user.User

	sessionDir := buildcfg.SESSIONDIR
	unprivUser = CurrentUser(t)

	Privileged(func(t *testing.T) {
		// there is no cleanup here because everything done (tmpfs, mounts)
		// in our dedicated mount namespace will be automatically discarded
		// by the kernel once all test processes exit

		privUser = CurrentUser(t)

		// create the temporary filesystem
		if err := syscall.Mount("tmpfs", sessionDir, "tmpfs", 0, "mode=0777"); err != nil {
			t.Fatalf("failed to mount temporary filesystem")
		}

		// want the already resolved current working directory
		cwd, err := os.Readlink("/proc/self/cwd")
		err = errors.Wrap(err, "getting current working directory from /proc/self/cwd")
		if err != nil {
			t.Fatalf("could not readlink /proc/self/cwd: %+v", err)
		}
		unprivResolvedHome, err := filepath.EvalSymlinks(unprivUser.Dir)
		err = errors.Wrapf(err, "resolving home from %q", unprivUser.Dir)
		if err != nil {
			t.Fatalf("could not resolve home directory: %+v", err)
		}
		privResolvedHome, err := filepath.EvalSymlinks(privUser.Dir)
		err = errors.Wrapf(err, "resolving home from %q", privUser.Dir)
		if err != nil {
			t.Fatalf("could not resolve home directory: %+v", err)
		}

		// prepare user temporary homes
		unprivSessionHome := filepath.Join(sessionDir, unprivUser.Name)
		privSessionHome := filepath.Join(sessionDir, privUser.Name)

		oldUmask := syscall.Umask(0)
		defer syscall.Umask(oldUmask)

		if err := os.Mkdir(unprivSessionHome, 0o700); err != nil {
			err = errors.Wrapf(err, "creating temporary home directory at %s", unprivSessionHome)
			t.Fatalf("failed to create temporary home: %+v", err)
		}
		if err := os.Chown(unprivSessionHome, int(unprivUser.UID), int(unprivUser.GID)); err != nil {
			err = errors.Wrapf(err, "changing temporary home directory ownership at %s", unprivSessionHome)
			t.Fatalf("failed to set temporary home owner: %+v", err)
		}
		if err := os.Mkdir(privSessionHome, 0o700); err != nil {
			err = errors.Wrapf(err, "changing temporary home directory %s", privSessionHome)
			t.Fatalf("failed to create temporary home: %+v", err)
		}

		sourceDir := buildcfg.SOURCEDIR

		// re-create the current source directory if it's located in the user
		// home directory and bind it. Root home directory is not checked because
		// the whole test suite can not run from there as we are dropping privileges
		if strings.HasPrefix(sourceDir, unprivResolvedHome) {
			trimmedSourceDir := strings.TrimPrefix(sourceDir, unprivResolvedHome)
			sessionSourceDir := filepath.Join(unprivSessionHome, trimmedSourceDir)
			if err := os.MkdirAll(sessionSourceDir, 0o755); err != nil {
				err = errors.Wrapf(err, "creating temporary source directory at %q", sessionSourceDir)
				t.Fatalf("failed to create temporary home source directory: %+v", err)
			}
			if err := syscall.Mount(sourceDir, sessionSourceDir, "", syscall.MS_BIND, ""); err != nil {
				err = errors.Wrapf(err, "bind mounting source directory from %q to %q", sourceDir, sessionSourceDir)
				t.Fatalf("failed to bind mount source directory: %+v", err)
			}
			// fix go directory permission for unprivileged user
			goDir := filepath.Join(unprivSessionHome, "go")
			if _, err := os.Stat(goDir); err == nil {
				if err := os.Chown(goDir, int(unprivUser.UID), int(unprivUser.GID)); err != nil {
					err = errors.Wrapf(err, "changing temporary home go directory ownership at %s", goDir)
					t.Fatalf("failed to set owner: %+v", err)
				}
			}
		}

		// finally bind temporary homes on top of real ones
		// in order to not screw them by accident during e2e
		// tests execution
		if err := syscall.Mount(unprivSessionHome, unprivResolvedHome, "", syscall.MS_BIND|syscall.MS_REC, ""); err != nil {
			err = errors.Wrapf(err, "bind mounting source directory from %q to %q", unprivSessionHome, unprivResolvedHome)
			t.Fatalf("failed to bind mount home directory: %+v", err)
		}
		if err := syscall.Mount(privSessionHome, privResolvedHome, "", syscall.MS_BIND, ""); err != nil {
			err = errors.Wrapf(err, "bind mounting source directory from %q to %q", privSessionHome, privResolvedHome)
			t.Fatalf("failed to bind mount home directory: %+v", err)
		}
		// change to the "new" working directory if above mount override
		// the current working directory
		if err := os.Chdir(cwd); err != nil {
			err = errors.Wrapf(err, "change working directory to %s", cwd)
			t.Fatalf("failed to change working directory: %+v", err)
		}
	})(t)
}
