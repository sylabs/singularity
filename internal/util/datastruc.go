// Copyright (c) 2023, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package util

import (
	"github.com/samber/lo"
)

// hashingListSubtract is a utility-function for subtracting a list from another list, using map's internal hashing function to do this more efficiently than lo.Difference or lo.Without (which only assume comparable, and thus run in quadratic time).
func HashingListSubtract[T comparable](toSubstractFrom []T, toSubstract []T) []T {
	subtractionMap := lo.FromEntries(lo.Map(toSubstractFrom, func(item T, _ int) lo.Entry[T, bool] {
		return lo.Entry[T, bool]{Key: item, Value: true}
	}))

	return lo.Keys(lo.OmitByKeys(subtractionMap, toSubstract))
}
