// Copyright 2026 Canonical Ltd
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License version 3 as
// published by the Free Software Foundation.

package squashfs

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
)

// FuzzNativeReader feeds arbitrary bytes at the squashfs reader's
// front door. Construction is allowed to fail (most random bytes
// won't look like a valid superblock); operations on a successfully
// constructed reader must not panic, hang, or yield more than a
// hard cap of decompressed output (decompression-bomb guard).
//
// Run with:
//
//	go test -run='^$' -fuzz=FuzzNativeReader -fuzztime=30s ./snap/squashfs/
func FuzzNativeReader(f *testing.F) {
	// Seed with a real squashfs image so the corpus has something
	// to mutate from instead of starting from pure-random bytes
	// that won't survive the superblock check.
	if seed := loadFuzzSeed(f); seed != nil {
		f.Add(seed)
	}
	// Plus a couple of trivially-malformed seeds so the corpus
	// hits short / wrong-magic paths immediately.
	f.Add([]byte{})
	f.Add(bytes.Repeat([]byte{0}, 96))
	f.Add(bytes.Repeat([]byte{0xff}, 96))

	const maxOutput = 16 << 20 // 16 MiB

	f.Fuzz(func(t *testing.T, blob []byte) {
		// Construct a reader directly from the blob -- avoids the
		// disk round-trip newNativeReader does and keeps the fuzz
		// target self-contained.
		ra := bytes.NewReader(blob)
		sb, err := readSuperblock(ra)
		if err != nil {
			return
		}
		r := &nativeReader{ra: ra, sb: sb, size: int64(len(blob))}

		// Exercise the four entry points an install-path caller
		// reaches. Each must error or noop on bad data, never
		// panic / hang / overflow.

		// resolvePath: arbitrary path lookup.
		_, _ = r.resolvePath("/")
		_, _ = r.resolvePath("/etc/passwd")
		_, _ = r.resolvePath("/usr/bin/x")

		// walkDir: full tree walk. Cap the work via the walkFn so a
		// pathological fixture doesn't loop forever in the visitor.
		visits := 0
		_ = r.walkDir(r.sb.RootInodeRef, "/", func(_ string, _ os.FileInfo, _ error) error {
			visits++
			if visits > 10_000 {
				return errFuzzVisitsExceeded
			}
			return nil
		})

		// extractAll: drives writeFileData + the worker pool.
		// Bound output via a temp dir + check disk usage afterward.
		dest := t.TempDir()
		_ = r.extractAll(dest)
		if used, err := dirSize(dest); err == nil && used > maxOutput {
			t.Fatalf("extractAll produced %d bytes from %d-byte input -- decompression bomb?",
				used, len(blob))
		}

		// readInode + readDir on the root inode -- direct, in case
		// the higher-level paths short-circuit.
		ino, err := r.readInode(r.sb.RootInodeRef)
		if err == nil && ino != nil && ino.IsDir() {
			_, _ = r.readDir(ino.DirStart, ino.DirOffset, ino.DirSize)
		}
	})
}

var errFuzzVisitsExceeded = errors.New("fuzz: visit count exceeded")

// loadFuzzSeed returns a tiny real squashfs image to seed the corpus,
// or nil if mksquashfs isn't available (in which case the fuzz still
// runs against the trivial seeds below).
func loadFuzzSeed(f *testing.F) []byte {
	f.Helper()
	// Reuse the bench fixture builder. If mksquashfs isn't on
	// $PATH the build sets fixture.err and we just skip the seed.
	fixtureXZ.once.Do(func() { fixtureXZ.build("xz") })
	if fixtureXZ.err != nil {
		return nil
	}
	data, err := os.ReadFile(fixtureXZ.path)
	if err != nil {
		return nil
	}
	return data
}

// dirSize returns the total bytes written under dir.
func dirSize(dir string) (int64, error) {
	var total int64
	err := filepath.Walk(dir, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			total += info.Size()
		}
		return nil
	})
	return total, err
}

// shut linter up about io being imported via a var ref so future
// editors don't strip the import when Read sites get refactored
// out of this file.
var _ = io.EOF
