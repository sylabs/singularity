// Copyright (c) 2025, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package sign

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/sylabs/oci-tools/pkg/sourcesink"
	"github.com/sylabs/singularity/v4/e2e/internal/e2e"
	"github.com/sylabs/singularity/v4/internal/pkg/util/fs"
)

func (c *ctx) signOCICosign(t *testing.T) {
	e2e.EnsureOCISIF(t, c.TestEnv)
	testSif := filepath.Join(t.TempDir(), "test.sif")
	if err := fs.CopyFile(c.TestEnv.OCISIFPath, testSif, 0o755); err != nil {
		t.Fatal(err)
	}
	keyPath := filepath.Join("..", "test", "keys", "cosign.key")

	tests := []struct {
		name             string
		flags            []string
		expectCode       int
		expectOps        []e2e.SingularityCmdResultOp
		expectSignatures int
	}{
		{
			flags:      []string{"--cosign"},
			name:       "NoKey",
			expectCode: 255,
			expectOps: []e2e.SingularityCmdResultOp{
				e2e.ExpectError(e2e.ContainMatch, "require a private --key"),
			},
		},
		{
			name:       "UnsupportedObjectID",
			expectCode: 255,
			flags:      []string{"--cosign", "--key=" + keyPath, "--sif-id", "1"},
			expectOps: []e2e.SingularityCmdResultOp{
				e2e.ExpectError(e2e.ContainMatch, "not supported"),
			},
		},
		{
			name:       "UnsupportedGroupIDFlag",
			expectCode: 255,
			flags:      []string{"--cosign", "--key=" + keyPath, "--group-id", "1"},
			expectOps: []e2e.SingularityCmdResultOp{
				e2e.ExpectError(e2e.ContainMatch, "not supported"),
			},
		},
		{
			name:       "UnsupportedAllFlag",
			expectCode: 255,
			flags:      []string{"--cosign", "--key=" + keyPath, "--all"},
			expectOps: []e2e.SingularityCmdResultOp{
				e2e.ExpectError(e2e.ContainMatch, "not supported"),
			},
		},
		{
			flags:            []string{"--cosign", "--key=" + keyPath},
			name:             "SignOnce",
			expectCode:       0,
			expectSignatures: 1,
		},
		{
			flags:            []string{"--cosign", "--key=" + keyPath},
			name:             "SignTwice",
			expectCode:       0,
			expectSignatures: 2,
		},
	}

	for _, tt := range tests {
		c.RunSingularity(t,
			e2e.AsSubtest(tt.name),
			e2e.WithProfile(e2e.UserProfile),
			e2e.WithCommand("sign"),
			e2e.WithArgs(append(tt.flags, testSif)...),
			e2e.ExpectExit(tt.expectCode, tt.expectOps...),
			e2e.PostRun(func(t *testing.T) {
				// Expected number of signatures can be retrieced from image.
				checkSignatures(t, testSif, tt.expectSignatures)
				// Signed image is still usable.
				if tt.expectSignatures > 0 {
					c.checkExec(t, testSif)
				}
			}),
		)
	}
}

func checkSignatures(t *testing.T, sifPath string, expectSignatures int) {
	t.Helper()

	s, err := sourcesink.SIFFromPath(sifPath)
	if err != nil {
		t.Fatal(err)
	}
	d, err := s.Get(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	sd, ok := d.(sourcesink.SignedDescriptor)
	if !ok {
		t.Fatal("could not upgrade Descriptor to SignedDescriptor")
	}

	si, err := sd.SignedImage(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	sigsImage, err := si.Signatures()
	if err != nil {
		t.Fatal(err)
	}

	sigs, err := sigsImage.Get()
	if err != nil {
		t.Fatal(err)
	}

	if len(sigs) != expectSignatures {
		t.Fatalf("expected %d signatures, found %d", expectSignatures, len(sigs))
	}
}

func (c *ctx) checkExec(t *testing.T, sifPath string) {
	t.Helper()
	c.RunSingularity(t,
		e2e.AsSubtest("exec"),
		e2e.WithProfile(e2e.OCIUserProfile),
		e2e.WithCommand("exec"),
		e2e.WithArgs(sifPath, "/bin/true"),
		e2e.ExpectExit(0),
	)
}
