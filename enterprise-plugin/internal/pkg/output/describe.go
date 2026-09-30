// This is a partial, and modified version of:
//    https://github.com/kubernetes/kubectl/blob/master/pkg/describe/describe.go
//    https://github.com/kubernetes/kubectl/blob/master/pkg/util/slice/slice.go
//

/*
Copyright 2014 The Kubernetes Authors.
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
    http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package output

import (
	"bytes"
	"fmt"
	"io"
	"reflect"
	"sort"
	"strings"
	"unicode"

	"github.com/sylabs/singularity/v4/enterprise-plugin/pkg/library/models"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"

	"github.com/fatih/camelcase"
	"github.com/fatih/structs"
)

type flusher interface {
	Flush()
}

// PrefixWriter can write text at various indentation levels.
type PrefixWriter interface {
	// Write writes text with the specified indentation level.
	Writef(level int, format string, a ...any)
	// WriteLine writes an entire line with no indentation level.
	WriteLine(a ...any)
	// Flush forces indentation to be reset.
	Flush()
}

// prefixWriter implements PrefixWriter.
type prefixWriter struct {
	out io.Writer
}

var _ PrefixWriter = &prefixWriter{}

// NewPrefixWriter creates a new PrefixWriter.
func NewPrefixWriter(out io.Writer) PrefixWriter {
	return &prefixWriter{out: out}
}

func (pw *prefixWriter) Writef(level int, format string, a ...any) {
	levelSpace := "  "
	var prefix strings.Builder
	for range level {
		prefix.WriteString(levelSpace)
	}
	fmt.Fprintf(pw.out, prefix.String()+format, a...)
}

func (pw *prefixWriter) WriteLine(a ...any) {
	fmt.Fprintln(pw.out, a...)
}

func (pw *prefixWriter) Flush() {
	if f, ok := pw.out.(flusher); ok {
		f.Flush()
	}
}

// nestedPrefixWriter implements PrefixWriter by increasing the level
// before passing text on to some other writer.
type nestedPrefixWriter struct {
	PrefixWriter
	indent int
}

var _ PrefixWriter = &prefixWriter{}

// NewPrefixWriter creates a new PrefixWriter.
func NewNestedPrefixWriter(out PrefixWriter, indent int) PrefixWriter {
	return &nestedPrefixWriter{PrefixWriter: out, indent: indent}
}

func (npw *nestedPrefixWriter) Writef(level int, format string, a ...any) {
	npw.PrefixWriter.Writef(level+npw.indent, format, a...)
}

func (npw *nestedPrefixWriter) WriteLine(a ...any) {
	npw.PrefixWriter.Writef(npw.indent, "%s", fmt.Sprintln(a...))
}

func smartLabelFor(field string) string {
	// skip creating smart label if field name contains
	// special characters other than '-'
	if strings.IndexFunc(field, func(r rune) bool {
		return !unicode.IsLetter(r) && r != '-'
	}) != -1 {
		return field
	}

	commonAcronyms := []string{"API", "URL", "UID", "OSB", "GUID"}
	parts := camelcase.Split(field)
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if part == "_" {
			continue
		}

		if containsString(commonAcronyms, strings.ToUpper(part), nil) {
			part = strings.ToUpper(part)
		} else {
			caser := cases.Title(language.English)
			part = caser.String(part)
		}
		result = append(result, part)
	}

	return strings.Join(result, " ")
}

func printUnstructuredContent(w PrefixWriter, level int, content map[string]any, skipPrefix string, skip ...string) {
	fields := make([]string, 0, len(content))
	for field := range content {
		fields = append(fields, field)
	}
	sort.Strings(fields)

	for _, field := range fields {
		value := content[field]
		switch typedValue := value.(type) {
		// TagMap is a map[string]string and we want to display the tags sorted
		case models.TagMap:
			w.Writef(level, "%s:\n", smartLabelFor(field))
			tags := make([]string, 0, len(typedValue))
			for tag := range typedValue {
				tags = append(tags, tag)
			}
			sort.Strings(tags)
			for _, tag := range tags {
				w.Writef(level+1, "%s:\t%v\n", tag, typedValue[tag])
			}
		// ArchTagMap is a 2-level map[string]map[string]string and we want to display the tags sorted
		case models.ArchTagMap:
			w.Writef(level, "%s:\n", smartLabelFor(field))
			for archKey, archVal := range typedValue {
				tags := make([]string, 0, len(typedValue))
				for tag := range archVal {
					tags = append(tags, tag)
				}
				sort.Strings(tags)
				w.Writef(level+1, "%s:\n", archKey)
				for _, tag := range tags {
					w.Writef(level+2, "%s:\t%v\n", tag, archVal[tag])
				}
			}
		case map[string]any:
			skipExpr := fmt.Sprintf("%s.%s", skipPrefix, field)
			if containsString(skip, skipExpr, nil) {
				continue
			}
			w.Writef(level, "%s:\n", smartLabelFor(field))
			printUnstructuredContent(w, level+1, typedValue, skipExpr, skip...)

		case []any:
			skipExpr := fmt.Sprintf("%s.%s", skipPrefix, field)
			if containsString(skip, skipExpr, nil) {
				continue
			}
			w.Writef(level, "%s:\n", smartLabelFor(field))
			for _, child := range typedValue {
				switch typedChild := child.(type) {
				case map[string]any:
					printUnstructuredContent(w, level+1, typedChild, skipExpr, skip...)
				default:
					// DCT - de-reference pointer values in the go-swagger structs
					val := reflect.ValueOf(typedChild)
					if val.Kind() == reflect.Pointer {
						typedChild = val.Elem()
					}
					w.Writef(level+1, "%v\n", typedChild)
				}
			}

		default:
			skipExpr := fmt.Sprintf("%s.%s", skipPrefix, field)
			if containsString(skip, skipExpr, nil) {
				continue
			}
			// DCT - de-reference pointer values in the go-swagger structs
			val := reflect.ValueOf(typedValue)
			if val.Kind() == reflect.Pointer {
				typedValue = val.Elem()
			}
			w.Writef(level, "%s:\t%v\n", smartLabelFor(field), typedValue)
		}
	}
}

// DescribeStruct returns a human readable representation of an arbitrary struct.
// A provided header is prepended, as a title to the output.
func DescribeStruct(header string, s any) (out string, err error) {
	if !structs.IsStruct(s) {
		return "", ErrNotAStruct
	}

	var b bytes.Buffer
	w := NewPrefixWriter(&b)
	w.WriteLine(header)
	printUnstructuredContent(w, 1, structs.Map(s), "")
	w.Flush()
	return b.String(), nil
}

// containsString checks if a given slice of strings contains the provided string.
// If a modifier func is provided, it is called with the slice item before the comparation.
//
//nolint:unparam
func containsString(slice []string, s string, modifier func(s string) string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
		if modifier != nil && modifier(item) == s {
			return true
		}
	}
	return false
}
