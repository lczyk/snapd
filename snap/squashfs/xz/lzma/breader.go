// Copyright 2014-2022 Ulrich Kunitz. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package lzma

import (
	"errors"
	"io"
)

// breader provides the ReadByte function for a Reader. It doesn't read
// more data from the reader than absolutely necessary -- buffering
// past the requested byte is unsafe in contexts (like LZMA2 chunks)
// where the consumer expects byte-exact bounds. The bulk-read path
// is handled at the chunk level via sliceByteReader.
type breader struct {
	io.Reader
	p []byte
}

// ByteReader converts an io.Reader into an io.ByteReader.
func ByteReader(r io.Reader) io.ByteReader {
	br, ok := r.(io.ByteReader)
	if !ok {
		return &breader{r, make([]byte, 1)}
	}
	return br
}

// ReadByte read byte function.
func (r *breader) ReadByte() (c byte, err error) {
	n, err := r.Reader.Read(r.p)
	if n < 1 {
		if err == nil {
			err = errors.New("breader.ReadByte: no data")
		}
		return 0, err
	}
	return r.p[0], nil
}

// sliceByteReader is an io.ByteReader backed by a []byte. Used to
// feed the LZMA range decoder from a pre-loaded chunk buffer with
// zero per-byte interface or allocation overhead.
type sliceByteReader struct {
	data []byte
	pos  int
}

// ReadByte returns the next byte or io.EOF when the slice is
// exhausted.
func (r *sliceByteReader) ReadByte() (byte, error) {
	if r.pos >= len(r.data) {
		return 0, io.EOF
	}
	c := r.data[r.pos]
	r.pos++
	return c, nil
}

// reset re-targets the reader at a new (or refilled) buffer.
func (r *sliceByteReader) reset(data []byte) {
	r.data = data
	r.pos = 0
}
