package main

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/snapcore/snapd/snap"
)

// -- shouldRestart --

func TestShouldRestart(t *testing.T) {
	cases := []struct {
		cond snap.RestartCondition
		code int
		want bool
	}{
		{snap.RestartAlways, 0, true},
		{snap.RestartAlways, 1, true},
		{snap.RestartNever, 0, false},
		{snap.RestartNever, 1, false},
		{snap.RestartOnFailure, 0, false},
		{snap.RestartOnFailure, 1, true},
		{snap.RestartOnFailure, 2, true},
		{snap.RestartOnSuccess, 0, true},
		{snap.RestartOnSuccess, 1, false},
		// unknown condition: should not restart
		{snap.RestartCondition("bogus"), 0, false},
		{snap.RestartCondition("bogus"), 1, false},
	}
	for _, tc := range cases {
		t.Run(fmt.Sprintf("%s/code=%d", tc.cond, tc.code), func(t *testing.T) {
			got := shouldRestart(tc.cond, tc.code)
			if got != tc.want {
				t.Errorf("shouldRestart(%q, %d) = %v, want %v", tc.cond, tc.code, got, tc.want)
			}
		})
	}
}

// -- resolveRestartCond --

func TestResolveRestartCond(t *testing.T) {
	cases := []struct {
		in   snap.RestartCondition
		want snap.RestartCondition
	}{
		// supported: pass through unchanged
		{snap.RestartAlways, snap.RestartAlways},
		{snap.RestartNever, snap.RestartNever},
		{snap.RestartOnFailure, snap.RestartOnFailure},
		{snap.RestartOnSuccess, snap.RestartOnSuccess},
		// empty (snap.yaml omitted restart:): default to on-failure
		{"", snap.RestartOnFailure},
		// unsupported: fall back to on-failure
		{snap.RestartOnAbnormal, snap.RestartOnFailure},
		{snap.RestartOnAbort, snap.RestartOnFailure},
		{snap.RestartOnWatchdog, snap.RestartOnFailure},
	}
	for _, tc := range cases {
		t.Run(string(tc.in), func(t *testing.T) {
			got := resolveRestartCond(tc.in, "testsnap", "daemon")
			if got != tc.want {
				t.Errorf("resolveRestartCond(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// -- exitCode --

func TestExitCode(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want int
	}{
		{"nil error", nil, 0},
		{"exit 0", &exec.ExitError{ProcessState: mustExitState(0)}, 0},
		{"exit 1", &exec.ExitError{ProcessState: mustExitState(1)}, 1},
		{"exit 42", &exec.ExitError{ProcessState: mustExitState(42)}, 42},
		// non-ExitError: unknown exit code
		{"other error", fmt.Errorf("something went wrong"), -1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := exitCode(tc.err)
			if got != tc.want {
				t.Errorf("exitCode(%v) = %d, want %d", tc.err, got, tc.want)
			}
		})
	}
}

// mustExitState returns a ProcessState with the given exit code by
// running a trivial subprocess.
func mustExitState(code int) *os.ProcessState {
	cmd := exec.Command("sh", "-c", fmt.Sprintf("exit %d", code))
	_ = cmd.Run()
	return cmd.ProcessState
}

// -- IPC handler (in-process via net.Pipe) --

func TestHandleIPCConn(t *testing.T) {
	t.Run("status", func(t *testing.T) {
		stopCh := make(chan struct{})
		restartCh := make(chan struct{}, 1)
		resp := sendIPCOver(t, "status", stopCh, restartCh)
		if !resp.OK {
			t.Fatalf("status: OK=false, error=%q", resp.Error)
		}
	})

	t.Run("restart queued", func(t *testing.T) {
		stopCh := make(chan struct{})
		restartCh := make(chan struct{}, 1)
		resp := sendIPCOver(t, "restart", stopCh, restartCh)
		if !resp.OK {
			t.Fatalf("restart: OK=false, error=%q", resp.Error)
		}
		if len(restartCh) != 1 {
			t.Fatal("restart: expected one entry in restartCh")
		}
	})

	t.Run("restart already pending", func(t *testing.T) {
		stopCh := make(chan struct{})
		restartCh := make(chan struct{}, 1)
		restartCh <- struct{}{} // pre-fill
		resp := sendIPCOver(t, "restart", stopCh, restartCh)
		if resp.OK {
			t.Fatal("restart with full channel: expected OK=false")
		}
	})

	t.Run("stop closes stopCh", func(t *testing.T) {
		stopCh := make(chan struct{})
		restartCh := make(chan struct{}, 1)
		resp := sendIPCOver(t, "stop", stopCh, restartCh)
		if !resp.OK {
			t.Fatalf("stop: OK=false, error=%q", resp.Error)
		}
		// handler sends ack then closes stopCh in the same goroutine;
		// give it a moment to schedule past the send.
		select {
		case <-stopCh:
			// good
		case <-time.After(100 * time.Millisecond):
			t.Fatal("stop: stopCh was not closed within 100ms")
		}
	})

	t.Run("unknown command", func(t *testing.T) {
		stopCh := make(chan struct{})
		restartCh := make(chan struct{}, 1)
		resp := sendIPCOver(t, "explode", stopCh, restartCh)
		if resp.OK {
			t.Fatal("unknown command: expected OK=false")
		}
	})
}

// sendIPCOver runs handleIPCConn in a goroutine over a net.Pipe pair,
// sends the given command, and returns the response.
func sendIPCOver(t *testing.T, command string, stopCh chan struct{}, restartCh chan struct{}) ipcResponse {
	t.Helper()
	client, server := net.Pipe()
	defer client.Close()

	go handleIPCConn(server, stopCh, restartCh)

	if err := json.NewEncoder(client).Encode(ipcRequest{Command: command}); err != nil {
		t.Fatalf("encode request: %v", err)
	}
	var resp ipcResponse
	if err := json.NewDecoder(client).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return resp
}
