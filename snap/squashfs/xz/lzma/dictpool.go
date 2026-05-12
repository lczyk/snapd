// Copyright 2026 Canonical Ltd
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License version 3 as
// published by the Free Software Foundation.

package lzma

import "sync"

// dictPools holds per-capacity sync.Pools of *decoderDict. The 8MB
// dictionary buffer dominates allocation on the xz decode hot path; this
// lets callers amortise it across blocks instead of paying per call.
var dictPools sync.Map // key: int (dictCap), value: *sync.Pool

func dictPoolFor(dictCap int) *sync.Pool {
	if v, ok := dictPools.Load(dictCap); ok {
		return v.(*sync.Pool)
	}
	cap := dictCap
	p := &sync.Pool{New: func() any {
		d, err := newDecoderDictUnpooled(cap)
		if err != nil {
			return nil
		}
		return d
	}}
	actual, _ := dictPools.LoadOrStore(dictCap, p)
	return actual.(*sync.Pool)
}

// acquireDecoderDict pulls a *decoderDict of the requested capacity from
// the pool, or makes a fresh one. The returned dict is empty (head=0,
// buffer reset).
func acquireDecoderDict(dictCap int) (*decoderDict, error) {
	p := dictPoolFor(dictCap)
	if d, ok := p.Get().(*decoderDict); ok && d != nil {
		d.Reset()
		d.buf.Reset()
		return d, nil
	}
	return newDecoderDictUnpooled(dictCap)
}

// releaseDecoderDict returns a dict to its capacity-keyed pool.
func releaseDecoderDict(d *decoderDict) {
	if d == nil {
		return
	}
	dictPoolFor(d.buf.Cap()).Put(d)
}
