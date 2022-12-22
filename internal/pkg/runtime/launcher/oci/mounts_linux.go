// Copyright (c) 2022, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

// Package oci implements a Launcher that will configure and launch a container
// with an OCI runtime. It also provides implementations of OCI state
// transitions that can be called directly, Create/Start/Kill etc.
package oci

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/opencontainers/runtime-spec/specs-go"
	"github.com/sylabs/singularity/internal/pkg/buildcfg"
	"github.com/sylabs/singularity/internal/pkg/util/gpu"
	"github.com/sylabs/singularity/internal/pkg/util/user"
	"github.com/sylabs/singularity/pkg/sylog"
	"github.com/sylabs/singularity/pkg/util/bind"
)

const containerLibDir = "/.singularity.d/libs"

// getMounts returns a mount list for the container's OCI runtime spec.
func (l *Launcher) getMounts() ([]specs.Mount, error) {
	mounts := &[]specs.Mount{}
	l.addProcMount(mounts)
	l.addSysMount(mounts)
	if err := l.addDevMounts(mounts); err != nil {
		return nil, fmt.Errorf("while configuring devpts mount: %w", err)
	}
	l.addTmpMounts(mounts)
	if err := l.addHomeMount(mounts); err != nil {
		return nil, fmt.Errorf("while configuring home mount: %w", err)
	}
	if err := l.addBindMounts(mounts); err != nil {
		return nil, fmt.Errorf("while configuring bind mount(s): %w", err)
	}
	if (l.cfg.Rocm || l.singularityConf.AlwaysUseRocm) && !l.cfg.NoRocm {
		if err := l.addRocmMounts(mounts); err != nil {
			return nil, fmt.Errorf("while configuring ROCm mount(s): %w", err)
		}
	}
	if (l.cfg.Nvidia || l.singularityConf.AlwaysUseNv) && !l.cfg.NoNvidia {
		if err := l.addNvidiaMounts(mounts); err != nil {
			return nil, fmt.Errorf("while configuring Nvidia mount(s): %w", err)
		}
	}

	return *mounts, nil
}

// addTmpMounts adds tmpfs mounts for /tmp and /var/tmp in the container.
func (l *Launcher) addTmpMounts(mounts *[]specs.Mount) {
	*mounts = append(*mounts,

		specs.Mount{
			Destination: "/tmp",
			Type:        "tmpfs",
			Source:      "tmpfs",
			Options: []string{
				"nosuid",
				"relatime",
				"mode=777",
				fmt.Sprintf("size=%dm", l.singularityConf.SessiondirMaxSize),
			},
		},
		specs.Mount{
			Destination: "/var/tmp",
			Type:        "tmpfs",
			Source:      "tmpfs",
			Options: []string{
				"nosuid",
				"relatime",
				"mode=777",
				fmt.Sprintf("size=%dm", l.singularityConf.SessiondirMaxSize),
			},
		})
}

// addDevMounts adds mounts to assemble a minimal /dev in the container.
func (l *Launcher) addDevMounts(mounts *[]specs.Mount) error {
	ptsMount := specs.Mount{
		Destination: "/dev/pts",
		Type:        "devpts",
		Source:      "devpts",
		Options:     []string{"nosuid", "noexec", "newinstance", "ptmxmode=0666", "mode=0620"},
	}

	if os.Getuid() == 0 {
		group, err := user.GetGrNam("tty")
		if err != nil {
			return fmt.Errorf("while identifying tty gid: %w", err)
		}
		ptsMount.Options = append(ptsMount.Options, fmt.Sprintf("gid=%d", group.GID))
	}

	*mounts = append(*mounts,
		specs.Mount{
			Destination: "/dev",
			Type:        "tmpfs",
			Source:      "tmpfs",
			Options: []string{
				"nosuid",
				"strictatime",
				"mode=755",
				fmt.Sprintf("size=%dm", l.singularityConf.SessiondirMaxSize),
			},
		},
		ptsMount,
		specs.Mount{
			Destination: "/dev/shm",
			Type:        "tmpfs",
			Source:      "shm",
			Options: []string{
				"nosuid",
				"noexec",
				"nodev",
				"mode=1777",
				fmt.Sprintf("size=%dm", l.singularityConf.SessiondirMaxSize),
			},
		},
		specs.Mount{
			Destination: "/dev/mqueue",
			Type:        "mqueue",
			Source:      "mqueue",
			Options:     []string{"nosuid", "noexec", "nodev"},
		},
	)

	return nil
}

