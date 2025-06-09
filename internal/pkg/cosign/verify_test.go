// Copyright (c) 2025, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package cosign

import (
	"context"
	"crypto"
	"errors"
	"io"
	"os"
	"testing"

	"github.com/sebdah/goldie/v2"
	"github.com/sigstore/cosign/v2/pkg/cosign"
	"github.com/sigstore/sigstore/pkg/signature"
	"github.com/sylabs/singularity/v4/test/oci"
)

func TestVerifyOCISIF(t *testing.T) {
	corpus := oci.NewCorpus("../../../test/oci")

	wrongKey, err := cosign.GeneratePrivateKey()
	if err != nil {
		t.Fatalf("while generating test key: %v", err)
	}
	wrongVerifier, err := signature.LoadSignerVerifier(wrongKey, crypto.SHA256)
	if err != nil {
		t.Fatalf("while generating test verifier: %v", err)
	}

	goodVerifier, err := signature.LoadVerifierFromPEMFile("../../../test/keys/cosign.pub", crypto.SHA256)
	if err != nil {
		t.Fatalf("while generating test verifier: %v", err)
	}

	tests := []struct {
		name      string
		sifPath   string
		verifier  signature.Verifier
		expectErr error
	}{
		{
			name:      "VerifyOK",
			sifPath:   corpus.SIF(t, "hello-world-cosign-manifest"),
			verifier:  goodVerifier,
			expectErr: nil,
		},
		{
			name:      "VerifyWrongKey",
			sifPath:   corpus.SIF(t, "hello-world-cosign-manifest"),
			verifier:  wrongVerifier,
			expectErr: ErrNoValidSignatures,
		},
		{
			name:      "VerifyUnsigned",
			sifPath:   corpus.SIF(t, "hello-world-docker-v2-manifest"),
			verifier:  goodVerifier,
			expectErr: ErrNoValidSignatures,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payloads, err := VerifyOCISIF(context.Background(), tt.sifPath, tt.verifier)
			if !errors.Is(err, tt.expectErr) {
				t.Errorf("Expected error %v, got %v", tt.expectErr, err)
			}
			g := goldie.New(t,
				goldie.WithTestNameForDir(true),
			)
			g.Assert(t, tt.name, payloads)
		})
	}
}

func TestVerifyOCIBlobDigests(t *testing.T) {
	corpus := oci.NewCorpus("../../../test/oci")

	// Bit of a hack - write at 16KiB into the file corrupts an OCI blob data region.
	badSIF := corpus.SIF(t, "hello-world-cosign-manifest")
	f, err := os.OpenFile(badSIF, os.O_RDWR, 0o755)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Seek(16384, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write([]byte{0xFF, 0xFF, 0xFF, 0xFF}); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name      string
		sifPath   string
		expectErr error
	}{
		{
			name:      "OK",
			sifPath:   corpus.SIF(t, "hello-world-cosign-manifest"),
			expectErr: nil,
		},
		{
			name:      "Mismatch",
			sifPath:   badSIF,
			expectErr: ErrOCIBlobMismatch,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := verifyOCIBlobDigests(tt.sifPath)
			if !errors.Is(err, tt.expectErr) {
				t.Errorf("Expected error %v, got %v", tt.expectErr, err)
			}
		})
	}
}
