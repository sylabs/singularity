// Copyright (c) 2018-2023, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package build

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"

	"github.com/sylabs/singularity/v4/internal/pkg/build/files"
	"github.com/sylabs/singularity/v4/internal/pkg/buildcfg"
	"github.com/sylabs/singularity/v4/pkg/build/types"
	"github.com/sylabs/singularity/v4/pkg/sylog"
)

// stage represents the process of constructing a root filesystem.
type stage struct {
	// name of the stage.
	name string
	// c Gets and Packs data needed to build a container into a Bundle from various sources.
	c ConveyorPacker
	// a Assembles a container from the information stored in a Bundle into various formats.
	a Assembler
	// b is an intermediate structure that encapsulates all information for the container, e.g., metadata, filesystems.
	b *types.Bundle
}

const (
	sLabelsPath  = "/.build.labels"
	sEnvironment = "SINGULARITY_ENVIRONMENT=/.singularity.d/env/91-environment.sh"
	sLabels      = "SINGULARITY_LABELS=" + sLabelsPath
)

// Assemble assembles the bundle to the specified path.
func (s *stage) Assemble(path string) error {
	return s.a.Assemble(s.b, path)
}

// runHostScript executes the stage's pre or setup script on host.
func (s *stage) runHostScript(name string, script types.Script) error {
	if s.b.RunSection(name) && script.Script != "" {
		if syscall.Getuid() != 0 {
			return fmt.Errorf("%%pre and %%setup scripts are only supported in root user or --fakeroot builds")
		}

		sRootfs := "SINGULARITY_ROOTFS=" + s.b.RootfsPath

		scriptPath := filepath.Join(s.b.TmpDir, name)
		if err := createScript(scriptPath, []byte(script.Script)); err != nil {
			return fmt.Errorf("while creating %s script: %s", name, err)
		}
		defer os.Remove(scriptPath)

		args, err := getSectionScriptArgs(name, scriptPath, script)
		if err != nil {
			return fmt.Errorf("while processing section %%%s arguments: %s", name, err)
		}

		// Run script section here
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Env = os.Environ()
		cmd.Env = append(cmd.Env, sEnvironment, sRootfs)

		sylog.Infof("Running %s scriptlet", name)
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to run %%%s script: %v", name, err)
		}
	}
	return nil
}

