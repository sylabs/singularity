// Copyright (c) 2023, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package oci

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"testing"

	"github.com/opencontainers/runtime-tools/validate"
)

func GetTestImg(url string) (path string, err error) {
	dl, err := os.CreateTemp("", "oci-test")
	if err != nil {
		log.Fatal(err)
	}
	defer dl.Close()

	r, err := http.Get(url)
	if err != nil {
		return "", err
	}
	defer r.Body.Close()

	if r.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download failed: %v", r.Status)
	}

	_, err = io.Copy(dl, r.Body)
	if err != nil {
		return "", err
	}

	return dl.Name(), nil
}

func ValidateBundle(t *testing.T, bundlePath string) {
	v, err := validate.NewValidatorFromPath(bundlePath, false, "linux")
	if err != nil {
		t.Errorf("Could not create bundle validator: %v", err)
	}
	if err := v.CheckAll(); err != nil {
		t.Errorf("Bundle not valid: %v", err)
	}
}
