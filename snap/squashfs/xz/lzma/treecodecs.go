// Copyright 2014-2022 Ulrich Kunitz. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package lzma

// treeCodec encodes or decodes values with a fixed bit size. It is using a
// tree of probability value. The root of the tree is the most-significant bit.
type treeCodec struct {
	probTree
}

// makeTreeCodec makes a tree codec. The bits value must be inside the range
// [1,32].
func makeTreeCodec(bits int) treeCodec {
	return treeCodec{makeProbTree(bits)}
}

// deepcopy initializes tc as a deep copy of the source.
func (tc *treeCodec) deepcopy(src *treeCodec) {
	tc.probTree.deepcopy(&src.probTree)
}

// Encode uses the range encoder to encode a fixed-bit-size value.
func (tc *treeCodec) Encode(e *rangeEncoder, v uint32) (err error) {
	m := uint32(1)
	for i := int(tc.bits) - 1; i >= 0; i-- {
		b := (v >> uint(i)) & 1
		if err := e.EncodeBit(b, &tc.probs[m]); err != nil {
			return err
		}
		m = (m << 1) | b
	}
	return nil
}

// Decode uses the range decoder to decode a fixed-bit-size value. The
// inner loop is inlined into (*rangeDecoder).decodeTree so the
// per-bit range-coder body sits in one hot function rather than
// behind the DecodeBit method call.
func (tc *treeCodec) Decode(d *rangeDecoder) (uint32, error) {
	return d.decodeTree(tc.probs, int(tc.bits))
}

// treeReverseCodec is another tree codec, where the least-significant bit is
// the start of the probability tree.
type treeReverseCodec struct {
	probTree
}

// deepcopy initializes the treeReverseCodec as a deep copy of the
// source.
func (tc *treeReverseCodec) deepcopy(src *treeReverseCodec) {
	tc.probTree.deepcopy(&src.probTree)
}

// makeTreeReverseCodec creates treeReverseCodec value. The bits argument must
// be in the range [1,32].
func makeTreeReverseCodec(bits int) treeReverseCodec {
	return treeReverseCodec{makeProbTree(bits)}
}

// Encode uses range encoder to encode a fixed-bit-size value. The range
// encoder may cause errors.
func (tc *treeReverseCodec) Encode(v uint32, e *rangeEncoder) (err error) {
	m := uint32(1)
	for i := uint(0); i < uint(tc.bits); i++ {
		b := (v >> i) & 1
		if err := e.EncodeBit(b, &tc.probs[m]); err != nil {
			return err
		}
		m = (m << 1) | b
	}
	return nil
}

// Decode uses the range decoder to decode a fixed-bit-size value. The
// inner loop is inlined into (*rangeDecoder).decodeTreeReverse so the
// per-bit range-coder body sits in one hot function rather than
// behind the DecodeBit method call.
func (tc *treeReverseCodec) Decode(d *rangeDecoder) (uint32, error) {
	return d.decodeTreeReverse(tc.probs, int(tc.bits))
}

// probTree stores enough probability values to be used by the treeEncode and
// treeDecode methods of the range coder types.
type probTree struct {
	probs []prob
	bits  byte
}

// deepcopy initializes the probTree value as a deep copy of the source.
func (t *probTree) deepcopy(src *probTree) {
	if t == src {
		return
	}
	t.probs = make([]prob, len(src.probs))
	copy(t.probs, src.probs)
	t.bits = src.bits
}

// makeProbTree initializes a probTree structure.
func makeProbTree(bits int) probTree {
	var t probTree
	t.init(bits)
	return t
}

// init (re)initialises the probTree in place. Reuses the existing
// probs slice when its capacity matches, avoiding the per-chunk
// allocation otherwise paid on each lengthCodec/distCodec reset.
func (t *probTree) init(bits int) {
	if !(1 <= bits && bits <= 32) {
		panic("bits outside of range [1,32]")
	}
	n := 1 << uint(bits)
	if cap(t.probs) < n {
		t.probs = make([]prob, n)
	} else {
		t.probs = t.probs[:n]
	}
	t.bits = byte(bits)
	for i := range t.probs {
		t.probs[i] = probInit
	}
}

// Bits provides the number of bits for the values to de- or encode.
func (t *probTree) Bits() int {
	return int(t.bits)
}
