// Copyright (c) 2019-2023, Sylabs Inc. All rights reserved.
// Copyright (c) Contributors to the Apptainer project, established as
//   Apptainer a Series of LF Projects LLC.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package build

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/samber/lo"
	"github.com/sylabs/singularity/v4/internal/pkg/util/fs"
	"github.com/sylabs/singularity/v4/pkg/util/fs/proc"
	"github.com/sylabs/singularity/v4/pkg/util/singularityconf"

	"github.com/sylabs/singularity/v4/internal/pkg/build/apps"
	"github.com/sylabs/singularity/v4/internal/pkg/build/args"
	"github.com/sylabs/singularity/v4/internal/pkg/build/assemblers"
	"github.com/sylabs/singularity/v4/internal/pkg/build/sources"
	"github.com/sylabs/singularity/v4/internal/pkg/buildcfg"
	"github.com/sylabs/singularity/v4/internal/pkg/util/uri"
	"github.com/sylabs/singularity/v4/pkg/build/types"
	"github.com/sylabs/singularity/v4/pkg/build/types/parser"
	"github.com/sylabs/singularity/v4/pkg/image"
	"github.com/sylabs/singularity/v4/pkg/sylog"
)

// Build is an abstracted way to look at the entire build process.
// For example calling NewBuild() will return this object.
// From there we can call Full() on this build object, which will:
//   - Call Bundle() to obtain all data needed to execute the specified build locally on the machine
//   - Execute all of a definition using AllSections()
//   - And finally call Assemble() to create our container image
type Build struct {
	// stages of the build
	stages []stage
	// Conf contains cross stage build configuration.
	Conf Config
}

// Config defines how build is executed, including things like where final image is written.
type Config struct {
	// Dest is the location for container after build is complete.
	Dest string
	// Format is the format of built container, e.g. SIF, sandbox.
	Format string
	// NoCleanUp allows a user to prevent a bundle from being cleaned
	// up after a failed build, useful for debugging.
	NoCleanUp bool
	// Opts for bundles.
	Opts types.Options
}

// NewBuild creates a new Build struct from a spec (URI, definition file, etc...).
func NewBuild(spec string, conf Config) (*Build, error) {
	def, err := makeDef(spec)
	if err != nil {
		return nil, fmt.Errorf("unable to parse spec %v: %v", spec, err)
	}

	return newBuild([]types.Definition{def}, conf)
}

// New creates a new build struct form a slice of definitions.
func New(defs []types.Definition, conf Config) (*Build, error) {
	return newBuild(defs, conf)
}

