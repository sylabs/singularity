// Copyright (c) 2019-2026, Sylabs Inc. All rights reserved.
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
		if err != nil {
			t.Fatalf("could not readlink /proc/self/cwd: %v", err)
		}
		unprivResolvedHome, err := filepath.EvalSymlinks(unprivUser.Dir)
		if err != nil {
			t.Fatalf("could not resolve home from %q: %v", unprivUser.Dir, err)
		}
		privResolvedHome, err := filepath.EvalSymlinks(privUser.Dir)
		if err != nil {
			t.Fatalf("could not resolve home directory from %q: %v", privUser.Dir, err)
		}

		// prepare user temporary homes
		unprivSessionHome := filepath.Join(sessionDir, unprivUser.Name)
		privSessionHome := filepath.Join(sessionDir, privUser.Name)

		oldUmask := syscall.Umask(0)
		defer syscall.Umask(oldUmask)

		if err := os.Mkdir(unprivSessionHome, 0o700); err != nil {
			t.Fatalf("failed to create temporary home %s: %v", unprivSessionHome, err)
		}
		if err := os.Chown(unprivSessionHome, int(unprivUser.UID), int(unprivUser.GID)); err != nil {
			t.Fatalf("failed to set temporary home ownership at %s: %v", unprivSessionHome, err)
		}
		if err := os.Mkdir(privSessionHome, 0o700); err != nil {
			t.Fatalf("failed to create temporary home %s: %v", privSessionHome, err)
		}

		sourceDir := buildcfg.SOURCEDIR

		// re-create the current source directory if it's located in the user
		// home directory and bind it. Root home directory is not checked because
		// the whole test suite can not run from there as we are dropping privileges
		if after, ok := strings.CutPrefix(sourceDir, unprivResolvedHome); ok {
			trimmedSourceDir := after
			sessionSourceDir := filepath.Join(unprivSessionHome, trimmedSourceDir)
			if err := os.MkdirAll(sessionSourceDir, 0o755); err != nil {
				t.Fatalf("failed to create temporary home source directory %q: %v", sessionSourceDir, err)
			}
			if err := syscall.Mount(sourceDir, sessionSourceDir, "", syscall.MS_BIND, ""); err != nil {
				t.Fatalf("failed to bind mount source directory %q to %q: %v", sourceDir, sessionSourceDir, err)
			}
			// fix go directory permission for unprivileged user
			goDir := filepath.Join(unprivSessionHome, "go")
			if _, err := os.Stat(goDir); err == nil {
				if err := os.Chown(goDir, int(unprivUser.UID), int(unprivUser.GID)); err != nil {
					t.Fatalf("failed to set temporary home go dir ownership at %s: %v", goDir, err)
				}
			}
		}

		// finally bind temporary homes on top of real ones
		// in order to not screw them by accident during e2e
		// tests execution
		if err := syscall.Mount(unprivSessionHome, unprivResolvedHome, "", syscall.MS_BIND|syscall.MS_REC, ""); err != nil {
			t.Fatalf("failed to bind mount home directory %q to %q: %v", unprivSessionHome, unprivResolvedHome, err)
		}
		if err := syscall.Mount(privSessionHome, privResolvedHome, "", syscall.MS_BIND, ""); err != nil {
			t.Fatalf("failed to bind mount home directory %q to %q: %v", privSessionHome, privResolvedHome, err)
		}
		// change to the "new" working directory if above mount override
		// the current working directory
		if err := os.Chdir(cwd); err != nil {
			t.Fatalf("failed to change working directory to %s: %v", cwd, err)
		}
	})(t)
}
