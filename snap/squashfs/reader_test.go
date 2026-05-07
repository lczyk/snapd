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
	"encoding/binary"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestReadSuperblock(t *testing.T) {
	// Build minimal superblock (magic + valid fields).
	buf := make([]byte, 96)
	put32 := func(off int, v uint32) { binary.LittleEndian.PutUint32(buf[off:], v) }
	put16 := func(off int, v uint16) { binary.LittleEndian.PutUint16(buf[off:], v) }
	put64 := func(off int, v uint64) { binary.LittleEndian.PutUint64(buf[off:], v) }

	put32(0, 0x73717368)               // magic
	put32(4, 11)                        // inode_count
	put32(8, 1555555555)                // mod_time
	put32(12, 131072)                   // block_size
	put32(16, 0)                        // frag_count
	put16(20, compXz)                   // compression
	put16(22, 17)                       // block_log
	put16(24, 0x0010)                   // flags: no fragments
	put16(26, 0)                        // id_count
	put16(28, 4)                        // version_major
	put16(30, 0)                        // version_minor
	put64(32, 0x000000010000)           // root_inode_ref
	put64(40, 4096)                     // bytes_used
	put64(48, 0xFFFFFFFFFFFFFFFF)       // id_table
	put64(56, 0xFFFFFFFFFFFFFFFF)       // xattr_table
	put64(64, 4096)                     // inode_table
	put64(72, 8192)                     // dir_table
	put64(80, 0xFFFFFFFFFFFFFFFF)       // frag_table
	put64(88, 0xFFFFFFFFFFFFFFFF)       // export_table

	ra := bytes.NewReader(buf)
	sb, err := readSuperblock(ra)
	if err != nil {
		t.Fatalf("readSuperblock: %v", err)
	}
	if sb.InodeCount != 11 {
		t.Errorf("InodeCount = %d, want 11", sb.InodeCount)
	}
	if sb.BlockSize != 131072 {
		t.Errorf("BlockSize = %d, want 131072", sb.BlockSize)
	}
	if sb.Compression != compXz {
		t.Errorf("Compression = %d, want %d", sb.Compression, compXz)
	}
	if sb.VersionMajor != 4 || sb.VersionMinor != 0 {
		t.Errorf("Version = %d.%d, want 4.0", sb.VersionMajor, sb.VersionMinor)
	}
	if sb.RootInodeRef != 0x000000010000 {
		t.Errorf("RootInodeRef = 0x%x, want 0x10000", sb.RootInodeRef)
	}
}

func TestReadSuperblockBadMagic(t *testing.T) {
	ra := bytes.NewReader(make([]byte, 96))
	_, err := readSuperblock(ra)
	if err == nil {
		t.Fatal("expected error for bad magic")
	}
}

func TestSplitPath(t *testing.T) {
	tests := []struct{ in string; want []string }{
		{".", nil},
		{"/", nil},
		{"", nil},
		{"foo", []string{"foo"}},
		{"foo/bar", []string{"foo", "bar"}},
		{"./foo", []string{"foo"}},
		{"/foo/bar", []string{"foo", "bar"}},
	}
	for _, tt := range tests {
		got := splitPath(tt.in)
		if len(got) != len(tt.want) {
			t.Errorf("splitPath(%q) = %v, want %v", tt.in, got, tt.want)
			continue
		}
		for i := range got {
			if got[i] != tt.want[i] {
				t.Errorf("splitPath(%q) = %v, want %v", tt.in, got, tt.want)
				break
			}
		}
	}
}

