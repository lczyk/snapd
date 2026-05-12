// -*- Mode: Go; indent-tabs-mode: t -*-

/*
 * Copyright (C) 2025 Canonical Ltd
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU General Public License version 3 as
 * published by the Free Software Foundation.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
 *
 */

package squashfs

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/klauspost/compress/zstd"
	"github.com/pierrec/lz4/v4"
	lzo "github.com/rasky/go-lzo"
	"github.com/snapcore/snapd/snap/squashfs/xz"
	"github.com/snapcore/snapd/snap/squashfs/xz/lzma"
)

// nativeReader is a pure-Go squashfs reader backed by an io.ReaderAt.
type nativeReader struct {
	ra   io.ReaderAt
	sb   superblock
	size int64
}

// --- superblock ----------------------------------------------------------------

// superblock is the 96-byte squashfs superblock.
type superblock struct {
	InodeCount   uint32
	ModTime      uint32
	BlockSize    uint32
	FragmentCount uint32
	Compression  uint16
	BlockLog     uint16
	Flags        uint16
	IDCount      uint16
	VersionMajor uint16
	VersionMinor uint16
	RootInodeRef uint64
	BytesUsed    uint64
	IDTableStart uint64
	XattrTableStart uint64
	InodeTableStart uint64
	DirTableStart   uint64
	FragTableStart  uint64
	ExportTableStart uint64
}

const (
	compGzip = 1
	compLzma = 2
	compLzo  = 3
	compXz   = 4
	compLz4  = 5
	compZstd = 6
)

func readSuperblock(ra io.ReaderAt) (superblock, error) {
	var sb superblock
	buf := make([]byte, 96)
	if _, err := ra.ReadAt(buf, 0); err != nil {
		return sb, err
	}
	r := bytes.NewReader(buf)

	magic := readU32(r)
	if magic != 0x73717368 {
		return sb, fmt.Errorf("not a squashfs image (magic: %08x)", magic)
	}

	sb.InodeCount = readU32(r)
	sb.ModTime = readU32(r)
	sb.BlockSize = readU32(r)
	sb.FragmentCount = readU32(r)
	sb.Compression = readU16(r)
	sb.BlockLog = readU16(r)
	sb.Flags = readU16(r)
	sb.IDCount = readU16(r)
	sb.VersionMajor = readU16(r)
	sb.VersionMinor = readU16(r)
	sb.RootInodeRef = readU64(r)
	sb.BytesUsed = readU64(r)
	sb.IDTableStart = readU64(r)
	sb.XattrTableStart = readU64(r)
	sb.InodeTableStart = readU64(r)
	sb.DirTableStart = readU64(r)
	sb.FragTableStart = readU64(r)
	sb.ExportTableStart = readU64(r)

	return sb, nil
}

// --- metadata reading ----------------------------------------------------------

// readMetadataBlocks reads a chain of metadata blocks starting at tableStart+blockStart,
// and returns the concatenated decompressed data, skipping skipBytes from the first block.
// tableEnd, if non-zero, is an absolute file offset beyond which no block should be read;
// this prevents the loop from walking off the end of the table into adjacent tables.
func (r *nativeReader) readMetadataBlocks(tableStart, blockStart uint64, skipBytes, totalBytes int, tableEnd uint64) ([]byte, error) {
	var buf []byte
	offset := blockStart
	need := totalBytes + skipBytes
	skipped := 0

	for need > 0 {
		// stop as soon as we have what was asked for. without this
		// the loop keeps reading metadata blocks past the end of the
		// table being walked and into whatever comes after (dir-,
		// frag-, export-table). those decode as bogus block headers
		// and the next ReadAt walks off the end of the file -- the
		// "EOF reading metadata at <near-eof-offset>" failure mode.
		if len(buf) >= totalBytes {
			break
		}
		// stop at table boundary to avoid reading adjacent tables
		if tableEnd > 0 && tableStart+offset >= tableEnd {
			break
		}
		pos := tableStart + offset
		header := readU16At(r.ra, pos)
		dataSize := int(header & 0x7FFF)
		compressed := (header & 0x8000) == 0

		data := make([]byte, dataSize)
		if _, err := r.ra.ReadAt(data, int64(pos)+2); err != nil {
			// EOF here means the chain has run out of valid metadata
			// blocks (e.g. we walked past the end of the table being
			// read). return what we have and let the caller decide
			// whether it's enough -- readInode in particular asks for
			// 256 bytes initially as a header probe and is happy with
			// fewer if there genuinely aren't more.
			if len(buf) >= 16 {
				break
			}
			return nil, fmt.Errorf("read metadata at %d: %w", pos, err)
		}

		if compressed {
			var err error
			data, err = r.decompress(data)
			if err != nil {
				return nil, fmt.Errorf("decompress metadata at %d: %w", pos, err)
			}
		}

		offset += 2 + uint64(dataSize)
		need -= len(data)

		// skip leading bytes from the first block
		if skipped < skipBytes {
			toSkip := skipBytes - skipped
			if toSkip >= len(data) {
				skipped += len(data)
				continue
			}
			data = data[toSkip:]
			skipped = skipBytes
		}

		buf = append(buf, data...)
	}

	if totalBytes < len(buf) {
		buf = buf[:totalBytes]
	}
	return buf, nil
}

