// Copyright (c) 2021-2026, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package output

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"text/tabwriter"

	"github.com/sylabs/singularity/v4/enterprise-plugin/internal/pkg/api"
	"gopkg.in/yaml.v3"
)

var (
	// Formats is the list of supported output formats.
	Formats = []string{"json", "yaml", "csv", "long", "short"}
	// ErrZeroValueOf is returned if when taking the reflect ValueOf an item we get a zero Value.
	ErrZeroValueOf = errors.New("ValueOf is zero")
	// ErrNotAStruct is returned when we expect a struct, but find something else.
	ErrNotAStruct = errors.New("not a struct")
	// ErrFieldNotFound is returned if we cannot find the requested field on a reflect.Value.
	ErrFieldNotFound = errors.New("field not found")
	// ErrPanic is returned if an unhandled panic occurs - this should not happen.
	ErrPanic = errors.New("unhandled panic")
	// ErrShortExtra is returned if not enough values are given for an extra field.
	ErrShortExtra = errors.New("not enough values for extra field")
	// zeroVal is for convenience to test for the Zero Value.
	zeroVal reflect.Value
)

// FormatSpec defines the way in which output will be displayed.
type FormatSpec struct {
	// ShortFields indicates which fields are to be included in a short output format.
	ShortFields []string
	// ShortHeaders provides header text per entry in ShortFields.
	ShortHeaders []string
	// LongFields indicates which fields are to be included in a long output format.
	LongFields []string
	// LongFields provides header text per entry in LongFields.
	LongHeaders []string
	// Transforms can be used to specifies an optional function per field that will transform output values.
	// Only applies to CSV and TSV output. JSON & YAML output is raw, and is not transformed.
	Transforms map[string]TransformFunc
	// Clients provides service clients for transformations that may need API access.
	Clients *api.Clients
}

// Format returns a string containing output in the requested format.
// data is a generic interface{} representation of the data retrieved from the API.
// extra is a map of computed values that can be used to add an extra field to TSV/CSV output.
// spec specifies fields to display, and transformations to apply on fields.
// Note: json and yaml output are raw dumps from the API, ignoring extra and spec.
func Format(format string, data []any, extra *map[string][]string, spec FormatSpec) (string, error) {
	switch format {
	case "json":
		return formatJSON(data)
	case "yaml":
		return formatYAML(data)
	case "csv":
		return formatCSV(data, extra, spec.LongFields, spec.LongHeaders, spec.Transforms, spec.Clients)
	case "long":
		return formatTSV(data, extra, spec.LongFields, spec.LongHeaders, spec.Transforms, spec.Clients)
	default:
		// Standard interactive output is TSV but restricted fields
		return formatTSV(data, extra, spec.ShortFields, spec.ShortHeaders, spec.Transforms, spec.Clients)
	}
}

// formatTSV returns a string containing a tab delimited table for supplied data.
// data is expected to be a slice of structs.
// extra is a map of slices of computed values that can be used to add an extra field.
// fields is a slice specifying the name and order of fields to be included.
// headers is a slice specifying the column header for the specified fields.
func formatTSV(data []any, extra *map[string][]string, fields, headers []string, transforms TransformMap, clients *api.Clients) (out string, err error) {
	// We should never hit this as all panic points should be covered,
	// but catch them since we are doing complex reflection work.
	defer func() {
		if r := recover(); r != nil {
			out = ""
			err = fmt.Errorf("%w: %v", ErrPanic, r)
		}
	}()

	// Check extra fields have values for all indices we will use
	if extra != nil {
		for fname, fslice := range *extra {
			if len(fslice) < len(data) {
				return "", fmt.Errorf("%s: %w", fname, ErrShortExtra)
			}
		}
	}

	b := bytes.NewBuffer(nil)
	tw := tabwriter.NewWriter(b, 0, 4, 1, ' ', 0)

	fmt.Fprint(tw, strings.Join(headers, "\t"))
	fmt.Fprint(tw, "\n")

	for idx, d := range data {
		val := reflect.ValueOf(d)
		// If we expected a struct, but got a zero Value, bail out
		if val == zeroVal {
			return "", fmt.Errorf("item %d: %w", idx, ErrZeroValueOf)
		}
		if val.Kind() == reflect.Pointer || val.Kind() == reflect.UnsafePointer {
			val = val.Elem()
		}
		if val.Kind() != reflect.Struct {
			return "", fmt.Errorf("item %d :%w", idx, ErrNotAStruct)
		}
		for _, fname := range fields {
			// Handle it if it's an extra (computed) field
			if extra != nil {
				if _, ok := (*extra)[fname]; ok {
					fmt.Fprintf(tw, "%v\t", (*extra)[fname][idx])
					continue
				}
			}
			// Now look at the API data
			// If the field was not found
			fval := val.FieldByName(fname)
			if fval == zeroVal {
				return "", fmt.Errorf("%s: %w", fname, ErrFieldNotFound)
			}
			if fval.Kind() == reflect.Pointer || fval.Kind() == reflect.UnsafePointer {
				fval = fval.Elem()
			}
			// We have the field we want from the struct
			val := fmt.Sprintf("%v", fval.Interface())
			if t, ok := transforms[fname]; ok {
				val = t(val, clients)
			}
			fmt.Fprintf(tw, "%v\t", val)
		}
		fmt.Fprint(tw, "\n")
	}
	tw.Flush()
	return b.String(), nil
}

