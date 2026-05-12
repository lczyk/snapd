// Copyright 2026 Canonical Ltd
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License version 3 as
// published by the Free Software Foundation.

package xz_test

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/snapcore/snapd/snap/squashfs/xz"
)

// TestGoldenVectors decodes each canonical .xz fixture under
// testdata/golden/ and verifies it matches the paired .txt
// plaintext. The fixtures were generated with the system xz(1)
// CLI (xz-utils 5.8.1) -- they're the reference implementation's
// output, so this test guards against silent drift in our
// decoder versus the spec.
//
// IMPORTANT: golden vectors decouple correctness from the
// upstream Go encoder we use elsewhere (compressXZ, etc.). every
// other roundtrip in the suite uses ulikunitz/xz to *generate*
// the fixture each run; if upstream's writer ever changes its
// output shape we wouldn't notice because the same code paths
// produce and consume the bytes. these fixtures are committed
// binary and don't move -- failure here means our decoder
// diverged from the xz reference.
//
// To add a new vector: drop `name.txt` + `name.xz` into
// testdata/golden/. The test picks them up automatically.
func TestGoldenVectors(t *testing.T) {
	const dir = "testdata/golden"
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read golden dir: %v", err)
	}
	matched := 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".xz") {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".xz")
		t.Run(name, func(t *testing.T) {
			compressed, err := os.ReadFile(filepath.Join(dir, name+".xz"))
			if err != nil {
				t.Fatalf("read .xz: %v", err)
			}
			want, err := os.ReadFile(filepath.Join(dir, name+".txt"))
			if err != nil {
				t.Fatalf("read .txt: %v", err)
			}
			r, err := xz.NewReader(bytes.NewReader(compressed))
			if err != nil {
				t.Fatalf("NewReader: %v", err)
			}
			got, err := io.ReadAll(r)
			if err != nil {
				t.Fatalf("ReadAll: %v", err)
			}
			if !bytes.Equal(got, want) {
				t.Fatalf("decode mismatch: got %d bytes, want %d; first diff at %d",
					len(got), len(want), firstDiff(got, want))
			}
		})
		matched++
	}
	if matched == 0 {
		t.Fatal("no golden vectors found -- testdata/golden empty?")
	}
}
