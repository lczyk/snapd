// Copyright 2026 Canonical Ltd
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License version 3 as
// published by the Free Software Foundation.

package xz_test

import (
	"bytes"
	"errors"
	"io"
	"testing"

	upstream "github.com/ulikunitz/xz"

	"github.com/snapcore/snapd/snap/squashfs/xz"
)

// FuzzRoundtrip is a differential fuzz: encode the fuzz-supplied
// payload with the upstream writer, decode with the inlined fork,
// byte-compare. catches any regression in the hand-inlined range
// coder / tree codec / literal codec hot paths.
//
// Run with:
//
//	go test -run='^$' -fuzz=FuzzRoundtrip -fuzztime=30s ./snap/squashfs/xz/
func FuzzRoundtrip(f *testing.F) {
	seeds := [][]byte{
		nil,
		{},
		{0},
		{0xff},
		bytes.Repeat([]byte{0xaa}, 1024),
		mkPayload(4096),
		mkPayload(64 * 1024),
	}
	for _, s := range seeds {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, payload []byte) {
		var buf bytes.Buffer
		w, err := upstream.NewWriter(&buf)
		if err != nil {
			t.Fatalf("upstream NewWriter: %v", err)
		}
		if _, err := w.Write(payload); err != nil {
			t.Fatalf("upstream Write: %v", err)
		}
		if err := w.Close(); err != nil {
			t.Fatalf("upstream Close: %v", err)
		}

		r, err := xz.NewReader(bytes.NewReader(buf.Bytes()))
		if err != nil {
			t.Fatalf("fork NewReader: %v", err)
		}
		got, err := io.ReadAll(r)
		if err != nil {
			t.Fatalf("fork ReadAll: %v", err)
		}
		if !bytes.Equal(got, payload) {
			t.Fatalf("decode mismatch: got %d bytes, want %d; first diff at %d",
				len(got), len(payload), firstDiff(got, payload))
		}
	})
}

// FuzzDecodeArbitrary feeds arbitrary bytes to the decoder. It is
// allowed to return an error; it must not panic, must not hang, and
// must not produce more output than a sane bound on what an attacker
// could ask for.
//
// Run with:
//
//	go test -run='^$' -fuzz=FuzzDecodeArbitrary -fuzztime=30s ./snap/squashfs/xz/
// xzMagic is the 6-byte xz stream header magic. Splicing it into
// every fuzz input lets the mutator explore the post-header parser
// path (block headers, filter chain, lzma2 chunks) without the gate
// rejecting most random bytes outright.
var xzMagic = []byte{0xfd, '7', 'z', 'X', 'Z', 0x00}

func FuzzDecodeArbitrary(f *testing.F) {
	// Diverse seeds so the mutator has multiple starting points
	// past the magic gate.
	f.Add(compressXZ(mkPayload(4096)))
	f.Add(compressXZ(mkPayload(64 * 1024)))
	f.Add(compressXZ(bytes.Repeat([]byte{0xaa}, 1024)))
	f.Add([]byte("not an xz stream"))
	f.Add([]byte{})
	f.Add(xzMagic) // valid magic, truncated

	const maxOutput = 16 << 20 // 16 MiB

	f.Fuzz(func(t *testing.T, blob []byte) {
		// Splice xz magic at offset 0 so the fuzzer can mutate
		// the rest without falling off the header check. Copy
		// first to avoid mutating the corpus entry.
		if len(blob) >= len(xzMagic) {
			blob = append([]byte(nil), blob...)
			copy(blob[:len(xzMagic)], xzMagic)
		}

		r, err := xz.NewReader(bytes.NewReader(blob))
		if err != nil {
			// rejecting at construction is fine.
			return
		}
		_, err = io.Copy(io.Discard, &boundedReader{r: r, max: maxOutput})
		if err != nil && !errors.Is(err, errBoundExceeded) {
			// any other error is acceptable -- bad bytes in,
			// error out.
			return
		}
		if errors.Is(err, errBoundExceeded) {
			t.Fatalf("decoder produced more than %d bytes from %d-byte input -- decompression bomb?",
				maxOutput, len(blob))
		}
	})
}

var errBoundExceeded = errors.New("output bound exceeded")

// boundedReader wraps a reader and returns errBoundExceeded once max
// bytes have been read. Lets fuzz detect runaway output without
// allocating an unbounded buffer.
type boundedReader struct {
	r   io.Reader
	max int64
	n   int64
}

func (br *boundedReader) Read(p []byte) (int, error) {
	if br.n >= br.max {
		return 0, errBoundExceeded
	}
	if int64(len(p)) > br.max-br.n {
		p = p[:br.max-br.n]
	}
	n, err := br.r.Read(p)
	br.n += int64(n)
	return n, err
}
