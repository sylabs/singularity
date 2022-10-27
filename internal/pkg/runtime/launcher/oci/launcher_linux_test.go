// Copyright (c) 2022, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package oci

import (
	"reflect"
	"testing"

	"github.com/sylabs/singularity/internal/pkg/runtime/launcher"
)

func TestNewLauncher(t *testing.T) {
	tests := []struct {
		name    string
		opts    []launcher.Option
		want    *Launcher
		wantErr bool
	}{
		{
			name:    "default",
			want:    &Launcher{},
			wantErr: false,
		},
		{
			name: "validOption",
			opts: []launcher.Option{
				launcher.OptHome("/home/test", false, false),
			},
			want: &Launcher{cfg: launcher.Options{HomeDir: "/home/test"}},
		},
		{
			name: "unsupportedOption",
			opts: []launcher.Option{
				launcher.OptCacheDisabled(true),
			},
			want:    nil,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewLauncher(tt.opts...)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewLauncher() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("NewLauncher() = %v, want %v", got, tt.want)
			}
		})
	}
}
