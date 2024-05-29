// Copyright (c) 2024, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package ocisif

import (
	"archive/tar"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
	"strings"
)

var errUnsupportedType = errors.New("unsupported file type")

// writeEntryToTAR writes the named path from fsys to tw.
func writeEntryToTAR(fsys fs.FS, name string, tw *tar.Writer) error {
	// Get file info.
	fi, err := fs.Stat(fsys, name)
	if err != nil {
		return err
	}

	// Populate TAR header based on file info, and normalize name.
	h, err := tar.FileInfoHeader(fi, "")
	if err != nil {
		return err
	}
	h.Name = name

	// Check that we're writing a supported type, and make any necessary adjustments.
	switch h.Typeflag {
	case tar.TypeReg:
		break

	case tar.TypeDir:
		// Normalize name.
		if !strings.HasSuffix(h.Name, "/") {
			h.Name += "/"
		}

	default:
		return fmt.Errorf("%v: %w (%v)", name, errUnsupportedType, h.Typeflag)
	}

	// Write TAR header.
	if err := tw.WriteHeader(h); err != nil {
		return err
	}

	// Write file contents, if applicable.
	if h.Typeflag == tar.TypeReg && h.Size > 0 {
		f, err := fsys.Open(name)
		if err != nil {
			return err
		}
		defer f.Close()

		if _, err := io.Copy(tw, f); err != nil {
			return err
		}
	}

	return nil
}

// writeDirAllToTAR writes a directory with the specified name, and all of its parents to tw.
func writeDirAllToTAR(fsys fs.FS, name string, tw *tar.Writer) error {
	if name == "." {
		return nil
	}

	if err := writeDirAllToTAR(fsys, path.Dir(name), tw); err != nil {
		return err
	}

	return writeEntryToTAR(fsys, name, tw)
}

type tarWriterFunc func(w io.Writer) error

// fileTARWriter returns a tarWriter that writes the named path from fsys.
func fileTARWriter(fsys fs.FS, name string) tarWriterFunc {
	return func(w io.Writer) error {
		tw := tar.NewWriter(w)
		defer tw.Close()

		// Write parent directories of file to TAR.
		if err := writeDirAllToTAR(fsys, path.Dir(name), tw); err != nil {
			return err
		}

		// WRite the file to the TAR.
		return writeEntryToTAR(fsys, name, tw)
	}
}

// fsTARWriter returns a tarWriter that writes entries found while walking the file tree from fsys
// rooted at root.
func fsTARWriter(fsys fs.FS, root string) tarWriterFunc {
	return func(w io.Writer) error {
		tw := tar.NewWriter(w)
		defer tw.Close()

		// Write parent directories of root path to TAR.
		if err := writeDirAllToTAR(fsys, path.Dir(root), tw); err != nil {
			return err
		}

		// Walk from root in filesystem, writing each entry to TAR.
		return fs.WalkDir(fsys, root, func(name string, _ fs.DirEntry, err error) error {
			if err != nil {
				return err
			}

			if name == "." {
				return nil
			}

			return writeEntryToTAR(fsys, name, tw)
		})
	}
}
