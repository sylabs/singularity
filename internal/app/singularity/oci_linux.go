// Copyright (c) 2018-2020, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package singularity

const (
	OciStateDir = "/run/singularity-oci"
	runc        = "/usr/bin/runc"
)

// OciArgs contains CLI arguments
type OciArgs struct {
	BundlePath   string
	LogPath      string
	LogFormat    string
	PidFile      string
	FromFile     string
	KillSignal   string
	KillTimeout  uint32
	EmptyProcess bool
	ForceKill    bool
}
