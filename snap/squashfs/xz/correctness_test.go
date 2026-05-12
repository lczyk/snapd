// Copyright 2026 Canonical Ltd
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License version 3 as
// published by the Free Software Foundation.

package xz_test

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"sync"
	"testing"

	upstream "github.com/ulikunitz/xz"
	upstreamlzma "github.com/ulikunitz/xz/lzma"

	"github.com/snapcore/snapd/snap/squashfs/xz"
	forklzma "github.com/snapcore/snapd/snap/squashfs/xz/lzma"
)

// TestDifferentialDecode runs the same xz blob through both the
// upstream decoder and the inlined fork, comparing bytes byte-for-byte
// and error states for parity. Catches any silent divergence in the
// hand-inlined hot paths.
func TestDifferentialDecode(t *testing.T) {
	sizes := []int{0, 1, 255, 4096, 8 * 1024, 128 * 1024, 1024 * 1024}
	for _, n := range sizes {
		t.Run(fmt.Sprintf("size=%d", n), func(t *testing.T) {
			payload := mkPayload(n)
			blob := compressXZ(payload)

			gotFork := mustDecode(t, "fork", func(r io.Reader) (io.Reader, error) {
				return xz.NewReader(r)
			}, blob)
			gotUp := mustDecode(t, "upstream", func(r io.Reader) (io.Reader, error) {
				return upstream.NewReader(r)
			}, blob)

			if !bytes.Equal(gotFork, gotUp) {
				t.Fatalf("decoders disagree: fork=%d upstream=%d first-diff=%d",
					len(gotFork), len(gotUp), firstDiff(gotFork, gotUp))
			}
			if !bytes.Equal(gotFork, payload) {
				t.Fatalf("fork output != payload: first-diff=%d",
					firstDiff(gotFork, payload))
			}
		})
	}
}

// TestPropertiesMatrix exercises the literalCodec.Decode hot path
// (which has hand-inlined range-coder bodies) across the LC/LP/PB
// matrix. The default upstream xz writer always uses LC=3 LP=0 PB=2,
// so this is the only way to make sure the inlined paths handle
// non-default property settings without breakage.
//
// Uses the lzma alone-format because the upstream xz writer doesn't
// expose per-block Properties.
func TestPropertiesMatrix(t *testing.T) {
	// Standard valid ranges per the LZMA spec: LC+LP <= 4; PB in [0,4].
	for lc := 0; lc <= 4; lc++ {
		for lp := 0; lp <= 4-lc; lp++ {
			for pb := 0; pb <= 4; pb++ {
				lc, lp, pb := lc, lp, pb
				t.Run(fmt.Sprintf("lc=%d/lp=%d/pb=%d", lc, lp, pb), func(t *testing.T) {
					payload := mkPayload(16 * 1024)
					blob := compressLZMA(t, payload, upstreamlzma.Properties{LC: lc, LP: lp, PB: pb})

					r, err := forklzma.NewReader(bytes.NewReader(blob))
					if err != nil {
						t.Fatalf("fork lzma.NewReader: %v", err)
					}
					got, err := io.ReadAll(r)
					if err != nil {
						t.Fatalf("fork ReadAll: %v", err)
					}
					if !bytes.Equal(got, payload) {
						t.Fatalf("decode mismatch: first-diff=%d",
							firstDiff(got, payload))
					}
				})
			}
		}
	}
}

// TestLzmaAloneRoundtrip covers the legacy alone-format path that
// keeps the io.ByteReader fallback in updateCodeSlow alive. This path
// is what squashfs compression code 2 (lzma) uses, so its correctness
// matters even though it isn't on the xz hot path.
func TestLzmaAloneRoundtrip(t *testing.T) {
	// size=0 is covered: a workaround in decompress() handles the
	// malformed empty stream upstream's writer produces in that
	// case. See decoder.go.
	sizes := []int{0, 1, 4096, 64 * 1024, 256 * 1024}
	for _, n := range sizes {
		t.Run(fmt.Sprintf("size=%d", n), func(t *testing.T) {
			payload := mkPayload(n)
			blob := compressLZMA(t, payload, upstreamlzma.Properties{LC: 3, LP: 0, PB: 2})

			r, err := forklzma.NewReader(bytes.NewReader(blob))
			if err != nil {
				t.Fatalf("fork lzma.NewReader: %v", err)
			}
			got, err := io.ReadAll(r)
			if err != nil {
				t.Fatalf("ReadAll: %v", err)
			}
			if !bytes.Equal(got, payload) {
				t.Fatalf("decode mismatch: first-diff=%d",
					firstDiff(got, payload))
			}
		})
	}
}

// TestConcatenatedXZStreams checks that the decoder handles multiple
// concatenated xz streams, which is legal per the xz format spec
// (`cat a.xz b.xz > both.xz`).
func TestConcatenatedXZStreams(t *testing.T) {
	a := mkPayload(8192)
	b := mkPayload(8192)
	blob := append(compressXZ(a), compressXZ(b)...)

	r, err := xz.NewReader(bytes.NewReader(blob))
	if err != nil {
		t.Fatalf("NewReader: %v", err)
	}
	got, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	want := append(append([]byte{}, a...), b...)
	if !bytes.Equal(got, want) {
		t.Fatalf("concat mismatch: got %d want %d first-diff=%d",
			len(got), len(want), firstDiff(got, want))
	}
}

// TestConcurrentDecode runs many roundtrip decodes from multiple
// goroutines against the shared per-capacity pools. Run with -race
// to surface any data race in the LIFO mutex paths or the Reader2
// recycling.
func TestConcurrentDecode(t *testing.T) {
	const goroutines = 16
	const iters = 8

	payload := mkPayload(16 * 1024)
	blob := compressXZ(payload)

	var wg sync.WaitGroup
	errs := make(chan error, goroutines*iters)
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < iters; j++ {
				r, err := xz.NewReader(bytes.NewReader(blob))
				if err != nil {
					errs <- fmt.Errorf("NewReader: %w", err)
					return
				}
				got, err := io.ReadAll(r)
				if err != nil {
					errs <- fmt.Errorf("ReadAll: %w", err)
					return
				}
				if !bytes.Equal(got, payload) {
					errs <- errors.New("decode mismatch under concurrency")
					return
				}
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
}

// mustDecode runs the given decoder constructor on the blob and
// returns the decoded bytes. Failure to construct or read aborts the
// test.
func mustDecode(t *testing.T, who string, ctor func(io.Reader) (io.Reader, error), blob []byte) []byte {
	t.Helper()
	r, err := ctor(bytes.NewReader(blob))
	if err != nil {
		t.Fatalf("%s NewReader: %v", who, err)
	}
	got, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("%s ReadAll: %v", who, err)
	}
	return got
}

// compressLZMA encodes payload as a legacy lzma-alone stream with the
// given Properties using the upstream encoder.
func compressLZMA(t *testing.T, payload []byte, p upstreamlzma.Properties) []byte {
	t.Helper()
	var buf bytes.Buffer
	cfg := upstreamlzma.WriterConfig{Properties: &p, SizeInHeader: true, Size: int64(len(payload))}
	w, err := cfg.NewWriter(&buf)
	if err != nil {
		t.Fatalf("upstream lzma.NewWriter: %v", err)
	}
	if _, err := w.Write(payload); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	return buf.Bytes()
}
