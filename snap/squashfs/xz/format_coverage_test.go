// Copyright 2026 Canonical Ltd
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License version 3 as
// published by the Free Software Foundation.

package xz_test

import (
	"bytes"
	"fmt"
	"io"
	"testing"

	upstream "github.com/ulikunitz/xz"

	"github.com/snapcore/snapd/snap/squashfs/xz"
)

// TestCheckTypeMatrix decodes xz streams written with each of the
// four spec-defined integrity check types. our fork's
// none-check.go + crc.go handle these, but the upstream default
// writer only ever picks CRC64 -- so without an explicit cover
// here the CRC32 / SHA-256 / None paths see no exercise.
func TestCheckTypeMatrix(t *testing.T) {
	cases := []struct {
		name string
		code byte
	}{
		{"None", upstream.None},
		{"CRC32", upstream.CRC32},
		{"CRC64", upstream.CRC64},
		{"SHA256", upstream.SHA256},
	}
	payload := mkPayload(16 * 1024)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			blob := compressXZWithCheck(t, payload, tc.code)

			r, err := xz.NewReader(bytes.NewReader(blob))
			if err != nil {
				t.Fatalf("NewReader: %v", err)
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

// TestMultiBlockSingleStream decodes an xz stream that's been
// split into multiple blocks via the writer's BlockSize knob.
// upstream's default writer emits one block per stream, so the
// streamReader's block-to-block loop is unexercised without an
// explicit cover.
func TestMultiBlockSingleStream(t *testing.T) {
	// 256 KiB payload, 16 KiB blocks -> 16 blocks.
	payload := mkPayload(256 * 1024)
	blob := compressXZWithBlockSize(t, payload, 16*1024)

	r, err := xz.NewReader(bytes.NewReader(blob))
	if err != nil {
		t.Fatalf("NewReader: %v", err)
	}
	got, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("decode mismatch: first-diff=%d (got %d bytes, want %d)",
			firstDiff(got, payload), len(got), len(payload))
	}
}

// TestRejectsCorruptedBlockChecksum confirms the decoder rejects a
// stream whose block-integrity check byte has been flipped after
// encode. catches any regression in the hash inline-write path
// (used to be io.TeeReader; now inline in blockReader.Read).
func TestRejectsCorruptedBlockChecksum(t *testing.T) {
	payload := mkPayload(8192)
	blob := compressXZ(payload)

	// Flip a byte near the end of the stream -- the integrity
	// check sits between the block payload and the index. Flipping
	// bytes there should produce a checksum error.
	if len(blob) < 32 {
		t.Fatalf("blob too short to corrupt safely")
	}
	corrupted := append([]byte(nil), blob...)
	// Aim for the block-check region: roughly 12 bytes before the
	// 12-byte stream footer. Pick a spot that's reliably past the
	// LZMA2 payload for our fixture size.
	corrupted[len(corrupted)-16] ^= 0xff

	r, err := xz.NewReader(bytes.NewReader(corrupted))
	if err != nil {
		// rejecting at construction is acceptable.
		return
	}
	_, err = io.ReadAll(r)
	if err == nil {
		t.Fatal("expected error decoding corrupted block, got nil")
	}
}

// compressXZWithCheck encodes payload as xz with the given integrity
// check type. Uses the upstream encoder since our fork is decode-
// only.
func compressXZWithCheck(t *testing.T, payload []byte, check byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	cfg := upstream.WriterConfig{CheckSum: check}
	if check == upstream.None {
		cfg.NoCheckSum = true
	}
	w, err := cfg.NewWriter(&buf)
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}
	if _, err := w.Write(payload); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	return buf.Bytes()
}

// compressXZWithBlockSize encodes payload with an explicit
// BlockSize so the resulting stream carries multiple blocks.
func compressXZWithBlockSize(t *testing.T, payload []byte, blockSize int64) []byte {
	t.Helper()
	var buf bytes.Buffer
	cfg := upstream.WriterConfig{BlockSize: blockSize}
	w, err := cfg.NewWriter(&buf)
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}
	// Write in small chunks so the writer actually splits into
	// blocks; a single big Write may coalesce.
	for off := 0; off < len(payload); off += 4096 {
		end := off + 4096
		if end > len(payload) {
			end = len(payload)
		}
		if _, err := w.Write(payload[off:end]); err != nil {
			t.Fatalf("write at %d: %v", off, err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	return buf.Bytes()
}

// quiet linter about fmt import in case future edits remove the
// only fmt user above.
var _ = fmt.Sprintf
