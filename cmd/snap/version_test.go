package main

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"
)

func TestGoarchToSnap(t *testing.T) {
	cases := []struct {
		goarch, want string
	}{
		{"amd64", "amd64"},
		{"arm64", "arm64"},
		{"arm", "armhf"},
		{"386", "i386"},
		{"s390x", "s390x"},
		{"ppc64le", "ppc64el"},
		{"riscv64", "riscv64"},
		{"mips", "mips"}, // unknown: pass through as-is
	}
	for _, tc := range cases {
		t.Run(tc.goarch, func(t *testing.T) {
			if got := goarchToSnap(tc.goarch); got != tc.want {
				t.Errorf("goarchToSnap(%q) = %q, want %q", tc.goarch, got, tc.want)
			}
		})
	}
}

func TestOsReleasePretty(t *testing.T) {
	t.Run("parses id and version_id", func(t *testing.T) {
		f, err := os.CreateTemp(t.TempDir(), "os-release")
		if err != nil {
			t.Fatal(err)
		}
		f.WriteString("ID=ubuntu\nVERSION_ID=24.04\n")
		f.Close()

		// swap the open call by temporarily replacing the file path --
		// osReleasePretty is a thin wrapper; test the parsing directly
		// via a helper that accepts a path.
		got := parseOsRelease(f.Name())
		if got != "ubuntu 24.04" {
			t.Errorf("got %q, want \"ubuntu 24.04\"", got)
		}
	})

	t.Run("quoted values stripped", func(t *testing.T) {
		f, err := os.CreateTemp(t.TempDir(), "os-release")
		if err != nil {
			t.Fatal(err)
		}
		f.WriteString(`ID="ubuntu"` + "\n" + `VERSION_ID="22.04"` + "\n")
		f.Close()

		got := parseOsRelease(f.Name())
		if got != "ubuntu 22.04" {
			t.Errorf("got %q, want \"ubuntu 22.04\"", got)
		}
	})

	t.Run("missing version_id", func(t *testing.T) {
		f, err := os.CreateTemp(t.TempDir(), "os-release")
		if err != nil {
			t.Fatal(err)
		}
		f.WriteString("ID=debian\n")
		f.Close()

		got := parseOsRelease(f.Name())
		if got != "debian" {
			t.Errorf("got %q, want \"debian\"", got)
		}
	})

	t.Run("missing file returns linux", func(t *testing.T) {
		got := parseOsRelease("/nonexistent/os-release")
		if got != "linux" {
			t.Errorf("got %q, want \"linux\"", got)
		}
	})
}

func TestCmdVersionOutput(t *testing.T) {
	// capture stdout by redirecting os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stdout
	os.Stdout = w

	cmdErr := cmdVersion(nil)

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	io.Copy(&buf, r)

	if cmdErr != nil {
		t.Fatalf("cmdVersion returned error: %v", cmdErr)
	}

	out := buf.String()
	for _, want := range []string{"snap", "snapd", "series", "os", "kernel", "architecture"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing field %q:\n%s", want, out)
		}
	}
}
