// Copyright 2026 Canonical Ltd
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License version 3 as
// published by the Free Software Foundation.

package lzma

import "sync"

// dictPool caches *decoderDict instances for reuse. We originally used
// sync.Pool but it gets flushed by the GC between bench iterations and
// (more importantly) between blocks under real workloads, which made
// the 8MB dictionary alloc the single largest decode-time alloc on
// every miss. A mutex-guarded bounded LIFO trades a small idle-memory
// cost (a few held dicts at the configured cap) for hit rates close to
// 100% in steady-state workloads like a snap install scanning many
// data blocks.
type dictPool struct {
	mu      sync.Mutex
	items   []*decoderDict
	dictCap int
}

const maxPooledDictsPerCap = 4

// dictPools holds one dictPool per dictionary capacity.
var dictPools sync.Map // key: int (dictCap), value: *dictPool

func dictPoolFor(dictCap int) *dictPool {
	if v, ok := dictPools.Load(dictCap); ok {
		return v.(*dictPool)
	}
	p := &dictPool{dictCap: dictCap}
	actual, _ := dictPools.LoadOrStore(dictCap, p)
	return actual.(*dictPool)
}

// acquireDecoderDict pulls a *decoderDict of the requested capacity
// from the pool, or makes a fresh one. The returned dict is empty
// (head=0, buffer reset).
func acquireDecoderDict(dictCap int) (*decoderDict, error) {
	p := dictPoolFor(dictCap)
	p.mu.Lock()
	if n := len(p.items); n > 0 {
		d := p.items[n-1]
		p.items = p.items[:n-1]
		p.mu.Unlock()
		d.Reset()
		d.buf.Reset()
		return d, nil
	}
	p.mu.Unlock()
	return newDecoderDictUnpooled(dictCap)
}

// releaseDecoderDict returns a dict to its capacity-keyed pool. Drops
// the dict on the floor if the pool is already at capacity, letting
// the GC reclaim the buffer.
func releaseDecoderDict(d *decoderDict) {
	if d == nil {
		return
	}
	p := dictPoolFor(d.buf.Cap())
	p.mu.Lock()
	if len(p.items) < maxPooledDictsPerCap {
		p.items = append(p.items, d)
	}
	p.mu.Unlock()
}
