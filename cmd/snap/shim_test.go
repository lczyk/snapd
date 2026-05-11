package main

import (
	"path/filepath"
	"testing"
)

func TestShimName(t *testing.T) {
	bin := filepath.Join(snapRoot, "snap", "bin")

	cases := []struct {
		invokedAs, want string
	}{
		// positive: in snapBinDir
		{filepath.Join(bin, "node"), "node"},
		{filepath.Join(bin, "go"), "go"},
		{filepath.Join(bin, "some-app.svc"), "some-app.svc"},

		// negative: wrong directory
		{"/usr/bin/snap", ""},
		{"/usr/local/bin/node", ""},

		// negative: bare name (regression -- used to panic)
		{"snap", ""},
		{"node", ""},

		// negative: empty
		{"", ""},
	}
	for _, tc := range cases {
		t.Run(tc.invokedAs, func(t *testing.T) {
			if got := shimName(tc.invokedAs); got != tc.want {
				t.Errorf("shimName(%q) = %q, want %q", tc.invokedAs, got, tc.want)
			}
		})
	}
}
