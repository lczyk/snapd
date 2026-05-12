// table-driven unit tests for the pure / easily-testable parts of
// the squashfs reader. covers the corner cases that produced bugs
// during prototype bring-up:
//
//   - readBlockSizes: the bug where a hardcoded 256-byte readInode
//     window dropped block-size entries past block 56 wasn't here,
//     but the block-count math is. this guards against future drift.
//   - readSuperblock: bad magic was producing weird downstream
//     errors before we added the explicit check. tested.
//   - splitPath: utility that drives the Unpack glob path; small
//     enough to be exhaustively tested.

package squashfs

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"reflect"
	"testing"

	"github.com/klauspost/compress/zstd"
	// test-only: upstream xz has a Writer; our vendored
	// snap/squashfs/xz fork is reader-only. production code stays on
	// the vendored fork.
	"github.com/ulikunitz/xz"
)

func TestReadBlockSizes(t *testing.T) {
	cases := []struct {
		name        string
		fileSize    uint64
		blockSize   uint64
		hasFragment bool
		wantN       int
	}{
		{"empty file, no fragment", 0, 131072, false, 0},
		{"smaller than block, no fragment", 1024, 131072, false, 1},
		{"exactly one block, no fragment", 131072, 131072, false, 1},
		{"two blocks, no fragment", 200000, 131072, false, 2},
		{"smaller than block, with fragment", 1024, 131072, true, 0},
		{"exactly one block, with fragment", 131072, 131072, true, 1},
		// 57 blocks at 128 KB == 7.3 MB. files this size or larger
		// are what surfaced the readInode 256-byte truncation bug.
		// the math here was always correct; the bug was upstream of
		// it, but a regression in this function would re-trigger.
		{"57 blocks, no fragment", 57 * 131072, 131072, false, 57},
		{"57 blocks + tail, no fragment", 57*131072 + 100, 131072, false, 58},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// readBlockSizes reads tc.wantN x u32 from the buffer.
			buf := bytes.Repeat([]byte{0}, tc.wantN*4)
			got := readBlockSizes(bytes.NewReader(buf), tc.fileSize, tc.blockSize, tc.hasFragment)
			if len(got) != tc.wantN {
				t.Errorf("len = %d, want %d", len(got), tc.wantN)
			}
		})
	}
}

func TestReadSuperblock(t *testing.T) {
	t.Run("bad magic", func(t *testing.T) {
		bad := bytes.Repeat([]byte{0}, 96)
		if _, err := readSuperblock(bytes.NewReader(bad)); err == nil {
			t.Fatal("expected error for bad magic, got nil")
		}
	})

	t.Run("happy path", func(t *testing.T) {
		buf := make([]byte, 96)
		binary.LittleEndian.PutUint32(buf[0:4], 0x73717368) // squashfs magic
		binary.LittleEndian.PutUint32(buf[4:8], 42)         // InodeCount
		binary.LittleEndian.PutUint32(buf[12:16], 131072)   // BlockSize
		binary.LittleEndian.PutUint16(buf[20:22], 1)        // Compression = gzip
		binary.LittleEndian.PutUint64(buf[80:88], 0xdeadbeef) // FragTableStart

		sb, err := readSuperblock(bytes.NewReader(buf))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if sb.InodeCount != 42 {
			t.Errorf("InodeCount = %d, want 42", sb.InodeCount)
		}
		if sb.BlockSize != 131072 {
			t.Errorf("BlockSize = %d, want 131072", sb.BlockSize)
		}
		if sb.Compression != 1 {
			t.Errorf("Compression = %d, want 1 (gzip)", sb.Compression)
		}
		if sb.FragTableStart != 0xdeadbeef {
			t.Errorf("FragTableStart = %#x, want 0xdeadbeef", sb.FragTableStart)
		}
	})

	t.Run("short buffer", func(t *testing.T) {
		short := bytes.Repeat([]byte{0}, 50)
		if _, err := readSuperblock(bytes.NewReader(short)); err == nil {
			t.Fatal("expected error reading short buffer, got nil")
		}
	})
}

// --- benchmarks ---------------------------------------------------------------

func BenchmarkReadSuperblock(b *testing.B) {
	buf := make([]byte, 96)
	binary.LittleEndian.PutUint32(buf[0:4], 0x73717368)
	binary.LittleEndian.PutUint32(buf[4:8], 42)
	binary.LittleEndian.PutUint32(buf[12:16], 131072)
	binary.LittleEndian.PutUint16(buf[20:22], 1)
	binary.LittleEndian.PutUint64(buf[80:88], 0xdeadbeef)

	b.ResetTimer()
	for b.Loop() {
		readSuperblock(bytes.NewReader(buf))
	}
}

func BenchmarkSplitPath(b *testing.B) {
	b.Run("empty", func(b *testing.B) {
		for b.Loop() {
			splitPath("")
		}
	})
	b.Run("single", func(b *testing.B) {
		for b.Loop() {
			splitPath("foo")
		}
	})
	b.Run("nested", func(b *testing.B) {
		for b.Loop() {
			splitPath("usr/share/doc/foo/bar")
		}
	})
	b.Run("absolute", func(b *testing.B) {
		for b.Loop() {
			splitPath("/usr/share/doc/foo/bar")
		}
	})
	b.Run("with-dots", func(b *testing.B) {
		for b.Loop() {
			splitPath("./foo/./bar/../baz")
		}
	})
}