func (s *stage) runPostScript(configFile, sessionResolv, sessionHosts string) error {
	if s.b.Recipe.BuildData.Post.Script != "" {
		useBuildConfig := os.Geteuid() == 0 || buildcfg.SINGULARITY_SUID_INSTALL == 0

		cmdArgs := []string{}
		if useBuildConfig {
			cmdArgs = append(cmdArgs, "-c", configFile)
		}

		cmdArgs = append(cmdArgs, "-s", "exec", "--pwd", "/", "--writable")
		cmdArgs = append(cmdArgs, "--cleanenv", "--env", sEnvironment, "--env", sLabels)

		// As non-root, non-fakeroot we must use the system config, subtracting any
		// bind path, home, and devpts mounts.
		if !useBuildConfig {
			cmdArgs = append(cmdArgs, "--no-mount", "bind-paths,home,devpts")
		}

		if sessionResolv != "" {
			cmdArgs = append(cmdArgs, "-B", sessionResolv+":/etc/resolv.conf")
		}
		if sessionHosts != "" {
			cmdArgs = append(cmdArgs, "-B", sessionHosts+":/etc/hosts")
		}

		script := s.b.Recipe.BuildData.Post
		scriptPath := filepath.Join(s.b.RootfsPath, ".post.script")
		if err := createScript(scriptPath, []byte(script.Script)); err != nil {
			return fmt.Errorf("while creating post script: %s", err)
		}
		defer os.Remove(scriptPath)

		args, err := getSectionScriptArgs("post", "/.post.script", script)
		if err != nil {
			return fmt.Errorf("while processing section %%post arguments: %s", err)
		}

		cmdArgs = append(cmdArgs, s.b.RootfsPath)

		if os.Getenv("SINGULARITY_PROOT") != "" {
			cmdArgs = append(cmdArgs, "/.singularity.d/libs/proot", "-0")
		}

		cmdArgs = append(cmdArgs, args...)

		exe := filepath.Join(buildcfg.BINDIR, "singularity")
		cmd := exec.Command(exe, cmdArgs...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Dir = "/"
		cmd.Env = currentEnvNoSingularity([]string{"DEBUG", "NV", "NVCCLI", "ROCM", "BINDPATH", "MOUNT", "PROOT"})

		sylog.Infof("Running post scriptlet")
		return cmd.Run()
	}
	return nil
}

func (s *stage) runTestScript(configFile, sessionResolv, sessionHosts string) error {
	if !s.b.Opts.NoTest && s.b.Recipe.BuildData.Test.Script != "" {
		useBuildConfig := os.Geteuid() == 0 || buildcfg.SINGULARITY_SUID_INSTALL == 0

		cmdArgs := []string{}
		if useBuildConfig {
			cmdArgs = append(cmdArgs, "-c", configFile)
		}

		cmdArgs = append(cmdArgs, "-s", "test", "--pwd", "/")

		// As non-root, non-fakeroot we must use the system config, subtracting any
		// bind path, home, and devpts mounts.
		if !useBuildConfig {
			cmdArgs = append(cmdArgs, "--no-mount", "bind-paths,home,devpts")
		}

		if sessionResolv != "" {
			cmdArgs = append(cmdArgs, "-B", sessionResolv+":/etc/resolv.conf")
		}
		if sessionHosts != "" {
			cmdArgs = append(cmdArgs, "-B", sessionHosts+":/etc/hosts")
		}

		cmdArgs = append(cmdArgs, s.b.RootfsPath)

		exe := filepath.Join(buildcfg.BINDIR, "singularity")
		cmd := exec.Command(exe, cmdArgs...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Dir = "/"
		cmd.Env = currentEnvNoSingularity([]string{"DEBUG", "NV", "NVCCLI", "ROCM", "BINDPATH", "MOUNT", "WRITABLE_TMPFS", "PROOT"})

		sylog.Infof("Running testscript")
		return cmd.Run()
	}
	return nil
}

func (s *stage) copyFilesFrom(b *Build) error {
	def := s.b.Recipe
	for _, f := range def.BuildData.Files {
		stageName := f.Stage()
		if stageName == "" {
			continue
		}

		stageIndex, err := b.findStageIndex(stageName)
		if err != nil {
			return err
		}

		srcRootfsPath := b.stages[stageIndex].b.RootfsPath
		dstRootfsPath := s.b.RootfsPath

		sylog.Debugf("Copying files from stage: %s", stageName)

		// iterate through filetransfers
		for _, transfer := range f.Files {
			// sanity
			if transfer.Src == "" {
				sylog.Warningf("Attempt to copy file with no name, skipping.")
				continue
			}
			// copy each file into bundle rootfs
			// Disable IDMapping entirely if it's a proot build
			proot := os.Getenv("SINGULARITY_PROOT") != ""
			sylog.Infof("Copying %v to %v", transfer.Src, transfer.Dst)
			if err := files.CopyFromStage(transfer.Src, transfer.Dst, srcRootfsPath, dstRootfsPath, proot); err != nil {
				return err
			}
		}
	}

	return nil
}

func (s *stage) copyFiles() error {
	def := s.b.Recipe
	filesSection := types.Files{}
	for _, f := range def.BuildData.Files {
		if f.Stage() == "" {
			filesSection.Files = append(filesSection.Files, f.Files...)
		}
	}
	// iterate through filetransfers
	for _, transfer := range filesSection.Files {
		// sanity
		if transfer.Src == "" {
			sylog.Warningf("Attempt to copy file with no name, skipping.")
			continue
		}
		// copy each file into bundle rootfs
		sylog.Infof("Copying %v to %v", transfer.Src, transfer.Dst)
		if err := files.CopyFromHost(transfer.Src, transfer.Dst, s.b.RootfsPath); err != nil {
			return err
		}
	}

	return nil
}
