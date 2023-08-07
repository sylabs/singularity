// Copyright (c) 2023, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package fuse

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/sylabs/singularity/v4/internal/pkg/util/bin"
	"github.com/sylabs/singularity/v4/pkg/image"
	"github.com/sylabs/singularity/v4/pkg/sylog"
)

type ImageMount struct {
	// Type represents what type of image this mount involves (from among the
	// values in pkg/image)
	Type int

	// Readonly represents whether this is a Readonly overlay
	Readonly bool

	// SourcePath is the path of the image, stripped of any colon-prefixed
	// options (like ":ro")
	SourcePath string

	// EnclosingDir is the location of a secure parent-directory in
	// which to create the actual mountpoint directory
	EnclosingDir string

	// mountpoint is the directory at which the image will be mounted
	mountpoint string

	// AllowSetuid is set to true to mount the image the "nosuid" option.
	AllowSetuid bool

	// AllowDev is set to true to mount the image without the "nodev" option.
	AllowDev bool

	// AllowOther is set to true to mount the image with the "allow_other" option.
	AllowOther bool
}

// Mount mounts an image to a temporary directory. It also verifies that
// the fusermount utility is present before performing the mount.
func (i *ImageMount) Mount() (err error) {
	fuseMountCmd, err := i.determineMountCmd()
	if err != nil {
		return err
	}

	args, err := i.generateCmdArgs()
	if err != nil {
		return err
	}

	fuseCmdLine := fmt.Sprintf("%s %s", fuseMountCmd, strings.Join(args, " "))
	sylog.Debugf("Executing FUSE mount command: %q", fuseCmdLine)
	execCmd := exec.Command(fuseMountCmd, args...)
	execCmd.Stderr = os.Stderr
	_, err = execCmd.Output()
	if err != nil {
		return fmt.Errorf("encountered error while trying to mount image %q as overlay at %s: %w", i.SourcePath, i.mountpoint, err)
	}

	exitCode := execCmd.ProcessState.ExitCode()
	if exitCode != 0 {
		return fmt.Errorf("FUSE mount command %q returned non-zero exit code (%d)", fuseCmdLine, exitCode)
	}

	return err
}

func (i *ImageMount) determineMountCmd() (string, error) {
	var fuseMountTool string
	switch i.Type {
	case image.SQUASHFS:
		fuseMountTool = "squashfuse"
	case image.EXT3:
		fuseMountTool = "fuse2fs"
	default:
		return "", fmt.Errorf("image %q is not of a type that can be mounted with FUSE (type: %v)", i.SourcePath, i.Type)
	}

	fuseMountCmd, err := bin.FindBin(fuseMountTool)
	if err != nil {
		return "", fmt.Errorf("use of image %q as overlay requires %s to be installed: %w", i.SourcePath, fuseMountTool, err)
	}

	return fuseMountCmd, nil
}

func (i *ImageMount) generateCmdArgs() ([]string, error) {
	args := make([]string, 0, 4)

	switch i.Type {
	case image.SQUASHFS:
		i.Readonly = true
	}

	// Even though fusermount is not needed for this step, we shouldn't perform
	// the mount unless we have the necessary tools to eventually unmount it
	_, err := bin.FindBin("fusermount")
	if err != nil {
		return args, fmt.Errorf("use of image %q as overlay requires fusermount to be installed: %w", i.SourcePath, err)
	}

	if i.mountpoint == "" {
		i.mountpoint, err = os.MkdirTemp(i.EnclosingDir, "mountpoint-")
		if err != nil {
			return args, fmt.Errorf("failed to create temporary dir %q for overlay %q: %w", i.mountpoint, i.SourcePath, err)
		}
	}

	// Best effort to cleanup temporary dir
	defer func() {
		if err != nil {
			sylog.Debugf("Encountered error with image %q; attempting to remove %q", i.SourcePath, i.mountpoint)
			os.Remove(i.mountpoint)
		}
	}()

	// TODO: Think through what makes sense for file ownership in FUSE-mounted
	// images, vis a vis id-mappings and user-namespaces.
	opts := []string{"uid=0", "gid=0"}
	if i.Readonly {
		// Not strictly necessary as will be read-only in assembled overlay,
		// however this stops any erroneous writes through the stagingDir.
		opts = append(opts, "ro")
	}
	// FUSE defaults to nosuid,nodev - attempt to reverse if AllowDev/Setuid requested.
	if i.AllowDev {
		opts = append(opts, "dev")
	}
	if i.AllowSetuid {
		opts = append(opts, "suid")
	}
	if i.AllowOther {
		opts = append(opts, "allow_other")
	}

	if len(opts) > 0 {
		args = append(args, "-o", strings.Join(opts, ","))
	}

	args = append(args, i.SourcePath)
	args = append(args, i.mountpoint)

	return args, nil
}

func (i ImageMount) GetMountPoint() string {
	return i.mountpoint
}

func (i *ImageMount) SetMountPoint(mountpoint string) {
	i.mountpoint = mountpoint
}

func (i ImageMount) Unmount() error {
	return UnmountWithFuse(i.GetMountPoint())
}

// UnmountWithFuse performs an unmount on the specified directory using
// fusermount -u.
func UnmountWithFuse(dir string) error {
	fusermountCmd, err := bin.FindBin("fusermount")
	if err != nil {
		// We should not be creating FUSE-based mounts in the first place
		// without checking that fusermount is available.
		return fmt.Errorf("fusermount not available while trying to perform unmount: %w", err)
	}
	sylog.Debugf("Executing FUSE unmount command: %s -u %s", fusermountCmd, dir)
	execCmd := exec.Command(fusermountCmd, "-u", dir)
	execCmd.Stderr = os.Stderr
	_, err = execCmd.Output()
	return err
}