// addProcMount adds the /proc tree in the container.
func (l *Launcher) addProcMount(mounts *[]specs.Mount) {
	*mounts = append(*mounts,
		specs.Mount{
			Source:      "proc",
			Destination: "/proc",
			Type:        "proc",
		})
}

// addSysMount adds the /sys tree in the container.
func (l *Launcher) addSysMount(mounts *[]specs.Mount) {
	if os.Getuid() == 0 {
		*mounts = append(*mounts,
			specs.Mount{
				Source:      "sysfs",
				Destination: "/sys",
				Type:        "sysfs",
				Options:     []string{"nosuid", "noexec", "nodev", "ro"},
			})
	} else {
		*mounts = append(*mounts,
			specs.Mount{
				Source:      "/sys",
				Destination: "/sys",
				Type:        "none",
				Options:     []string{"rbind", "nosuid", "noexec", "nodev", "ro"},
			})
	}
}

// addHomeMount adds a user home directory as a tmpfs mount. We are currently
// emulating `--compat` / `--containall`, so the user must specifically bind in
// their home directory from the host for it to be available.
func (l *Launcher) addHomeMount(mounts *[]specs.Mount) error {
	if l.cfg.Fakeroot {
		*mounts = append(*mounts,
			specs.Mount{
				Destination: "/root",
				Type:        "tmpfs",
				Source:      "tmpfs",
				Options: []string{
					"nosuid",
					"relatime",
					"mode=755",
					fmt.Sprintf("size=%dm", l.singularityConf.SessiondirMaxSize),
				},
			})
		return nil
	}

	pw, err := user.CurrentOriginal()
	if err != nil {
		return err
	}
	*mounts = append(*mounts,
		specs.Mount{
			Destination: pw.Dir,
			Type:        "tmpfs",
			Source:      "tmpfs",
			Options: []string{
				"nosuid",
				"relatime",
				"mode=755",
				fmt.Sprintf("size=%dm", l.singularityConf.SessiondirMaxSize),
				fmt.Sprintf("uid=%d", pw.UID),
				fmt.Sprintf("gid=%d", pw.GID),
			},
		})
	return nil
}

func (l *Launcher) addBindMounts(mounts *[]specs.Mount) error {
	// First get binds from -B/--bind and env var
	binds, err := bind.ParseBindPath(strings.Join(l.cfg.BindPaths, ","))
	if err != nil {
		return fmt.Errorf("while parsing bind path: %w", err)
	}
	// Now add binds from one or more --mount and env var.
	for _, m := range l.cfg.Mounts {
		bps, err := bind.ParseMountString(m)
		if err != nil {
			return fmt.Errorf("while parsing mount %q: %w", m, err)
		}
		binds = append(binds, bps...)
	}

	for _, b := range binds {
		if !l.singularityConf.UserBindControl {
			sylog.Warningf("Ignoring bind mount request: user bind control disabled by system administrator")
			return nil
		}
		if err := addBindMount(mounts, b); err != nil {
			return fmt.Errorf("while adding mount %q: %w", b.Source, err)
		}
	}
	return nil
}

func addBindMount(mounts *[]specs.Mount, b bind.Path) error {
	if b.ID() != "" || b.ImageSrc() != "" {
		return fmt.Errorf("image binds are not yet supported by the OCI runtime")
	}

	opts := []string{"rbind", "nosuid", "nodev"}
	if b.Readonly() {
		opts = append(opts, "ro")
	}

	absSource, err := filepath.Abs(b.Source)
	if err != nil {
		return fmt.Errorf("cannot determine absolute path of %s: %w", b.Source, err)
	}
	if _, err := os.Stat(absSource); err != nil {
		return fmt.Errorf("cannot stat bind source %s: %w", b.Source, err)
	}

	if !filepath.IsAbs(b.Destination) {
		return fmt.Errorf("bind destination %s must be an absolute path", b.Destination)
	}

	sylog.Debugf("Adding bind of %s to %s, with options %v", absSource, b.Destination, opts)

	*mounts = append(*mounts,
		specs.Mount{
			Source:      absSource,
			Destination: b.Destination,
			Type:        "none",
			Options:     opts,
		})
	return nil
}

