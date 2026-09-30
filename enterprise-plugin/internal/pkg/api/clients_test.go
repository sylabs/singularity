// Copyright (c) 2020-2026 Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

//revive:disable:var-naming
package api

import (
	"errors"
	"testing"

	"github.com/sylabs/singularity/v4/enterprise-plugin/internal/pkg/config"
)

func TestNewClients(t *testing.T) {
	tests := []struct {
		name    string
		sURIs   *config.ServiceURIs
		hasErr  bool
		errType error
	}{
		{
			name: "NoLibraryURI",
			sURIs: &config.ServiceURIs{ //nolint:gosec	// No credentials here
				LibraryURI:        "",
				BuildServerURI:    "https://buildserver.example.com",
				BuildManagerURI:   "https://buildmanager.example.com",
				KeyServiceURI:     "https://keys.example.com",
				ConsentServiceURI: "https://consent.example.com",
				TokenServiceURI:   "https://token.example.com",
			},
			hasErr:  true,
			errType: ErrNoURI,
		},
		{
			name: "BadLibraryURI",
			sURIs: &config.ServiceURIs{ //nolint:gosec	// No credentials here
				LibraryURI:        "https://library:example.com",
				BuildServerURI:    "https://buildserver.example.com",
				BuildManagerURI:   "https://buildmanager.example.com",
				KeyServiceURI:     "https://keys.example.com",
				ConsentServiceURI: "https://consent.example.com",
				TokenServiceURI:   "https://token.example.com",
			},
			hasErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewClients(tt.sURIs, nil)

			if tt.hasErr && err == nil {
				t.Errorf("Expected error: %v, got no error", tt.errType)
			}

			if !tt.hasErr && err != nil {
				t.Errorf("Expected no error, got error: %v", err)
			}

			if tt.errType != nil && !errors.Is(err, tt.errType) {
				t.Errorf("Expected error: %v, got error: %v", tt.errType, err)
			}
		})
	}
}
