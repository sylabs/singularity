// Copyright (c) 2026, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

//go:build singularity_engine

package bin

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_parsePath(t *testing.T) {
	tests := []struct {
		name string
		p    string
		want string
	}{
		{
			name: "empty path",
			p:    "",
		},
		{
			name: "simple path",
			p:    "/usr/bin:/bin",
			want: "/usr/bin:/bin",
		},
		{
			name: "$PATH first",
			p:    "$PATH:/an/other/dir",
			want: os.Getenv("PATH") + ":/an/other/dir",
		},
		{
			name: "$PATH last",
			p:    "/an/other/dir:$PATH",
			want: "/an/other/dir:" + os.Getenv("PATH"),
		},
		{
			name: "trim empty entries",
			p:    ":$PATH:/usr/bin::/bin:::/usr/local/bin:",
			want: os.Getenv("PATH") + ":/usr/bin:/bin:/usr/local/bin",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parsePath(tt.p)
			assert.Equal(t, tt.want, got)
		})
	}
}
