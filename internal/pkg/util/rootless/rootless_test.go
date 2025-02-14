// Copyright (c) 2023, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package rootless

import (
	"os"
	"os/user"
	"reflect"
	"testing"

	"github.com/ccoveille/go-safecast"
)

//nolint:dupl
func TestGetuid(t *testing.T) {
	currUID, err := safecast.ToUint32(os.Getuid())
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name    string
		setEnv  bool
		envVal  string
		wantUID uint32
		wantErr bool
	}{
		{
			name:    "unset",
			setEnv:  false,
			envVal:  "",
			wantUID: currUID,
			wantErr: false,
		},
		{
			name:    "empty",
			setEnv:  true,
			envVal:  "",
			wantUID: currUID,
			wantErr: false,
		},
		{
			name:    "valid",
			setEnv:  true,
			envVal:  "123",
			wantUID: 123,
			wantErr: false,
		},
		{
			name:    "invalid",
			setEnv:  true,
			envVal:  "abc",
			wantUID: 0,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setEnv {
				os.Setenv(UIDEnv, tt.envVal)
				defer os.Unsetenv(UIDEnv)
			}
			gotUID, err := Getuid()
			if (err != nil) != tt.wantErr {
				t.Errorf("Getuid() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if gotUID != tt.wantUID {
				t.Errorf("Getuid() = %v, want %v", gotUID, tt.wantUID)
			}
		})
	}
}

//nolint:dupl
func TestGetgid(t *testing.T) {
	currGID, err := safecast.ToUint32(os.Getgid())
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name    string
		setEnv  bool
		envVal  string
		wantGID uint32
		wantErr bool
	}{
		{
			name:    "unset",
			setEnv:  false,
			envVal:  "",
			wantGID: currGID,
			wantErr: false,
		},
		{
			name:    "empty",
			setEnv:  true,
			envVal:  "",
			wantGID: currGID,
			wantErr: false,
		},
		{
			name:    "valid",
			setEnv:  true,
			envVal:  "456",
			wantGID: 456,
			wantErr: false,
		},
		{
			name:    "invalid",
			setEnv:  true,
			envVal:  "abc",
			wantGID: 0,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setEnv {
				os.Setenv(GIDEnv, tt.envVal)
				defer os.Unsetenv(GIDEnv)
			}
			gotGID, err := Getgid()
			if (err != nil) != tt.wantErr {
				t.Errorf("Getgid() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if gotGID != tt.wantGID {
				t.Errorf("Getgid() = %v, want %v", gotGID, tt.wantGID)
			}
		})
	}
}

func TestGetUser(t *testing.T) {
	currentUser, err := user.Current()
	if err != nil {
		t.Fatal(err)
	}
	rootUser, err := user.LookupId("0")
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name    string
		setEnv  bool
		envVal  string
		want    *user.User
		wantErr bool
	}{
		{
			name:    "unset",
			setEnv:  false,
			envVal:  "",
			want:    currentUser,
			wantErr: false,
		},
		{
			name:    "empty",
			setEnv:  true,
			envVal:  "",
			want:    currentUser,
			wantErr: false,
		},
		{
			name:    "valid",
			setEnv:  true,
			envVal:  "0",
			want:    rootUser,
			wantErr: false,
		},
		{
			name:    "invalid",
			setEnv:  true,
			envVal:  "abc",
			want:    nil,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setEnv {
				os.Setenv(UIDEnv, tt.envVal)
				defer os.Unsetenv(UIDEnv)
			}
			got, err := GetUser()
			if (err != nil) != tt.wantErr {
				t.Errorf("GetUser() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("GetUser() = %v, want %v", got, tt.want)
			}
		})
	}
}