func (r *nativeReader) decompress(data []byte) ([]byte, error) {
	switch r.sb.Compression {
	case compGzip:
		rd, err := zlib.NewReader(bytes.NewReader(data))
		if err != nil {
			return nil, err
		}
		defer rd.Close()
		return io.ReadAll(rd)
	case compXz:
		rd, err := xz.NewReader(bytes.NewReader(data))
		if err != nil {
			return nil, err
		}
		return io.ReadAll(rd)
	case compZstd:
		rd, err := zstd.NewReader(bytes.NewReader(data))
		if err != nil {
			return nil, err
		}
		defer rd.Close()
		return io.ReadAll(rd)
	case compLzo:
		// LZO blocks in squashfs are LZO1X. the output size for a
		// data block is at most blockSize (capped by the spec); for
		// metadata blocks it's at most 8KB. pass 0 here -- the
		// decoder grows the buffer as it goes when outLen is 0.
		return lzo.Decompress1X(bytes.NewReader(data), len(data), 0)
	case compLzma:
		// squashfs lzma blocks are raw lzma streams (legacy format,
		// pre-xz). ulikunitz/xz/lzma.Reader handles the alone-format
		// 13-byte header used by the old squashfs encoder.
		rd, err := lzma.NewReader(bytes.NewReader(data))
		if err != nil {
			return nil, err
		}
		return io.ReadAll(rd)
	case compLz4:
		// squashfs lz4 blocks are raw lz4 blocks (no frame). the
		// decompressed size is bounded by BlockSize for data blocks
		// and 8KB for metadata; use BlockSize as the ceiling, falling
		// back to 8KB if BlockSize is zero (metadata-block decode
		// before the superblock is fully parsed -- unusual but safe).
		max := int(r.sb.BlockSize)
		if max < 8192 {
			max = 8192
		}
		out := make([]byte, max)
		n, err := lz4.UncompressBlock(data, out)
		if err != nil {
			return nil, err
		}
		return out[:n], nil
	default:
		return nil, fmt.Errorf("unsupported compression type: %d", r.sb.Compression)
	}
}

// --- inode reading -------------------------------------------------------------

// inode type constants
const (
	inodeBasicDir     uint16 = 1
	inodeBasicFile    uint16 = 2
	inodeBasicSymlink uint16 = 3
	inodeBasicBlock   uint16 = 4
	inodeBasicChar    uint16 = 5
	inodeBasicFifo    uint16 = 6
	inodeBasicSocket  uint16 = 7
	inodeExtDir       uint16 = 8
	inodeExtFile      uint16 = 9
	inodeExtSymlink   uint16 = 10
)

// inode holds a parsed inode from the squashfs image.
type inode struct {
	Type       uint16
	Perm       uint16
	Uid        uint16
	Gid        uint16
	ModTime    uint32
	Number     uint32
	FileSize   uint64
	StartBlock uint64
	FragIndex  uint32
	FragOffset uint32
	SymTarget  string
	DirStart   uint64
	DirSize    uint32
	DirOffset  uint16
	Nlinks     uint32
	BlockSizes []uint32
}

