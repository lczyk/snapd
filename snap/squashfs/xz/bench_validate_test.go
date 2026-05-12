// Copyright 2026 Canonical Ltd
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License version 3 as
// published by the Free Software Foundation.

package xz_test

import (
	"bytes"
	"io"
	"testing"

	"github.com/snapcore/snapd/snap/squashfs/xz"
)

// TestBenchFixturesRoundtrip decodes each bench fixture and
// compares the result against the retained plaintext payload.
//
// IMPORTANT: the BenchmarkDecode* benches in bench_test.go discard
// the decoded bytes (io.Copy to io.Discard) for timing reasons --
// they would not catch a regression that produced wrong output at
// full throughput. this test is the validating shadow for those
// benches, run as part of the normal test suite.
func TestBenchFixturesRoundtrip(t *testing.T) {
	makeFixtures()
	cases := []struct {
		name string
		blob []byte
		want []byte
	}{
		{"8k", fixture8k, payload8k},
		{"128k", fixture128k, payload128k},
		{"1m", fixture1m, payload1m},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r, err := xz.NewReader(bytes.NewReader(tc.blob))
			if err != nil {
				t.Fatalf("NewReader: %v", err)
			}
			got, err := io.ReadAll(r)
			if err != nil {
				t.Fatalf("ReadAll: %v", err)
			}
			if !bytes.Equal(got, tc.want) {
				t.Fatalf("decode mismatch: got %d bytes, want %d; first diff at %d",
					len(got), len(tc.want), firstDiff(got, tc.want))
			}
		})
	}
}