func BenchmarkReadBlockSizes(b *testing.B) {
	nBlocks := 100
	buf := bytes.Repeat([]byte{0}, nBlocks*4)
	for i := range nBlocks {
		binary.LittleEndian.PutUint32(buf[i*4:], uint32(131072|0x800000))
	}

	b.Run("no-fragment", func(b *testing.B) {
		for b.Loop() {
			r := bytes.NewReader(buf)
			readBlockSizes(r, uint64(nBlocks)*131072, 131072, false)
		}
	})
	b.Run("with-fragment", func(b *testing.B) {
		for b.Loop() {
			r := bytes.NewReader(buf)
			readBlockSizes(r, uint64(nBlocks)*131072, 131072, true)
		}
	})
}

// helper to build a nativeReader with given compression type for decompress benchmarks
func makeReader(comp uint16) *nativeReader {
	return &nativeReader{
		sb: superblock{Compression: comp, BlockSize: 131072},
	}
}

// generateTestPayload returns data that compresses reasonably well - a
// mix of repeated strings and some entropy, like typical squashfs content
// (metadata YAML, ELF headers, etc.)
func generateTestPayload(size int) []byte {
	parts := [][]byte{
		[]byte(`name: hello-world
version: "2.10"
summary: Hello world example
`),
		bytes.Repeat([]byte{0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07}, 32),
		bytes.Repeat([]byte{0x7f, 'E', 'L', 'F'}, 64),
		bytes.Repeat([]byte("some repeated text that simulates source code or yaml content\n"), 16),
	}
	var buf []byte
	for len(buf) < size {
		for _, p := range parts {
			buf = append(buf, p...)
			if len(buf) >= size {
				break
			}
		}
	}
	return buf[:size]
}

func BenchmarkDecompress(b *testing.B) {
	payload := generateTestPayload(8192)

	compressedGzip := compressGzip(payload)
	compressedXZ := compressXZ(payload)
	compressedZstd := compressZstd(payload)
	b.Run("gzip-8k", func(b *testing.B) {
		r := makeReader(compGzip)
		b.ResetTimer()
		for b.Loop() {
			r.decompress(compressedGzip, 0)
		}
	})
	b.Run("xz-8k", func(b *testing.B) {
		r := makeReader(compXz)
		b.ResetTimer()
		for b.Loop() {
			r.decompress(compressedXZ, 0)
		}
	})
	b.Run("zstd-8k", func(b *testing.B) {
		r := makeReader(compZstd)
		b.ResetTimer()
		for b.Loop() {
			r.decompress(compressedZstd, 0)
		}
	})
	// larger payloads (128k), more typical of squashfs data blocks
	bigPayload := generateTestPayload(131072)
	bigGzip := compressGzip(bigPayload)
	bigXZ := compressXZ(bigPayload)
	bigZstd := compressZstd(bigPayload)
	b.Run("gzip-128k", func(b *testing.B) {
		r := makeReader(compGzip)
		b.ResetTimer()
		for b.Loop() {
			r.decompress(bigGzip, 0)
		}
	})
	b.Run("xz-128k", func(b *testing.B) {
		r := makeReader(compXz)
		b.ResetTimer()
		for b.Loop() {
			r.decompress(bigXZ, 0)
		}
	})
	b.Run("zstd-128k", func(b *testing.B) {
		r := makeReader(compZstd)
		b.ResetTimer()
		for b.Loop() {
			r.decompress(bigZstd, 0)
		}
	})
}

func compressGzip(data []byte) []byte {
	var buf bytes.Buffer
	w := zlib.NewWriter(&buf)
	w.Write(data)
	w.Close()
	return buf.Bytes()
}

// compressXZ uses upstream ulikunitz/xz for its Writer. our vendored
// snap/squashfs/xz fork is reader-only, but the upstream package is
// kept in go.mod purely for these test-time encoders.
func compressXZ(data []byte) []byte {
	var buf bytes.Buffer
	w, err := xz.NewWriter(&buf)
	if err != nil {
		panic("xz writer: " + err.Error())
	}
	w.Write(data)
	w.Close()
	return buf.Bytes()
}

func compressZstd(data []byte) []byte {
	var buf bytes.Buffer
	w, err := zstd.NewWriter(&buf)
	if err != nil {
		panic("zstd writer: " + err.Error())
	}
	w.Write(data)
	w.Close()
	return buf.Bytes()
}

func TestSplitPath(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"", nil},
		{".", nil},
		{"/", nil},
		{"/foo", []string{"foo"}},
		{"foo", []string{"foo"}},
		{"foo/bar", []string{"foo", "bar"}},
		{"/foo/bar", []string{"foo", "bar"}},
		{"./foo/bar", []string{"foo", "bar"}},
		// filepath.Clean strips trailing slash
		{"/foo/bar/", []string{"foo", "bar"}},
		// filepath.Clean collapses .. and .
		{"foo/./bar", []string{"foo", "bar"}},
		{"foo/../bar", []string{"bar"}},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			got := splitPath(tc.in)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("splitPath(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}
