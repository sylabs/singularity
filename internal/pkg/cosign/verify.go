// Copyright (c) 2025, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package cosign

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/sigstore/cosign/v2/pkg/oci"
	"github.com/sigstore/sigstore/pkg/signature"
	sigPayload "github.com/sigstore/sigstore/pkg/signature/payload"
	"github.com/sylabs/oci-tools/pkg/sourcesink"
	"github.com/sylabs/sif/v2/pkg/sif"
	"github.com/sylabs/singularity/v4/pkg/image"
	"github.com/sylabs/singularity/v4/pkg/sylog"
)

var ErrNoValidSignatures = errors.New("no valid signatures found")

// VerifyOCISIF checks that a single OCI container image, contained in the
// OCI-SIF file at sifPath, has at least 1 cosign signature that can be verified
// with the provided verifier. The digests of the OCI blobs store in sifPath are
// also checked vs their actual content. Returns a JSON representation of valid
// payloads.
func VerifyOCISIF(ctx context.Context, sifPath string, verifier signature.Verifier) ([]byte, error) {
	ok, err := image.IsOCISIF(sifPath)
	if err != nil {
		return nil, fmt.Errorf("while checking OCI-SIF: %w", err)
	}
	if !ok {
		return nil, fmt.Errorf("not an OCI-SIF: %q", sifPath)
	}

	if err := verifyOCIBlobDigests(sifPath); err != nil {
		return nil, err
	}

	payloads, err := checkSignatures(ctx, sifPath, verifier)
	if err != nil {
		return nil, err
	}
	if len(payloads) == 0 {
		return nil, ErrNoValidSignatures
	}

	return json.Marshal(payloads)
}

var ErrOCIBlobMismatch = errors.New("OCI blob digest mismatch")

// verifyOCIBlobDigest checks that the OCIBlobDigest stored for each OCI blob
// descriptor in sifPath is correct given the blob's content.
func verifyOCIBlobDigests(sifPath string) error {
	fi, err := sif.LoadContainerFromPath(sifPath)
	defer fi.UnloadContainer()
	if err != nil {
		return fmt.Errorf("while loading SIF: %w", err)
	}

	descriptors, err := fi.GetDescriptors(sif.WithDataType(sif.DataOCIBlob))
	if err != nil {
		return fmt.Errorf("while loading SIF: %w", err)
	}
	sylog.Infof("Verifying digests for %d OCI Blobs\n", len(descriptors))
	for _, d := range descriptors {
		sifDigest, err := d.OCIBlobDigest()
		if err != nil {
			return fmt.Errorf("failed to fetch digest: %w", err)
		}
		sylog.Debugf("Descriptor %d: OCIBlobDigest: %v", d.ID(), sifDigest)

		calcDigest, _, err := v1.SHA256(d.GetReader())
		if err != nil {
			return fmt.Errorf("failed to compute digest: %w", err)
		}
		sylog.Debugf("Descriptor %d: Calculated Digest: %v", d.ID(), calcDigest)

		if calcDigest != sifDigest {
			return fmt.Errorf("%w: expected %q, found %q", ErrOCIBlobMismatch, sifDigest, calcDigest)
		}
	}
	return nil
}

// checkSignatures retrieves each signature associated with a single OCI
// container image in SIFPath, verifies the signature using the provided
// verifier, and checks the payload manifest digest is a match for the image.
// The payloads of valid signatures are returned.
func checkSignatures(ctx context.Context, sifPath string, verifier signature.Verifier) ([]sigPayload.SimpleContainerImage, error) {
	ss, err := sourcesink.SIFFromPath(sifPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open OCI-SIF: %w", err)
	}
	d, err := ss.Get(ctx)
	if err != nil {
		return nil, fmt.Errorf("while fetching image from OCI-SIF: %v", err)
	}
	sd, ok := d.(sourcesink.SignedDescriptor)
	if !ok {
		return nil, fmt.Errorf("failed to upgrade Descriptor to SignedDescriptor")
	}
	si, err := sd.SignedImage(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve image: %w", err)
	}
	imgDigest, err := si.Digest()
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve image digest: %w", err)
	}
	sylog.Infof("Image digest: %s", imgDigest.String())

	sigImg, err := si.Signatures()
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve signatures: %w", err)
	}
	sigs, err := sigImg.Get()
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve signatures: %w", err)
	}

	sylog.Infof("Image has %d associated signatures", len(sigs))

	validPayloads := []sigPayload.SimpleContainerImage{}
	for i, s := range sigs {
		payload, err := verifySignature(s, verifier)
		if err != nil {
			sylog.Verbosef("signature %d invalid for provided key material: %v", i, err)
			continue
		}
		if payload.Critical.Image.DockerManifestDigest != imgDigest.String() {
			sylog.Verbosef("signature %d invalid for image %s", i, imgDigest.String())
			continue
		}
		validPayloads = append(validPayloads, *payload)
	}

	sylog.Infof("Image has %d signatures that are valid with provided key material", len(validPayloads))
	return validPayloads, nil
}

func verifySignature(s oci.Signature, verifier signature.Verifier) (*sigPayload.SimpleContainerImage, error) {
	sigBytes, err := s.Signature()
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve signature: %w", err)
	}
	payloadBytes, err := s.Payload()
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve payload: %w", err)
	}
	payload := sigPayload.SimpleContainerImage{}
	if err := json.Unmarshal(payloadBytes, &payload); err != nil {
		return nil, fmt.Errorf("failed to unmarshal payload: %w", err)
	}

	if err := verifier.VerifySignature(bytes.NewBuffer(sigBytes), bytes.NewBuffer(payloadBytes)); err != nil {
		return nil, err
	}
	return &payload, nil
}
