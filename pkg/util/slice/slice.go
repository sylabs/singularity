// Copyright (c) 2021, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package slice

import "github.com/samber/lo"

// ContainsString returns true if string slice s contains match
func ContainsString(s []string, match string) bool {
	for _, a := range s {
		if a == match {
			return true
		}
	}
	return false
}

// ContainsAnyString returns true if string slice s contains any of matches
func ContainsAnyString(s []string, matches []string) bool {
	for _, m := range matches {
		for _, a := range s {
			if a == m {
				return true
			}
		}
	}
	return false
}

// ContainsInt returns true if int slice s contains match
func ContainsInt(s []int, match int) bool {
	for _, a := range s {
		if a == match {
			return true
		}
	}
	return false
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