func newBuild(defs []types.Definition, conf Config) (*Build, error) {
	sandboxCopy := false
	oldumask := syscall.Umask(0o002)
	defer syscall.Umask(oldumask)

	dest, err := fs.Abs(conf.Dest)
	if err != nil {
		return nil, fmt.Errorf("failed to determine absolute path for %q: %v", conf.Dest, err)
	}
	conf.Dest = dest

	// always build a sandbox if updating an existing sandbox
	if conf.Opts.Update {
		conf.Format = "sandbox"
	}

	b := &Build{
		Conf: conf,
	}

	// look if there is mount options set which could conflict
	// with the build process like nodev and noexec
	entries, err := proc.GetMountInfoEntry("/proc/self/mountinfo")
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve mount information: %v", err)
	}

	lastStageIndex := len(defs) - 1

	// create stages
	for i, d := range defs {
		// verify every definition has a header if there are multiple stages
		if d.Header == nil {
			return nil, fmt.Errorf("multiple stages detected, all must have headers")
		}

		rootfsParent := conf.Opts.TmpDir
		if conf.Format == "sandbox" {
			rootfsParent = filepath.Dir(conf.Dest)
		}
		parentPath, err := os.MkdirTemp(rootfsParent, "build-temp-")
		if err != nil {
			return nil, fmt.Errorf("failed to create build parent dir: %w", err)
		}

		var s stage
		if conf.Opts.EncryptionKeyInfo != nil {
			s.b, err = types.NewEncryptedBundle(parentPath, conf.Opts.TmpDir, conf.Opts.EncryptionKeyInfo)
		} else {
			s.b, err = types.NewBundle(parentPath, conf.Opts.TmpDir)
		}
		if err != nil {
			return nil, err
		}
		s.name = d.Header["stage"]
		s.b.Recipe = d

		if conf.Format == "sandbox" && lastStageIndex == i {
			// rootfs path changed during bundle creation it means that chown
			// is not possible within the temporary rootfs, we will switch to
			// the old behavior which is to create the temporary rootfs inside
			// $TMPDIR and copy the final root filesystem to the destination
			// provided
			if !strings.HasPrefix(s.b.RootfsPath, parentPath) {
				sandboxCopy = true
				sylog.Warningf("The underlying filesystem on which resides %q won't allow to set ownership, "+
					"as a consequence the sandbox could not preserve image's files/directories ownerships", conf.Dest)
			} else {
				// check if the final sandbox directory doesn't have noexec set
				destEntry, err := proc.FindParentMountEntry(rootfsParent, entries)
				if err != nil {
					return nil, fmt.Errorf("failed to find mount point for %s: %v", rootfsParent, err)
				}
				for _, opt := range destEntry.Options {
					if opt == "noexec" {
						return nil, fmt.Errorf("'noexec' mount option set on %s, sandbox %s won't be usable at this location", destEntry.Point, conf.Dest)
					}
				}
			}
		}
		if lastStageIndex == i {
			// check if TMPDIR mount point have nodev and/or noexec set
			tmpdirEntry, err := proc.FindParentMountEntry(conf.Opts.TmpDir, entries)
			if err != nil {
				return nil, fmt.Errorf("failed to find mount point for %s: %v", conf.Opts.TmpDir, err)
			}
			for _, opt := range tmpdirEntry.Options {
				switch opt {
				case "nodev":
					sylog.Warningf("'nodev' mount option set on %s, it could be a source of failure during build process", tmpdirEntry.Point)
				case "noexec":
					return nil, fmt.Errorf("'noexec' mount option set on %s, temporary root filesystem won't be usable at this location", tmpdirEntry.Point)
				}
			}
		}

		s.b.Opts = conf.Opts
		// dont need to get cp if we're skipping bootstrap
		if !conf.Opts.Update || conf.Opts.Force {
			if c, err := NewConveyorPacker(d); err == nil {
				s.c = c
			} else {
				return nil, fmt.Errorf("unable to get conveyorpacker: %s", err)
			}
		}

		b.stages = append(b.stages, s)
	}

	// only need an assembler for last stage
	switch conf.Format {
	case "sandbox":
		b.stages[lastStageIndex].a = &assemblers.SandboxAssembler{Copy: sandboxCopy}
	case "sif":
		b.stages[lastStageIndex].a = &assemblers.SIFAssembler{}
	default:
		return nil, fmt.Errorf("unrecognized output format %s", conf.Format)
	}

	return b, nil
}

// cleanUp removes remnants of build from file system unless NoCleanUp is specified.
func (b Build) cleanUp() {
	if b.Conf.NoCleanUp {
		var bundlePaths []string
		for _, s := range b.stages {
			bundlePaths = append(bundlePaths, s.b.RootfsPath, s.b.TmpDir)
		}
		sylog.Infof("Build performed with no clean up option, build bundle(s) located at: %v", bundlePaths)
		return
	}

	for _, s := range b.stages {
		sylog.Debugf("Cleaning up %q and %q", s.b.RootfsPath, s.b.TmpDir)
		err := s.b.Remove()
		if err != nil {
			sylog.Errorf("Could not remove bundle: %v", err)
		}
	}
}

