// Copyright (c) 2019-2023, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package tools

import (
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"syscall"

	specs "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/sylabs/singularity/v4/internal/pkg/runtime/engine/config/oci"
	"github.com/sylabs/singularity/v4/internal/pkg/runtime/engine/config/oci/generate"
	"github.com/sylabs/singularity/v4/internal/pkg/util/passwdfile"
)

// RootFs provides functions for accessing the rootfs directory
// of a bundle. It is initialized with the path of the bundle.
type RootFs string

// Path returns the rootfs path inside the bundle.
func (r RootFs) Path() string {
	return filepath.Join(string(r), "rootfs")
}

// Volumes provides functions for accessing the volumes directory
// of a bundle, which will hold any volume directories that are created. It is
// initialized with the path of the bundle.
type Volumes string

// Path returns the path of the volumes directory in the bundle, in which any
// volumes should be created.
func (v Volumes) Path() string {
	return filepath.Join(string(v), "volumes")
}

// Layers provides functions for accessing the layers directory
// of a bundle, which will hold any image layer directories that are created. It
// is initialized with the path of the bundle.
type Layers string

// Path returns the path of the layers directory in the bundle, in which any
// image layer directories should be created.
func (l Layers) Path() string {
	return filepath.Join(string(l), "layers")
}

// Config provides functions for accessing the runtime configuration (JSON) of a
// bundle. It is initialized with the path of the bundle.
type Config string

// Path returns the path to the runtime configuration (JSON) of a bundle.
func (c Config) Path() string {
	return filepath.Join(string(c), "config.json")
}

// RunScript is the default process argument
const RunScript = "/.singularity.d/actions/run"

// GenerateBundleConfig generates a minimal OCI bundle directory
// with the provided OCI configuration or a default one
// if there is no configuration
func GenerateBundleConfig(bundlePath string, config *specs.Spec) (*generate.Generator, error) {
	var err error
	var g *generate.Generator

	oldumask := syscall.Umask(0)
	defer syscall.Umask(oldumask)

	rootFsDir := RootFs(bundlePath).Path()
	if err := os.MkdirAll(rootFsDir, 0o700); err != nil {
		return nil, fmt.Errorf("failed to create %s: %s", rootFsDir, err)
	}
	volumesDir := Volumes(bundlePath).Path()
	if err := os.MkdirAll(volumesDir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create %s: %s", volumesDir, err)
	}
	layersDir := Layers(bundlePath).Path()
	if err := os.MkdirAll(layersDir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create %s: %s", layersDir, err)
	}
	defer func() {
		if err != nil {
			DeleteBundle(bundlePath)
		}
	}()

	if config == nil {
		// generate and write config.json in bundle
		g, err = oci.DefaultConfig()
		if err != nil {
			return nil, fmt.Errorf("failed to generate OCI config: %s", err)
		}
		g.SetProcessArgs([]string{RunScript})
	} else {
		g = generate.New(config)
	}
	g.SetRootPath(rootFsDir)
	return g, nil
}

// SaveBundleConfig creates config.json in OCI bundle directory and
// saves OCI configuration
func SaveBundleConfig(bundlePath string, g *generate.Generator) error {
	return g.SaveToFile(Config(bundlePath).Path())
}

// DeleteBundle deletes bundle directory
func DeleteBundle(bundlePath string) error {
	if err := os.RemoveAll(Layers(bundlePath).Path()); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete layers directory: %s", err)
	}
	if err := os.RemoveAll(Volumes(bundlePath).Path()); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete volumes directory: %s", err)
	}
	if err := os.Remove(RootFs(bundlePath).Path()); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete rootfs directory: %s", err)
	}
	if err := os.Remove(Config(bundlePath).Path()); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete config.json file: %s", err)
	}
	if err := os.Remove(bundlePath); err != nil && !os.IsExist(err) {
		return fmt.Errorf("failed to delete bundle %s directory: %s", bundlePath, err)
	}
	return nil
}

// BundleUser returns a user struct for the specified user, from the bundle passwd file.
func BundleUser(bundlePath, user string) (u *user.User, err error) {
	passwd := filepath.Join(RootFs(bundlePath).Path(), "etc", "passwd")
	if _, err := os.Stat(passwd); err != nil {
		return nil, fmt.Errorf("cannot access container passwd file: %w", err)
	}

	// We have a numeric container uid
	if _, err := strconv.Atoi(user); err == nil {
		return passwdfile.LookupUserIDInFile(passwd, user)
	}
	// We have a container username
	return passwdfile.LookupUserInFile(passwd, user)
}