func (r *nativeReader) readInode(ref uint64) (*inode, error) {
	blockOff := ref >> 16
	byteOff := int(ref & 0xFFFF)

	// 256 bytes covers every inode's fixed header (max 56 bytes for
	// extended file) and the block-sizes array for files up to 56 blocks
	// (~7MB at 128KB blocksize). for larger files we re-read with the
	// exact size below, after parsing FileSize. without the re-read,
	// readU32 on the exhausted bytes.Reader returns zero for missing
	// block sizes and writeFileData treats dataSize==0 as a sparse
	// all-zero block, silently truncating the file.
	data, err := r.readMetadataBlocks(r.sb.InodeTableStart, blockOff, byteOff, 256, r.sb.DirTableStart)
	if err != nil {
		return nil, fmt.Errorf("read inode block: %w", err)
	}
	if len(data) < 16 {
		return nil, fmt.Errorf("inode data too short: %d bytes", len(data))
	}

	// for file inodes, compute exact metadata length and re-read if 256
	// wasn't enough. for everything else 256 is plenty.
	inoType := binary.LittleEndian.Uint16(data[0:2])
	blockSize := uint64(r.sb.BlockSize)
	var needed int
	switch inoType {
	case inodeBasicFile:
		if len(data) >= 32 {
			fragIndex := binary.LittleEndian.Uint32(data[20:24])
			fileSize := uint64(binary.LittleEndian.Uint32(data[28:32]))
			var blocks uint64
			if fragIndex != 0xFFFFFFFF {
				blocks = fileSize / blockSize
			} else {
				blocks = (fileSize + blockSize - 1) / blockSize
			}
			needed = 32 + int(blocks)*4
		}
	case inodeExtFile:
		if len(data) >= 56 {
			fileSize := binary.LittleEndian.Uint64(data[24:32])
			fragIndex := binary.LittleEndian.Uint32(data[44:48])
			var blocks uint64
			if fragIndex != 0xFFFFFFFF {
				blocks = fileSize / blockSize
			} else {
				blocks = (fileSize + blockSize - 1) / blockSize
			}
			needed = 56 + int(blocks)*4
		}
	}
	if needed > len(data) {
		data, err = r.readMetadataBlocks(r.sb.InodeTableStart, blockOff, byteOff, needed, r.sb.DirTableStart)
		if err != nil {
			return nil, fmt.Errorf("re-read inode block (%d bytes): %w", needed, err)
		}
	}

	ino := &inode{}
	rd := bytes.NewReader(data)
	ino.Type = readU16(rd)
	ino.Perm = readU16(rd)
	ino.Uid = readU16(rd)
	ino.Gid = readU16(rd)
	ino.ModTime = readU32(rd)
	ino.Number = readU32(rd)

	switch ino.Type {
	case inodeBasicDir:
		ino.DirStart = uint64(readU32(rd))
		ino.Nlinks = readU32(rd)
		ino.DirSize = uint32(readU16(rd))
		ino.DirOffset = readU16(rd)
		ino.FileSize = uint64(readU32(rd)) // parent inode; unused by us

	case inodeExtDir:
		ino.Nlinks = readU32(rd)
		ino.DirSize = readU32(rd)
		ino.DirStart = uint64(readU32(rd))
		_ = readU32(rd) // parent inode
		_ = readU16(rd) // index count
		ino.DirOffset = readU16(rd)
		_ = readU32(rd) // xattr index

	case inodeBasicFile:
		ino.StartBlock = uint64(readU32(rd))
		ino.FragIndex = readU32(rd)
		ino.FragOffset = readU32(rd)
		ino.FileSize = uint64(readU32(rd))
		// block sizes follow - read remaining data
		ino.BlockSizes = readBlockSizes(rd, ino.FileSize, uint64(r.sb.BlockSize), ino.FragIndex != 0xFFFFFFFF)

	case inodeExtFile:
		ino.StartBlock = readU64(rd)
		ino.FileSize = readU64(rd)
		_ = readU64(rd) // sparse
		ino.Nlinks = readU32(rd)
		ino.FragIndex = readU32(rd)
		ino.FragOffset = readU32(rd)
		_ = readU32(rd) // xattr index
		ino.BlockSizes = readBlockSizes(rd, ino.FileSize, uint64(r.sb.BlockSize), ino.FragIndex != 0xFFFFFFFF)

	case inodeBasicSymlink:
		ino.Nlinks = readU32(rd)
		targetSize := readU32(rd)
		// target follows the fixed fields
		if int(targetSize) > len(data)-24 {
			return nil, fmt.Errorf("symlink target exceeds inode data")
		}
		ino.SymTarget = string(data[24 : 24+targetSize])

	case inodeExtSymlink:
		ino.Nlinks = readU32(rd)
		targetSize := readU32(rd)
		if int(targetSize) > len(data)-24 {
			return nil, fmt.Errorf("symlink target exceeds inode data")
		}
		ino.SymTarget = string(data[24 : 24+targetSize])
		// xattr index follows, skip

	case inodeBasicBlock, inodeBasicChar:
		ino.Nlinks = readU32(rd)
		// device number follows - store as fragIndex for reconstruction
		ino.FragIndex = readU32(rd)

	case inodeBasicFifo, inodeBasicSocket:
		ino.Nlinks = readU32(rd)

	default:
		return nil, fmt.Errorf("unsupported inode type: %d", ino.Type)
	}

	return ino, nil
}

