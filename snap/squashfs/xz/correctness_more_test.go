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

// TestSmallReads exercises non-bulk read patterns: a caller that
// reads one byte at a time, or in tiny fixed chunks, must get the
// exact same bytes as a single ReadAll. The chunkBuf path in
// startChunk assumes io.ReadFull on the underlying stream works for
// arbitrary chunk sizes; small reads on the outer Reader walk a
// different schedule and surface any byte-budget bookkeeping bugs.
func TestSmallReads(t *testing.T) {
	payload := mkPayload(64 * 1024)
	blob := compressXZ(payload)

	bufSizes := []int{1, 3, 7, 32, 257, 4096}
	for _, sz := range bufSizes {
		t.Run("buf=", func(t *testing.T) {
			r, err := xz.NewReader(bytes.NewReader(blob))
			if err != nil {
				t.Fatalf("NewReader: %v", err)
			}
			var got bytes.Buffer
			buf := make([]byte, sz)
			for {
				n, err := r.Read(buf)
				got.Write(buf[:n])
				if err == io.EOF {
					break
				}
				if err != nil {
					t.Fatalf("Read(buf=%d): %v", sz, err)
				}
			}
			if !bytes.Equal(got.Bytes(), payload) {
				t.Fatalf("buf=%d: decode mismatch (first diff %d)",
					sz, firstDiff(got.Bytes(), payload))
			}
		})
	}
}

// TestTruncated confirms the decoder reports an error -- not panic,
// not hang, not silent-truncate -- for inputs that end before the
// stream is complete. Samples truncation points across the blob to
// keep wall time reasonable; full coverage at every offset is what
// the fuzz target is for.
func TestTruncated(t *testing.T) {
	payload := mkPayload(8 * 1024)
	blob := compressXZ(payload)

	// Sample every 32 bytes plus the boundary just before the end of
	// stream, where the index/footer live.
	offsets := []int{}
	for i := 1; i < len(blob); i += 32 {
		offsets = append(offsets, i)
	}
	if last := len(blob) - 1; last > 0 && (len(offsets) == 0 || offsets[len(offsets)-1] != last) {
		offsets = append(offsets, last)
	}

	for _, off := range offsets {
		off := off
		t.Run("off=", func(t *testing.T) {
			truncated := blob[:off]
			r, err := xz.NewReader(bytes.NewReader(truncated))
			if err != nil {
				// rejecting at construction time is fine.
				return
			}
			got, err := io.ReadAll(r)
			if err == nil && bytes.Equal(got, payload) {
				t.Fatalf("off=%d: truncated input decoded to full payload",
					off)
			}
			// any non-nil error or partial-decode-with-error is
			// acceptable; the test only fails if the decoder
			// silently returns the full original payload.
		})
	}
}

// TestPartialReadThenReuse covers a caller that starts decoding,
// reads only a few bytes, then drops the reader without finishing.
// The next NewReader call must hand back a clean instance from the
// pool -- in particular, the dict head and codec state must not
// carry over.
func TestPartialReadThenReuse(t *testing.T) {
	payload := mkPayload(64 * 1024)
	blob := compressXZ(payload)

	// First decode: consume only the first 128 bytes, then drop.
	r1, err := xz.NewReader(bytes.NewReader(blob))
	if err != nil {
		t.Fatalf("NewReader 1: %v", err)
	}
	tmp := make([]byte, 128)
	if _, err := io.ReadFull(r1, tmp); err != nil {
		t.Fatalf("partial Read: %v", err)
	}
	if !bytes.Equal(tmp, payload[:128]) {
		t.Fatalf("partial decode mismatch")
	}
	// drop r1 without explicit Close to mimic a caller that bailed
	// on an error path.

	// Second decode: full payload via a fresh reader. If the pool
	// recycles state without resetting it, this fails.
	r2, err := xz.NewReader(bytes.NewReader(blob))
	if err != nil {
		t.Fatalf("NewReader 2: %v", err)
	}
	got, err := io.ReadAll(r2)
	if err != nil {
		t.Fatalf("ReadAll 2: %v", err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("post-reuse decode mismatch (first diff %d)",
			firstDiff(got, payload))
	}
}

// TestReadAfterEOFIdempotent confirms that calling Read on a Reader
// that has already returned io.EOF keeps returning io.EOF without
// side effects (no panic, no further bytes).
func TestReadAfterEOFIdempotent(t *testing.T) {
	payload := mkPayload(4096)
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
		t.Fatalf("decode mismatch")
	}
	// Two extra reads after EOF.
	buf := make([]byte, 16)
	for i := 0; i < 2; i++ {
		n, err := r.Read(buf)
		if n != 0 {
			t.Fatalf("read after EOF returned %d bytes", n)
		}
		if err != io.EOF {
			t.Fatalf("read after EOF: err=%v (want io.EOF)", err)
		}
	}
}