func addDevBindMount(mounts *[]specs.Mount, b bind.Path) error {
	opts := []string{"bind", "nosuid"}
	if b.Readonly() {
		opts = append(opts, "ro")
	}

	b.Source = filepath.Clean(b.Source)
	if !strings.HasPrefix(b.Source, "/dev") {
		return fmt.Errorf("device bind source must be an absolute path under /dev: %s", b.Source)
	}
	if b.Source != b.Destination {
		return fmt.Errorf("device bind source %s must be the same as destination %s", b.Source, b.Destination)
	}
	if _, err := os.Stat(b.Source); err != nil {
		return fmt.Errorf("cannot stat bind source %s: %w", b.Source, err)
	}

	sylog.Debugf("Adding device bind of %s to %s, with options %v", b.Source, b.Destination, opts)

	*mounts = append(*mounts,
		specs.Mount{
			Source:      b.Source,
			Destination: b.Destination,
			Type:        "none",
			Options:     opts,
		})
	return nil
}

func (l *Launcher) addRocmMounts(mounts *[]specs.Mount) error {
	gpuConfFile := filepath.Join(buildcfg.SINGULARITY_CONFDIR, "rocmliblist.conf")

	libs, bins, err := gpu.RocmPaths(gpuConfFile)
	if err != nil {
		sylog.Warningf("While finding ROCm bind points: %v", err)
	}
	if len(libs) == 0 {
		sylog.Warningf("Could not find any ROCm libraries on this host!")
	}

	devs, err := gpu.RocmDevices()
	if err != nil {
		sylog.Warningf("While finding ROCm devices: %v", err)
	}
	if len(devs) == 0 {
		sylog.Warningf("Could not find any ROCm devices on this host!")
	}

	for _, binary := range bins {
		containerBinary := filepath.Join("/usr/bin", filepath.Base(binary))
		bind := bind.Path{
			Source:      binary,
			Destination: containerBinary,
			Options:     map[string]*bind.Option{"ro": {}},
		}
		if err := addBindMount(mounts, bind); err != nil {
			return err
		}
	}

	for _, lib := range libs {
		containerLib := filepath.Join(containerLibDir, filepath.Base(lib))
		bind := bind.Path{
			Source:      lib,
			Destination: containerLib,
			Options:     map[string]*bind.Option{"ro": {}},
		}
		if err := addBindMount(mounts, bind); err != nil {
			return err
		}
	}

	for _, dev := range devs {
		bind := bind.Path{
			Source:      dev,
			Destination: dev,
		}
		if err := addDevBindMount(mounts, bind); err != nil {
			return err
		}
	}

	return nil
}

func (l *Launcher) addNvidiaMounts(mounts *[]specs.Mount) error {
	if l.singularityConf.UseNvCCLI {
		sylog.Warningf("--nvccli not yet supported with --oci. Falling back to legacy --nv support.")
	}

	gpuConfFile := filepath.Join(buildcfg.SINGULARITY_CONFDIR, "nvliblist.conf")
	libs, bins, err := gpu.NvidiaPaths(gpuConfFile)
	if err != nil {
		sylog.Warningf("While finding Nvidia bind points: %v", err)
	}
	if len(libs) == 0 {
		sylog.Warningf("Could not find any Nvidia libraries on this host!")
	}

	ipcs, err := gpu.NvidiaIpcsPath()
	if err != nil {
		sylog.Warningf("While finding Nvidia IPCs: %v", err)
	}

	devs, err := gpu.NvidiaDevices(true)
	if err != nil {
		sylog.Warningf("While finding Nvidia devices: %v", err)
	}
	if len(devs) == 0 {
		sylog.Warningf("Could not find any ROCm devices on this host!")
	}

	for _, binary := range bins {
		containerBinary := filepath.Join("/usr/bin", filepath.Base(binary))
		bind := bind.Path{
			Source:      binary,
			Destination: containerBinary,
			Options:     map[string]*bind.Option{"ro": {}},
		}
		if err := addBindMount(mounts, bind); err != nil {
			return err
		}
	}

	for _, lib := range libs {
		containerLib := filepath.Join(containerLibDir, filepath.Base(lib))
		bind := bind.Path{
			Source:      lib,
			Destination: containerLib,
			Options:     map[string]*bind.Option{"ro": {}},
		}
		if err := addBindMount(mounts, bind); err != nil {
			return err
		}
	}

	for _, ipc := range ipcs {
		bind := bind.Path{
			Source:      ipc,
			Destination: ipc,
		}
		if err := addBindMount(mounts, bind); err != nil {
			return err
		}
	}

	for _, dev := range devs {
		bind := bind.Path{
			Source:      dev,
			Destination: dev,
		}
		if err := addDevBindMount(mounts, bind); err != nil {
			return err
		}
	}

	return nil
}
