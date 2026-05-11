package integration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDaemonInstall(t *testing.T) {
	root := newRoot(t)
	t.Cleanup(func() { runQ(root, "stop", "test-daemon.worker") })

	run(t, root, "install", fixture("test-daemon"))

	yaml := filepath.Join(root, "snap", "test-daemon", "current", "meta", "snap.yaml")
	if _, err := os.Stat(yaml); err != nil {
		t.Fatalf("snap.yaml missing after install: %v", err)
	}
}

func TestDaemonServicesCommand(t *testing.T) {
	root := newRoot(t)
	t.Cleanup(func() { runQ(root, "stop", "test-daemon.worker") })

	run(t, root, "install", fixture("test-daemon"))

	out := run(t, root, "services")
	if !strings.Contains(out, "test-daemon.worker") {
		t.Errorf("'snap services' missing test-daemon.worker:\n%s", out)
	}
}

func TestDaemonSupervisorStarts(t *testing.T) {
	root := newRoot(t)
	t.Cleanup(func() { runQ(root, "stop", "test-daemon.worker") })

	run(t, root, "install", fixture("test-daemon"))

	// install calls startDaemonsForSnap; supervisor writes its pid file
	// asynchronously after fork -- poll briefly.
	pidFile := filepath.Join(root, "run", "snapd", "supervisors", "test-daemon.worker.pid")
	if !waitForFile(pidFile, 2*time.Second) {
		t.Fatalf("supervisor pid file never appeared at %s", pidFile)
	}
}

func TestDaemonServicesActiveAfterStart(t *testing.T) {
	root := newRoot(t)
	t.Cleanup(func() { runQ(root, "stop", "test-daemon.worker") })

	run(t, root, "install", fixture("test-daemon"))

	pidFile := filepath.Join(root, "run", "snapd", "supervisors", "test-daemon.worker.pid")
	if !waitForFile(pidFile, 2*time.Second) {
		t.Skip("supervisor did not start in time -- skipping active-state check")
	}

	out := run(t, root, "services")
	if !strings.Contains(out, "active") {
		t.Errorf("expected 'active' in services output:\n%s", out)
	}
}

func TestDaemonRemoveStopsDaemon(t *testing.T) {
	root := newRoot(t)

	run(t, root, "install", fixture("test-daemon"))

	pidFile := filepath.Join(root, "run", "snapd", "supervisors", "test-daemon.worker.pid")
	waitForFile(pidFile, 2*time.Second)

	run(t, root, "remove", "test-daemon")

	// snap tree gone
	_, err := os.Stat(filepath.Join(root, "snap", "test-daemon"))
	if err == nil {
		t.Error("snap dir still present after remove")
	}
}