// Full runs a standard build from start to finish.
func (b *Build) Full(ctx context.Context) error {
	sylog.Infof("Starting build...")

	// monitor build for termination signal and clean up
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		b.cleanUp()
		os.Exit(1)
	}()
	// clean up build normally
	defer b.cleanUp()

	oldumask := syscall.Umask(0o002)

	// generate the default configuration
	config, err := singularityconf.Parse("")
	if err != nil {
		return err
	}
	config.BindPath = nil
	config.ConfigResolvConf = false
	config.MountHome = false
	config.MountDevPts = false

	// nvidia-container-cli path / ldconfig path will be needed by %post/%test
	// in builds run with --nv / --nvccli. Must grab paths from the main config.
	sysConfig := singularityconf.GetCurrentConfig()
	if sysConfig == nil {
		configFile := buildcfg.SINGULARITY_CONF_FILE
		sysConfig, err = singularityconf.Parse(configFile)
		if err != nil {
			return fmt.Errorf("could not parse %q: %v", configFile, err)
		}
	}
	config.LdconfigPath = sysConfig.LdconfigPath
	config.NvidiaContainerCliPath = sysConfig.NvidiaContainerCliPath

	var buffer bytes.Buffer

	if err := singularityconf.Generate(&buffer, "", config); err != nil {
		return fmt.Errorf("while generating configuration file: %s", err)
	}
	configData := buffer.Bytes()

	// build each stage one after the other
	for i, stage := range b.stages {
		if err := stage.runHostScript("pre", stage.b.Recipe.BuildData.Pre); err != nil {
			return err
		}

		// only update last stage if specified
		update := stage.b.Opts.Update && !stage.b.Opts.Force && i == len(b.stages)-1
		if update {
			// updating, extract dest container to bundle
			sylog.Infof("Building into existing container: %s", b.Conf.Dest)
			p, err := sources.GetLocalPacker(ctx, b.Conf.Dest, stage.b)
			if err != nil {
				return err
			}

			_, err = p.Pack(ctx)
			if err != nil {
				return err
			}
		} else {
			// regular build or force, start build from scratch
			if b.Conf.Opts.ImgCache == nil {
				return fmt.Errorf("undefined image cache")
			}
			if err := stage.c.Get(ctx, stage.b); err != nil {
				return fmt.Errorf("conveyor failed to get: %v", err)
			}

			_, err := stage.c.Pack(ctx)
			if err != nil {
				return fmt.Errorf("packer failed to pack: %v", err)
			}
		}

		// create apps in bundle
		a := apps.New()
		for k, v := range stage.b.Recipe.CustomData {
			a.HandleSection(k, v)
		}

		a.HandleBundle(stage.b)
		appPost, err := a.HandlePost(stage.b)
		if err != nil {
			return fmt.Errorf("unable to get app post information: %v", err)
		}
		stage.b.Recipe.BuildData.Post.Script += appPost

		// copy potential files from previous stage
		if stage.b.RunSection("files") {
			if err := stage.copyFilesFrom(b); err != nil {
				return fmt.Errorf("unable to copy files from stage to container fs: %v", err)
			}
		}

		if err := stage.runHostScript("setup", stage.b.Recipe.BuildData.Setup); err != nil {
			return err
		}

		// copy files from host
		if stage.b.RunSection("files") {
			if err := stage.copyFiles(); err != nil {
				return fmt.Errorf("unable to copy files from host to container fs: %v", err)
			}
		}

		// create stage file for /etc/resolv.conf and /etc/hosts
		sessionResolv, err := createStageFile("/etc/resolv.conf", stage.b, "Name resolution could fail")
		if err != nil {
			return err
		} else if sessionResolv != "" {
			defer os.Remove(sessionResolv)
		}
		sessionHosts, err := createStageFile("/etc/hosts", stage.b, "Host resolution could fail")
		if err != nil {
			return err
		} else if sessionHosts != "" {
			defer os.Remove(sessionHosts)
		}

		// write the build configuration used for %post and %test sections
		// as a root or non-setuid user.
		configFile := filepath.Join(stage.b.TmpDir, "singularity.conf")
		if err := os.WriteFile(configFile, configData, 0o644); err != nil {
			return fmt.Errorf("while creating %s: %s", configFile, err)
		}
		defer os.Remove(configFile)

		if stage.b.Recipe.BuildData.Post.Script != "" {
			if err := stage.runPostScript(configFile, sessionResolv, sessionHosts); err != nil {
				return fmt.Errorf("while running engine: %v", err)
			}
		}

		sylog.Debugf("Inserting Metadata")
		if err := stage.insertMetadata(); err != nil {
			return fmt.Errorf("while inserting metadata to bundle: %v", err)
		}

		if err := stage.runTestScript(configFile, sessionResolv, sessionHosts); err != nil {
			return fmt.Errorf("failed to execute %%test script: %v", err)
		}
	}

	syscall.Umask(oldumask)

	sylog.Debugf("Calling assembler")
	if err := b.stages[len(b.stages)-1].Assemble(b.Conf.Dest); err != nil {
		return err
	}

	sylog.Verbosef("Build complete: %s", b.Conf.Dest)
	return nil
}

