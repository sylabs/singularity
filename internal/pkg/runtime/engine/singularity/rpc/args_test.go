// Copyright (c) 2026, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package rpc

import (
	"bytes"
	"encoding/gob"
	"errors"
	"os"
	"syscall"
	"testing"
)

func TestMountErrorReplyGob(t *testing.T) {
	err := errors.New("path escapes from parent")
	reply := NewMountErrorReply(err)

	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(reply); err != nil {
		t.Fatalf("failed to gob encode reply: %s", err)
	}

	var decoded MountErrorReply
	if err := gob.NewDecoder(&buf).Decode(&decoded); err != nil {
		t.Fatalf("failed to gob decode reply: %s", err)
	}
	if got := decoded.Error(); got != err.Error() {
		t.Fatalf("got %q, want %q", got, err.Error())
	}
}

func TestMountErrorReplyNil(t *testing.T) {
	var reply MountErrorReply

	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(reply); err != nil {
		t.Fatalf("failed to gob encode reply: %s", err)
	}

	var decoded MountErrorReply
	if err := gob.NewDecoder(&buf).Decode(&decoded); err != nil {
		t.Fatalf("failed to gob decode reply: %s", err)
	}
	if err := decoded.Err(); err != nil {
		t.Fatalf("got non-nil error from nil reply: %q", err)
	}
}

func TestErrorReplyIs(t *testing.T) {
	tests := []struct {
		name   string
		err    error
		target error
		want   bool
	}{
		{
			name:   "ErrnoMismatch",
			err:    syscall.EPERM,
			target: syscall.ENOENT,
			want:   false,
		},
		{
			name:   "NotExist",
			err:    &os.PathError{Op: "open", Path: "/x", Err: os.ErrNotExist},
			target: os.ErrNotExist,
			want:   true,
		},
		{
			name:   "Permission",
			err:    &os.PathError{Op: "open", Path: "/x", Err: os.ErrPermission},
			target: os.ErrPermission,
			want:   true,
		},
		// Explicitly test syscall mount errors we match against in the engine.
		{
			name:   "EPERM",
			err:    syscall.EPERM,
			target: syscall.EPERM,
			want:   true,
		},
		{
			name:   "ESTALE",
			err:    syscall.ESTALE,
			target: syscall.ESTALE,
			want:   true,
		},
		{
			name:   "EINVAL",
			err:    syscall.EINVAL,
			target: syscall.EINVAL,
			want:   true,
		},
		{
			name:   "ENODEV",
			err:    syscall.ENODEV,
			target: syscall.ENODEV,
			want:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := gob.NewEncoder(&buf).Encode(NewMountErrorReply(tt.err)); err != nil {
				t.Fatalf("failed to gob encode reply: %s", err)
			}
			var decoded MountErrorReply
			if err := gob.NewDecoder(&buf).Decode(&decoded); err != nil {
				t.Fatalf("failed to gob decode reply: %s", err)
			}
			if got := errors.Is(decoded.Err(), tt.target); got != tt.want {
				t.Fatalf("errors.Is(%q, %v) = %v, want %v", tt.err, tt.target, got, tt.want)
			}
		})
	}
}
