// Copyright (c) 2019-2022, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package require

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"

	"github.com/google/uuid"
	"github.com/opencontainers/cgroups"
	"github.com/sylabs/singularity/v4/internal/pkg/buildcfg"
	"github.com/sylabs/singularity/v4/internal/pkg/security/seccomp"
	"github.com/sylabs/singularity/v4/internal/pkg/util/bin"
	"github.com/sylabs/singularity/v4/internal/pkg/util/rpm"
	"github.com/sylabs/singularity/v4/pkg/network"
	"github.com/sylabs/singularity/v4/pkg/util/fs/proc"
	"github.com/sylabs/singularity/v4/pkg/util/slice"
)

var (
	hasUserNamespace     bool
	hasUserNamespaceOnce sync.Once
)

// UserNamespace checks that the current test could use
// user namespace, if user namespaces are not enabled or
// supported, the current test is skipped with a message.
func UserNamespace(t *testing.T) {
	// not performance critical, just save extra execution
	// to get the same result
	hasUserNamespaceOnce.Do(func() {
		// user namespace is a bit special, as there is no simple
		// way to detect if it's supported or enabled via a call
		// on /proc/self/ns/user, the easiest and reliable way seems
		// to directly execute a command by requesting user namespace
		cmd := exec.Command("/bin/true")
		cmd.SysProcAttr = &syscall.SysProcAttr{
			Cloneflags: syscall.CLONE_NEWUSER,
		}
		// no error means user namespaces are enabled
		err := cmd.Run()
		hasUserNamespace = err == nil
		if !hasUserNamespace {
			t.Logf("Could not use user namespaces: %s", err)
		}
	})
	if !hasUserNamespace {
		t.Skipf("user namespaces seems not enabled or supported")
	}
}

var (
	supportNetwork     bool
	supportNetworkOnce sync.Once
)

// Network check that bridge network is supported by
// system, if not the current test is skipped with a
// message.
func Network(t *testing.T) {
	supportNetworkOnce.Do(func() {
		logFn := func(err error) {
			t.Logf("Could not use network: %s", err)
		}

		ctx := t.Context()

		cmd := exec.Command("/bin/cat")
		cmd.SysProcAttr = &syscall.SysProcAttr{}
		cmd.SysProcAttr.Cloneflags = syscall.CLONE_NEWNET

		stdinPipe, err := cmd.StdinPipe()
		if err != nil {
			logFn(err)
			return
		}

		err = cmd.Start()
		if err != nil {
			logFn(err)
			return
		}

		nsPath := fmt.Sprintf("/proc/%d/ns/net", cmd.Process.Pid)

		cniPath := new(network.CNIPath)
		cniPath.Conf = filepath.Join(buildcfg.SYSCONFDIR, "singularity", "network")
		cniPath.Plugin = filepath.Join(buildcfg.LIBEXECDIR, "singularity", "cni")
		containerID := "singularity-e2e-" + uuid.New().String()

		setup, err := network.NewSetup([]string{"bridge"}, containerID, nsPath, cniPath)
		if err != nil {
			logFn(err)
			return
		}
		if err := setup.AddNetworks(ctx); err != nil {
			logFn(err)
			return
		}
		if err := setup.DelNetworks(ctx); err != nil {
			logFn(err)
			return
		}

		stdinPipe.Close()

		if err := cmd.Wait(); err != nil {
			logFn(err)
			return
		}

		supportNetwork = true
	})
	if !supportNetwork {
		t.Skipf("Network (bridge) not supported")
	}
}

// Cgroups checks that any cgroups version is enabled, if not the
// current test is skipped with a message.
func Cgroups(t *testing.T) {
	subsystems, err := cgroups.GetAllSubsystems()
	if err != nil || len(subsystems) == 0 {
		t.Skipf("cgroups not available")
	}
}

// CgroupsV1 checks that legacy cgroups is enabled, if not the
// current test is skipped with a message.
func CgroupsV1(t *testing.T) {
	Cgroups(t)
	if cgroups.IsCgroup2UnifiedMode() {
		t.Skipf("cgroups v1 legacy mode not available")
	}
}

// CgroupsV2 checks that cgroups v2 unified mode is enabled, if not the
// current test is skipped with a message.
func CgroupsV2Unified(t *testing.T) {
	if !cgroups.IsCgroup2UnifiedMode() {
		t.Skipf("cgroups v2 unified mode not available")
	}
}

// CgroupsFreezer checks that cgroup freezer subsystem is
// available, if not the current test is skipped with a
// message
func CgroupsFreezer(t *testing.T) {
	subsystems, err := cgroups.GetAllSubsystems()
	if err != nil {
		t.Skipf("couldn't get cgroups subsystems: %v", err)
	}
	if !slice.ContainsString(subsystems, "freezer") {
		t.Skipf("no cgroups freezer subsystem available")
	}
}

// CgroupsResourceExists checks that the requested controller and resource exist
// in the cgroupfs.
func CgroupsResourceExists(t *testing.T, controller string, resource string) {
	cgs, err := cgroups.ParseCgroupFile("/proc/self/cgroup")
	if err != nil {
		t.Error(err)
	}
	cgPath, ok := cgs[controller]
	if !ok {
		t.Skipf("controller %s cgroup path not found", controller)
	}

	resourcePath := filepath.Join("/sys/fs/cgroup", controller, cgPath, resource)
	if _, err := os.Stat(resourcePath); err != nil {
		t.Skipf("cannot stat resource %s: %s", resource, err)
	}
}

