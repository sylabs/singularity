// Copyright (c) 2022-2023, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

// Package oci implements a Launcher that will configure and launch a container
// with an OCI runtime. It also provides implementations of OCI state
// transitions that can be called directly, Create/Start/Kill etc.
package oci

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/opencontainers/runtime-spec/specs-go"
	"github.com/samber/lo"
	"github.com/sylabs/singularity/v4/internal/pkg/buildcfg"
	"github.com/sylabs/singularity/v4/internal/pkg/ocisif"
	"github.com/sylabs/singularity/v4/internal/pkg/util/fs"
	"github.com/sylabs/singularity/v4/internal/pkg/util/fs/fuse"
	"github.com/sylabs/singularity/v4/internal/pkg/util/gpu"
	"github.com/sylabs/singularity/v4/internal/pkg/util/rootless"
	"github.com/sylabs/singularity/v4/internal/pkg/util/user"
	"github.com/sylabs/singularity/v4/pkg/image"
	"github.com/sylabs/singularity/v4/pkg/sylog"
	"github.com/sylabs/singularity/v4/pkg/util/bind"
	"github.com/sylabs/singularity/v4/pkg/util/slice"
)

const (
	containerLibDir = "/.singularity.d/libs"
	tmpDir          = "/tmp"
	varTmpDir       = "/var/tmp"
)