func readBlockSizes(r *bytes.Reader, fileSize, blockSize uint64, hasFragment bool) []uint32 {
	var nBlocks int
	if hasFragment {
		nBlocks = int(fileSize / blockSize)
	} else {
		nBlocks = int((fileSize + blockSize - 1) / blockSize)
	}
	if nBlocks == 0 {
		return nil
	}
	sizes := make([]uint32, nBlocks)
	for i := range sizes {
		sizes[i] = readU32(r)
	}
	return sizes
}

// --- directory reading ---------------------------------------------------------

// dirEntry is a directory entry.
type dirEntry struct {
	Name     string
	InodeRef uint64
	Type     uint16
	InodeNum uint32
}

func (r *nativeReader) readDir(blockStart uint64, blockOffset uint16, dirSize uint32) ([]dirEntry, error) {
	// dirSize includes the +3 for . and ..
	actualSize := int(dirSize) - 3

if actualSize <= 0 {
		return nil, nil
	}

	data, err := r.readMetadataBlocks(r.sb.DirTableStart, blockStart, int(blockOffset), actualSize, r.sb.FragTableStart)
	if err != nil {
		return nil, fmt.Errorf("read dir blocks: %w", err)
	}

	var entries []dirEntry
	rd := bytes.NewReader(data)

	for {
		if rd.Len() < 12 {
			break
		}
		// directory header
		count := int(readU32(rd)) + 1 // off-by-one
		headerStart := readU32(rd)    // start (inode table block)
		baseInode := readU32(rd)

		for i := 0; i < count; i++ {
			if rd.Len() < 6 {
				return entries, nil
			}

			entryOffset := readU16(rd)
			inodeOff := int16(readU16(rd))
			entryType := readU16(rd)
			nameLen := int(readU16(rd)) + 1

			if rd.Len() < nameLen {
				return entries, fmt.Errorf("truncated directory entry name")
			}
			name := make([]byte, nameLen)
			_, _ = rd.Read(name)

			inodeNum := uint32(int32(baseInode) + int32(inodeOff))
			// reconstruct full 64-bit inode ref: upper 48 bits = block location, lower 16 = within-block offset
			ref := (uint64(headerStart) << 16) | uint64(entryOffset)

			entries = append(entries, dirEntry{
				Name:     string(name),
				InodeRef: ref,
				Type:     entryType,
				InodeNum: inodeNum,
			})
		}
	}

	return entries, nil
}

// --- file data reading ---------------------------------------------------------

