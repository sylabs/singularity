// Copyright (c) 2026 Sylabs Inc. All rights reserved.
// This software is commercially licensed for use with SingularityPRO.

package config_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/sylabs/singularity/v4/enterprise-plugin/internal/pkg/config"
)

const (
	// Subject is the subject of standard user test tokens in the testtokens package.
	Subject = "5b7aec2f606bb803640f6aaa"
	// TokenValid is a full token with subject "5b7aec2f606bb803640f6aaa"
	TokenValid = "eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9.eyJhbGxSb2xlcyI6W10sImV4cCI6MTg1MjAzNDk0OCwiZ3JvdXBzIjpbXSwiaWF0IjoxNTM2Njc0OTQ4LCJpc3MiOiJ0ZXN0LXRva2VucyIsImp0aSI6IjViOTdjYzg0MDJlMDkwNTVjYmI3ZTlmZiIsInJvbGVzIjpbXSwic3ViIjoiNWI3YWVjMmY2MDZiYjgwMzY0MGY2YWFhIiwidXNlcm5hbWUiOiJ1c2VyMTIzIn0.BH5WT5kgwKo4j2PyfRTBSMqam7_AiSvc9MBcvDhRHxqXkcaLk_W6hyYx-XhxeT1SIrc7qFV9ZGUxwL3RGj52knq3vMMikxNpsFxC_7sHwsAz8L92ldYf_VdiBctIqFsJJuO6zZgQkcvL2rNLGYjTw_6YuiN8d3fsopdcl_QK_xCy6ou7C1YkWsJMC8HdaS6ymotVGIJPZ1x3YhktubCGhwVpwJgwCd9ucT-X3rMQeXbuY01u0bDvR57Pi0LHdDTbJt42r227ciwH-noKJZO2qZsIa2T8mxy7aFbWqKQlPHL68Pbc93WW3XhJcMnxIUkfv1HjAujQD26w3nNI7a0ZkQ" //nolint:gosec
)

func TestUserFromToken(t *testing.T) {
	got, err := config.UserFromToken(TokenValid)
	assert.Equal(t, got, Subject)
	assert.NoError(t, err)
}
