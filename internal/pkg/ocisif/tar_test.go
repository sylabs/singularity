// Copyright (c) 2024, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package ocisif

import (
	"bytes"
	"io/fs"
	"testing"

	"github.com/sebdah/goldie/v2"
)

func Test_fileTARWriter(t *testing.T) {
	tests := []struct {
		name string
		fsys fs.FS
		path string
	}{
		{
			name: "FileInRootDir",
			fsys: getSourceFS(t, "../../../test/images/tar-walker.sqfs"),
			path: "file.txt",
		},
		{
			name: "FileInSubDir",
			fsys: getSourceFS(t, "../../../test/images/tar-walker.sqfs"),
			path: "subdir/subfile.txt",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var b bytes.Buffer

			if err := fileTARWriter(tt.fsys, tt.path)(&b); err != nil {
				t.Fatal(err)
			}

			g := goldie.New(t,
				goldie.WithTestNameForDir(true),
			)

			g.Assert(t, tt.name, b.Bytes())
		})
	}
}

func Test_fsTARWriter(t *testing.T) {
	tests := []struct {
		name string
		fsys fs.FS
		path string
	}{
		{
			name: "RootDir",
			fsys: getSourceFS(t, "../../../test/images/tar-walker.sqfs"),
			path: ".",
		},
		{
			name: "SubDir",
			fsys: getSourceFS(t, "../../../test/images/tar-walker.sqfs"),
			path: "subdir",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var b bytes.Buffer

			if err := fsTARWriter(tt.fsys, tt.path)(&b); err != nil {
				t.Fatal(err)
			}

			g := goldie.New(t,
				goldie.WithTestNameForDir(true),
			)

			g.Assert(t, tt.name, b.Bytes())
		})
	}
}
