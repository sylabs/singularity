// Copyright (c) 2018-2025 Sylabs, Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license.  Please
// consult LICENSE.md file distributed with the sources of this project regarding
// your rights to use or distribute this software.

package shell

import "strings"

// ArgsQuoted concatenates a slice of string shell args, quoting each item
func ArgsQuoted(a []string) (quoted string) {
	for _, val := range a {
		quoted = quoted + `"` + Escape(val) + `" `
	}
	quoted = strings.TrimRight(quoted, " ")
	return
}

// Escape performs escaping of shell double quotes, backticks and $ characters.
// Does not escape single quotes - apply EscapeSingleQuotes separately for this.
func Escape(s string) string {
	escaped := strings.ReplaceAll(s, `\`, `\\`)
	escaped = strings.ReplaceAll(escaped, `"`, `\"`)
	escaped = strings.ReplaceAll(escaped, "`", "\\`")
	escaped = strings.ReplaceAll(escaped, `$`, `\$`)
	return escaped
}

// EscapeQuotes performs shell escaping of double quotes only
func EscapeDoubleQuotes(s string) string {
	return strings.ReplaceAll(s, `"`, `\"`)
}

// EscapeSingleQuotes performs shell escaping of single quotes only
func EscapeSingleQuotes(s string) string {
	return strings.ReplaceAll(s, `'`, `'"'"'`)
}
