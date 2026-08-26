// Copyright (c) 2021-2026, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package output

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/sylabs/singularity/v4/enterprise-plugin/internal/pkg/api"
	"github.com/sylabs/singularity/v4/enterprise-plugin/internal/pkg/user"
)

// TransformFunc are used to convert a string into a different representation for display.
// Service clients are provided so transformations can be performed that involve API calls.
type TransformFunc func(in string, clients *api.Clients) string

// TransformMap is a map from field name to a TransformFunc.
type TransformMap map[string]TransformFunc

// UpperCaseTransform will transform a string to upper-case.
func UpperCaseTransform(in string, _ *api.Clients) string {
	return strings.ToUpper(in)
}

// UsernameTransform will transform a bson user ID to a username for display.
func UsernameTransform(id string, clients *api.Clients) string {
	return user.IDToUsername(id, clients)
}

// Truncate50Transform will truncate a string to 50 characters.
func Truncate50Transform(s string, _ *api.Clients) string {
	if len(s) > 50 {
		return s[0:46] + "..."
	}
	return s
}

// HumanByteTransform will format a number of bytes into K/M/G/TiB.
func HumanByteTransform(s string, _ *api.Clients) string {
	b, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return s
	}

	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%5.1d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%5.1f %ciB",
		float64(b)/float64(div), "KMGTPE"[exp])
}

// TimezoneTransform will reformat a date/time into the current timezone.
func TimezoneTransform(s string, _ *api.Clients) string {
	t, err := time.Parse(time.RFC3339, s)
	// Can't parse? Show as-is.
	if err != nil {
		return s
	}
	// Nil value? Empty string.
	if t.IsZero() {
		return ""
	}
	// Local time in default string format.
	return t.Local().String()
}
