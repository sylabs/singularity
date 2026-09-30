// Copyright (c) 2020-2026, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/sylabs/singularity/v4/internal/pkg/util/fs"
)

func TestLoadConfig(t *testing.T) {
	tmpDir := t.TempDir()

	p := pluginImplementation{
		configPath: tmpDir,
	}

	tests := map[string]error{
		"valid-single":   nil,
		"valid-multiple": nil,
		"invalid-empty":  ErrNoLogOutput,
		"invalid-type":   ErrInvalidLogType,
	}

	for name, expected := range tests {
		t.Run(name, func(t *testing.T) {
			src := "testdata/config-" + name + ".yml"
			dst := filepath.Join(tmpDir, "config.yml")
			err := fs.CopyFile(src, dst, 0o644)
			if err != nil {
				t.Fatalf("Could not copy test config: %v", err)
			}
			defer os.Remove(dst)

			err = p.loadConfig()

			if expected == nil && err != nil {
				t.Errorf("expected: %v, got: %v", expected, err)
			}

			if expected != nil && !errors.Is(err, expected) {
				t.Errorf("expected: %v, got: %v", expected, err)
			}
		})
	}
}
