// Copyright (c) 2023, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package rootless

import (
	"errors"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/sylabs/singularity/internal/pkg/buildcfg"
	fakerootConfig "github.com/sylabs/singularity/internal/pkg/runtime/engine/fakeroot/config"
	"github.com/sylabs/singularity/internal/pkg/util/starter"
	"github.com/sylabs/singularity/pkg/runtime/engine/config"
	"github.com/sylabs/singularity/pkg/sylog"
)

const (
	NSEnv  = "_SINGULARITY_NAMESPACE"
	UIDEnv = "_CONTAINERS_ROOTLESS_UID"
	GIDEnv = "_CONTAINERS_ROOTLESS_GID"
)

// Getuid retrieves the uid stored in the env var _CONTAINERS_ROOTLESS_UID, or
// the current euid if the env var is not set.
func Getuid() (uid int, err error) {
	u := os.Getenv(UIDEnv)
	if u != "" {
		return strconv.Atoi(u)
	}
	return os.Geteuid(), nil
}

// Getgid retrieves the uid stored in the env var _CONTAINERS_ROOTLESS_GID, or
// the current egid if the env var is not set.
func Getgid() (uid int, err error) {
	g := os.Getenv(GIDEnv)
	if g != "" {
		return strconv.Atoi(g)
	}
	return os.Getegid(), nil
}

// GetUser retrieves the User struct for the uid stored in the env var
// _CONTAINERS_ROOTLESS_UID, or the current euid if the env var is not set.
func GetUser() (*user.User, error) {
	u := os.Getenv(UIDEnv)
	if u != "" {
		return user.LookupId(u)
	}
	return user.Current()
}

// InNS returns true if we are in a namespace created using this package.
func InNS() bool {
	_, envSet := os.LookupEnv(NSEnv)
	return envSet
}

// ExecWithFakeroot will exec singularity with provided args, in a
// subuid/gid-mapped fakeroot user namespace. This uses the fakeroot engine.
func ExecWithFakeroot(args []string) error {
	singularityBin := []string{
		filepath.Join(buildcfg.BINDIR, "singularity"),
	}
	args = append(singularityBin, args...)

	env := os.Environ()
	env = append(env, NSEnv+"=TRUE")
	// Use _CONTAINERS_ROOTLESS_xID naming for these vars as they are required
	// by our use of containers/image for OCI image handling.
	env = append(env, UIDEnv+"="+strconv.Itoa(os.Geteuid()))
	env = append(env, GIDEnv+"="+strconv.Itoa(os.Getegid()))

	sylog.Debugf("Calling fakeroot engine to execute %q", strings.Join(args, " "))

	cfg := &config.Common{
		EngineName:  fakerootConfig.Name,
		ContainerID: "fakeroot",
		EngineConfig: &fakerootConfig.EngineConfig{
			Envs:        env,
			Args:        args,
			NoPIDNS:     true,
			NoSetgroups: true,
		},
	}

	return starter.Exec(
		"Singularity oci fakeroot",
		cfg,
	)
}

// RunInMountNS will run singularity with provided args, in a mount
// namespace only.
func RunInMountNS(args []string) error {
	singularityBin := filepath.Join(buildcfg.BINDIR, "singularity")

	env := os.Environ()
	env = append(env, NSEnv+"=TRUE")

	cmd := exec.Command(singularityBin, args...)
	cmd.Env = env
	cmd.SysProcAttr = &syscall.SysProcAttr{}
	// Unshare mount namespace
	cmd.SysProcAttr.Unshareflags = syscall.CLONE_NEWNS
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err := cmd.Run()
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		os.Exit(exitErr.ExitCode())
	}

	if err == nil {
		os.Exit(0)
	}

	return err
}
