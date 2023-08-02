// Copyright (c) 2018, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package uri

import (
	"testing"
)

func Test_GetName(t *testing.T) {
	tests := []struct {
		name     string
		uri      string
		suffix   string
		expected string
	}{
		{"docker basic", "docker://ubuntu", "sif", "ubuntu_latest.sif"},
		{"docker scoped", "docker://user/image", "oci.sif", "image_latest.oci.sif"},
		{"dave's magical lolcow", "docker://sylabs.io/lolcow", "sif", "lolcow_latest.sif"},
		{"docker w/ tags", "docker://sylabs.io/lolcow:3.7", "sif", "lolcow_3.7.sif"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if n := Filename(tt.uri, tt.suffix); n != tt.expected {
				t.Errorf("incorrectly parsed name as \"%v\" (expected \"%s\")", n, tt.expected)
			}
		})
	}
}

func Test_Split(t *testing.T) {
	tests := []struct {
		name      string
		uri       string
		transport string
		ref       string
	}{
		{"docker basic", "docker://ubuntu", "docker", "//ubuntu"},
		{"docker scoped", "docker://user/image", "docker", "//user/image"},
		{"dave's magical lolcow", "docker://sylabs.io/lolcow", "docker", "//sylabs.io/lolcow"},
		{"docker with tags", "docker://sylabs.io/lolcow:latest", "docker", "//sylabs.io/lolcow:latest"},
		{"library basic", "library://image", "library", "//image"},
		{"library scoped", "library://collection/image", "library", "//collection/image"},
		{"without transport", "ubuntu", "", "ubuntu"},
		{"without transport with colon", "ubuntu:18.04.img", "", "ubuntu:18.04.img"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tr, r := Split(tt.uri); tr != tt.transport || r != tt.ref {
				t.Errorf("incorrectly parsed uri as %s : %s (expected %s : %s)", tr, r, tt.transport, tt.ref)
			}
		})
	}
}