// writeFileData streams decompressed file data to w without buffering the entire file in memory.
func (r *nativeReader) writeFileData(ino *inode, w io.Writer) error {
	if ino.FileSize == 0 {
		return nil
	}

	blockSize := uint64(r.sb.BlockSize)
	hasFragment := ino.FragIndex != 0xFFFFFFFF
	var totalBlocks int
	if hasFragment {
		totalBlocks = int(ino.FileSize / blockSize)
	} else {
		totalBlocks = int((ino.FileSize + blockSize - 1) / blockSize)
	}

	if len(ino.BlockSizes) != totalBlocks {
		return fmt.Errorf("block size count mismatch: have %d, want %d", len(ino.BlockSizes), totalBlocks)
	}

	pos := ino.StartBlock
	for i, sz := range ino.BlockSizes {
		compressed := (sz & (1 << 24)) == 0
		dataSize := sz & 0xFFFFFF

		var blockData []byte
		if dataSize == 0 {
			blockData = make([]byte, blockSize)
		} else {
			blockData = make([]byte, dataSize)
			if _, err := r.ra.ReadAt(blockData, int64(pos)); err != nil {
				return fmt.Errorf("read data block %d at %d (size %d): %w", i, pos, dataSize, err)
			}
			if compressed {
				var err error
				blockData, err = r.decompress(blockData)
				if err != nil {
					return fmt.Errorf("decompress data block %d at %d (size %d, compressed): %w", i, pos, dataSize, err)
				}
			}
		}

		pos += uint64(dataSize)

		if i == totalBlocks-1 && !hasFragment {
			remainder := ino.FileSize - uint64(i)*blockSize
			if uint64(len(blockData)) > remainder {
				blockData = blockData[:remainder]
			}
		}

		if _, err := w.Write(blockData); err != nil {
			return err
		}
	}

	if hasFragment && ino.FileSize%blockSize > 0 {
		fragData, err := r.readFragment(ino.FragIndex, ino.FragOffset, int(ino.FileSize%blockSize))
		if err != nil {
			return fmt.Errorf("read fragment: %w", err)
		}
		if _, err := w.Write(fragData); err != nil {
			return err
		}
	}

	return nil
}

func (r *nativeReader) readFileData(ino *inode) ([]byte, error) {
	if ino.FileSize == 0 {
		return nil, nil
	}

	blockSize := uint64(r.sb.BlockSize)
	var buf bytes.Buffer
	buf.Grow(int(ino.FileSize))

	hasFragment := ino.FragIndex != 0xFFFFFFFF
	var totalBlocks int
	if hasFragment {
		totalBlocks = int(ino.FileSize / blockSize)
	} else {
		totalBlocks = int((ino.FileSize + blockSize - 1) / blockSize)
	}

	if len(ino.BlockSizes) != totalBlocks {
		return nil, fmt.Errorf("block size count mismatch: have %d, want %d", len(ino.BlockSizes), totalBlocks)
	}

	pos := ino.StartBlock
	for i, sz := range ino.BlockSizes {
		compressed := (sz & (1 << 24)) == 0
		dataSize := sz & 0xFFFFFF

		var blockData []byte
		if dataSize == 0 {
			// sparse: all zeros
			blockData = make([]byte, blockSize)
		} else {
			blockData = make([]byte, dataSize)
			if _, err := r.ra.ReadAt(blockData, int64(pos)); err != nil {
				return nil, fmt.Errorf("read data block %d at %d: %w", i, pos, err)
			}
			if compressed {
				var err error
				blockData, err = r.decompress(blockData)
				if err != nil {
					return nil, fmt.Errorf("decompress data block %d: %w", i, err)
				}
			}
		}

		pos += uint64(dataSize)

		// trim last block to actual file remainder
		if i == totalBlocks-1 && !hasFragment {
			remainder := ino.FileSize - uint64(i)*blockSize
			if uint64(len(blockData)) > remainder {
				blockData = blockData[:remainder]
			}
		}

		buf.Write(blockData)
	}

	// read fragment tail
	if hasFragment && ino.FileSize%blockSize > 0 {
		fragData, err := r.readFragment(ino.FragIndex, ino.FragOffset, int(ino.FileSize%blockSize))
		if err != nil {
			return nil, fmt.Errorf("read fragment: %w", err)
		}
		buf.Write(fragData)
	}

	return buf.Bytes(), nil
}

