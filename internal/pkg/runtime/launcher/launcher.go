// Copyright (c) 2022-2023, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

// Package launcher is responsible for implementing launchers, which can start a
// container, with configuration passed from the CLI layer.
package launcher

import (
	"context"
	"fmt"
	"path/filepath"
)

// Launcher is responsible for configuring and launching a container image.
// It will execute a runtime, such as Singularity's native runtime (via the starter
// binary), or an external OCI runtime (e.g. runc).
type Launcher interface {
	// Exec will execute the container image 'image', starting 'process', and
	// passing arguments 'args'. If instanceName is specified, the container
	// must be launched as a background instance, otherwise it must run
	// interactively, attached to the console.
	Exec(ctx context.Context, ep ExecParams) error
}

// ExecParams specifies the image and process for a launcher to Exec.
type ExecParams struct {
	// Image is the container image to execute, as a bare path, or <transport>:<path>.
	Image string
	// Action is one of exec/run/shell/start/test as specified on the CLI.
	Action string
	// Process is the command to execute as the container process, where applicable.
	Process string
	// Args are the arguments passed to the container process.
	Args []string
	// Instance is the name of an instance (optional).
	Instance string
}

const singularityActions = "/.singularity.d/actions"

// ActionScriptArgs returns the args that will appropriately exec the action
// script in a singularity (non-oci) container, for a given ExecParams.
func (ep ExecParams) ActionScriptArgs() (args []string, err error) {
	if ep.Image == "" {
		return []string{}, fmt.Errorf("%s action requires an image", ep.Action)
	}

	args = []string{filepath.Join(singularityActions, ep.Action)}

	switch ep.Action {
	case "exec":
		if ep.Process == "" {
			return []string{}, fmt.Errorf("%s action requires a process", ep.Action)
		}
		if ep.Instance != "" {
			return []string{}, fmt.Errorf("%s action doesn't support specifying an instance", ep.Action)
		}
		args = append(args, ep.Process)
		args = append(args, ep.Args...)
	case "shell", "test":
		if ep.Process != "" {
			return []string{}, fmt.Errorf("%s action doesn't support specifying a process", ep.Action)
		}
		if ep.Instance != "" {
			return []string{}, fmt.Errorf("%s action doesn't support specifying an instance", ep.Action)
		}
		args = append(args, ep.Args...)
	case "run":
		if ep.Process != "" {
			return []string{}, fmt.Errorf("%s action doesn't support specifying a process", ep.Action)
		}
		args = append(args, ep.Args...)
	case "start":
		if ep.Process != "" {
			return []string{}, fmt.Errorf("%s action doesn't support specifying a process", ep.Action)
		}
		if ep.Instance == "" {
			return []string{}, fmt.Errorf("%s action requires an instance", ep.Action)
		}
		args = append(args, ep.Args...)
	default:
		return []string{}, fmt.Errorf("unknown action %q", ep.Action)
	}
	return args, nil
}
