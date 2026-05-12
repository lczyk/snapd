// Copyright 2014-2022 Ulrich Kunitz. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package lzma

import (
	"errors"
	"io"
)

// rangeEncoder implements range encoding of single bits. The low value can
// overflow therefore we need uint64. The cache value is used to handle
// overflows.
type rangeEncoder struct {
	lbw      *LimitedByteWriter
	nrange   uint32
	low      uint64
	cacheLen int64
	cache    byte
}

// maxInt64 provides the  maximal value of the int64 type
const maxInt64 = 1<<63 - 1

// newRangeEncoder creates a new range encoder.
func newRangeEncoder(bw io.ByteWriter) (re *rangeEncoder, err error) {
	lbw, ok := bw.(*LimitedByteWriter)
	if !ok {
		lbw = &LimitedByteWriter{BW: bw, N: maxInt64}
	}
	return &rangeEncoder{
		lbw:      lbw,
		nrange:   0xffffffff,
		cacheLen: 1}, nil
}

// Available returns the number of bytes that still can be written. The
// method takes the bytes that will be currently written by Close into
// account.
func (e *rangeEncoder) Available() int64 {
	return e.lbw.N - (e.cacheLen + 4)
}

// writeByte writes a single byte to the underlying writer. An error is
// returned if the limit is reached. The written byte will be counted if
// the underlying writer doesn't return an error.
func (e *rangeEncoder) writeByte(c byte) error {
	if e.Available() < 1 {
		return ErrLimit
	}
	return e.lbw.WriteByte(c)
}

// DirectEncodeBit encodes the least-significant bit of b with probability 1/2.
func (e *rangeEncoder) DirectEncodeBit(b uint32) error {
	e.nrange >>= 1
	e.low += uint64(e.nrange) & (0 - (uint64(b) & 1))

	// normalize
	const top = 1 << 24
	if e.nrange >= top {
		return nil
	}
	e.nrange <<= 8
	return e.shiftLow()
}

// EncodeBit encodes the least significant bit of b. The p value will be
// updated by the function depending on the bit encoded.
func (e *rangeEncoder) EncodeBit(b uint32, p *prob) error {
	bound := p.bound(e.nrange)
	if b&1 == 0 {
		e.nrange = bound
		p.inc()
	} else {
		e.low += uint64(bound)
		e.nrange -= bound
		p.dec()
	}

	// normalize
	const top = 1 << 24
	if e.nrange >= top {
		return nil
	}
	e.nrange <<= 8
	return e.shiftLow()
}

// Close writes a complete copy of the low value.
func (e *rangeEncoder) Close() error {
	for i := 0; i < 5; i++ {
		if err := e.shiftLow(); err != nil {
			return err
		}
	}
	return nil
}

// shiftLow shifts the low value for 8 bit. The shifted byte is written into
// the byte writer. The cache value is used to handle overflows.
func (e *rangeEncoder) shiftLow() error {
	if uint32(e.low) < 0xff000000 || (e.low>>32) != 0 {
		tmp := e.cache
		for {
			err := e.writeByte(tmp + byte(e.low>>32))
			if err != nil {
				return err
			}
			tmp = 0xff
			e.cacheLen--
			if e.cacheLen <= 0 {
				if e.cacheLen < 0 {
					panic("negative cacheLen")
				}
				break
			}
		}
		e.cache = byte(uint32(e.low) >> 24)
	}
	e.cacheLen++
	e.low = uint64(uint32(e.low) << 8)
	return nil
}

// rangeDecoder decodes single bits of the range encoding stream.
//
// On the squashfs decode hot path the range decoder is fed a
// pre-buffered chunk slice (see Reader2.startChunk). The legacy
// io.ByteReader path is retained for callers that stream byte-by-byte
// (the lzma-alone format reader). When `data != nil` the fast path
// reads directly from the slice; otherwise updateCode falls back to
// br.ReadByte.
type rangeDecoder struct {
	br     io.ByteReader
	data   []byte
	pos    int
	nrange uint32
	code   uint32
}

// newRangeDecoder initializes a range decoder. It reads five bytes from the
// reader and therefore may return an error.
func newRangeDecoder(br io.ByteReader) (d *rangeDecoder, err error) {
	// If the underlying reader is our sliceByteReader, hand the
	// rangeDecoder direct slice access so updateCode can bypass the
	// io.ByteReader interface call on the per-byte path.
	if sr, ok := br.(*sliceByteReader); ok {
		d = &rangeDecoder{
			data:   sr.data[sr.pos:],
			nrange: 0xffffffff,
		}
	} else {
		d = &rangeDecoder{br: br, nrange: 0xffffffff}
	}

	b, err := d.readByte()
	if err != nil {
		return nil, err
	}
	if b != 0 {
		return nil, errors.New("newRangeDecoder: first byte not zero")
	}

	for i := 0; i < 4; i++ {
		if err = d.updateCode(); err != nil {
			return nil, err
		}
	}

	if d.code >= d.nrange {
		return nil, errors.New("newRangeDecoder: d.code >= d.nrange")
	}

	return d, nil
}

// readByte returns the next byte from either the direct slice or the
// fallback io.ByteReader.
func (d *rangeDecoder) readByte() (byte, error) {
	if d.data != nil {
		if d.pos >= len(d.data) {
			return 0, io.EOF
		}
		c := d.data[d.pos]
		d.pos++
		return c, nil
	}
	return d.br.ReadByte()
}

// possiblyAtEnd checks whether the decoder may be at the end of the stream.
func (d *rangeDecoder) possiblyAtEnd() bool {
	return d.code == 0
}

