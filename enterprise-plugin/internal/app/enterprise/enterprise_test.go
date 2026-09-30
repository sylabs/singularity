// Copyright (c) 2020-2026 Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

//go:build enterprise_integration

package enterprise

import (
	"flag"
	"os"
	"testing"
)

// mockURI holds the URI to access services mocked from the OpenAPI yaml.
// See README.md r.e. how to start the mock services.
var mockURI = flag.String("uri", "http://localhost:8080", "URI of mock services for tests")

// mockToken holds a Bearer token to send to the mock services.
//
//nolint:gosec
const mockToken = "eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9.eyJhbGxSb2xlcyI6W10sImV4cCI6MTg1MjAzNDk0OCwiZ3JvdXBzIjpbXSwiaWF0IjoxNTM2Njc0OTQ4LCJpc3MiOiJ0ZXN0LXRva2VucyIsImp0aSI6IjViOTdjYzg0MDJlMDkwNTVjYmI3ZTlmZiIsInJvbGVzIjpbXSwic3ViIjoiNWI3YWVjMmY2MDZiYjgwMzY0MGY2YWFhIiwidXNlcm5hbWUiOiJ1c2VyMTIzIn0.BH5WT5kgwKo4j2PyfRTBSMqam7_AiSvc9MBcvDhRHxqXkcaLk_W6hyYx-XhxeT1SIrc7qFV9ZGUxwL3RGj52knq3vMMikxNpsFxC_7sHwsAz8L92ldYf_VdiBctIqFsJJuO6zZgQkcvL2rNLGYjTw_6YuiN8d3fsopdcl_QK_xCy6ou7C1YkWsJMC8HdaS6ymotVGIJPZ1x3YhktubCGhwVpwJgwCd9ucT-X3rMQeXbuY01u0bDvR57Pi0LHdDTbJt42r227ciwH-noKJZO2qZsIa2T8mxy7aFbWqKQlPHL68Pbc93WW3XhJcMnxIUkfv1HjAujQD26w3nNI7a0ZkQ"

func TestMain(m *testing.M) {
	flag.Parse()
	os.Exit(m.Run())
}
