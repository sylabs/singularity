// Copyright (c) 2025, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package cosign

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/match"
	"github.com/sigstore/cosign/v2/pkg/oci/mutate"
	cosignremote "github.com/sigstore/cosign/v2/pkg/oci/remote"
	"github.com/sigstore/cosign/v2/pkg/oci/static"
	"github.com/sigstore/sigstore/pkg/signature"
	signatureoptions "github.com/sigstore/sigstore/pkg/signature/options"
	sigPayload "github.com/sigstore/sigstore/pkg/signature/payload"
	ocisif "github.com/sylabs/oci-tools/pkg/sif"
	"github.com/sylabs/oci-tools/pkg/sourcesink"
	"github.com/sylabs/sif/v2/pkg/sif"
	"github.com/sylabs/singularity/v4/pkg/image"
	"github.com/sylabs/singularity/v4/pkg/sylog"
	useragent "github.com/sylabs/singularity/v4/pkg/util/user-agent"
)

func cosignPayload(digest v1.Hash) ([]byte, error) {
	opt := make(map[string]interface{}, 2)
	opt["creator"] = useragent.Value()
	opt["timestamp"] = time.Now().Unix()

	payload := sigPayload.SimpleContainerImage{
		Critical: sigPayload.Critical{
			Identity: sigPayload.Identity{
				DockerReference: "",
			},
			Image: sigPayload.Image{
				DockerManifestDigest: digest.String(),
			},
			Type: sigPayload.CosignSignatureType,
		},
		Optional: opt,
	}
	return json.Marshal(&payload)
}

func SignOCISIF(ctx context.Context, sifPath string, signer signature.Signer) error {
	ok, err := image.IsOCISIF(sifPath)
	if err != nil {
		return fmt.Errorf("while checking OCI-SIF: %w", err)
	}
	if !ok {
		return fmt.Errorf("image is not an OCI-SIF: %q", sifPath)
	}

	ss, err := sourcesink.SIFFromPath(sifPath)
	if err != nil {
		return fmt.Errorf("failed to open OCI-SIF: %w", err)
	}
	d, err := ss.Get(ctx)
	if err != nil {
		return fmt.Errorf("while fetching image from OCI-SIF: %v", err)
	}
	sd, ok := d.(sourcesink.SignedDescriptor)
	if !ok {
		return fmt.Errorf("failed to upgrade Descriptor to SignedDescriptor")
	}
	si, err := sd.SignedImage(ctx)
	if err != nil {
		return fmt.Errorf("failed to retrieve image: %w", err)
	}
	digest, err := si.Digest()
	if err != nil {
		return fmt.Errorf("failed to retrieve digest: %w", err)
	}

	payload, err := cosignPayload(digest)
	if err != nil {
		return fmt.Errorf("while generating signature payload: %w", err)
	}
	sOpts := []signature.SignOption{signatureoptions.WithContext(ctx)}
	sig, err := signer.SignMessage(bytes.NewReader(payload), sOpts...)
	if err != nil {
		return err
	}
	b64sig := base64.StdEncoding.EncodeToString(sig)
	ociSig, err := static.NewSignature(payload, b64sig)
	if err != nil {
		return err
	}
	sylog.Debugf("Generated cosign signature: %v", b64sig)

	si, err = mutate.AttachSignatureToImage(si, ociSig)
	if err != nil {
		return err
	}
	sigs, err := si.Signatures()
	if err != nil {
		return err
	}

	csRef, err := sourcesink.CosignRef(digest, nil, cosignremote.SignatureTagSuffix)
	if err != nil {
		return err
	}
	sylog.Debugf("Writing cosign image as: %s", csRef)
	fi, err := sif.LoadContainerFromPath(sifPath)
	defer fi.UnloadContainer()
	if err != nil {
		return fmt.Errorf("while loading SIF: %w", err)
	}
	ofi, err := ocisif.FromFileImage(fi)
	if err != nil {
		return fmt.Errorf("while loading SIF: %w", err)
	}
	return ofi.ReplaceImage(sigs, match.Name(csRef.Name()), ocisif.OptAppendReference(csRef))
}
