// Copyright (c) 2020-2022, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package sources

import (
	"context"

	scskeyclient "github.com/sylabs/scs-key-client/client"
	"github.com/sylabs/singularity/v4/internal/pkg/signature"
	"github.com/sylabs/singularity/v4/pkg/sylog"
)

// checkSIFFingerprint checks whether a bootstrap SIF image verifies, and was signed with a specified fingerprint
func checkSIFFingerprint(ctx context.Context, imagePath string, fingerprints []string, co ...scskeyclient.Option) error {
	sylog.Infof("Checking bootstrap image verifies with fingerprint(s): %v", fingerprints)
	return signature.VerifyFingerprints(ctx, imagePath, fingerprints, signature.OptVerifyWithPGP(co...))
}

// verifySIF checks whether a bootstrap SIF image verifies
func verifySIF(ctx context.Context, imagePath string, co ...scskeyclient.Option) error {
	sylog.Infof("Verifying bootstrap image %s", imagePath)
	return signature.Verify(ctx, imagePath, signature.OptVerifyWithPGP(co...))
}
