// Copyright 2014-2022 Ulrich Kunitz. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package xz

import "errors"

// verifyFilters checks the filter list for the length and the right
// sequence of filters.
func verifyFilters(f []filter) error {
	if len(f) == 0 {
		return errors.New("xz: no filters")
	}
	if len(f) > 4 {
		return errors.New("xz: more than four filters")
	}
	for _, g := range f[:len(f)-1] {
		if g.last() {
			return errors.New("xz: last filter is not last")
		}
	}
	if !f[len(f)-1].last() {
		return errors.New("xz: wrong last filter")
	}
	return nil
}