// makeDef gets a definition object from a spec.
func makeDef(spec string) (types.Definition, error) {
	if ok, err := uri.IsValid(spec); ok && err == nil {
		// URI passed as spec
		return types.NewDefinitionFromURI(spec)
	}

	// Check if spec is an image/sandbox
	if _, err := image.Init(spec, false); err == nil {
		return types.NewDefinitionFromURI("localimage" + "://" + spec)
	}

	// default to reading file as definition
	defFile, err := os.Open(spec)
	if err != nil {
		return types.Definition{}, fmt.Errorf("unable to open file %s: %v", spec, err)
	}
	defer defFile.Close()

	d, err := parser.ParseDefinitionFile(defFile)
	if err != nil {
		return types.Definition{}, fmt.Errorf("while parsing definition: %s: %v", spec, err)
	}

	return d, nil
}

// MakeAllDefs gets a definition object from a spec
func MakeAllDefs(spec string, buildArgsMap map[string]string) ([]types.Definition, error) {
	if ok, err := uri.IsValid(spec); ok && err == nil {
		// URI passed as spec
		d, err := types.NewDefinitionFromURI(spec)
		return []types.Definition{d}, err
	}

	// check if spec is an image/sandbox
	if i, err := image.Init(spec, false); err == nil {
		_ = i.File.Close()
		d, err := types.NewDefinitionFromURI("localimage://" + spec)
		return []types.Definition{d}, err
	}

	// default to reading file as definition
	defFile, err := os.Open(spec)
	if err != nil {
		return nil, fmt.Errorf("unable to open file %s: %w", spec, err)
	}
	defer defFile.Close()

	defsPreBuildArgs, err := parser.All(defFile)
	nDefs := len(defsPreBuildArgs)
	if err != nil {
		return nil, fmt.Errorf("while parsing definition: %s: %w", spec, err)
	}

	revisedDefs := make([]types.Definition, 0, nDefs)
	var overallConsumedArgs []string
	for _, def := range defsPreBuildArgs {
		defaultArgsMap := args.ReadDefaults(def)

		reader, err := args.NewReader(
			bytes.NewReader(def.Raw),
			buildArgsMap,
			defaultArgsMap,
			&overallConsumedArgs,
		)
		if err != nil {
			return nil, err
		}

		revisedDef, err := parser.ParseDefinitionFile(reader)
		if err != nil {
			return nil, err
		}
		revisedDefs = append(revisedDefs, revisedDef)
	}

	totalRawLength := 0
	for _, def := range revisedDefs {
		totalRawLength += len(def.Raw)
	}

	fullRaw := make([]byte, 0, totalRawLength)
	for _, def := range revisedDefs {
		fullRaw = append(fullRaw, def.Raw...)
	}

	for i := range revisedDefs {
		revisedDefs[i].FullRaw = fullRaw
	}

	unusedArgs, _ := lo.Difference(lo.Keys(buildArgsMap), lo.Uniq(overallConsumedArgs))
	if len(unusedArgs) > 0 {
		sylog.Warningf("Unused build variables: %s", strings.Join(unusedArgs, ", "))
	}

	return revisedDefs, nil
}

func (b *Build) findStageIndex(name string) (int, error) {
	for i, s := range b.stages {
		if name == s.name {
			return i, nil
		}
	}

	return -1, fmt.Errorf("stage %s was not found", name)
}
