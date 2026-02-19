// Copyright (c) 2021-2026, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package slice

import (
	"slices"

	"github.com/samber/lo"
)

// ContainsString returns true if string slice s contains match
// Deprecated: use Go slices package.
func ContainsString(s []string, match string) bool {
	return slices.Contains(s, match)
}

// ContainsAnyString returns true if string slice s contains any of matches
func ContainsAnyString(s []string, matches []string) bool {
	for _, m := range matches {
		if slices.Contains(s, m) {
			return true
		}
	}
	return false
}

// ContainsInt returns true if int slice s contains match
// Deprecated: use Go slices package.
func ContainsInt(s []int, match int) bool {
	return slices.Contains(s, match)
}

// Subtract removes items in slice b from slice a, returning the result.
// Implemented using a map for greater efficiency than lo.Difference / lo.Without, when operating on large slices.
func Subtract[T comparable](a []T, b []T) []T {
	subtractionMap := lo.FromEntries(lo.Map(a, func(item T, _ int) lo.Entry[T, bool] {
		return lo.Entry[T, bool]{Key: item, Value: true}
	}))
	subtractionMap = lo.OmitByKeys(subtractionMap, b)

	return lo.Filter(a, func(x T, _ int) bool {
		_, ok := subtractionMap[x]
		return ok
	})
}
