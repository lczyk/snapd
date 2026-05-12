// Copyright 2014-2022 Ulrich Kunitz. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package lzma

// literalCodec supports the encoding of literal. It provides 768 probability
// values per literal state. The upper 512 probabilities are used with the
// context of a match bit.
type literalCodec struct {
	probs []prob
}

// deepcopy initializes literal codec c as a deep copy of the source.
func (c *literalCodec) deepcopy(src *literalCodec) {
	if c == src {
		return
	}
	c.probs = make([]prob, len(src.probs))
	copy(c.probs, src.probs)
}

// init initializes the literal codec. Reuses an existing probs slice
// if it is already the right size to avoid re-allocation when the same
// codec is reset across chunks with matching properties.
func (c *literalCodec) init(lc, lp int) {
	switch {
	case !(minLC <= lc && lc <= maxLC):
		panic("lc out of range")
	case !(minLP <= lp && lp <= maxLP):
		panic("lp out of range")
	}
	n := 0x300 << uint(lc+lp)
	if cap(c.probs) < n {
		c.probs = make([]prob, n)
	} else {
		c.probs = c.probs[:n]
	}
	for i := range c.probs {
		c.probs[i] = probInit
	}
}

// Encode encodes the byte s using a range encoder as well as the current LZMA
// encoder state, a match byte and the literal state.
func (c *literalCodec) Encode(e *rangeEncoder, s byte,
	state uint32, match byte, litState uint32,
) (err error) {
	k := litState * 0x300
	probs := c.probs[k : k+0x300]
	symbol := uint32(1)
	r := uint32(s)
	if state >= 7 {
		m := uint32(match)
		for {
			matchBit := (m >> 7) & 1
			m <<= 1
			bit := (r >> 7) & 1
			r <<= 1
			i := ((1 + matchBit) << 8) | symbol
			if err = probs[i].Encode(e, bit); err != nil {
				return
			}
			symbol = (symbol << 1) | bit
			if matchBit != bit {
				break
			}
			if symbol >= 0x100 {
				break
			}
		}
	}
	for symbol < 0x100 {
		bit := (r >> 7) & 1
		r <<= 1
		if err = probs[symbol].Encode(e, bit); err != nil {
			return
		}
		symbol = (symbol << 1) | bit
	}
	return nil
}

// Decode decodes a literal byte using the range decoder as well as the LZMA
// state, a match byte, and the literal state.
//
// Both inner bit-decode loops have the range-coder body inlined to
// avoid the per-bit method-call overhead that (*rangeDecoder).DecodeBit
// otherwise pays. literalCodec.Decode is the single hottest function
// on the decode path; the gain from skipping the call is worth the
// duplication.
func (c *literalCodec) Decode(d *rangeDecoder,
	state uint32, match byte, litState uint32,
) (byte, error) {
	const (
		topBit   = 1 << 24
		probMax  = 1 << probbits
		moveBits = movebits
	)
	k := litState * 0x300
	probs := c.probs[k : k+0x300]
	symbol := uint32(1)
	if state >= 7 {
		m := uint32(match)
		for {
			matchBit := (m >> 7) & 1
			m <<= 1
			i := ((1 + matchBit) << 8) | symbol
			p := &probs[i]
			pv := uint32(*p)
			bound := (d.nrange >> probbits) * pv
			var bit uint32
			if d.code < bound {
				d.nrange = bound
				*p = prob(pv + (probMax-pv)>>moveBits)
			} else {
				d.code -= bound
				d.nrange -= bound
				*p = prob(pv - pv>>moveBits)
				bit = 1
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
			symbol = (symbol << 1) | bit
			if matchBit != bit {
				break
			}
			if symbol >= 0x100 {
				break
			}
		}
	}
	for symbol < 0x100 {
		p := &probs[symbol]
		pv := uint32(*p)
		bound := (d.nrange >> probbits) * pv
		var bit uint32
		if d.code < bound {
			d.nrange = bound
			*p = prob(pv + (probMax-pv)>>moveBits)
		} else {
			d.code -= bound
			d.nrange -= bound
			*p = prob(pv - pv>>moveBits)
			bit = 1
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
		symbol = (symbol << 1) | bit
	}
	return byte(symbol - 0x100), nil
}

// minLC and maxLC define the range for LC values.
const (
	minLC = 0
	maxLC = 8
)

// minLC and maxLC define the range for LP values.
const (
	minLP = 0
	maxLP = 4
)
