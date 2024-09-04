// Copyright (c) 2024, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package ocisif

import (
	"io/fs"
	"os"
	"testing"

	"github.com/sebdah/goldie/v2"
	"github.com/sylabs/squashfs"
)

func Test_newDataContainerFromFSPath(t *testing.T) {
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
			img, err := newDataContainerFromFSPath(tt.fsys, tt.path)
			if err != nil {
				t.Fatal(err)
			}

			g := goldie.New(t,
				goldie.WithTestNameForDir(true),
			)

			m, err := img.RawManifest()
			if err != nil {
				t.Fatal(err)
			}

			g.Assert(t, tt.name, m)
		})
	}
}

func getSourceFS(t *testing.T, src string) fs.FS { //nolint:unparam
	t.Helper()
	r, err := os.Open(src)
	if err != nil {
		t.Fatal(err)
	}

	fs, err := squashfs.NewReader(r)
	if err != nil {
		t.Fatal(err)
	}

	return fs
}
