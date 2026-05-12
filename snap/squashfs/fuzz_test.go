// Copyright 2026 Canonical Ltd
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License version 3 as
// published by the Free Software Foundation.

package squashfs

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// squashfsMagic is the 4-byte little-endian magic at offset 0 of
// every valid squashfs superblock. Splicing it into every fuzz
// input lets the fuzzer mutate the rest of the file without paying
// the readSuperblock gate; coverage past the superblock check opens
// up immediately.
const squashfsMagic uint32 = 0x73717368

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
	// Seed with several real squashfs images so the corpus has
	// diverse mutation starting points across compressors and
	// fileset shapes. Without this every worker starts from the
	// trivial seeds below, none of which clear the superblock
	// magic check.
	for _, seed := range loadSquashfsSeeds(f) {
		f.Add(seed)
	}
	// Plus trivially-malformed seeds for the short / wrong-magic
	// paths.
	f.Add([]byte{})
	f.Add(bytes.Repeat([]byte{0}, 96))
	f.Add(bytes.Repeat([]byte{0xff}, 96))

	const maxOutput = 16 << 20 // 16 MiB

	f.Fuzz(func(t *testing.T, blob []byte) {
		// Splice the squashfs magic at offset 0 unconditionally.
		// Lets the fuzzer mutate every other byte without falling
		// off the readSuperblock gate -- the coverage-driven
		// mutator can then explore the inode / dir / metadata
		// parsers properly. We make a copy first so we don't
		// mutate the corpus entry the runner re-uses.
		if len(blob) >= 4 {
			blob = append([]byte(nil), blob...)
			binary.LittleEndian.PutUint32(blob[:4], squashfsMagic)
		}

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

// loadSquashfsSeeds returns several mksquashfs-built fixtures of
// different shapes (compressors, file counts, sizes) so the fuzzer
// has diverse starting points past the superblock gate. Returns nil
// if mksquashfs isn't on $PATH.
func loadSquashfsSeeds(f *testing.F) [][]byte {
	f.Helper()
	if _, err := exec.LookPath("mksquashfs"); err != nil {
		return nil
	}
	// Reuse the existing fixture (xz, 100 files) plus build a few
	// more with different compressors and sizes inline so we get
	// diverse parser shapes in the corpus.
	fixtureXZ.once.Do(func() { fixtureXZ.build("xz") })
	var seeds [][]byte
	if fixtureXZ.err == nil {
		if data, err := os.ReadFile(fixtureXZ.path); err == nil {
			seeds = append(seeds, data)
		}
	}
	for _, comp := range []string{"gzip", "zstd"} {
		if data := buildTinySquashfs(comp); data != nil {
			seeds = append(seeds, data)
		}
	}
	return seeds
}

// buildTinySquashfs builds a minimal in-memory squashfs image with
// the given compressor. Returns nil on any failure (mksquashfs
// missing, comp unsupported, etc.) -- caller treats that as "skip
// this seed".
func buildTinySquashfs(comp string) []byte {
	dir, err := os.MkdirTemp("", "snapd-fuzz-tiny-*")
	if err != nil {
		return nil
	}
	defer os.RemoveAll(dir)
	root := filepath.Join(dir, "root")
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil
	}
	// One file, one dir -- keeps the image small (a few KB) so
	// mutations land within the corpus's per-input cap.
	if err := os.WriteFile(filepath.Join(root, "f"), []byte("hello squashfs"), 0o644); err != nil {
		return nil
	}
	out := filepath.Join(dir, "tiny.snap")
	cmd := exec.Command("mksquashfs", root, out, "-comp", comp,
		"-noappend", "-no-progress", "-no-xattrs")
	if err := cmd.Run(); err != nil {
		return nil
	}
	data, err := os.ReadFile(out)
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