// formatCSV returns a string containing a tab delimited table for supplied data.
// data is expected to be a slice of structs.
// extra is a map of slices of computed values that can be used to add an extra field
// fields is a slice specifying the name and order of fields to be included.
// headers is a slice specifying the column header for the specified fields.
func formatCSV(data []any, extra *map[string][]string, fields, headers []string, transforms TransformMap, clients *api.Clients) (out string, err error) {
	// We should never hit this as all panic points should be covered,
	// but catch them since we are doing complex reflection work.
	defer func() {
		if r := recover(); r != nil {
			out = ""
			err = fmt.Errorf("%w: %v", ErrPanic, r)
		}
	}()

	// Check extra fields have values for all indices we will use
	if extra != nil {
		for fname, fslice := range *extra {
			if len(fslice) < len(data) {
				return "", fmt.Errorf("%s: %w", fname, ErrShortExtra)
			}
		}
	}

	b := bytes.NewBuffer(nil)
	cw := csv.NewWriter(b)

	if err = cw.Write(headers); err != nil {
		return "", err
	}

	for idx, d := range data {
		line := []string{}
		val := reflect.ValueOf(d)
		// If we expected a struct, but got a zero Value, bail out
		if val == zeroVal {
			return "", fmt.Errorf("item %d: %w", idx, ErrZeroValueOf)
		}
		if val.Kind() == reflect.Pointer {
			val = val.Elem()
		}
		if val.Kind() != reflect.Struct {
			return "", fmt.Errorf("item %d :%w", idx, ErrNotAStruct)
		}
		for _, fname := range fields {
			// Handle it if it's an extra (computed) field
			if extra != nil {
				if _, ok := (*extra)[fname]; ok {
					val := fmt.Sprintf("%v", (*extra)[fname][idx])
					line = append(line, val)
					continue
				}
			}
			// If the field was not found
			fval := val.FieldByName(fname)
			if fval == zeroVal {
				return "", fmt.Errorf("%s: %w", fname, ErrFieldNotFound)
			}
			if fval.Kind() == reflect.Pointer || fval.Kind() == reflect.UnsafePointer {
				fval = fval.Elem()
			}
			// We have the field we want from the struct
			val := fmt.Sprintf("%v", fval.Interface())
			if t, ok := transforms[fname]; ok {
				val = t(val, clients)
			}
			line = append(line, val)
		}
		if err = cw.Write(line); err != nil {
			return "", err
		}
	}
	cw.Flush()
	return b.String(), nil
}

// formatJSON returns pretty printed JSON for supplied data.
func formatJSON(data []any) (string, error) {
	out, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// formatYAML returns pretty printed JSON for supplied data.
func formatYAML(data []any) (string, error) {
	out, err := yaml.Marshal(data)
	if err != nil {
		return "", err
	}
	return string(out), nil
}
