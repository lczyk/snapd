package integration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSideloadRevision(t *testing.T) {
	root := newRoot(t)
	run(t, root, "install", fixture("test-sideload"))

	// sideloaded snaps get xN revisions
	target, err := os.Readlink(filepath.Join(root, "snap", "test-sideload", "current"))
	if err != nil {
		t.Fatalf("current symlink missing: %v", err)
	}
	if target != "x1" {
		t.Errorf("first sideload: current -> %q, want x1", target)
	}
}

func TestSideloadSecondInstall(t *testing.T) {
	root := newRoot(t)
	run(t, root, "install", fixture("test-sideload"))
	run(t, root, "install", fixture("test-sideload"))

	// second sideload bumps revision to x2; x1 is pruned
	target, err := os.Readlink(filepath.Join(root, "snap", "test-sideload", "current"))
	if err != nil {
		t.Fatalf("current symlink missing: %v", err)
	}
	if target != "x2" {
		t.Errorf("second sideload: current -> %q, want x2", target)
	}

	// x1 must have been pruned
	_, err = os.Stat(filepath.Join(root, "snap", "test-sideload", "x1"))
	if err == nil {
		t.Error("old revision x1 still present after second install")
	}
}

func TestSideloadShim(t *testing.T) {
	root := newRoot(t)
	run(t, root, "install", fixture("test-sideload"))

	shim := filepath.Join(root, "snap", "bin", "test-sideload")
	shimTarget, err := os.Readlink(shim)
	if err != nil {
		t.Fatalf("shim missing: %v", err)
	}
	if shimTarget != snapBin {
		t.Errorf("shim -> %q, want %q", shimTarget, snapBin)
	}
}

func TestSideloadInstallOutput(t *testing.T) {
	root := newRoot(t)
	out := run(t, root, "install", fixture("test-sideload"))

	if !strings.Contains(out, "sideloaded") {
		t.Errorf("install output missing 'sideloaded': %q", out)
	}
}
