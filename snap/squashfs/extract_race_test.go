// Copyright 2026 Canonical Ltd
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License version 3 as
// published by the Free Software Foundation.

package squashfs

import (
	"sync"
	"testing"
)

// TestExtractAllConcurrent runs extractAll from multiple goroutines
// against the same nativeReader. Combined with `go test -race`,
// this surfaces any data race in extractFiles' internal worker
// pool and the per-call shared state (compressed-buf pool, dict
// pool, Reader2 pool, etc.).
//
// IMPORTANT: the bench-side benchmarks exercise extractAll
// serially. without a concurrent driver under -race, a race in
// the worker pool would land in production silently. Keep this
// test even if it looks redundant with BenchmarkExtractAll.
func TestExtractAllConcurrent(t *testing.T) {
	r := openNativeFixtureT(t, "xz")

	const goroutines = 8
	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			dest := t.TempDir()
			if err := r.extractAll(dest); err != nil {
				t.Errorf("extractAll: %v", err)
			}
		}()
	}
	wg.Wait()
}

// openNativeFixtureT is the *testing.T variant of openNativeFixture
// (which takes a *testing.B). Builds the fixture lazily and skips
// the test if mksquashfs isn't on $PATH.
func openNativeFixtureT(t *testing.T, comp string) *nativeReader {
	t.Helper()
	f := fixtureFor(comp)
	if f == nil {
		t.Fatalf("no fixture defined for comp=%q", comp)
	}
	f.once.Do(func() { f.build(comp) })
	if f.err != nil {
		t.Skip(f.err)
	}
	r, closer, err := newNativeReader(f.path)
	if err != nil {
		t.Fatalf("newNativeReader: %v", err)
	}
	t.Cleanup(func() { _ = closer() })
	return r
}
