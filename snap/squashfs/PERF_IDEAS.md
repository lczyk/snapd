# perf ideas (squashfs + xz fork)

remaining knobs after the round of pool/inline/Reset wins that got us to
~13 MB/s xz decode and WalkDir = 4534 allocs / 1.63 MB on the 100-file
xz fixture. ordered by estimated install-path impact.

## 1. specialised xz fast-path decoder

squashfs xz blocks are always the same shape: single-stream,
single-block, LZMA2 filter, CRC32 integrity check. the current path
goes through the general-purpose `xz.Reader` / `streamReader` /
`blockReader` chain (multi-stream w/ padding, multi-block, arbitrary
filter chains, configurable hash). everything but the actual LZMA2
decode is overhead for our use case.

idea: a `xz.DecodeSquashfsBlock(data []byte, outCap int) ([]byte, error)`
that bypasses the framing machinery:

- parse 12-byte stream header inline (verify magic + flags).
- parse block header inline (size byte, flags, sizes, filter list,
    padding, header CRC32).
- stream the LZMA2 payload directly through pooled `lzma.Reader2`.
- read block padding + block check + index + footer (or just skip).

~150 LOC. one alloc per block (the `Reader2` from pool). estimated
WalkDir ~ 4534 -> ~1000 allocs.

**risk**: must handle every valid block shape any kernel / mksquashfs
version emits, not just our test fixtures. needs broad differential
fuzz vs upstream before shipping.

## 2. directory-table cache

`walkDir` / `resolvePath` repeatedly decompress the same metadata
blocks during a single install scan. cache decompressed metadata
blocks by `(tableStart, blockStart)` key, LRU, capped at e.g. 32
entries (~256 KB ceiling).

cuts redundant work entirely on repeat lookups. big win for tree-walk
shapes that revisit the same directory chains. ~80 LOC, scoped to
`nativeReader`.

orthogonal to (1) -- both stack.

## 3. inode + dirEntry slice pool

`readInode` allocs `*inode` per call. `readDir` allocs `[]dirEntry`
sized per directory. wide-call, modest per-call cost; sync.Pool both.

- inode pool: trivial. sync.Pool of `*inode`, zero-out on Put.
- dirEntry slice pool: keyed loosely on capacity buckets so repeat
    dir walks reuse the slice; reset to `[:0]` on Put.

~50 LOC. estimated 200 - 400 allocs off WalkDir.

## 4. mmap the squashfs file

drop `io.ReaderAt` in favour of a single mmap'd `[]byte`. removes
per-`ReadAt` syscall + the small bounce buffers inside readers (the
metadata-buf pool would become unnecessary -- you'd slice directly
into the mmap region).

linux-easy, portable-harder (windows/darwin diverge). ~120 LOC + a
build-tag fallback for non-mmap platforms.

big I/O latency win for cold-cache reads. modest alloc win on top of
what (3) gets us. doesn't conflict w/ (1) or (2).

## 5. zero-copy metadata parsing

inode / dir entry parse currently wraps a `[]byte` in `bytes.NewReader`
and calls `readU16(r)` / `readU32(r)` / `readU64(r)` per field. each
`bytes.NewReader` allocs.

replace with direct `binary.LittleEndian.Uint16(buf[off:])` style. no
heap, stays on stack. mechanical, wide -- touches every parser
helper.

~150 LOC. several allocs / call saved, called many times during walk.

## 6. drop `io.Reader` wrapping in decompress

gzip / zstd / lzma readers in `decompress()` currently wrap `data` in
`bytes.NewReader`. for decoders that accept a slice directly (zstd has
`(*Decoder).DecodeAll`; lzo already takes `bytes.NewReader(data)` but
could be sliced; xz fork already preloads the chunk into a slice but
still wraps at the top), use the slice path.

~30 LOC. one alloc / decompress saved on the codec paths that support
it. compound w/ (1) for xz.

## stacked impact (rough estimate)

- baseline (current): WalkDir 4534 / 1.63 MB.
- + (3): ~4100 / ~1.5 MB.
- + (1): ~1000 / ~0.4 MB.
- + (2): ~400 / ~0.15 MB on repeat-walk shapes; first walk unchanged.
- + (5): another ~10 % off whatever's left.
- + (4): wall-clock win, alloc impact already mostly captured.

the order matters: (1) is the biggest single architectural win, but
(2) magnifies it on the install-loop shape that re-resolves paths.
(4) is independent and worth doing for cold-disk I/O regardless.

## tradeoff notes

- (1) and (4) are the only ones that need user-visible interface
    changes or build-tag gating. the others are mechanical refactors
    behind the existing internal API.
- (1) is the highest-risk for regression -- xz format is full of edge
    cases. budget time for a real fuzz campaign before shipping.
- (2) needs cache invalidation thought if the squashfs file is ever
    reopened / replaced mid-process.

## tldr;

(1) for the biggest single win. (1) + (2) for the install-perf push.
the rest is cleanup that compounds.
