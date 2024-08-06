// Copyright (c) 2019-2024, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

//go:build singularity_engine

package squashfs

import (
	"bytes"
	"fmt"
	"os/exec"

	"github.com/sylabs/singularity/v4/internal/pkg/buildcfg"
	"github.com/sylabs/singularity/v4/internal/pkg/util/bin"
	"github.com/sylabs/singularity/v4/pkg/sylog"
	"github.com/sylabs/singularity/v4/pkg/util/singularityconf"
)

func getConfig() (*singularityconf.File, error) {
	// if the caller has set the current config use it
	// otherwise parse the default configuration file
	cfg := singularityconf.GetCurrentConfig()
	if cfg == nil {
		var err error

		configFile := buildcfg.SINGULARITY_CONF_FILE
		cfg, err = singularityconf.Parse(configFile)
		if err != nil {
			return nil, fmt.Errorf("unable to parse singularity.conf file: %s", err)
		}
	}
	return cfg, nil
}

// mksquashfsOpts accumulates mksquashfs options.
type mksquashfsOpts struct {
	path      string
	procs     uint
	mem       string
	comp      string
	allRoot   bool
	wildcards bool
	excludes  []string
}

func defaultPath() (string, error) {
	return bin.FindBin("mksquashfs")
}

func defaultProcs() (uint, error) {
	c, err := getConfig()
	if err != nil {
		return 0, err
	}
	// proc is either "" or the string value in the conf file
	proc := c.MksquashfsProcs

	return proc, err
}

func defaultMem() (string, error) {
	c, err := getConfig()
	if err != nil {
		return "", err
	}
	// mem is either "" or the string value in the conf file
	mem := c.MksquashfsMem

	return mem, err
}

func defaultOpts() (*mksquashfsOpts, error) {
	path, err := defaultPath()
	if err != nil {
		return nil, err
	}
	procs, err := defaultProcs()
	if err != nil {
		return nil, err
	}
	mem, err := defaultMem()
	if err != nil {
		return nil, err
	}
	opts := &mksquashfsOpts{
		path:    path,
		procs:   procs,
		mem:     mem,
		comp:    "gzip",
		allRoot: false,
	}
	return opts, nil
}

// MksquashfsOpt are used to specify options to apply when creating a squashfs.
type MksquashfsOpt func(*mksquashfsOpts) error

// OptPath sets the path to the mksquashfs binary.
func OptPath(p string) MksquashfsOpt {
	return func(o *mksquashfsOpts) error {
		o.path = p
		return nil
	}
}

// OptProcs sets the number of processors to use when creating a squashfs.
func OptProcs(p uint) MksquashfsOpt {
	return func(o *mksquashfsOpts) error {
		o.procs = p
		return nil
	}
}

// OptMem sets the memory to use when creating a squashfs.
func OptMem(m string) MksquashfsOpt {
	return func(o *mksquashfsOpts) error {
		o.mem = m
		return nil
	}
}

// OptComp sets the compression algorithm to use when creating a squashfs.
func OptComp(c string) MksquashfsOpt {
	return func(o *mksquashfsOpts) error {
		o.comp = c
		return nil
	}
}

// OptAllRoot forces ownership of all files in the squashfs to root.
func OptAllRoot(a bool) MksquashfsOpt {
	return func(o *mksquashfsOpts) error {
		o.allRoot = a
		return nil
	}
}

// OptWildcards enables wildcard matching for exclude patterns.
func OptWildcards(w bool) MksquashfsOpt {
	return func(o *mksquashfsOpts) error {
		o.wildcards = w
		return nil
	}
}

// OptExcludes specifies patterns to match against files to be excluded.
func OptExcludes(e []string) MksquashfsOpt {
	return func(o *mksquashfsOpts) error {
		o.excludes = e
		return nil
	}
}

// Mksquashfs calls the mksquashfs binary to create a squashfs image at dest,
// containing items listed in files. By default, zlib compression is used, and
// the processor and memory resource limits specified in singularity.conf are
// applied. To override the defaults, use the OptX functions. If running as
// non-root, consider using OptAllRoot to squash ownership of files to root, to
// avoid uid/gid mismatch when moving images between systems.
func Mksquashfs(files []string, dest string, opts ...MksquashfsOpt) error {
	mo, err := defaultOpts()
	if err != nil {
		return err
	}

	for _, opt := range opts {
		if err := opt(mo); err != nil {
			return err
		}
	}

	flags := []string{"-noappend"}
	if mo.procs != 0 {
		flags = append(flags, "-processors", fmt.Sprintf("%d", mo.procs))
	}
	if mo.mem != "" {
		flags = append(flags, "-mem", mo.mem)
	}
	if mo.comp != "" {
		flags = append(flags, "-comp", mo.comp)
	}
	if mo.allRoot {
		flags = append(flags, "-all-root")
	}
	if mo.wildcards {
		flags = append(flags, "-wildcards")
	}
	if len(mo.excludes) > 0 {
		flags = append(flags, "-e")
		flags = append(flags, mo.excludes...)
	}

	var stderr bytes.Buffer

	// mksquashfs takes args of the form: source1 source2 ... destination [options]
	args := files
	args = append(args, dest)
	args = append(args, flags...)
	sylog.Debugf("Executing %q with args: %v", mo.path, args)
	cmd := exec.Command(mo.path, args...)
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("create command failed: %v: %s", err, stderr.String())
	}
	return nil
}
