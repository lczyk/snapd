// Copyright 2026 Canonical Ltd
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License version 3 as
// published by the Free Software Foundation.

package xz_test

import (
	"bytes"
	"crypto/rand"
	"io"
	"testing"

	upstream "github.com/ulikunitz/xz"

	"github.com/snapcore/snapd/snap/squashfs/xz"
)

// TestRoundtrip encodes payloads of various sizes with the upstream
// xz writer, decodes them with the inlined fork, and verifies the
// decoded bytes match the original. Catches correctness regressions
// in the decoder hot path.
func TestRoundtrip(t *testing.T) {
	sizes := []int{
		0,
		1,
		255,
		4096,
		8 * 1024,
		128 * 1024,
		1024 * 1024,
	}
	for _, n := range sizes {
		t.Run("size", func(t *testing.T) {
			payload := mkPayload(n)
			blob := compressXZ(payload)

			r, err := xz.NewReader(bytes.NewReader(blob))
			if err != nil {
				t.Fatalf("NewReader: %v", err)
			}
			got, err := io.ReadAll(r)
			if err != nil {
				t.Fatalf("ReadAll: %v", err)
			}
			if !bytes.Equal(got, payload) {
				t.Fatalf("size=%d decode mismatch: got %d bytes, want %d bytes; first diff at byte %d",
					n, len(got), len(payload), firstDiff(got, payload))
			}
		})
	}
}

// mkPayload returns a payload that compresses non-trivially: random
// noise with short runs sprinkled in, matching the shape of bench
// fixtures.
func mkPayload(n int) []byte {
	b := make([]byte, n)
	rand.Read(b)
	for i := 0; i < n; i += 64 {
		end := i + 16
		if end > n {
			end = n
		}
		for j := i; j < end; j++ {
			b[j] = byte(i)
		}
	}
	return b
}

func firstDiff(a, b []byte) int {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	for i := 0; i < n; i++ {
		if a[i] != b[i] {
			return i
		}
	}
	return n
}

// pull upstream reference in to mirror compressXZ when bench_test
// isn't compiled into the test binary.
var _ = upstream.NewWriter
