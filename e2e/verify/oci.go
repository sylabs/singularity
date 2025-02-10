// Copyright (c) 2025, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package verify

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sigstore/cosign/v2/pkg/cosign"
	"github.com/sylabs/singularity/v4/e2e/internal/e2e"
	"github.com/sylabs/singularity/v4/test/oci"
)

func (c *ctx) verifyOCICosign(t *testing.T) {
	corpus := oci.NewCorpus("../test/oci")
	signedSIF := corpus.SIF(t, "hello-world-cosign-manifest")
	unsignedSIF := corpus.SIF(t, "hello-world-docker-v2-manifest")
	goodKeyPath := filepath.Join("..", "test", "keys", "cosign.pub")
	badKeyPath := filepath.Join(t.TempDir(), "bad.pub")
	kb, err := cosign.GenerateKeyPair(nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(badKeyPath, kb.PublicBytes, 0o644); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name             string
		flags            []string
		sifPath          string
		expectCode       int
		expectOps        []e2e.SingularityCmdResultOp
		expectSignatures int
	}{
		{
			name:       "OK",
			flags:      []string{"--cosign", "--key", goodKeyPath},
			sifPath:    signedSIF,
			expectCode: 0,
			expectOps: []e2e.SingularityCmdResultOp{
				e2e.ExpectOutput(e2e.ContainMatch, "cosign container image signature"),
			},
		},
		{
			name:       "WrongKey",
			flags:      []string{"--cosign", "--key", badKeyPath},
			sifPath:    signedSIF,
			expectCode: 255,
			expectOps: []e2e.SingularityCmdResultOp{
				e2e.ExpectError(e2e.ContainMatch, "no valid signatures found"),
			},
		},
		{
			name:       "Unsigned",
			flags:      []string{"--cosign", "--key", goodKeyPath},
			sifPath:    unsignedSIF,
			expectCode: 255,
			expectOps: []e2e.SingularityCmdResultOp{
				e2e.ExpectError(e2e.ContainMatch, "no valid signatures found"),
			},
		},
		{
			name:       "NoKey",
			flags:      []string{"--cosign"},
			sifPath:    signedSIF,
			expectCode: 255,
			expectOps: []e2e.SingularityCmdResultOp{
				e2e.ExpectError(e2e.ContainMatch, "requires a public --key"),
			},
		},
	}

	for _, tt := range tests {
		c.RunSingularity(t,
			e2e.AsSubtest(tt.name),
			e2e.WithProfile(e2e.UserProfile),
			e2e.WithCommand("verify"),
			e2e.WithArgs(append(tt.flags, tt.sifPath)...),
			e2e.ExpectExit(tt.expectCode, tt.expectOps...),
		)
	}
}
