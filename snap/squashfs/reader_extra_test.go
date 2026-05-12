// extra unit tests for small/pure reader helpers that don't need a
// real squashfs image: binary readers, inode predicates, FileInfo
// projection, glob shortcut, and FileHasSquashfsHeader.

package squashfs

import (
	"bytes"
	"encoding/binary"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/pierrec/lz4/v4"
)

// memReaderAt is a minimal io.ReaderAt over a byte slice, for readU16At.
type memReaderAt struct {
	data []byte
}

func (m *memReaderAt) ReadAt(p []byte, off int64) (int, error) {
	if off >= int64(len(m.data)) {
		return 0, io.EOF
	}
	n := copy(p, m.data[off:])
	if n < len(p) {
		return n, io.EOF
	}
	return n, nil
}

func TestReadU16At(t *testing.T) {
	buf := []byte{0, 0, 0x34, 0x12, 0xff, 0xff}
	m := &memReaderAt{data: buf}
	if got := readU16At(m, 2); got != 0x1234 {
		t.Errorf("got %#x, want 0x1234", got)
	}
	// short read returns 0 (the helper ignores errors)
	if got := readU16At(m, 100); got != 0 {
		t.Errorf("got %#x, want 0 for OOB read", got)
	}
}

func TestInodePredicates(t *testing.T) {
	cases := []struct {
		typ                       uint16
		isDir, isSymlink, isReg   bool
	}{
		{inodeBasicDir, true, false, false},
		{inodeExtDir, true, false, false},
		{inodeBasicFile, false, false, true},
		{inodeExtFile, false, false, true},
		{inodeBasicSymlink, false, true, false},
		{inodeExtSymlink, false, true, false},
		{inodeBasicFifo, false, false, false},
		{inodeBasicSocket, false, false, false},
		{inodeBasicBlock, false, false, false},
		{inodeBasicChar, false, false, false},
	}
	for _, tc := range cases {
		ino := &inode{Type: tc.typ}
		if ino.IsDir() != tc.isDir {
			t.Errorf("type %d: IsDir = %v, want %v", tc.typ, ino.IsDir(), tc.isDir)
		}
		if ino.IsSymlink() != tc.isSymlink {
			t.Errorf("type %d: IsSymlink = %v, want %v", tc.typ, ino.IsSymlink(), tc.isSymlink)
		}
		if ino.IsRegular() != tc.isReg {
			t.Errorf("type %d: IsRegular = %v, want %v", tc.typ, ino.IsRegular(), tc.isReg)
		}
	}
}

func TestInodeToFileInfo(t *testing.T) {
	cases := []struct {
		name     string
		typ      uint16
		perm     uint16
		wantMode os.FileMode
	}{
		{"dir", inodeBasicDir, 0755, os.ModeDir | 0755},
		{"ext dir", inodeExtDir, 0700, os.ModeDir | 0700},
		{"file", inodeBasicFile, 0644, 0644},
		{"ext file", inodeExtFile, 0600, 0600},
		// symlinks always render as 0777, perm bits in inode are ignored
		{"symlink", inodeBasicSymlink, 0644, os.ModeSymlink | 0777},
		{"ext symlink", inodeExtSymlink, 0000, os.ModeSymlink | 0777},
		{"block dev", inodeBasicBlock, 0660, os.ModeDevice | 0660},
		{"char dev", inodeBasicChar, 0660, os.ModeCharDevice | 0660},
		{"fifo", inodeBasicFifo, 0644, os.ModeNamedPipe | 0644},
		{"socket", inodeBasicSocket, 0666, os.ModeSocket | 0666},
		// unknown type: only perm bits remain
		{"unknown", 0xffff, 0700, 0700},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ino := &inode{Type: tc.typ, Perm: tc.perm, FileSize: 42, ModTime: 1700000000}
			fi := inodeToFileInfo("foo", ino)
			if fi.Name() != "foo" {
				t.Errorf("Name = %q", fi.Name())
			}
			if fi.Size() != 42 {
				t.Errorf("Size = %d", fi.Size())
			}
			if fi.Mode() != tc.wantMode {
				t.Errorf("Mode = %v, want %v", fi.Mode(), tc.wantMode)
			}
			if fi.IsDir() != (tc.wantMode&os.ModeDir != 0) {
				t.Errorf("IsDir = %v", fi.IsDir())
			}
			if !fi.ModTime().Equal(time.Unix(1700000000, 0)) {
				t.Errorf("ModTime = %v", fi.ModTime())
			}
			if fi.Sys() != ino {
				t.Errorf("Sys = %v, want inode", fi.Sys())
			}
		})
	}
}

func TestNativeReaderModTime(t *testing.T) {
	r := &nativeReader{sb: superblock{ModTime: 1700000000}}
	if !r.ModTime().Equal(time.Unix(1700000000, 0)) {
		t.Errorf("ModTime = %v", r.ModTime())
	}
}

func TestDecompressUnsupported(t *testing.T) {
	// no codec at id 0; exercises the default branch.
	r := &nativeReader{sb: superblock{Compression: 0}}
	if _, err := r.decompress([]byte{0, 0, 0, 0}); err == nil {
		t.Fatal("expected error for unsupported compression")
	}
}