func (r *nativeReader) readFragment(index uint32, offset uint32, length int) ([]byte, error) {
	// fragment table: lookup table of 16-byte entries
	// stored as metadata blocks (512 entries per 8192-byte block)
	entriesPerBlock := 8192 / 16 // 512
	blockIdx := int(index) / entriesPerBlock
	entryOff := (int(index) % entriesPerBlock) * 16

	// read the fragment table location list
	locListSize := int((r.sb.FragmentCount*16 + 8191) / 8192)
	locList := make([]uint64, locListSize)
	locBuf := make([]byte, locListSize*8)
	if _, err := r.ra.ReadAt(locBuf, int64(r.sb.FragTableStart)); err != nil {
		return nil, fmt.Errorf("read frag location list: %w", err)
	}
	for i := range locList {
		locList[i] = binary.LittleEndian.Uint64(locBuf[i*8:])
	}

	// locList entries are *absolute* offsets into the squashfs file
	// pointing at the metadata block holding the fragment entry. don't
	// add FragTableStart here -- doing so walks past the actual block
	// and chains hit unexpected EOF reading the next "metadata block"
	// header from random file bytes. only the entry at `entryOff` is
	// needed (16 bytes), but read enough to cover any offset within
	// the standard 8KB metadata block.
	blockData, err := r.readMetadataBlocks(0, locList[blockIdx], 0, entryOff+16, 0)
	if err != nil {
		return nil, fmt.Errorf("read frag block: %w", err)
	}
	if len(blockData) < entryOff+16 {
		return nil, fmt.Errorf("fragment entry out of bounds")
	}

	fragStart := binary.LittleEndian.Uint64(blockData[entryOff:])
	fragSize := binary.LittleEndian.Uint32(blockData[entryOff+8:])
	compressed := (fragSize & (1 << 24)) == 0
	dataSize := fragSize & 0xFFFFFF

	fragData := make([]byte, dataSize)
	if _, err := r.ra.ReadAt(fragData, int64(fragStart)); err != nil {
		return nil, fmt.Errorf("read fragment data: %w", err)
	}
	if compressed {
		fragData, err = r.decompress(fragData)
		if err != nil {
			return nil, fmt.Errorf("decompress fragment: %w", err)
		}
	}

	if offset+uint32(length) > uint32(len(fragData)) {
		return nil, fmt.Errorf("fragment offset/length out of bounds")
	}
	return fragData[offset : offset+uint32(length)], nil
}

// --- walk ----------------------------------------------------------------------

const maxWalkDepth = 1000

func (r *nativeReader) walkDir(inodeRef uint64, basePath string, walkFn filepath.WalkFunc) error {
	ino, err := r.readInode(inodeRef)
	if err != nil {
		return walkFn(basePath, nil, err)
	}
	return r.walkDirInode(ino, basePath, walkFn, 0)
}

func (r *nativeReader) walkDirInode(ino *inode, basePath string, walkFn filepath.WalkFunc, depth int) error {
	if depth > maxWalkDepth {
		return fmt.Errorf("walk depth exceeded at %s", basePath)
	}
	if ino.Type != inodeBasicDir && ino.Type != inodeExtDir {
		return fmt.Errorf("expected directory inode, got type %d", ino.Type)
	}

	entries, err := r.readDir(ino.DirStart, ino.DirOffset, ino.DirSize)
	if err != nil {
		return walkFn(basePath, nil, err)
	}

	for _, e := range entries {
		p := filepath.Join(basePath, e.Name)
		entryIno, err := r.readInode(e.InodeRef)
		if err != nil {
			if err2 := walkFn(p, nil, err); err2 != nil {
				return err2
			}
			continue
		}

		st := inodeToFileInfo(e.Name, entryIno)

		if err := walkFn(p, st, nil); err != nil {
			if err == filepath.SkipDir && entryIno.IsDir() {
				continue
			}
			return err
		}

		if entryIno.IsDir() {
			if err := r.walkDirInode(entryIno, p, walkFn, depth+1); err != nil {
				return err
			}
		}
	}

	return nil
}

// --- extraction ----------------------------------------------------------------

// fileToExtract holds the info needed to extract a single file.
type fileToExtract struct {
	path     string
	destPath string
	ino      *inode
	mode     os.FileMode
}

func (r *nativeReader) extractAll(dest string) error {
	// Phase 1: walk the tree, create dirs/symlinks, collect files
	var files []fileToExtract
	err := r.walkDir(r.sb.RootInodeRef, ".", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		destPath := filepath.Join(dest, path)
		ino := info.Sys().(*inode)

		switch {
		case info.IsDir():
			if err := os.MkdirAll(destPath, 0755); err != nil {
				return err
			}
		case info.Mode()&os.ModeSymlink != 0:
			if err := os.Symlink(ino.SymTarget, destPath); err != nil {
				return err
			}
		case info.Mode().IsRegular():
			files = append(files, fileToExtract{path, destPath, ino, info.Mode()})
		}
		return nil
	})
	if err != nil {
		return err
	}

	// Phase 2: extract files in parallel
	return r.extractFiles(files)
}

