// Copyright (c) 2022, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package bind

import (
	"reflect"
	"testing"
)

func TestParseBindPath(t *testing.T) {
	tests := []struct {
		name      string
		bindpaths string
		want      []Path
		wantErr   bool
	}{
		{
			name:      "srcOnly",
			bindpaths: "/opt",
			want: []Path{
				{
					Source:      "/opt",
					Destination: "/opt",
				},
			},
		},
		{
			name:      "srcOnlyMultiple",
			bindpaths: "/opt,/tmp",
			want: []Path{
				{
					Source:      "/opt",
					Destination: "/opt",
				},
				{
					Source:      "/tmp",
					Destination: "/tmp",
				},
			},
		},
		{
			name:      "srcDst",
			bindpaths: "/opt:/other",
			want: []Path{
				{
					Source:      "/opt",
					Destination: "/other",
				},
			},
		},
		{
			name:      "srcDstMultiple",
			bindpaths: "/opt:/other,/tmp:/other2",
			want: []Path{
				{
					Source:      "/opt",
					Destination: "/other",
				},
				{
					Source:      "/tmp",
					Destination: "/other2",
				},
			},
		},
		{
			name:      "srcDstRO",
			bindpaths: "/opt:/other:ro",
			want: []Path{
				{
					Source:      "/opt",
					Destination: "/other",
					Options: map[string]*Option{
						"ro": {},
					},
				},
			},
		},
		{
			name:      "srcDstROMultiple",
			bindpaths: "/opt:/other:ro,/tmp:/other2:ro",
			want: []Path{
				{
					Source:      "/opt",
					Destination: "/other",
					Options: map[string]*Option{
						"ro": {},
					},
				},
				{
					Source:      "/tmp",
					Destination: "/other2",
					Options: map[string]*Option{
						"ro": {},
					},
				},
			},
		},
		{
			// This doesn't make functional sense (ro & rw), but is testing
			// parsing multiple simple options.
			name:      "srcDstRORW",
			bindpaths: "/opt:/other:ro,rw",
			want: []Path{
				{
					Source:      "/opt",
					Destination: "/other",
					Options: map[string]*Option{
						"ro": {},
						"rw": {},
					},
				},
			},
		},
		{
			// This doesn't make functional sense (ro & rw), but is testing
			// parsing multiple binds, with multiple options each. Note the
			// complex parsing here that has to distinguish between comma
			// delimiting an additional option, vs an additional bind.
			name:      "srcDstRORWMultiple",
			bindpaths: "/opt:/other:ro,rw,/tmp:/other2:ro,rw",
			want: []Path{
				{
					Source:      "/opt",
					Destination: "/other",
					Options: map[string]*Option{
						"ro": {},
						"rw": {},
					},
				},
				{
					Source:      "/tmp",
					Destination: "/other2",
					Options: map[string]*Option{
						"ro": {},
						"rw": {},
					},
				},
			},
		},
		{
			name:      "srcDstImageSrc",
			bindpaths: "test.sif:/other:image-src=/opt",
			want: []Path{
				{
					Source:      "test.sif",
					Destination: "/other",
					Options: map[string]*Option{
						"image-src": {"/opt"},
					},
				},
			},
		},
		{
			// Can't use image-src without a value
			name:      "srcDstImageSrcNoVal",
			bindpaths: "test.sif:/other:image-src",
			want:      []Path{},
			wantErr:   true,
		},
		{
			name:      "srcDstId",
			bindpaths: "test.sif:/other:image-src=/opt,id=2",
			want: []Path{
				{
					Source:      "test.sif",
					Destination: "/other",
					Options: map[string]*Option{
						"image-src": {"/opt"},
						"id":        {"2"},
					},
				},
			},
		},
		{
			name:      "invalidOption",
			bindpaths: "/opt:/other:invalid",
			want:      []Path{},
			wantErr:   true,
		},
		{
			name:      "invalidSpec",
			bindpaths: "/opt:/other:rw:invalid",
			want:      []Path{},
			wantErr:   true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseBindPath(tt.bindpaths)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseBindPath() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ParseBindPath() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseDataBindPath(t *testing.T) {
	tests := []struct {
		name      string
		bindpaths string
		want      Path
		wantErr   bool
	}{
		{
			name:      "valid",
			bindpaths: "data.oci.sif:/data",
			want: Path{
				Source:      "data.oci.sif",
				Destination: "/data",
				Options:     map[string]*Option{"image-src": {"/"}},
			},
		},
		{
			name:      "srcOnly",
			bindpaths: "data.oci.sif",
			wantErr:   true,
		},
		{
			name:      "emptySrc",
			bindpaths: ":/data",
			wantErr:   true,
		},
		{
			name:      "emptyDest",
			bindpaths: "data.oci.sif:",
			wantErr:   true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseDataBindPath(tt.bindpaths)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseDataBindPath() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ParseDataBindPath() = %v, want %v", got, tt.want)
			}
		})
	}
}
