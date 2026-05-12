// Copyright 2014-2022 Ulrich Kunitz. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package lzma

// op represents one decode-side operation: either a literal byte or a
// dictionary match. Originally upstream had an `operation` interface
// with `lit` / `match` concrete types, which boxed every op into an
// interface value on the hot path -- one heap alloc per LZMA op,
// dominating decode allocs. Replacing it with a plain struct value
// eliminates that allocation entirely.
type op struct {
	isLit    bool
	b        byte  // valid if isLit
	distance int64 // valid if !isLit
	n        int   // match length (or 1 for literal)
}
