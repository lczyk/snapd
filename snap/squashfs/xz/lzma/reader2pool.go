// Copyright 2026 Canonical Ltd
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License version 3 as
// published by the Free Software Foundation.

package lzma

import "sync"

// reader2Pool caches *Reader2 instances complete with their dict, decoder,
// uncompressed-chunk reader, and state. Reusing a Reader2 amortises not
// just the 8MB dictionary alloc that dictPool already covers but also
// the codec prob-table allocations inside state (literalCodec.probs,
// length/dist tree codecs). The cache is keyed by dictionary capacity
// since dict sizing is fixed at construction time; codec sizes inside
// state depend on per-chunk Properties and are handled by the in-place
// Reset path in state.Reset / *Codec.init.
type reader2Pool struct {
	mu      sync.Mutex
	items   []*Reader2
	dictCap int
}

const maxPooledReader2sPerCap = 4

var reader2Pools sync.Map // key: int (dictCap), value: *reader2Pool

func reader2PoolFor(dictCap int) *reader2Pool {
	if v, ok := reader2Pools.Load(dictCap); ok {
		return v.(*reader2Pool)
	}
	p := &reader2Pool{dictCap: dictCap}
	actual, _ := reader2Pools.LoadOrStore(dictCap, p)
	return actual.(*reader2Pool)
}

// acquireReader2 returns a *Reader2 ready for reuse, or nil if the
// pool is empty.
func acquireReader2(dictCap int) *Reader2 {
	p := reader2PoolFor(dictCap)
	p.mu.Lock()
	defer p.mu.Unlock()
	if n := len(p.items); n > 0 {
		r := p.items[n-1]
		p.items = p.items[:n-1]
		return r
	}
	return nil
}

// releaseReader2 returns a *Reader2 to its capacity-keyed pool. Drops
// it on the floor if the pool is at capacity.
func releaseReader2(r *Reader2) {
	if r == nil || r.dict == nil {
		return
	}
	p := reader2PoolFor(r.dict.buf.Cap())
	p.mu.Lock()
	if len(p.items) < maxPooledReader2sPerCap {
		p.items = append(p.items, r)
	}
	p.mu.Unlock()
}
