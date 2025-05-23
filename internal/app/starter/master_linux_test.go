// Copyright (c) 2018-2021, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package starter

import (
	"testing"

	"github.com/sylabs/singularity/v4/internal/pkg/runtime/engine"
	"github.com/sylabs/singularity/v4/internal/pkg/test"
)

// TODO: actually we can't really test Master function which is
// part of the main function, as it exits, it would require mock at
// some point and that would make code more complex than necessary.
// createContainer and startContainer are quickly tested and only
// cover case with bad socket file descriptors or non socket
// file descriptor (stderr).

func TestCreateContainer(t *testing.T) {
	test.DropPrivilege(t)
	defer test.ResetPrivilege(t)

	var fatal error
	fatalChan := make(chan error, 1)

	tests := []struct {
		name         string
		rpcSocket    int
		containerPid int
		engine       *engine.Engine
		shallPass    bool
	}{
		{
			name:         "nil engine; bad rpcSocket",
			rpcSocket:    -1,
			containerPid: -1,
			engine:       nil,
			shallPass:    false,
		},
		{
			name:         "nil engine; wrong socket",
			rpcSocket:    2,
			containerPid: -1,
			engine:       nil,
			shallPass:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			go createContainer(t.Context(), tt.rpcSocket, tt.containerPid, tt.engine, fatalChan)
			// createContainer is creating a separate thread and we sync with that
			// thread through a channel similarly to the createContainer function itself,
			// as well as the Master function.
			// For more details, please refer to the master_linux.go code.
			fatal = <-fatalChan
			if tt.shallPass && fatal != nil {
				t.Fatalf("test %s expected to succeed but failed: %s", tt.name, fatal)
			} else if !tt.shallPass && fatal == nil {
				t.Fatalf("test %s expected to fail but succeeded", tt.name)
			}
		})
	}
}

func TestStartContainer(t *testing.T) {
	test.DropPrivilege(t)
	defer test.ResetPrivilege(t)

	var fatal error
	fatalChan := make(chan error, 1)

	tests := []struct {
		name            string
		masterSocket    int
		postStartSocket int
		containerPid    int
		engine          *engine.Engine
		shallPass       bool
	}{
		{
			name:            "nil engine; bad sockets",
			masterSocket:    -1,
			postStartSocket: -1,
			containerPid:    -1,
			engine:          nil,
			shallPass:       false,
		},
		{
			name:            "nil engine; wrong sockets",
			masterSocket:    2,
			postStartSocket: 2,
			containerPid:    -1,
			engine:          nil,
			shallPass:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			go startContainer(t.Context(), tt.masterSocket, tt.postStartSocket, tt.containerPid, tt.engine, fatalChan)
			fatal = <-fatalChan
			if tt.shallPass && fatal != nil {
				t.Fatalf("test %s expected to succeed but failed: %s", tt.name, fatal)
			} else if !tt.shallPass && fatal == nil {
				t.Fatalf("test %s expected to fail but succeeded", tt.name)
			}
		})
	}
}
