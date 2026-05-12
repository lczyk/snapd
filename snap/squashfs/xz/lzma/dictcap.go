// Copyright 2014-2022 Ulrich Kunitz. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package lzma

// MinDictCap and MaxDictCap provide the range of supported dictionary
// capacities.
const (
	MinDictCap = 1 << 12
	MaxDictCap = 1<<32 - 1
)
