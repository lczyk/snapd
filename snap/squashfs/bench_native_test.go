// Copyright 2026 Canonical Ltd
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License version 3 as
// published by the Free Software Foundation.

package squashfs

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
)

// nativeFixture is a small synthetic squashfs file built once per
// process, lazily, via the system mksquashfs CLI. Skips the
// benchmarks if mksquashfs isn't available.
type nativeFixture struct {
	once sync.Once
	path string
	err  error
}

var fixtureXZ nativeFixture

// build constructs a ~100-file squashfs image with nested directories
// at the given compression. Returns the image path. Caller is
// responsible for invoking once via fixture.once.Do.
func (f *nativeFixture) build(comp string) {
	if _, err := exec.LookPath("mksquashfs"); err != nil {
		f.err = fmt.Errorf("mksquashfs not on $PATH: %w", err)
		return
	}
	dir, err := os.MkdirTemp("", "snapd-bench-fixture-*")
	if err != nil {
		f.err = err
		return
	}
	root := filepath.Join(dir, "root")
	for i := 0; i < 10; i++ {
		sub := filepath.Join(root, fmt.Sprintf("d%02d", i))
		if err := os.MkdirAll(sub, 0o755); err != nil {
			f.err = err
			return
		}
		for j := 0; j < 10; j++ {
			p := filepath.Join(sub, fmt.Sprintf("f%02d.bin", j))
			content := make([]byte, 8*1024)
			for k := range content {
				content[k] = byte((i*j + k) & 0xff)
			}
			if err := os.WriteFile(p, content, 0o644); err != nil {
				f.err = err
				return
			}
		}
	}
	out := filepath.Join(dir, "fixture.snap")
	cmd := exec.Command("mksquashfs", root, out, "-comp", comp,
		"-noappend", "-no-progress", "-no-xattrs")
	if outBytes, err := cmd.CombinedOutput(); err != nil {
		f.err = fmt.Errorf("mksquashfs: %w: %s", err, string(outBytes))
		return
	}
	f.path = out
}

func openNativeFixture(b *testing.B, comp string) *nativeReader {
	b.Helper()
	fixtureXZ.once.Do(func() { fixtureXZ.build(comp) })
	if fixtureXZ.err != nil {
		b.Skip(fixtureXZ.err)
	}
	r, closer, err := newNativeReader(fixtureXZ.path)
	if err != nil {
		b.Fatalf("newNativeReader: %v", err)
	}
	b.Cleanup(func() { _ = closer() })
	return r
}

// BenchmarkReadMetadataBlocks measures the hot metadata-block read
// path -- the one called on every inode / dir lookup during a snap
// install. Walks the inode table region of the fixture so each
// iteration hits a fresh chunk.
func BenchmarkReadMetadataBlocks(b *testing.B) {
	r := openNativeFixture(b, "xz")
	b.ResetTimer()
	for b.Loop() {
		// Read the first metadata block of the inode table.
		_, err := r.readMetadataBlocks(r.sb.InodeTableStart, 0, 0, 1024, r.sb.DirTableStart)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkReadInode measures a single readInode call against the
// root directory inode of the fixture.
func BenchmarkReadInode(b *testing.B) {
	r := openNativeFixture(b, "xz")
	ref := r.sb.RootInodeRef
	b.ResetTimer()
	for b.Loop() {
		_, err := r.readInode(ref)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkWalkDir runs a full directory walk over the fixture's
// 10 dirs x 10 files. Models the install path's manifest scan.
func BenchmarkWalkDir(b *testing.B) {
	r := openNativeFixture(b, "xz")
	b.ResetTimer()
	for b.Loop() {
		err := r.walkDir(r.sb.RootInodeRef, "/", func(string, os.FileInfo, error) error {
			return nil
		})
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkResolvePath looks up a deeply-nested path. Models the
// per-file dispatch on extract / list operations.
func BenchmarkResolvePath(b *testing.B) {
	r := openNativeFixture(b, "xz")
	b.ResetTimer()
	for b.Loop() {
		_, err := r.resolvePath("/d05/f05.bin")
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkExtractAll runs the full extract path -- the worker pool,
// per-file writeFileData, compressed block decode, write to disk.
// Closest single bench to the real install hot path.
func BenchmarkExtractAll(b *testing.B) {
	r := openNativeFixture(b, "xz")
	b.ResetTimer()
	for b.Loop() {
		dest := b.TempDir()
		if err := r.extractAll(dest); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkReadFileData benches the per-file read path against a
// single 8KB file in the fixture. Models the install path's
// per-file decode.
func BenchmarkReadFileData(b *testing.B) {
	r := openNativeFixture(b, "xz")
	ino, err := r.resolvePath("/d05/f05.bin")
	if err != nil {
		b.Fatalf("resolve: %v", err)
	}
	b.ResetTimer()
	for b.Loop() {
		_, err := r.readFileData(ino)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkWriteFileData benches the streaming-write path that
// extractFiles uses internally.
func BenchmarkWriteFileData(b *testing.B) {
	r := openNativeFixture(b, "xz")
	ino, err := r.resolvePath("/d05/f05.bin")
	if err != nil {
		b.Fatalf("resolve: %v", err)
	}
	b.ResetTimer()
	for b.Loop() {
		if err := r.writeFileData(ino, io.Discard); err != nil {
			b.Fatal(err)
		}
	}
}
