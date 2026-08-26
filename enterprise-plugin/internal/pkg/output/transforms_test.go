// Copyright (c) 2021-2026, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

//go:build enterprise_integration

package output

import (
	"flag"
	"os"
	"testing"

	httptransport "github.com/go-openapi/runtime/client"
	"github.com/sylabs/singularity/v4/enterprise-plugin/internal/pkg/api"
	"github.com/sylabs/singularity/v4/enterprise-plugin/internal/pkg/config"
	useragent "github.com/sylabs/singularity/v4/pkg/util/user-agent"
)

// mockURI holds the URI to access services mocked from the OpenAPI yaml.
// See README.md r.e. how to start the mock services.
var mockURI = flag.String("uri", "http://localhost:8080", "URI of mock services for tests")

func TestUpperCaseTransform(t *testing.T) {
	expected := "ABC"
	result := UpperCaseTransform("abc", nil)
	if result != expected {
		t.Errorf("Expected: %s, got %s", expected, result)
	}
}

func TestUserNameTransform(t *testing.T) {
	id := "507f1f77bcf86cd799439aaa"
	expected := "user123"

	su, err := config.GetServiceURIs(*mockURI)
	if err != nil {
		t.Fatalf("%v", err)
	}

	authInfo := httptransport.BearerToken("")
	clients, err := api.NewClients(su, authInfo)
	if err != nil {
		t.Fatalf("%v", err)
	}

	result := UsernameTransform(id, clients)
	if result != expected {
		t.Errorf("Expected: %s, got %s", expected, result)
	}
}
func TestTimezoneTransform(t *testing.T) {
	tests := []struct {
		name     string
		in       string
		expected string
	}{
		{
			name:     "valid",
			in:       "2022-01-18T18:52:28Z",
			expected: "2022-01-19 03:52:28 +0900 JST",
		},
		{
			name:     "invalid",
			in:       "Not a date",
			expected: "Not a date",
		},
		{
			name:     "nil",
			in:       "0001-01-01T00:00:00Z",
			expected: "",
		},
	}

	oldTZ := os.Getenv("TZ")
	if oldTZ != "" {
		defer os.Setenv("TZ", oldTZ)
	} else {
		defer os.Unsetenv("TZ")
	}
	os.Setenv("TZ", "Asia/Tokyo")

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := TimezoneTransform(tt.in, nil)
			if result != tt.expected {
				t.Errorf("Expected: %s, got %s", tt.expected, result)
			}
		})
	}
}

func TestMain(m *testing.M) {
	flag.Parse()
	useragent.InitValue("transforms_test", "")
	os.Exit(m.Run())
}