// TestReaderWithRealSnap tests the native reader against a real snap file created by mksquashfs.
func TestReaderWithRealSnap(t *testing.T) {
	// Check if mksquashfs is available
	if _, err := exec.LookPath("mksquashfs"); err != nil {
		t.Skip("mksquashfs not available")
	}

	tmpDir := t.TempDir()

	// Create a minimal snap content
	contentDir := filepath.Join(tmpDir, "content")
	metaDir := filepath.Join(contentDir, "meta")
	binDir := filepath.Join(contentDir, "bin")
	for _, d := range []string{metaDir, binDir} {
		if err := os.MkdirAll(d, 0755); err != nil {
			t.Fatal(err)
		}
	}

	// Write meta/snap.yaml
	if err := os.WriteFile(filepath.Join(metaDir, "snap.yaml"), []byte("name: test\nversion: 1.0\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Write a regular file
	if err := os.WriteFile(filepath.Join(binDir, "hello"), []byte("hello world\n"), 0755); err != nil {
		t.Fatal(err)
	}

	// Write a symlink
	if err := os.Symlink("hello", filepath.Join(binDir, "link")); err != nil {
		t.Fatal(err)
	}

	// Build snap with mksquashfs
	snapPath := filepath.Join(tmpDir, "test.snap")
	mksq := exec.Command("mksquashfs", contentDir, snapPath, "-noappend", "-comp", "xz", "-no-progress", "-all-root")
	if out, err := mksq.CombinedOutput(); err != nil {
		t.Fatalf("mksquashfs: %v\n%s", err, out)
	}
	// mksquashfs pads small images
	if st, _ := os.Stat(snapPath); st.Size() < 4096 {
		if err := os.WriteFile(snapPath, append(readSnapFile(t, snapPath), make([]byte, 4096)...), 0644); err != nil {
			t.Fatal(err)
		}
	}

	sn := New(snapPath)

	// Test BuildDate (should be non-zero)
	bd := sn.BuildDate()
	if bd.IsZero() {
		t.Error("BuildDate is zero")
	}
	t.Logf("BuildDate: %v", bd)

	// Test ListDir root
	rootEntries, err := sn.ListDir(".")
	if err != nil {
		t.Fatalf("ListDir(.): %v", err)
	}
	if len(rootEntries) < 2 {
		t.Errorf("ListDir(.) has %d entries, want >= 2: %v", len(rootEntries), rootEntries)
	}
	t.Logf("ListDir(.): %v", rootEntries)

	// Test ListDir bin
	binEntries, err := sn.ListDir("bin")
	if err != nil {
		t.Fatalf("ListDir(bin): %v", err)
	}
	t.Logf("ListDir(bin): %v", binEntries)

	// Test ReadFile
	data, err := sn.ReadFile("bin/hello")
	if err != nil {
		t.Fatalf("ReadFile(bin/hello): %v", err)
	}
	if string(data) != "hello world\n" {
		t.Errorf("ReadFile(bin/hello) = %q, want %q", string(data), "hello world\n")
	}

	// Test ReadLink
	target, err := sn.ReadLink("bin/link")
	if err != nil {
		t.Fatalf("ReadLink(bin/link): %v", err)
	}
	if target != "hello" {
		t.Errorf("ReadLink(bin/link) = %q, want %q", target, "hello")
	}

	// Test Walk
	count := 0
	err = sn.Walk(".", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		t.Logf("  %s mode=%s size=%d", path, info.Mode(), info.Size())
		count++
		return nil
	})
	if err != nil {
		t.Fatalf("Walk: %v", err)
	}
	if count < 4 {
		t.Errorf("Walk count = %d, want >= 4", count)
	}

	// Test Unpack
	destDir := filepath.Join(tmpDir, "unpack")
	if err := sn.Unpack("*", destDir); err != nil {
		t.Fatalf("Unpack: %v", err)
	}
	extracted, err := os.ReadFile(filepath.Join(destDir, "bin", "hello"))
	if err != nil {
		t.Fatalf("ReadFile on extracted: %v", err)
	}
	if string(extracted) != "hello world\n" {
		t.Errorf("extracted = %q, want %q", string(extracted), "hello world\n")
	}
	linkTarget, err := os.Readlink(filepath.Join(destDir, "bin", "link"))
	if err != nil {
		t.Fatalf("Readlink on extracted: %v", err)
	}
	if linkTarget != "hello" {
		t.Errorf("extracted link target = %q, want %q", linkTarget, "hello")
	}

	t.Log("All native reader tests with real snap passed!")
}

func readSnapFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
