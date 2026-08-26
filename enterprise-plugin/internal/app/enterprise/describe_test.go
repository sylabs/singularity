// Copyright (c) 2021-2026, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

//go:build enterprise_integration

package enterprise

import (
	"testing"
)

//nolint:dupl
func TestDescribe_entity(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		expectError bool
	}{
		{
			name:        "no-args",
			args:        []string{},
			expectError: true,
		},
		{
			name:        "valid",
			args:        []string{"library"},
			expectError: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e, err := NewExecutor()
			if err != nil {
				t.Fatal(err)
			}

			args := []string{"--uri", *mockURI, "describe", "entity"}
			args = append(args, tt.args...)

			ct := newCommandTester(e.rootCmd)
			err = ct.execute(args...)
			if err != nil && !tt.expectError {
				t.Fatalf("command execution failed: %v", err)
			}
			if err == nil && tt.expectError {
				t.Fatalf("error was expected but success indicated")
			}

			ct.assertStdout(t, nil)
			ct.assertStderr(t, nil)
		})
	}
}

//nolint:dupl
func TestDescribe_collection(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		expectError bool
	}{
		{
			name:        "no-args",
			args:        []string{},
			expectError: true,
		},
		{
			name:        "valid",
			args:        []string{"library/default"},
			expectError: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e, err := NewExecutor()
			if err != nil {
				t.Fatal(err)
			}

			args := []string{"--uri", *mockURI, "describe", "collection"}
			args = append(args, tt.args...)

			ct := newCommandTester(e.rootCmd)
			err = ct.execute(args...)
			if err != nil && !tt.expectError {
				t.Fatalf("command execution failed: %v", err)
			}
			if err == nil && tt.expectError {
				t.Fatalf("error was expected but success indicated")
			}

			ct.assertStdout(t, nil)
			ct.assertStderr(t, nil)
		})
	}
}

//nolint:dupl
func TestDescribe_user(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		expectError bool
	}{
		{
			name:        "no-args",
			args:        []string{},
			expectError: true,
		},
		{
			name:        "no-auth",
			args:        []string{"5eaa1ea9df974cd942a00000"},
			expectError: true,
		},
		{
			name:        "valid",
			args:        []string{"--token", mockToken, "5eaa1ea9df974cd942a00000"},
			expectError: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e, err := NewExecutor()
			if err != nil {
				t.Fatal(err)
			}

			args := []string{"--uri", *mockURI, "describe", "user"}
			args = append(args, tt.args...)

			ct := newCommandTester(e.rootCmd)
			err = ct.execute(args...)
			if err != nil && !tt.expectError {
				t.Fatalf("command execution failed: %v", err)
			}
			if err == nil && tt.expectError {
				t.Fatalf("error was expected but success indicated")
			}

			ct.assertStdout(t, nil)
			ct.assertStderr(t, nil)
		})
	}
}

//nolint:dupl
func TestDescribe_token(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		expectError bool
	}{
		{
			name:        "no-args",
			args:        []string{},
			expectError: true,
		},
		{
			name:        "no-auth",
			args:        []string{"5eaa1ea9df974cd942a00000"},
			expectError: true,
		},
		{
			name:        "valid",
			args:        []string{"--token", mockToken, "5eaa1ea9df974cd942a00000"},
			expectError: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e, err := NewExecutor()
			if err != nil {
				t.Fatal(err)
			}

			args := []string{"--uri", *mockURI, "describe", "token"}
			args = append(args, tt.args...)

			ct := newCommandTester(e.rootCmd)
			err = ct.execute(args...)
			if err != nil && !tt.expectError {
				t.Fatalf("command execution failed: %v", err)
			}
			if err == nil && tt.expectError {
				t.Fatalf("error was expected but success indicated")
			}

			ct.assertStdout(t, nil)
			ct.assertStderr(t, nil)
		})
	}
}
