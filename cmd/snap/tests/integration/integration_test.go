package integration

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// snapBin is the snap binary under test; set in TestMain.
var snapBin string

func TestMain(m *testing.M) {
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, "getwd:", err)
		os.Exit(1)
	}
	snapBin = filepath.Clean(filepath.Join(cwd, "../../../../bin/snap"))
	if _, err := os.Stat(snapBin); err != nil {
		fmt.Fprintln(os.Stderr, "bin/snap not found -- run 'make build' first")
		os.Exit(1)
	}
	os.Exit(m.Run())
}

// snapCmd constructs a snap subprocess with SNAP_ROOT and SNAP_SELF set.
func snapCmd(snapRoot string, args ...string) *exec.Cmd {
	cmd := exec.Command(snapBin, args...)
	cmd.Env = append(os.Environ(),
		"SNAP_ROOT="+snapRoot,
		"SNAP_SELF="+snapBin,
	)
	return cmd
}

// run runs a snap subcommand and returns combined output. fails t on non-zero exit.
func run(t *testing.T, snapRoot string, args ...string) string {
	t.Helper()
	cmd := snapCmd(snapRoot, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("snap %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return string(out)
}

// runExpectFail runs a snap subcommand and returns combined output.
// fails t if the command exits 0.
func runExpectFail(t *testing.T, snapRoot string, args ...string) string {
	t.Helper()
	cmd := snapCmd(snapRoot, args...)
	out, _ := cmd.CombinedOutput()
	if cmd.ProcessState != nil && cmd.ProcessState.ExitCode() == 0 {
		t.Fatalf("snap %s: expected non-zero exit, got 0\n%s", strings.Join(args, " "), out)
	}
	return string(out)
}

// runQ runs a snap subcommand silently -- used in cleanup paths where
// failure is acceptable (e.g. stopping a daemon that may already be dead).
func runQ(snapRoot string, args ...string) {
	snapCmd(snapRoot, args...).Run() //nolint:errcheck
}

// fixture returns the path to a prebuilt .snap file in testdata/.
func fixture(name string) string {
	return filepath.Join("testdata", name+".snap")
}

// newRoot creates a per-test temp dir to use as SNAP_ROOT.
func newRoot(t *testing.T) string {
	t.Helper()
	return t.TempDir()
}

// waitForFile polls path until it appears or timeout expires.
func waitForFile(path string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return true
		}
		time.Sleep(50 * time.Millisecond)
	}
	return false
}
