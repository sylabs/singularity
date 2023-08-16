// Copyright (c) 2023, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package rootless

import (
	"os"
	"os/exec"
	"os/signal"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/sylabs/singularity/v4/internal/pkg/buildcfg"
	fakerootConfig "github.com/sylabs/singularity/v4/internal/pkg/runtime/engine/fakeroot/config"
	"github.com/sylabs/singularity/v4/internal/pkg/util/starter"
	"github.com/sylabs/singularity/v4/pkg/runtime/engine/config"
	"github.com/sylabs/singularity/v4/pkg/sylog"
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

	signals := make(chan os.Signal, 2)
	signal.Notify(signals)
	errChan := make(chan error, 1)

	err := cmd.Start()
	if err != nil {
		return err
	}

	go func() {
		errChan <- cmd.Wait()
	}()

	for {
		select {
		case s := <-signals:
			sylog.Debugf("Received signal %s", s.String())
			switch s {
			case syscall.SIGCHLD:
				break
			case syscall.SIGURG:
				// Ignore SIGURG, which is used for non-cooperative goroutine
				// preemption starting with Go 1.14. For more information, see
				// https://github.com/golang/go/issues/24543.
				break
			default:
				//nolint:forcetypeassert
				signal := s.(syscall.Signal)
				if err := syscall.Kill(cmd.Process.Pid, signal); err != nil {
					return err
				}
			}
		case err := <-errChan:
			if e, ok := err.(*exec.ExitError); ok {
				status, ok := e.Sys().(syscall.WaitStatus)
				if ok && status.Signaled() {
					os.Exit(128 + int(status.Signal()))
				}
				os.Exit(e.ExitCode())
			}
			if err == nil {
				os.Exit(0)
			}
			return err
		}
	}
}
