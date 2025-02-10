// Copyright 2023-2025 Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package oci

type Corpus struct {
	dir string
}

// NewCorpus returns a new Corpus. The path specifies the location of the "test" directory.
func NewCorpus(path string) *Corpus {
	return &Corpus{path}
}
