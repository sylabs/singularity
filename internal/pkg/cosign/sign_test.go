// Copyright (c) 2025, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package cosign

import (
	"bytes"
	"context"
	"crypto"
	"encoding/json"
	"path/filepath"
	"testing"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/random"
	"github.com/sigstore/cosign/v2/pkg/cosign"
	"github.com/sigstore/sigstore/pkg/signature"
	sigPayload "github.com/sigstore/sigstore/pkg/signature/payload"
	ocisif "github.com/sylabs/oci-tools/pkg/sif"
	"github.com/sylabs/oci-tools/pkg/sourcesink"
	useragent "github.com/sylabs/singularity/v4/pkg/util/user-agent"
)

func TestSignOCISIF(t *testing.T) {
	useragent.InitValue(t.Name(), "0.0")

	img, err := random.Image(64, 3)
	if err != nil {
		t.Fatalf("while generating test image: %v", err)
	}
	digest, err := img.Digest()
	if err != nil {
		t.Fatalf("while getting test image digest: %v", err)
	}
	ii := mutate.AppendManifests(empty.Index, mutate.IndexAddendum{Add: img})
	testSIF := filepath.Join(t.TempDir(), "test.sif")
	if err := ocisif.Write(testSIF, ii, ocisif.OptWriteWithSpareDescriptorCapacity(16)); err != nil {
		t.Fatalf("while writing test image: %v", err)
	}

	privKey, err := cosign.GeneratePrivateKey()
	if err != nil {
		t.Fatalf("while generating test key: %v", err)
	}
	sv, err := signature.LoadECDSASignerVerifier(privKey, crypto.SHA256)
	if err != nil {
		t.Fatalf("while generating test signer: %v", err)
	}

	if err := SignOCISIF(context.Background(), testSIF, sv); err != nil {
		t.Error(err)
	}

	checkSignature(t, sv, digest, testSIF)
}

func checkSignature(t *testing.T, verifier signature.Verifier, imgDigest v1.Hash, sifPath string) {
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

	if len(sigs) != 1 {
		t.Errorf("expected 1 signature, found %d", len(sigs))
	}

	sig := sigs[0]

	sigBytes, err := sig.Signature()
	if err != nil {
		t.Fatal(err)
	}
	sigBuf := bytes.NewBuffer(sigBytes)

	payloadBytes, err := sig.Payload()
	if err != nil {
		t.Fatal(err)
	}
	payload := sigPayload.SimpleContainerImage{}
	if err := json.Unmarshal(payloadBytes, &payload); err != nil {
		t.Fatalf("while decoding payload: %v", err)
	}
	if payload.Critical.Identity.DockerReference != "" {
		t.Errorf("expected empty Identity.DockerReference in payload, found %q", payload.Critical.Identity.DockerReference)
	}
	if payload.Critical.Image.DockerManifestDigest != imgDigest.String() {
		t.Errorf("expected Image.DockerManifestDigest %q, found %q", imgDigest, payload.Critical.Image.DockerManifestDigest)
	}
	if payload.Optional["creator"] != useragent.Value() {
		t.Errorf("expected Optional.creator %q, found %q", useragent.Value(), payload.Optional["creator"])
	}

	payloadBuf := bytes.NewBuffer(payloadBytes)

	if err := verifier.VerifySignature(sigBuf, payloadBuf); err != nil {
		t.Errorf("while verifying signature: %v", err)
	}
}