// extractFiles extracts files using a worker pool for parallel decompression.
func (r *nativeReader) extractFiles(files []fileToExtract) error {
	if len(files) == 0 {
		return nil
	}

	workers := 4
	if len(files) < workers {
		workers = len(files)
	}

	type workResult struct {
		index int
		err   error
	}

	jobs := make(chan int, len(files))
	results := make(chan workResult, len(files))

	for w := 0; w < workers; w++ {
		go func() {
			for idx := range jobs {
				f := &files[idx]
				out, err := os.OpenFile(f.destPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.mode)
				if err != nil {
					results <- workResult{idx, err}
					continue
				}
				if err := r.writeFileData(f.ino, out); err != nil {
					out.Close()
					results <- workResult{idx, fmt.Errorf("extract %s: %w", f.path, err)}
					continue
				}
				if err := out.Close(); err != nil {
					results <- workResult{idx, err}
					continue
				}
				results <- workResult{idx, nil}
			}
		}()
	}

	for i := range files {
		jobs <- i
	}
	close(jobs)

	var firstErr error
	for range files {
		res := <-results
		if res.err != nil && firstErr == nil {
			firstErr = res.err
		}
	}
	return firstErr
}

// extractMatching extracts files matching a glob pattern relative to root.
func (r *nativeReader) extractMatching(pattern, dest string) error {
	// build a set of paths matching the pattern
	matches, err := r.globMatches(pattern)
	if err != nil {
		return err
	}

	for _, match := range matches {
		destPath := filepath.Join(dest, match)
		ino, err := r.resolvePath(match)
		if err != nil {
			return fmt.Errorf("resolve %s: %w", match, err)
		}

		info := inodeToFileInfo(filepath.Base(match), ino)

		if info.IsDir() {
			if err := r.extractDir(ino, destPath); err != nil {
				return err
			}
		} else if info.Mode()&os.ModeSymlink != 0 {
			if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
				return err
			}
			if err := os.Symlink(ino.SymTarget, destPath); err != nil {
				return err
			}
		} else if info.Mode().IsRegular() {
			if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
				return err
			}
			f, err := os.OpenFile(destPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, info.Mode())
			if err != nil {
				return err
			}
			if err := r.writeFileData(ino, f); err != nil {
				f.Close()
				return fmt.Errorf("extract %s: %w", match, err)
			}
			if err := f.Close(); err != nil {
				return err
			}
		}
	}
	return nil
}

func (r *nativeReader) extractDir(ino *inode, dest string) error {
	if err := os.MkdirAll(dest, 0755); err != nil {
		return err
	}
	entries, err := r.readDir(ino.DirStart, ino.DirOffset, ino.DirSize)
	if err != nil {
		return err
	}
	for _, e := range entries {
		entryIno, err := r.readInode(e.InodeRef)
		if err != nil {
			return err
		}
		destPath := filepath.Join(dest, e.Name)
		info := inodeToFileInfo(e.Name, entryIno)
		if info.IsDir() {
			if err := r.extractDir(entryIno, destPath); err != nil {
				return err
			}
		} else if info.Mode()&os.ModeSymlink != 0 {
			if err := os.Symlink(entryIno.SymTarget, destPath); err != nil {
				return err
			}
		} else if info.Mode().IsRegular() {
			data, err := r.readFileData(entryIno)
			if err != nil {
				return err
			}
			if err := os.WriteFile(destPath, data, info.Mode()); err != nil {
				return err
			}
		}
	}
	return nil
}

func (r *nativeReader) globMatches(pattern string) ([]string, error) {
	// handle simple cases
	if pattern == "*" || pattern == "." {
		// match everything
		var paths []string
		err := r.walkDir(r.sb.RootInodeRef, "", func(p string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			paths = append(paths, p)
			return nil
		})
		return paths, err
	}

	// for specific paths, just return them
	return []string{pattern}, nil
}

// --- path resolution -----------------------------------------------------------

