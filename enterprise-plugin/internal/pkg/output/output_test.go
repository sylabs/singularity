// Copyright (c) 2021-2026, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package output

import (
	"errors"
	"testing"

	"github.com/sebdah/goldie/v2"
)

type testStruct struct {
	Name    string
	Age     int
	Dancing bool
}

var (
	testA     = testStruct{"Person A", 30, true}
	testB     = testStruct{"Person B", 30, false}
	testExtra = map[string][]string{
		"Extra": {"extraA", "extraB"},
	}
	badExtra = map[string][]string{
		"Extra": {"extraA"},
	}
	shortFields   = []string{"Name", "Age"}
	longFields    = []string{"Name", "Age", "Dancing"}
	extraFields   = []string{"Name", "Age", "Dancing", "Extra"}
	badFields     = []string{"Name", "Address"}
	testTransform = map[string]TransformFunc{
		"Name": UpperCaseTransform,
	}
)

func TestSV(t *testing.T) {
	tests := []struct {
		name    string
		format  string
		data    []*testStruct
		extra   *map[string][]string
		spec    FormatSpec
		wantErr bool
		errType error
	}{
		{
			name:   "ShortGood",
			format: "short",
			data:   []*testStruct{&testA, &testB},
			spec: FormatSpec{
				ShortFields:  shortFields,
				ShortHeaders: shortFields,
				LongFields:   longFields,
				LongHeaders:  longFields,
			},
			wantErr: false,
			errType: nil,
		},
		{
			name:   "ShortBadFields",
			format: "short",
			data:   []*testStruct{&testA, &testB},
			spec: FormatSpec{
				ShortFields:  badFields,
				ShortHeaders: badFields,
				LongFields:   badFields,
				LongHeaders:  badFields,
			},
			wantErr: true,
			errType: ErrFieldNotFound,
		},
		{
			name:   "ShortNilItem",
			format: "short",
			data:   []*testStruct{&testA, nil},
			spec: FormatSpec{
				ShortFields:  shortFields,
				ShortHeaders: shortFields,
				LongFields:   longFields,
				LongHeaders:  longFields,
			},
			wantErr: true,
			errType: ErrNotAStruct,
		},
		{
			name:   "ShortTransform",
			format: "short",
			data:   []*testStruct{&testA, &testB},
			spec: FormatSpec{
				ShortFields:  shortFields,
				ShortHeaders: shortFields,
				LongFields:   longFields,
				LongHeaders:  longFields,
				Transforms:   testTransform,
			},
			wantErr: false,
			errType: nil,
		},
		{
			name:   "LongGood",
			format: "long",
			data:   []*testStruct{&testA, &testB},
			spec: FormatSpec{
				ShortFields:  shortFields,
				ShortHeaders: shortFields,
				LongFields:   longFields,
				LongHeaders:  longFields,
			},
			wantErr: false,
			errType: nil,
		},
		{
			name:   "CSVGood",
			format: "csv",
			data:   []*testStruct{&testA, &testB},
			spec: FormatSpec{
				ShortFields:  shortFields,
				ShortHeaders: shortFields,
				LongFields:   longFields,
				LongHeaders:  longFields,
			},
			wantErr: false,
			errType: nil,
		},
		{
			name:   "CSVBadFields",
			format: "csv",
			data:   []*testStruct{&testA, &testB},
			spec: FormatSpec{
				ShortFields:  badFields,
				ShortHeaders: badFields,
				LongFields:   badFields,
				LongHeaders:  badFields,
			},
			wantErr: true,
			errType: ErrFieldNotFound,
		},
		{
			name:   "CSVNilItem",
			format: "csv",
			data:   []*testStruct{&testA, nil},
			spec: FormatSpec{
				ShortFields:  shortFields,
				ShortHeaders: shortFields,
				LongFields:   longFields,
				LongHeaders:  longFields,
			},
			wantErr: true,
			errType: ErrNotAStruct,
		},
		{
			name:   "CSVTransform",
			format: "csv",
			data:   []*testStruct{&testA, &testB},
			spec: FormatSpec{
				ShortFields:  shortFields,
				ShortHeaders: shortFields,
				LongFields:   longFields,
				LongHeaders:  longFields,
				Transforms:   testTransform,
			},
			wantErr: false,
			errType: nil,
		},
		{
			name:   "ShortExtra",
			format: "short",
			data:   []*testStruct{&testA, &testB},
			extra:  &testExtra,
			spec: FormatSpec{
				ShortFields:  extraFields,
				ShortHeaders: extraFields,
				LongFields:   extraFields,
				LongHeaders:  extraFields,
			},
			wantErr: false,
			errType: nil,
		},
		{
			name:   "LongExtra",
			format: "long",
			data:   []*testStruct{&testA, &testB},
			extra:  &testExtra,
			spec: FormatSpec{
				ShortFields:  extraFields,
				ShortHeaders: extraFields,
				LongFields:   extraFields,
				LongHeaders:  extraFields,
			},
			wantErr: false,
			errType: nil,
		},
		{
			name:   "CSVExtra",
			format: "csv",
			data:   []*testStruct{&testA, &testB},
			extra:  &testExtra,
			spec: FormatSpec{
				ShortFields:  extraFields,
				ShortHeaders: extraFields,
				LongFields:   extraFields,
				LongHeaders:  extraFields,
			},
			wantErr: false,
			errType: nil,
		},
		{
			name:   "ShortBadLengthExtra",
			format: "short",
			data:   []*testStruct{&testA, &testB},
			extra:  &badExtra,
			spec: FormatSpec{
				ShortFields:  extraFields,
				ShortHeaders: extraFields,
				LongFields:   extraFields,
				LongHeaders:  extraFields,
			},
			wantErr: true,
			errType: ErrShortExtra,
		},
		{
			name:   "CSVBadLengthExtra",
			format: "short",
			data:   []*testStruct{&testA, &testB},
			extra:  &badExtra,
			spec: FormatSpec{
				ShortFields:  extraFields,
				ShortHeaders: extraFields,
				LongFields:   extraFields,
				LongHeaders:  extraFields,
			},
			wantErr: true,
			errType: ErrShortExtra,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dataGeneric := make([]any, 0, len(tt.data))
			for _, val := range tt.data {
				dataGeneric = append(dataGeneric, val)
			}

			out, err := Format(tt.format, dataGeneric, tt.extra, tt.spec)

			if err != nil && !tt.wantErr {
				t.Errorf("Error when not expected: %v", err)
			}

			if err == nil && tt.wantErr {
				t.Error("No Error when expected")
			}

			if tt.errType != nil && !errors.Is(err, tt.errType) {
				t.Errorf("Got error %v, expected %v", err, tt.errType)
			}

			g := goldie.New(t)
			g.Assert(t, t.Name(), []byte(out))
		})
	}
}

func TestJSONYAML(t *testing.T) {
	tests := []struct {
		name    string
		format  string
		data    []*testStruct
		extra   *map[string][]string
		wantErr bool
		errType error
	}{
		{
			name:    "JSON",
			format:  "json",
			data:    []*testStruct{&testA, &testB},
			wantErr: false,
			errType: nil,
		},
		{
			name:    "YAML",
			format:  "yaml",
			data:    []*testStruct{&testA, &testB},
			wantErr: false,
			errType: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dataGeneric := make([]any, 0, len(tt.data))
			for _, val := range tt.data {
				dataGeneric = append(dataGeneric, val)
			}

			out, err := Format(tt.format, dataGeneric, tt.extra, FormatSpec{})

			if err != nil && !tt.wantErr {
				t.Errorf("Error when not expected: %v", err)
			}

			if err == nil && tt.wantErr {
				t.Error("No Error when expected")
			}

			if tt.errType != nil && !errors.Is(err, tt.errType) {
				t.Errorf("Got error %v, expected %v", err, tt.errType)
			}

			g := goldie.New(t)
			g.Assert(t, t.Name(), []byte(out))
		})
	}
}