func TestDecompressBadData(t *testing.T) {
	cases := []struct {
		name string
		comp uint16
	}{
		{"gzip", compGzip},
		{"xz", compXz},
		{"zstd", compZstd},
		{"lzma", compLzma},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := &nativeReader{sb: superblock{Compression: tc.comp}}
			if _, err := r.decompress([]byte{0xff, 0xff, 0xff, 0xff}); err == nil {
				t.Errorf("expected error for garbage %s input", tc.name)
			}
		})
	}
}

func TestNewNativeReaderErrors(t *testing.T) {
	t.Run("missing file", func(t *testing.T) {
		if _, _, err := newNativeReader("/nonexistent/path/here.snap"); err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("bad magic", func(t *testing.T) {
		dir := t.TempDir()
		p := filepath.Join(dir, "bogus.snap")
		buf := make([]byte, 200)
		// fill in a deliberately wrong magic
		binary.LittleEndian.PutUint32(buf[0:4], 0xdeadbeef)
		if err := os.WriteFile(p, buf, 0644); err != nil {
			t.Fatal(err)
		}
		if _, _, err := newNativeReader(p); err == nil {
			t.Fatal("expected error for non-squashfs file")
		}
	})
}

func TestFileHasSquashfsHeader(t *testing.T) {
	dir := t.TempDir()

	t.Run("missing path", func(t *testing.T) {
		if FileHasSquashfsHeader(filepath.Join(dir, "nope")) {
			t.Error("returned true for missing file")
		}
	})

	t.Run("too small", func(t *testing.T) {
		p := filepath.Join(dir, "small")
		if err := os.WriteFile(p, []byte("hsqs"), 0644); err != nil {
			t.Fatal(err)
		}
		if FileHasSquashfsHeader(p) {
			t.Error("returned true for truncated header")
		}
	})

	t.Run("wrong magic", func(t *testing.T) {
		p := filepath.Join(dir, "wrong")
		buf := make([]byte, 200)
		copy(buf, []byte("XXXX"))
		if err := os.WriteFile(p, buf, 0644); err != nil {
			t.Fatal(err)
		}
		if FileHasSquashfsHeader(p) {
			t.Error("returned true for wrong magic")
		}
	})

	t.Run("happy", func(t *testing.T) {
		p := filepath.Join(dir, "ok")
		buf := make([]byte, 200)
		copy(buf, []byte("hsqs"))
		if err := os.WriteFile(p, buf, 0644); err != nil {
			t.Fatal(err)
		}
		if !FileHasSquashfsHeader(p) {
			t.Error("returned false for valid magic prefix")
		}
	})
}

func TestSnapNewAndPath(t *testing.T) {
	s := New("/some/path.snap")
	if s.Path() != "/some/path.snap" {
		t.Errorf("Path = %q", s.Path())
	}
}

func TestReadDirEmpty(t *testing.T) {
	// dirSize <= 3 hits the early-return branch w/out doing any IO.
	r := &nativeReader{}
	entries, err := r.readDir(0, 0, 3)
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if entries != nil {
		t.Errorf("want nil, got %v", entries)
	}
}

// compressLzma shells out to the lzma CLI to produce a legacy
// lzma-alone stream (squashfs compression code 2). the vendored
// snap/squashfs/xz/lzma package is reader-only, so we rely on the
// system lzma binary (part of xz-utils) for the encoder side.
func compressLzma(data []byte) []byte {
	cmd := exec.Command("lzma", "-c", "-z")
	cmd.Stdin = bytes.NewReader(data)
	out, err := cmd.Output()
	if err != nil {
		panic("lzma CLI: " + err.Error())
	}
	return out
}

// compressLz4Block produces a raw lz4 block (no frame), matching the
// squashfs on-disk encoding for compression code 5.
func compressLz4Block(data []byte) []byte {
	out := make([]byte, lz4.CompressBlockBound(len(data)))
	var c lz4.Compressor
	n, err := c.CompressBlock(data, out)
	if err != nil {
		panic(err)
	}
	if n == 0 {
		// incompressible: squashfs stores raw in that case, but for
		// the test we just want non-empty compressed output -- pick
		// a payload that compresses.
		panic("payload did not compress; pick a more repetitive input")
	}
	return out[:n]
}

func TestDecompressRoundtrip(t *testing.T) {
	payload := []byte(bytes.Repeat([]byte("squashfs roundtrip test payload\n"), 64))

	cases := []struct {
		name string
		comp uint16
		enc  func([]byte) []byte
	}{
		{"gzip", compGzip, compressGzip},
		{"xz", compXz, compressXZ},
		{"zstd", compZstd, compressZstd},
		{"lzma", compLzma, compressLzma},
		{"lz4", compLz4, compressLz4Block},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := &nativeReader{sb: superblock{Compression: tc.comp, BlockSize: 131072}}
			got, err := r.decompress(tc.enc(payload))
			if err != nil {
				t.Fatalf("decompress: %v", err)
			}
			if !bytes.Equal(got, payload) {
				t.Errorf("roundtrip mismatch: got %d bytes, want %d", len(got), len(payload))
			}
		})
	}
}

func TestReadBlockSizesFragmentExact(t *testing.T) {
	// regression: a fragment-bearing file of exactly N*blockSize bytes
	// has N full blocks and no fragment tail; the math must not
	// off-by-one into N-1 blocks.
	r := bytes.NewReader(make([]byte, 4*4))
	got := readBlockSizes(r, 4*131072, 131072, true)
	if len(got) != 4 {
		t.Errorf("got %d, want 4", len(got))
	}
}