// getMounts returns a mount list for the container's OCI runtime spec.
func (l *Launcher) getMounts() ([]specs.Mount, error) {
	mounts := &[]specs.Mount{}
	if err := l.addProcMount(mounts); err != nil {
		return nil, fmt.Errorf("while configuring proc mount: %w", err)
	}
	if err := l.addSysMount(mounts); err != nil {
		return nil, fmt.Errorf("while configuring sys mount: %w", err)
	}
	if err := l.addDevMounts(mounts); err != nil {
		return nil, fmt.Errorf("while configuring devpts mount: %w", err)
	}

	// To facilitate handleVarTmpToTmpSymlink(), store indices of mounts added
	// by addTmpMounts(), in addition to adding them to spec.
	mountsCount := len(*mounts)
	if err := l.addTmpMounts(mounts); err != nil {
		return nil, fmt.Errorf("while configuring tmp mounts: %w", err)
	}
	for mountsCount < len(*mounts) {
		l.defaultTmpMountIndices = append(l.defaultTmpMountIndices, mountsCount)
		mountsCount++
	}

	if err := l.addHomeMount(mounts); err != nil {
		return nil, fmt.Errorf("while configuring home mount: %w", err)
	}
	if err := l.addScratchMounts(mounts); err != nil {
		return nil, fmt.Errorf("while configuring scratch mount(s): %w", err)
	}
	// System bind path mounts (singularity.conf) are only added with --no-compat (native emulation)
	if l.cfg.NoCompat {
		if err := l.addSystemBindMounts(mounts); err != nil {
			return nil, fmt.Errorf("while configuring system bind mount(s): %w", err)
		}
	}
	if err := l.addUserBindMounts(mounts); err != nil {
		return nil, fmt.Errorf("while configuring user bind mount(s): %w", err)
	}
	if l.cfg.NoCompat {
		if err := l.addCwdMount(mounts); err != nil {
			return nil, fmt.Errorf("while configuring cwd mount: %w", err)
		}
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
	if len(l.cfg.ContainLibs) > 0 {
		if err := l.addLibrariesMounts(mounts); err != nil {
			return nil, fmt.Errorf("while configuring containlibs mount(s): %w", err)
		}
	}

	return *mounts, nil
}

// addTmpMounts adds mounts for /tmp and /var/tmp in the container.
func (l *Launcher) addTmpMounts(mounts *[]specs.Mount) error {
	if !l.singularityConf.MountTmp {
		sylog.Debugf("Skipping mount of /tmp due to singularity.conf")
		return nil
	}
	if slice.ContainsString(l.cfg.NoMount, "tmp") {
		sylog.Debugf("Skipping mount of /tmp due to --no-mount")
		return nil
	}

	// Non-OCI compatibility, i.e. native mode emulation, binds from host by default.
	if l.cfg.NoCompat && !l.cfg.Contain && !l.cfg.ContainAll {
		return l.addTmpBinds(mounts)
	}

	if len(l.cfg.WorkDir) > 0 {
		sylog.Debugf("WorkDir specification provided: %s", l.cfg.WorkDir)
		const (
			tmpSrcSubdir    = "tmp"
			vartmpSrcSubdir = "var_tmp"
		)

		workdir, err := filepath.Abs(filepath.Clean(l.cfg.WorkDir))
		if err != nil {
			return fmt.Errorf("can't determine absolute path of workdir %s: %s", workdir, err)
		}

		tmpSrc := filepath.Join(workdir, tmpSrcSubdir)
		vartmpSrc := filepath.Join(workdir, vartmpSrcSubdir)

		if err := fs.Mkdir(tmpSrc, os.ModeSticky|0o777); err != nil && !os.IsExist(err) {
			return fmt.Errorf("failed to create %s: %s", tmpSrc, err)
		}
		if err := fs.Mkdir(vartmpSrc, os.ModeSticky|0o777); err != nil && !os.IsExist(err) {
			return fmt.Errorf("failed to create %s: %s", vartmpSrc, err)
		}

		opts := []string{
			"rbind",
			"relatime",
			"mode=777",
		}
		if !l.cfg.AllowSUID {
			opts = append(opts, "nosuid")
		}

		*mounts = append(*mounts,

			specs.Mount{
				Destination: tmpDir,
				Type:        "none",
				Source:      tmpSrc,
				Options:     opts,
			},
			specs.Mount{
				Destination: varTmpDir,
				Type:        "none",
				Source:      vartmpSrc,
				Options:     opts,
			},
		)

		return nil
	}

	sylog.Debugf(("No workdir specification provided. Proceeding with tmpfs mounts for /tmp and /var/tmp"))
	*mounts = append(*mounts,

		specs.Mount{
			Destination: tmpDir,
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
			Destination: varTmpDir,
			Type:        "tmpfs",
			Source:      "tmpfs",
			Options: []string{
				"nosuid",
				"relatime",
				"mode=777",
				fmt.Sprintf("size=%dm", l.singularityConf.SessiondirMaxSize),
			},
		},
	)

	return nil
}

// addTmpBinds adds tmpfs bind mounts from /tmp and /var/tmp on the host, into the container.
func (l *Launcher) addTmpBinds(mounts *[]specs.Mount) error {
	err := l.addBindMount(mounts,
		bind.Path{
			Source:      tmpDir,
			Destination: tmpDir,
		},
		l.cfg.AllowSUID)
	if err != nil {
		return err
	}

	return l.addBindMount(mounts,
		bind.Path{
			Source:      varTmpDir,
			Destination: varTmpDir,
		},
		l.cfg.AllowSUID)
}

// addDevMounts adds mounts to assemble a minimal /dev in the container.
func (l *Launcher) addDevMounts(mounts *[]specs.Mount) error {
	if l.singularityConf.MountDev == "no" {
		return fmt.Errorf("OCI-mode requires /dev to be mounted, but 'mount dev = no' in singularity.conf")
	}
	if slice.ContainsString(l.cfg.NoMount, "dev") {
		return fmt.Errorf("OCI-mode requires /dev to be mounted, but '--no-mount dev' was requested")
	}

	// When emulating native runtime (--no-compat), the default is to bind all of /dev into
	// the container. If --contain / --containall is used, or `mount dev = minimal` in
	// singularity.conf, then we use an OCI-like minimal dev.
	contained := l.cfg.Contain || l.cfg.ContainAll
	minimalDev := contained || (l.singularityConf.MountDev == "minimal")
	// Binding all of /dev like native mode only works with crun
	if l.cfg.NoCompat && !minimalDev {
		rt, err := Runtime()
		if err != nil {
			return err
		}
		if filepath.Base(rt) == "crun" {
			return l.addDevBind(mounts)
		}
		sylog.Warningf("Can only bind /dev when using 'crun' as OCI runtime. Currently using %q. Will use minimal /dev.", rt)
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

	return l.addDevPtsMount(mounts)
}

// addDevPtsMount adds a devpts mount in the container
func (l *Launcher) addDevPtsMount(mounts *[]specs.Mount) error {
	if !l.singularityConf.MountDevPts {
		return fmt.Errorf("OCI-mode requires /dev/pts to be mounted, but 'mount devpts = no' in singularity.conf")
	}
	if slice.ContainsString(l.cfg.NoMount, "devpts") {
		return fmt.Errorf("OCI-mode requires /dev/pts to be mounted, but '--no-mount devpts' was requested")
	}

	ptsMount := specs.Mount{
		Destination: "/dev/pts",
		Type:        "devpts",
		Source:      "devpts",
		Options:     []string{"nosuid", "noexec", "newinstance", "ptmxmode=0666", "mode=0620"},
	}

	rootlessUID, err := rootless.Getuid()
	if err != nil {
		return fmt.Errorf("while fetching uid: %w", err)
	}

	if rootlessUID == 0 {
		group, err := user.GetGrNam("tty")
		if err != nil {
			return fmt.Errorf("while identifying tty gid: %w", err)
		}
		ptsMount.Options = append(ptsMount.Options, fmt.Sprintf("gid=%d", group.GID))
	}

	*mounts = append(*mounts, ptsMount)
	return nil
}

// addDevBind bind mounts all of /dev into the container.
func (l *Launcher) addDevBind(mounts *[]specs.Mount) error {
	*mounts = append(*mounts,
		specs.Mount{
			Destination: "/dev",
			Type:        "bind",
			Source:      "/dev",
			Options: []string{
				"nosuid",
				"rprivate",
				"rbind",
				"rw",
			},
		},
	)
	// Still need a /dev/pts mount with correct perms
	return l.addDevPtsMount(mounts)
}

// addProcMount adds the /proc tree in the container.
func (l *Launcher) addProcMount(mounts *[]specs.Mount) error {
	if !l.singularityConf.MountProc {
		sylog.Debugf("Skipping mount of /proc due to singularity.conf")
		return nil
	}
	if slice.ContainsString(l.cfg.NoMount, "proc") {
		sylog.Debugf("Skipping mount of /proc due to --no-mount")
		return nil
	}

	if l.cfg.Namespaces.NoPID {
		return l.addBindMount(mounts,
			bind.Path{
				Source:      "/proc",
				Destination: "/proc",
			},
			false)
	}

	*mounts = append(*mounts,
		specs.Mount{
			Source:      "proc",
			Destination: "/proc",
			Type:        "proc",
			Options:     []string{"nosuid", "noexec", "nodev"},
		})
	return nil
}

// addSysMount adds the /sys tree in the container.
func (l *Launcher) addSysMount(mounts *[]specs.Mount) error {
	if !l.singularityConf.MountSys {
		sylog.Debugf("Skipping mount of /sys due to singularity.conf")
		return nil
	}
	if slice.ContainsString(l.cfg.NoMount, "sys") {
		sylog.Debugf("Skipping mount of /sys due to --no-mount")
		return nil
	}

	rootlessUID, err := rootless.Getuid()
	if err != nil {
		return fmt.Errorf("while fetching uid: %w", err)
	}

	if rootlessUID == 0 {
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

	return nil
}

// addHomeMount adds the user home directory to the container, according to the
// src and dest computed by parseHomeDir from launcher.New.
func (l *Launcher) addHomeMount(mounts *[]specs.Mount) error {
	if !l.singularityConf.MountHome {
		sylog.Debugf("Skipping mount of $HOME due to singularity.conf")
		return nil
	}
	if l.cfg.NoHome {
		sylog.Debugf("Skipping mount of $HOME due to --no-home")
		return nil
	}
	if slice.ContainsString(l.cfg.NoMount, "home") {
		sylog.Debugf("Skipping mount of /home due to --no-mount")
		return nil
	}

	if l.homeDest == "" {
		return fmt.Errorf("cannot add home mount with empty destination")
	}

	// In --no-compat we bind $HOME from host like native mode default.
	if l.cfg.NoCompat && l.homeSrc == "" {
		l.homeSrc = l.homeHost
	}

	// If l.homeSrc is set, then we are simply bind mounting from the host.
	if l.homeSrc != "" {
		return l.addBindMount(mounts,
			bind.Path{
				Source:      l.homeSrc,
				Destination: l.homeDest,
			},
			l.cfg.AllowSUID)
	}

	// Otherwise we setup a tmpfs, mounted onto l.homeDst.
	tmpfsOpt := []string{
		"relatime",
		"mode=755",
		fmt.Sprintf("size=%dm", l.singularityConf.SessiondirMaxSize),
	}
	if !l.cfg.AllowSUID {
		tmpfsOpt = append(tmpfsOpt, "nosuid")
	}

	// If we aren't using fakeroot, ensure the tmpfs ownership is correct for our real uid/gid.
	if !l.cfg.Fakeroot {
		uid, err := rootless.Getuid()
		if err != nil {
			return fmt.Errorf("while fetching uid: %w", err)
		}
		gid, err := rootless.Getgid()
		if err != nil {
			return fmt.Errorf("while fetching gid: %w", err)
		}

		tmpfsOpt = append(tmpfsOpt,
			fmt.Sprintf("uid=%d", uid),
			fmt.Sprintf("gid=%d", gid),
		)
	}

	*mounts = append(*mounts,
		specs.Mount{
			Destination: l.homeDest,
			Type:        "tmpfs",
			Source:      "tmpfs",
			Options:     tmpfsOpt,
		})

	return nil
}

// addScratchMounts adds tmpfs mounts for scratch directories in the container.
func (l *Launcher) addScratchMounts(mounts *[]specs.Mount) error {
	const scratchContainerDirName = "/scratch"

	if len(l.cfg.WorkDir) > 0 {
		workdir, err := filepath.Abs(filepath.Clean(l.cfg.WorkDir))
		if err != nil {
			return fmt.Errorf("can't determine absolute path of workdir %s: %s", workdir, err)
		}
		scratchContainerDirPath := filepath.Join(workdir, scratchContainerDirName)
		if err := fs.Mkdir(scratchContainerDirPath, os.ModeSticky|0o777); err != nil && !os.IsExist(err) {
			return fmt.Errorf("failed to create %s: %s", scratchContainerDirPath, err)
		}

		for _, s := range l.cfg.ScratchDirs {
			scratchDirPath := filepath.Join(scratchContainerDirPath, s)
			if err := fs.Mkdir(scratchDirPath, os.ModeSticky|0o777); err != nil && !os.IsExist(err) {
				return fmt.Errorf("failed to create %s: %s", scratchDirPath, err)
			}

			opts := []string{
				"rbind",
				"relatime",
				"nodev",
			}
			if !l.cfg.AllowSUID {
				opts = append(opts, "nosuid")
			}

			*mounts = append(*mounts,
				specs.Mount{
					Destination: s,
					Type:        "",
					Source:      scratchDirPath,
					Options:     opts,
				},
			)
		}
	} else {
		opts := []string{
			"relatime",
			"nodev",
			fmt.Sprintf("size=%dm", l.singularityConf.SessiondirMaxSize),
		}
		if !l.cfg.AllowSUID {
			opts = append(opts, "nosuid")
		}

		for _, s := range l.cfg.ScratchDirs {
			*mounts = append(*mounts,
				specs.Mount{
					Destination: s,
					Type:        "tmpfs",
					Source:      "tmpfs",
					Options:     opts,
				},
			)
		}
	}

	return nil
}

func (l *Launcher) addSystemBindMounts(mounts *[]specs.Mount) error {
	if slice.ContainsString(l.cfg.NoMount, "bind-paths") {
		sylog.Debugf("Skipping singularity.conf bind path entries due to --no-mount bind-paths")
		return nil
	}

	binds, err := bind.ParseBindPath(strings.Join(l.singularityConf.BindPath, ","))
	if err != nil {
		return fmt.Errorf("while parsing singularity.conf bind path: %w", err)
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
		if slice.ContainsString(l.cfg.NoMount, b.Destination) {
			continue
		}
		if err := l.addBindMount(mounts, b, l.cfg.AllowSUID); err != nil {
			return fmt.Errorf("while adding mount %q: %w", b.Source, err)
		}
	}
	return nil
}

func (l *Launcher) addUserBindMounts(mounts *[]specs.Mount) error {
	if !l.singularityConf.UserBindControl {
		sylog.Warningf("Ignoring bind mount request(s): user bind control disabled by system administrator")
		return nil
	}

	// First get binds from -B/--bind and env var
	binds, err := bind.ParseBindPath(strings.Join(l.cfg.BindPaths, ","))
	if err != nil {
		return fmt.Errorf("while parsing bind path: %w", err)
	}
	// A --data bind is an image-src=/ bind from an OCI-SIF data container image.
	for _, d := range l.cfg.DataBinds {
		bp, err := bind.ParseDataBindPath(d)
		if err != nil {
			return err
		}
		binds = append(binds, bp)
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
		// Special Case - user is manually requesting all of /dev to be bound
		if b.Source == "/dev" {
			if b.Destination != "/dev" {
				return fmt.Errorf("host /dev may only be bound to /dev in container")
			}
			// Handle as we do in --no-compat, including devpts setup.
			if err := l.addDevBind(mounts); err != nil {
				return fmt.Errorf("while adding mount %q: %w", b.Source, err)
			}
			continue
		}
		// Anything else
		if err := l.addBindMount(mounts, b, l.cfg.AllowSUID); err != nil {
			return fmt.Errorf("while adding mount %q: %w", b.Source, err)
		}
	}

	return nil
}

// addCwdMount adds the CWD to the container.
func (l *Launcher) addCwdMount(mounts *[]specs.Mount) error {
	if slice.ContainsString(l.cfg.NoMount, "cwd") {
		sylog.Debugf("Skipping mount of cwd due to --no-mount")
		return nil
	}

	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	return l.addBindMount(mounts,
		bind.Path{
			Source:      cwd,
			Destination: cwd,
		},
		l.cfg.AllowSUID,
	)
}

func (l *Launcher) addBindMount(mounts *[]specs.Mount, b bind.Path, allowSUID bool) (err error) {
	// If request is for a /dev/xxx device, then we handle with device specific checks and flags.
	if strings.HasPrefix(b.Source, "/dev") {
		return addDevBindMount(mounts, b)
	}

	if b.ID() != "" || b.ImageSrc() != "" {
		if !l.singularityConf.UserBindControl {
			sylog.Warningf("Ignoring image bind mount request: user bind control disabled by system administrator")
			return nil
		}

		im, err := l.prepareImageBindMount(b)
		if err != nil {
			return err
		}

		sylog.Debugf("mountpoint for image bind-mount is: %q", im.GetMountPoint())

		bindMountSource := filepath.Join(im.GetMountPoint(), b.ImageSrc())

		return l.addBindMount(
			mounts,
			bind.Path{
				Source:      bindMountSource,
				Destination: b.Destination,
			},
			allowSUID,
		)
	}

	// /proc and /sys need nosuid,noexec,nodev set explicitly for runc
	// See: https://github.com/opencontainers/runc/discussions/3801
	opts := map[string]bool{
		"rbind": true,
		"nodev": true,
	}
	if strings.HasPrefix(b.Source, "/proc") || strings.HasPrefix(b.Source, "/sys") {
		opts["nosuid"] = true
		opts["noexec"] = true
	}
	if !allowSUID {
		opts["nosuid"] = true
	}
	if b.Readonly() {
		opts["ro"] = true
	}

	absSource, err := filepath.Abs(b.Source)
	if err != nil {
		return fmt.Errorf("cannot determine absolute path of %s: %w", b.Source, err)
	}
	if _, err := os.Stat(absSource); err != nil {
		_, ok := l.imageMountsByMountpoint[absSource]
		if !(errors.Is(err, os.ErrNotExist) && ok) {
			return fmt.Errorf("cannot stat bind source %s: %w", b.Source, err)
		}
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
			Options:     lo.Keys(opts),
		})

	return nil
}

func (l *Launcher) prepareImageBindMount(bindPath bind.Path) (*fuse.ImageMount, error) {
	// We don't support mounting from native style SIF images with a descriptor ID in OCI-Mode.
	if bindPath.ID() != "" {
		return nil, fmt.Errorf("image bind id values are not supported, but one was supplied (%v %v)", bindPath.ImageSrc(), bindPath.ID())
	}

	imagePath := bindPath.Source
	img, err := image.Init(imagePath, false)
	if err != nil {
		return nil, err
	}

	resolvedPath := img.Path
	readonly := bindPath.Readonly()
	opts := []string{}

	switch img.Type {
	case image.EXT3:
	case image.SQUASHFS:
		readonly = true
	case image.OCISIF:
		readonly = true
		offset, err := ocisif.DataContainerLayerOffset(img.File)
		if err != nil {
			return nil, err
		}
		opts = append(opts, fmt.Sprintf("offset=%d", offset))
	default:
		return nil, fmt.Errorf("unsupported image type %v for image bind", img.Type)
	}

	enclosingDir, err := os.MkdirTemp(buildcfg.SESSIONDIR, "fusemount-enclosure")
	if err != nil {
		return nil, err
	}

	im := fuse.ImageMount{
		Type:         img.Type,
		Readonly:     readonly,
		SourcePath:   resolvedPath,
		EnclosingDir: enclosingDir,
		AllowSetuid:  l.cfg.AllowSUID,
		AllowOther:   true,
		ExtraOpts:    opts,
	}

	mountpoint := filepath.Join(enclosingDir, fmt.Sprintf("fusemount-%d", len(l.imageMountsByMountpoint)))
	im.SetMountPoint(mountpoint)

	if otherIm, ok := l.imageMountsByImagePath[resolvedPath]; ok && !otherIm.Readonly && !im.Readonly {
		return nil, fmt.Errorf("image %q already mounted as writable once; cannot mount it again as writable at %q", resolvedPath, bindPath.Destination)
	}

	l.imageMountsByImagePath[resolvedPath] = &im
	l.imageMountsByMountpoint[mountpoint] = &im

	return &im, nil
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
		if err := l.addBindMount(mounts, bind, false); err != nil {
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
		if err := l.addBindMount(mounts, bind, false); err != nil {
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
		sylog.Warningf("While finding NVIDIA bind points: %v", err)
	}
	if len(libs) == 0 {
		sylog.Warningf("Could not find any NVIDIA libraries on this host!")
	}

	ipcs, err := gpu.NvidiaIpcsPath()
	if err != nil {
		sylog.Warningf("While finding NVIDIA IPCs: %v", err)
	}

	devs, err := gpu.NvidiaDevices(true)
	if err != nil {
		sylog.Warningf("While finding NVIDIA devices: %v", err)
	}
	if len(devs) == 0 {
		sylog.Warningf("Could not find any NVIDIA devices on this host!")
	}

	for _, binary := range bins {
		containerBinary := filepath.Join("/usr/bin", filepath.Base(binary))
		bind := bind.Path{
			Source:      binary,
			Destination: containerBinary,
			Options:     map[string]*bind.Option{"ro": {}},
		}
		if err := l.addBindMount(mounts, bind, false); err != nil {
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
		if err := l.addBindMount(mounts, bind, false); err != nil {
			return err
		}
	}

	for _, ipc := range ipcs {
		bind := bind.Path{
			Source:      ipc,
			Destination: ipc,
		}
		if err := l.addBindMount(mounts, bind, false); err != nil {
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

func (l *Launcher) addLibrariesMounts(mounts *[]specs.Mount) error {
	if !l.singularityConf.UserBindControl {
		sylog.Warningf("Ignoring containlibs mount request: user bind control disabled by system administrator")
		return nil
	}

	for _, lib := range l.cfg.ContainLibs {
		containerLib := filepath.Join(containerLibDir, filepath.Base(lib))
		bind := bind.Path{
			Source:      lib,
			Destination: containerLib,
			Options:     map[string]*bind.Option{"ro": {}},
		}
		if err := l.addBindMount(mounts, bind, false); err != nil {
			return err
		}
	}

	return nil
}