func (r *nativeReader) resolvePath(path string) (*inode, error) {
	parts := splitPath(path)
	ref := r.sb.RootInodeRef

	for _, part := range parts {
		ino, err := r.readInode(ref)
		if err != nil {
			return nil, err
		}
		if !ino.IsDir() {
			return nil, fmt.Errorf("not a directory in path: %s", part)
		}
		entries, err := r.readDir(ino.DirStart, ino.DirOffset, ino.DirSize)
		if err != nil {
			return nil, err
		}

		found := false
		for _, e := range entries {
			if e.Name == part {
				ref = e.InodeRef
				found = true
				break
			}
		}
		if !found {
			return nil, os.ErrNotExist
		}
	}

	return r.readInode(ref)
}

func splitPath(p string) []string {
	p = filepath.Clean(p)
	if p == "." || p == "/" || p == "" {
		return nil
	}
	// trim leading ./
	p = strings.TrimPrefix(p, "./")
	p = strings.TrimPrefix(p, "/")
	return strings.Split(p, "/")
}

// --- file info conversion ------------------------------------------------------

// inodeFileInfo implements os.FileInfo for a squashfs inode.
type inodeFileInfo struct {
	name    string
	size    int64
	mode    os.FileMode
	modTime time.Time
	ino     *inode
}

func (fi *inodeFileInfo) Name() string       { return fi.name }
func (fi *inodeFileInfo) Size() int64        { return fi.size }
func (fi *inodeFileInfo) Mode() os.FileMode  { return fi.mode }
func (fi *inodeFileInfo) ModTime() time.Time { return fi.modTime }
func (fi *inodeFileInfo) IsDir() bool        { return fi.mode.IsDir() }
func (fi *inodeFileInfo) Sys() any           { return fi.ino }

func inodeToFileInfo(name string, ino *inode) os.FileInfo {
	fi := &inodeFileInfo{
		name:    name,
		size:    int64(ino.FileSize),
		modTime: time.Unix(int64(ino.ModTime), 0),
		ino:     ino,
	}

	perm := fs.FileMode(ino.Perm & 0xFFF)

	switch ino.Type {
	case inodeBasicDir, inodeExtDir:
		fi.mode = os.ModeDir | perm
	case inodeBasicSymlink, inodeExtSymlink:
		fi.mode = os.ModeSymlink | 0777
	case inodeBasicBlock:
		fi.mode = os.ModeDevice | perm
	case inodeBasicChar:
		fi.mode = os.ModeCharDevice | perm
	case inodeBasicFifo:
		fi.mode = os.ModeNamedPipe | perm
	case inodeBasicSocket:
		fi.mode = os.ModeSocket | perm
	default:
		fi.mode = perm
	}

	return fi
}

// --- inode helpers -------------------------------------------------------------

func (ino *inode) IsDir() bool {
	return ino.Type == inodeBasicDir || ino.Type == inodeExtDir
}

func (ino *inode) IsSymlink() bool {
	return ino.Type == inodeBasicSymlink || ino.Type == inodeExtSymlink
}

func (ino *inode) IsRegular() bool {
	return ino.Type == inodeBasicFile || ino.Type == inodeExtFile
}

// --- binary helpers ------------------------------------------------------------

func readU16(r *bytes.Reader) uint16 {
	var buf [2]byte
	_, _ = r.Read(buf[:])
	return binary.LittleEndian.Uint16(buf[:])
}

func readU32(r *bytes.Reader) uint32 {
	var buf [4]byte
	_, _ = r.Read(buf[:])
	return binary.LittleEndian.Uint32(buf[:])
}

func readU64(r *bytes.Reader) uint64 {
	var buf [8]byte
	_, _ = r.Read(buf[:])
	return binary.LittleEndian.Uint64(buf[:])
}

func readU16At(ra io.ReaderAt, off uint64) uint16 {
	var buf [2]byte
	_, _ = ra.ReadAt(buf[:], int64(off))
	return binary.LittleEndian.Uint16(buf[:])
}

// --- top-level API -------------------------------------------------------------

// newNativeReader opens the given file and returns a native squashfs reader.
func newNativeReader(path string) (*nativeReader, func() error, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}

	info, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, nil, err
	}

	sb, err := readSuperblock(f)
	if err != nil {
		f.Close()
		return nil, nil, err
	}

	return &nativeReader{ra: f, sb: sb, size: info.Size()}, f.Close, nil
}

func (r *nativeReader) ModTime() time.Time {
	return time.Unix(int64(r.sb.ModTime), 0)
}
