// Copyright (c) 2023, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package maps

func HasKey[K comparable, V any](m map[K]V, k K) bool {
	_, ok := m[k]

	return ok
}
