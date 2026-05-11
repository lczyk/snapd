package integration

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRemove(t *testing.T) {
	root := newRoot(t)
	run(t, root, "install", fixture("test-app"))
	run(t, root, "remove", "test-app")

	// snap tree must be gone
	_, err := os.Stat(filepath.Join(root, "snap", "test-app"))
	if err == nil {
		t.Error("snap dir still present after remove")
	}
}

func TestRemoveCleansShim(t *testing.T) {
	root := newRoot(t)
	run(t, root, "install", fixture("test-app"))

	shim := filepath.Join(root, "snap", "bin", "test-app")
	if _, err := os.Readlink(shim); err != nil {
		t.Fatalf("shim missing before remove: %v", err)
	}

	run(t, root, "remove", "test-app")

	if _, err := os.Lstat(shim); err == nil {
		t.Error("shim still present after remove")
	}
}

func TestRemoveCleansMultiAppShims(t *testing.T) {
	root := newRoot(t)
	run(t, root, "install", fixture("test-multi"))
	run(t, root, "remove", "test-multi")

	for _, app := range []string{"first", "second"} {
		shim := filepath.Join(root, "snap", "bin", "test-multi."+app)
		if _, err := os.Lstat(shim); err == nil {
			t.Errorf("shim for %q still present after remove", app)
		}
	}
}

func TestRemoveNotInstalled(t *testing.T) {
	root := newRoot(t)
	out := runExpectFail(t, root, "remove", "test-app")
	if out == "" {
		t.Error("expected error output for remove of uninstalled snap")
	}
}
