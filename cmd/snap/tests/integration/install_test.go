package integration

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInstallMinimal(t *testing.T) {
	root := newRoot(t)
	run(t, root, "install", fixture("test-minimal"))

	// snap.yaml must be extracted under current/
	yaml := filepath.Join(root, "snap", "test-minimal", "current", "meta", "snap.yaml")
	if _, err := os.Stat(yaml); err != nil {
		t.Fatalf("snap.yaml missing after install: %v", err)
	}

	// current symlink must resolve to a revision
	target, err := os.Readlink(filepath.Join(root, "snap", "test-minimal", "current"))
	if err != nil {
		t.Fatalf("current symlink missing: %v", err)
	}
	if target != "x1" {
		t.Errorf("current -> %q, want x1", target)
	}

	// no apps declared, so no shims under snap/bin/
	_, err = os.Stat(filepath.Join(root, "snap", "bin", "test-minimal"))
	if err == nil {
		t.Error("unexpected shim for snap with no apps")
	}
}

func TestInstallApp(t *testing.T) {
	root := newRoot(t)
	run(t, root, "install", fixture("test-app"))

	yaml := filepath.Join(root, "snap", "test-app", "current", "meta", "snap.yaml")
	if _, err := os.Stat(yaml); err != nil {
		t.Fatalf("snap.yaml missing: %v", err)
	}

	// app shim must be a symlink pointing at the snap binary
	shim := filepath.Join(root, "snap", "bin", "test-app")
	target, err := os.Readlink(shim)
	if err != nil {
		t.Fatalf("shim symlink missing at %s: %v", shim, err)
	}
	if target != snapBin {
		t.Errorf("shim -> %q, want %q", target, snapBin)
	}
}

func TestInstallBase(t *testing.T) {
	root := newRoot(t)
	run(t, root, "install", fixture("test-base"))

	yaml := filepath.Join(root, "snap", "test-base", "current", "meta", "snap.yaml")
	if _, err := os.Stat(yaml); err != nil {
		t.Fatalf("snap.yaml missing: %v", err)
	}

	// base snaps have no apps, no shim
	_, err := os.Stat(filepath.Join(root, "snap", "bin", "test-base"))
	if err == nil {
		t.Error("unexpected shim for base snap")
	}
}

func TestInstallMultiApp(t *testing.T) {
	root := newRoot(t)
	run(t, root, "install", fixture("test-multi"))

	// both apps get <snap>.<app> shims
	for _, app := range []string{"first", "second"} {
		shim := filepath.Join(root, "snap", "bin", "test-multi."+app)
		if _, err := os.Readlink(shim); err != nil {
			t.Errorf("shim for %q missing: %v", app, err)
		}
	}
}

func TestInstallBaseNone(t *testing.T) {
	root := newRoot(t)
	run(t, root, "install", fixture("test-base-none"))

	yaml := filepath.Join(root, "snap", "test-base-none", "current", "meta", "snap.yaml")
	if _, err := os.Stat(yaml); err != nil {
		t.Fatalf("snap.yaml missing: %v", err)
	}

	shim := filepath.Join(root, "snap", "bin", "test-base-none")
	if _, err := os.Readlink(shim); err != nil {
		t.Fatalf("shim missing: %v", err)
	}
}

func TestInstallIdempotent(t *testing.T) {
	root := newRoot(t)
	run(t, root, "install", fixture("test-app"))
	run(t, root, "install", fixture("test-app"))

	// still only one revision on disk after double install
	entries, err := os.ReadDir(filepath.Join(root, "snap", "test-app"))
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	// expect: current (symlink) + x1 (dir) = 2 entries
	if len(entries) != 2 {
		t.Errorf("expected 2 entries (current + x1), got %d", len(entries))
	}
}
