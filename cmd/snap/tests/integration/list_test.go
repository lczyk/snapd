package integration

import (
	"strings"
	"testing"
)

func TestListEmpty(t *testing.T) {
	root := newRoot(t)
	out := run(t, root, "list")
	// no snaps installed; only the header line is printed
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 1 {
		t.Errorf("expected 1 header line, got %d lines:\n%s", len(lines), out)
	}
}

func TestListAfterInstall(t *testing.T) {
	root := newRoot(t)
	run(t, root, "install", fixture("test-app"))
	run(t, root, "install", fixture("test-minimal"))

	out := run(t, root, "list")

	if !strings.Contains(out, "test-app") {
		t.Errorf("list missing test-app:\n%s", out)
	}
	if !strings.Contains(out, "test-minimal") {
		t.Errorf("list missing test-minimal:\n%s", out)
	}
}

func TestListShowsRevision(t *testing.T) {
	root := newRoot(t)
	run(t, root, "install", fixture("test-app"))

	out := run(t, root, "list")

	if !strings.Contains(out, "x1") {
		t.Errorf("list does not show revision x1:\n%s", out)
	}
}
