package main

import (
	"testing"
)

func TestResolveServiceTarget(t *testing.T) {
	cases := []struct {
		in      string
		snapOut string
		svcOut  string
	}{
		{"foo", "foo", ""},
		{"foo.bar", "foo", "bar"},
		{"foo.bar.baz", "foo", "bar.baz"}, // only first dot splits
		{".", "", ""},
		{".bar", "", "bar"},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			snap, svc := resolveServiceTarget(tc.in)
			if snap != tc.snapOut || svc != tc.svcOut {
				t.Errorf("resolveServiceTarget(%q) = (%q, %q), want (%q, %q)",
					tc.in, snap, svc, tc.snapOut, tc.svcOut)
			}
		})
	}
}

func TestParseServiceTarget(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		snap, svc, err := parseServiceTarget("foo.bar")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if snap != "foo" || svc != "bar" {
			t.Errorf("got (%q, %q), want (\"foo\", \"bar\")", snap, svc)
		}
	})

	t.Run("bare snap name rejected", func(t *testing.T) {
		if _, _, err := parseServiceTarget("foo"); err == nil {
			t.Fatal("expected error for bare snap name, got nil")
		}
	})

	t.Run("empty string rejected", func(t *testing.T) {
		if _, _, err := parseServiceTarget(""); err == nil {
			t.Fatal("expected error for empty string, got nil")
		}
	})
}

func TestSupervisorPaths(t *testing.T) {
	cases := []struct {
		snap, svc string
		wantPid   string
		wantSock  string
		wantLog   string
	}{
		{
			"foo", "bar",
			"/run/snapd/supervisors/foo.bar.pid",
			"/run/snapd/supervisors/foo.bar.sock",
			"/var/log/snapd/foo.bar.log",
		},
		{
			"my-snap", "my-daemon",
			"/run/snapd/supervisors/my-snap.my-daemon.pid",
			"/run/snapd/supervisors/my-snap.my-daemon.sock",
			"/var/log/snapd/my-snap.my-daemon.log",
		},
	}
	for _, tc := range cases {
		t.Run(tc.snap+"."+tc.svc, func(t *testing.T) {
			if got := supervisorPidPath(tc.snap, tc.svc); got != tc.wantPid {
				t.Errorf("pidPath = %q, want %q", got, tc.wantPid)
			}
			if got := supervisorSockPath(tc.snap, tc.svc); got != tc.wantSock {
				t.Errorf("sockPath = %q, want %q", got, tc.wantSock)
			}
			if got := supervisorLogPath(tc.snap, tc.svc); got != tc.wantLog {
				t.Errorf("logPath = %q, want %q", got, tc.wantLog)
			}
		})
	}
}