// CroupsV2Delegated checks that the controller is delegated to users.
func CgroupsV2Delegated(t *testing.T, controller string) {
	CgroupsV2Unified(t)
	cgs, err := cgroups.ParseCgroupFile("/proc/self/cgroup")
	if err != nil {
		t.Error(err)
	}

	cgPath, ok := cgs[""]
	if !ok {
		t.Skipf("unified cgroup path not found")
	}

	delegatePath := filepath.Join("/sys/fs/cgroup", cgPath, "cgroup.controllers")

	data, err := os.ReadFile(delegatePath)
	if err != nil {
		t.Skipf("while reading delegation file: %s", err)
	}

	if !strings.Contains(string(data), controller) {
		t.Skipf("%s controller is not delegated", controller)
	}
}

// Nvidia checks that an NVIDIA stack is available
func Nvidia(t *testing.T) {
	nvsmi, err := exec.LookPath("nvidia-smi")
	if err != nil {
		t.Skipf("nvidia-smi not found on PATH: %v", err)
	}
	cmd := exec.Command(nvsmi)
	if err := cmd.Run(); err != nil {
		t.Skipf("nvidia-smi failed to run: %v", err)
	}
}

// NvCCLI checks that nvidia-container-cli is available
func NvCCLI(t *testing.T) {
	_, err := exec.LookPath("nvidia-container-cli")
	if err != nil {
		t.Skipf("nvidia-container-cli not found on PATH: %v", err)
	}
}

// Rocm checks that a Rocm stack is available
func Rocm(t *testing.T) {
	rocminfo, err := exec.LookPath("rocminfo")
	if err != nil {
		t.Skipf("rocminfo not found on PATH: %v", err)
	}
	cmd := exec.Command(rocminfo)
	if output, err := cmd.Output(); err != nil {
		t.Skipf("rocminfo failed to run: %v - %v", err, string(output))
	}
}

// Filesystem checks that the current test could use the
// corresponding filesystem, if the filesystem is not
// listed in /proc/filesystems, the current test is skipped
// with a message.
func Filesystem(t *testing.T, fs string) {
	has, err := proc.HasFilesystem(fs)
	if err != nil {
		t.Fatalf("error while checking filesystem presence: %s", err)
	}
	if !has {
		t.Skipf("%s filesystem seems not supported", fs)
	}
}

// Command checks if the provided command is available (via Singularity's
// internal bin.FindBin() facility, or else simply on the PATH). If not found,
// the current test is skipped with a message.
func Command(t *testing.T, command string) {
	if _, err := bin.FindBin(command); err == nil {
		return
	}

	if _, err := exec.LookPath(command); err == nil {
		return
	}

	t.Skipf("%s command not found in $PATH", command)
}

// OneCommand checks if one of the provided commands is available (via
// Singularity's internal bin.FindBin() facility, or else simply on the PATH).
// If none are found, the current test is skipped with a message.
func OneCommand(t *testing.T, commands []string) {
	for _, c := range commands {
		if _, err := bin.FindBin(c); err == nil {
			return
		}

		if _, err := exec.LookPath(c); err == nil {
			return
		}
	}

	t.Skipf("%v commands not found in $PATH", commands)
}

// Seccomp checks that seccomp is enabled, if not the
// current test is skipped with a message.
func Seccomp(t *testing.T) {
	if !seccomp.Enabled() {
		t.Skipf("seccomp disabled, Singularity was compiled without the seccomp library")
	}
}

// Arch checks the test machine has the specified architecture.
// If not, the test is skipped with a message.
func Arch(t *testing.T, arch string) {
	if arch != "" && runtime.GOARCH != arch {
		t.Skipf("test requires architecture %s", arch)
	}
}

// ArchIn checks the test machine is one of the specified archs.
// If not, the test is skipped with a message.
func ArchIn(t *testing.T, archs []string) {
	if len(archs) > 0 {
		b := runtime.GOARCH
		for _, a := range archs {
			if b == a {
				return
			}
		}
		t.Skipf("test requires architecture %s", strings.Join(archs, "|"))
	}
}

// MkfsExt3 checks that mkfs.ext3 is available and
// support -d option to create writable overlay layout.
func MkfsExt3(t *testing.T) {
	mkfs, err := exec.LookPath("mkfs.ext3")
	if err != nil {
		t.Skipf("mkfs.ext3 not found in $PATH")
	}

	buf := new(bytes.Buffer)
	cmd := exec.Command(mkfs, "--help")
	cmd.Stderr = buf
	_ = cmd.Run()

	if !strings.Contains(buf.String(), "[-d ") {
		t.Skipf("mkfs.ext3 is too old and doesn't support -d")
	}
}

// Kernel checks that the kernel version is equal or greater than
// the minimum specified.
func Kernel(t *testing.T, reqMajor, reqMinor int) {
	release, err := os.ReadFile("/proc/sys/kernel/osrelease")
	if err != nil {
		t.Skipf("couldn't read kernel version: %s", err)
	}

	version := strings.SplitN(string(release), ".", 3)
	if len(version) != 3 {
		t.Skipf("kernel version didn't have 3 components: %s", release)
	}

	major, err := strconv.Atoi(version[0])
	if err != nil {
		t.Skipf("couldn't parse kernel major version: %s", err)
	}
	minor, err := strconv.Atoi(version[1])
	if err != nil {
		t.Skipf("couldn't parse kernel minor version: %s", err)
	}

	if major > reqMajor {
		return
	}

	if major == reqMajor && minor >= reqMinor {
		return
	}

	t.Skipf("Kernel %d.%d found, but %d.%d required", major, minor, reqMajor, reqMinor)
}

func RPMMacro(t *testing.T, name, value string) {
	eval, err := rpm.GetMacro(name)
	if err != nil {
		t.Skipf("Couldn't get value of %s: %s", name, err)
	}

	if eval != value {
		t.Skipf("Need %s as value of %s, got %s", value, name, eval)
	}
}
