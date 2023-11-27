// Copyright (c) 2023, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package cache

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"

	v1 "github.com/google/go-containerregistry/pkg/v1"
)

func TestHandle_PutOciCacheBlob(t *testing.T) {
	tmpDir := t.TempDir()
	content := "BLOB CONTENT"
	contentDigest, _, err := v1.SHA256(bytes.NewBufferString(content))
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name         string
		disableCache bool
		cacheType    string
		blobDigest   v1.Hash
		content      string
		wantErr      bool
	}{
		{
			name:         "Success",
			disableCache: false,
			cacheType:    OciBlobCacheType,
			blobDigest:   contentDigest,
			content:      content,
			wantErr:      false,
		},
		{
			name:         "Disabled",
			disableCache: true,
			cacheType:    OciBlobCacheType,
			blobDigest:   contentDigest,
			content:      content,
			wantErr:      true,
		},
		{
			name:         "BadCacheType",
			disableCache: false,
			cacheType:    LibraryCacheType,
			blobDigest:   contentDigest,
			content:      content,
			wantErr:      true,
		},
		{
			name:         "BadDigest",
			disableCache: false,
			cacheType:    OciBlobCacheType,
			blobDigest:   v1.Hash{},
			content:      content,
			wantErr:      true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, err := New(Config{
				ParentDir: tmpDir,
				Disable:   tt.disableCache,
			})
			if err != nil {
				t.Fatal(err)
			}
			err = h.PutOciCacheBlob(tt.cacheType, tt.blobDigest, io.NopCloser(bytes.NewBufferString(tt.content)))
			if (err != nil) != tt.wantErr {
				t.Errorf("Handle.PutOciCacheBlob() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				cacheFile := filepath.Join(tmpDir, "cache", "blob", "blobs", tt.blobDigest.Algorithm, tt.blobDigest.Hex)
				cacheContent, err := os.ReadFile(cacheFile)
				if err != nil {
					t.Errorf("Couldn't read expected cache file: %v", err)
				}
				if string(cacheContent) != string(tt.content) {
					t.Errorf("Content was %q, expected %q", cacheContent, tt.content)
				}
			}
		})
	}
}

func TestHandle_GetOciCacheBlob(t *testing.T) {
	tmpDir := t.TempDir()
	content := "BLOB CONTENT"
	contentDigest, _, err := v1.SHA256(bytes.NewBufferString(content))
	if err != nil {
		t.Fatal(err)
	}

	PutHandle, err := New(Config{
		ParentDir: tmpDir,
		Disable:   false,
	})
	if err != nil {
		t.Fatal(err)
	}

	err = PutHandle.PutOciCacheBlob(
		OciBlobCacheType,
		contentDigest,
		io.NopCloser(bytes.NewBufferString(content)))
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name         string
		disableCache bool
		cacheType    string
		blobDigest   v1.Hash
		wantContent  []byte
		wantErr      bool
	}{
		{
			name:         "Success",
			disableCache: false,
			cacheType:    OciBlobCacheType,
			blobDigest:   contentDigest,
			wantContent:  nil,
			wantErr:      false,
		},
		{
			name:         "Disabled",
			disableCache: true,
			cacheType:    OciBlobCacheType,
			blobDigest:   contentDigest,
			wantContent:  nil,
			wantErr:      true,
		},
		{
			name:         "BadCacheType",
			disableCache: false,
			cacheType:    LibraryCacheType,
			blobDigest:   contentDigest,
			wantContent:  nil,
			wantErr:      true,
		},
		{
			name:         "BadDigest",
			disableCache: false,
			cacheType:    OciBlobCacheType,
			blobDigest:   v1.Hash{},
			wantContent:  nil,
			wantErr:      true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, err := New(Config{
				ParentDir: tmpDir,
				Disable:   tt.disableCache,
			})
			if err != nil {
				t.Fatal(err)
			}
			r, err := h.GetOciCacheBlob(tt.cacheType, tt.blobDigest)
			if (err != nil) != tt.wantErr {
				t.Errorf("Handle.PutOciCacheBlob() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantContent != nil {
				cacheContent, err := io.ReadAll(r)
				if err != nil {
					t.Errorf("Couldn't read from cache: %v", err)
				}
				if string(cacheContent) != string(tt.wantContent) {
					t.Errorf("Content was %q, expected %q", cacheContent, tt.wantContent)
				}
			}
		})
	}
}
