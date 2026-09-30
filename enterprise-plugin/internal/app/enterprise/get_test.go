// Copyright (c) 2021-2026, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

//go:build enterprise_integration

package enterprise

import (
	"testing"

	"github.com/sylabs/singularity/v4/enterprise-plugin/internal/pkg/output"
)

func TestGet_buildagent(t *testing.T) {
	tests := []struct {
		name        string
		actionType  string
		args        []string
		expectError bool
	}{
		{
			name:        "build-agent-no-pool",
			actionType:  "build-agent",
			args:        []string{},
			expectError: true,
		},
		{
			name:        "build-agent-no-auth",
			actionType:  "build-agent",
			args:        []string{"default"},
			expectError: true,
		},
		{
			name:       "build-agent-default-pool",
			actionType: "build-agent",
			args:       []string{"default", "--token", mockToken},
		},
	}
	for _, tt := range tests {
		args := []string{"--uri", *mockURI, "get", tt.actionType}
		args = append(args, tt.args...)
		testAllFormats(t, tt.name, args, tt.expectError)
	}
}

func TestGet_builds(t *testing.T) {
	tests := []struct {
		name        string
		actionType  string
		args        []string
		expectError bool
	}{
		{
			name:        "builds-no-auth",
			actionType:  "builds",
			args:        []string{},
			expectError: true,
		},
		{
			name:        "builds",
			actionType:  "builds",
			args:        []string{"--token", mockToken},
			expectError: false,
		},
	}
	for _, tt := range tests {
		args := []string{"--uri", *mockURI, "get", tt.actionType}
		args = append(args, tt.args...)
		testAllFormats(t, tt.name, args, tt.expectError)
	}
}

func TestGet_entities(t *testing.T) {
	tests := []struct {
		name        string
		actionType  string
		args        []string
		expectError bool
	}{
		{
			name:        "entities",
			actionType:  "entities",
			args:        []string{},
			expectError: false,
		},
		{
			name:        "entity-single",
			actionType:  "entities",
			args:        []string{"user123"},
			expectError: false,
		},
		{
			name:        "entity-multiple",
			actionType:  "entities",
			args:        []string{"user123", "user456", "user789"},
			expectError: false,
		},
		{
			name:        "projects",
			actionType:  "projects",
			args:        []string{},
			expectError: false,
		},
	}
	for _, tt := range tests {
		args := []string{"--uri", *mockURI, "get", tt.actionType}
		args = append(args, tt.args...)
		testAllFormats(t, tt.name, args, tt.expectError)
	}
}

func TestGet_collections(t *testing.T) {
	tests := []struct {
		name        string
		actionType  string
		args        []string
		expectError bool
	}{
		{
			name:        "collection-user123",
			actionType:  "collections",
			args:        []string{"user123"},
			expectError: false,
		},
		{
			name:        "collections-single",
			actionType:  "collections",
			args:        []string{"user123/collection1"},
			expectError: false,
		},
		{
			name:        "collections-multiple",
			actionType:  "collections",
			args:        []string{"user123/collection1", "user456/collection2", "user789/collection3"},
			expectError: false,
		},
	}
	for _, tt := range tests {
		args := []string{"--uri", *mockURI, "get", tt.actionType}
		args = append(args, tt.args...)
		testAllFormats(t, tt.name, args, tt.expectError)
	}
}

func TestGet_users(t *testing.T) {
	tests := []struct {
		name        string
		actionType  string
		args        []string
		expectError bool
	}{
		{
			name:        "users-no-auth",
			actionType:  "users",
			args:        []string{},
			expectError: true,
		},
		{
			name:        "users",
			actionType:  "users",
			args:        []string{"--token", mockToken},
			expectError: false,
		},
		{
			name:        "users-single",
			actionType:  "users",
			args:        []string{"--token", mockToken, "60d2172a0014a7262b134562"},
			expectError: false,
		},
		{
			name:        "users-multiple",
			actionType:  "users",
			args:        []string{"--token", mockToken, "60d2172a0014a7262b134562", "60d2172a0014a7262b134563", "60d2172a0014a7262b134564"},
			expectError: false,
		},
	}
	for _, tt := range tests {
		args := []string{"--uri", *mockURI, "get", tt.actionType}
		args = append(args, tt.args...)
		testAllFormats(t, tt.name, args, tt.expectError)
	}
}

func TestGet_keys(t *testing.T) {
	tests := []struct {
		name        string
		actionType  string
		args        []string
		expectError bool
	}{
		{
			name:        "keys-no-auth",
			actionType:  "keys",
			args:        []string{},
			expectError: true,
		},
		{
			name:        "keys",
			actionType:  "keys",
			args:        []string{"--token", mockToken},
			expectError: false,
		},
	}
	for _, tt := range tests {
		args := []string{"--uri", *mockURI, "get", tt.actionType}
		args = append(args, tt.args...)
		testAllFormats(t, tt.name, args, tt.expectError)
	}
}

func TestGet_tokens(t *testing.T) {
	tests := []struct {
		name        string
		actionType  string
		args        []string
		expectError bool
	}{
		{
			name:        "tokens-no-auth",
			actionType:  "tokens",
			args:        []string{},
			expectError: true,
		},
		{
			name:        "tokens",
			actionType:  "tokens",
			args:        []string{"--token", mockToken},
			expectError: false,
		},
		{
			name:        "tokens-single",
			actionType:  "tokens",
			args:        []string{"--token", mockToken, "60d2172a0014a7262b134562"},
			expectError: false,
		},
		{
			name:        "tokens-multiple",
			actionType:  "tokens",
			args:        []string{"--token", mockToken, "60d2172a0014a7262b134562", "60d2172a0014a7262b134563", "60d2172a0014a7262b134564"},
			expectError: false,
		},
	}
	for _, tt := range tests {
		args := []string{"--uri", *mockURI, "get", tt.actionType}
		args = append(args, tt.args...)
		testAllFormats(t, tt.name, args, tt.expectError)
	}
}

// testAllFormats runs a commandTester based subtest for each supported output format.
func testAllFormats(t *testing.T, name string, args []string, expectError bool) {
	for _, format := range output.Formats {
		t.Run(name+"_"+format, func(t *testing.T) {
			e, err := NewExecutor()
			if err != nil {
				t.Fatal(err)
			}

			execArgs := []string{"--output", format}
			execArgs = append(execArgs, args...)

			ct := newCommandTester(e.rootCmd)
			err = ct.execute(execArgs...)
			if err != nil && !expectError {
				t.Fatalf("command execution failed: %v", err)
			}
			if err == nil && expectError {
				t.Fatalf("error was expected but success indicated")
			}

			ct.assertStdout(t, nil)
			ct.assertStderr(t, nil)
		})
	}
}