// DirectDecodeBit decodes a bit with probability 1/2. The return value b will
// contain the bit at the least-significant position. All other bits will be
// zero.
func (d *rangeDecoder) DirectDecodeBit() (b uint32, err error) {
	d.nrange >>= 1
	d.code -= d.nrange
	t := 0 - (d.code >> 31)
	d.code += d.nrange & t
	b = (t + 1) & 1

	// d.code will stay less then d.nrange

	// normalize
	// assume d.code < d.nrange
	const top = 1 << 24
	if d.nrange >= top {
		return b, nil
	}
	d.nrange <<= 8
	// d.code < d.nrange will be maintained
	return b, d.updateCode()
}

// DecodeBit decodes a single bit. The bit will be returned at the
// least-significant position. All other bits will be zero. The probability
// value will be updated.
//
// Hand-inlined hot path: prob.bound / prob.inc / prob.dec are folded
// into this body, and the common-case slice-backed refill (one byte
// from d.data) is inlined too. The legacy io.ByteReader path is
// kicked out to updateCodeSlow so the hot body stays within the Go
// inliner's budget for callers.
func (d *rangeDecoder) DecodeBit(p *prob) (uint32, error) {
	const (
		topBit   = 1 << 24
		probMax  = 1 << probbits
		moveBits = movebits
	)
	pv := uint32(*p)
	bound := (d.nrange >> probbits) * pv
	var b uint32
	if d.code < bound {
		d.nrange = bound
		*p = prob(pv + (probMax-pv)>>moveBits)
	} else {
		d.code -= bound
		d.nrange -= bound
		*p = prob(pv - pv>>moveBits)
		b = 1
	}
	if d.nrange >= topBit {
		return b, nil
	}
	d.nrange <<= 8
	if d.pos < len(d.data) {
		d.code = (d.code << 8) | uint32(d.data[d.pos])
		d.pos++
		return b, nil
	}
	return b, d.updateCodeSlow()
}

// decodeTree decodes a fixed-bit-size MSB-first value using the given
// probs slice. The hot inner loop is the inlined range-coder bit
// decode -- skipping the per-bit method-call overhead that
// (*rangeDecoder).DecodeBit otherwise pays. Probs is expected to be
// sized to (1<<bits) at minimum.
func (d *rangeDecoder) decodeTree(probs []prob, bits int) (uint32, error) {
	const (
		topBit   = 1 << 24
		probMax  = 1 << probbits
		moveBits = movebits
	)
	m := uint32(1)
	for j := 0; j < bits; j++ {
		p := &probs[m]
		pv := uint32(*p)
		bound := (d.nrange >> probbits) * pv
		var b uint32
		if d.code < bound {
			d.nrange = bound
			*p = prob(pv + (probMax-pv)>>moveBits)
		} else {
			d.code -= bound
			d.nrange -= bound
			*p = prob(pv - pv>>moveBits)
			b = 1
		}
		if d.nrange < topBit {
			d.nrange <<= 8
			if d.pos < len(d.data) {
				d.code = (d.code << 8) | uint32(d.data[d.pos])
				d.pos++
			} else if err := d.updateCodeSlow(); err != nil {
				return 0, err
			}
		}
		m = (m << 1) | b
	}
	return m - (1 << uint(bits)), nil
}

// decodeTreeReverse decodes a fixed-bit-size LSB-first value using the
// given probs slice. Inlined range-coder body matches decodeTree.
func (d *rangeDecoder) decodeTreeReverse(probs []prob, bits int) (uint32, error) {
	const (
		topBit   = 1 << 24
		probMax  = 1 << probbits
		moveBits = movebits
	)
	m := uint32(1)
	var v uint32
	for j := uint(0); j < uint(bits); j++ {
		p := &probs[m]
		pv := uint32(*p)
		bound := (d.nrange >> probbits) * pv
		var b uint32
		if d.code < bound {
			d.nrange = bound
			*p = prob(pv + (probMax-pv)>>moveBits)
		} else {
			d.code -= bound
			d.nrange -= bound
			*p = prob(pv - pv>>moveBits)
			b = 1
		}
		if d.nrange < topBit {
			d.nrange <<= 8
			if d.pos < len(d.data) {
				d.code = (d.code << 8) | uint32(d.data[d.pos])
				d.pos++
			} else if err := d.updateCodeSlow(); err != nil {
				return 0, err
			}
		}
		m = (m << 1) | b
		v |= b << j
	}
	return v, nil
}

// updateCodeSlow handles the io.ByteReader fallback path for the rare
// legacy lzma-alone reader; the slice-backed hot path inlines its
// equivalent directly into DecodeBit.
func (d *rangeDecoder) updateCodeSlow() error {
	if d.data != nil {
		return io.EOF
	}
	bb, err := d.br.ReadByte()
	if err != nil {
		return err
	}
	d.code = (d.code << 8) | uint32(bb)
	return nil
}

// updateCode reads a new byte into the code. Fast path uses direct
// slice indexing; the io.ByteReader path is only used for the legacy
// lzma-alone format reader.
func (d *rangeDecoder) updateCode() error {
	if d.data != nil {
		if d.pos >= len(d.data) {
			return io.EOF
		}
		d.code = (d.code << 8) | uint32(d.data[d.pos])
		d.pos++
		return nil
	}
	b, err := d.br.ReadByte()
	if err != nil {
		return err
	}
	d.code = (d.code << 8) | uint32(b)
	return nil
}
