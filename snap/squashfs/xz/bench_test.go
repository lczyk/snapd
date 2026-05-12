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
	"sync"
	"testing"

	upstream "github.com/ulikunitz/xz"

	"github.com/snapcore/snapd/snap/squashfs/xz"
)

var (
	fixtureOnce sync.Once
	fixture8k   []byte
	fixture128k []byte
	fixture1m   []byte

	// the plaintext payloads behind each fixture, retained so a
	// companion test (TestBenchFixturesRoundtrip) can verify the
	// benchmarks aren't silently measuring a decoder that produces
	// wrong output. benchmarks themselves discard the decoded
	// bytes for timing reasons; this is the validating shadow.
	payload8k   []byte
	payload128k []byte
	payload1m   []byte
)

func makeFixtures() {
	fixtureOnce.Do(func() {
		payload8k = randomPayload(8 * 1024)
		payload128k = randomPayload(128 * 1024)
		payload1m = randomPayload(1024 * 1024)
		fixture8k = compressXZ(payload8k)
		fixture128k = compressXZ(payload128k)
		fixture1m = compressXZ(payload1m)
	})
}

func randomPayload(n int) []byte {
	b := make([]byte, n)
	rand.Read(b)
	// inject runs to give xz something to compress
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

func compressXZ(data []byte) []byte {
	var buf bytes.Buffer
	w, err := upstream.NewWriter(&buf)
	if err != nil {
		panic(err)
	}
	if _, err := w.Write(data); err != nil {
		panic(err)
	}
	if err := w.Close(); err != nil {
		panic(err)
	}
	return buf.Bytes()
}

func benchDecode(b *testing.B, blob []byte) {
	b.ReportAllocs()
	b.SetBytes(int64(len(blob)))
	b.ResetTimer()
	for b.Loop() {
		r, err := xz.NewReader(bytes.NewReader(blob))
		if err != nil {
			b.Fatal(err)
		}
		if _, err := io.Copy(io.Discard, r); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkDecode8k(b *testing.B)   { makeFixtures(); benchDecode(b, fixture8k) }
func BenchmarkDecode128k(b *testing.B) { makeFixtures(); benchDecode(b, fixture128k) }
func BenchmarkDecode1M(b *testing.B)   { makeFixtures(); benchDecode(b, fixture1m) }

// BenchmarkNewReader isolates the xz.NewReader cost (header parse +
// dictionary alloc) from the actual decode work.
func BenchmarkNewReader(b *testing.B) {
	makeFixtures()
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_, err := xz.NewReader(bytes.NewReader(fixture8k))
		if err != nil {
			b.Fatal(err)
		}
	}
}
