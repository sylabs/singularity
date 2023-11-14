// Copyright (c) 2023, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package tmpl

import (
	"html/template"
	"os"
	"path/filepath"
	"testing"
)

// Execute creates a file in tmpdir based on namePattern whose contents are the
// result of executing the Go template in tmplPath, over the struct passed in
// the values argument. Returns the full path of the created file. The created
// file will be automatically removed at the end of the test t unless t fails.
func Execute(t *testing.T, tmpdir, namePattern, tmplPath string, values any) string {
	outfile, err := os.CreateTemp(tmpdir, namePattern)
	if err != nil {
		t.Fatalf("failed to open temp file: %v", err)
	}
	outfilePath := outfile.Name()
	t.Cleanup(func() {
		if !t.Failed() {
			os.Remove(outfilePath)
		}
	})
	defer outfile.Close()

	tmplBytes, err := os.ReadFile(tmplPath)
	if err != nil {
		t.Fatalf("While trying to read template file %q: %v", tmplPath, err)
	}
	tmpl, err := template.New(filepath.Base(outfilePath)).Parse(string(tmplBytes))
	if err != nil {
		t.Fatalf("While trying to parse template file %q: %v", tmplPath, err)
	}

	err = tmpl.Execute(outfile, values)
	if err != nil {
		t.Fatalf("While trying to execute template %q: %v", tmplPath, err)
	}

	return outfilePath
}
