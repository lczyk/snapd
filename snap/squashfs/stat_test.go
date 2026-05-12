// table-driven unit tests for stat.go -- the unsquashfs -lln line parser
// and its helpers. these are pure functions: feed in bytes, assert the
// resulting stat fields or the error part.

package squashfs

import (
	"errors"
	"os"
	"testing"
	"time"
)

func TestModeFromTriplet(t *testing.T) {
	cases := []struct {
		name  string
		trip  string
		shift uint
		want  os.FileMode
		err   bool
	}{
		{"all dashes", "---", 0, 0, false},
		{"rwx user shift 2", "rwx", 2, 0700, false},
		{"rwx group shift 1", "rwx", 1, 0070, false},
		{"rwx other shift 0", "rwx", 0, 0007, false},
		{"r-x", "r-x", 2, 0500, false},
		{"-w-", "-w-", 2, 0200, false},
		{"setuid cap S", "--S", 2, 04000, false},
		{"setuid lower s", "--s", 2, 04100, false},
		{"sticky cap T", "--T", 0, 01000, false},
		{"sticky lower t", "--t", 0, 01001, false},
		{"bad r slot", "xrx", 2, 0, true},
		{"bad w slot", "rxx", 2, 0, true},
		{"bad x slot", "rwz", 2, 0, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := modeFromTriplet([]byte(tc.trip), tc.shift)
			if tc.err {
				if err == nil {
					t.Fatalf("expected error, got %v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("modeFromTriplet(%q,%d) = %o, want %o", tc.trip, tc.shift, got, tc.want)
			}
		})
	}
}

func TestModeFromTripletPanicsOnShortInput(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic")
		}
	}()
	_, _ = modeFromTriplet([]byte("rw"), 0)
}

func TestParseMode(t *testing.T) {
	cases := []struct {
		name string
		in   string
		mode os.FileMode
		err  bool
	}{
		{"regular file", "-rwxr-xr-x", 0755, false},
		{"dir", "drwxr-xr-x", os.ModeDir | 0755, false},
		{"symlink", "lrwxrwxrwx", os.ModeSymlink | 0777, false},
		{"socket", "srwxrwxrwx", os.ModeSocket | 0777, false},
		{"char dev", "crw-rw----", os.ModeCharDevice | 0660, false},
		{"block dev", "brw-rw----", os.ModeDevice | 0660, false},
		{"fifo", "prw-r--r--", os.ModeNamedPipe | 0644, false},
		{"unknown type", "?rwxrwxrwx", 0, true},
		{"bad triplet", "-rwzr-xr-x", 0, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			st := &stat{}
			n, err := st.parseMode([]byte(tc.in))
			if tc.err {
				if err == nil {
					t.Fatalf("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected: %v", err)
			}
			if n != 10 {
				t.Errorf("n = %d, want 10", n)
			}
			if st.mode != tc.mode {
				t.Errorf("mode = %o, want %o", st.mode, tc.mode)
			}
		})
	}
}

func TestParseOwner(t *testing.T) {
	cases := []struct {
		name      string
		in        string
		user, grp string
		err       bool
	}{
		{"simple", "root/root ", "root", "root", false},
		{"numeric", "1000/1000 ", "1000", "1000", false},
		{"audio", "root/audio ", "root", "audio", false},
		{"leading space", " oot/root ", "", "", true},
		{"second space", "r /root ", "", "", true},
		{"no slash", "rootroot      ", "", "", true},
		{"empty group", "root/ ", "", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			st := &stat{}
			_, err := st.parseOwner([]byte(tc.in))
			if tc.err {
				if err == nil {
					t.Fatalf("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected: %v", err)
			}
			if st.user != tc.user || st.group != tc.grp {
				t.Errorf("user/group = %q/%q, want %q/%q", st.user, st.group, tc.user, tc.grp)
			}
		})
	}
}

func TestParseSize(t *testing.T) {
	t.Run("regular file", func(t *testing.T) {
		st := &stat{}
		// "         1234 2017-12-08 11:19 ."
		in := []byte("         1234 2017-12-08 11:19 .")
		n, err := st.parseSize(in)
		if err != nil {
			t.Fatalf("unexpected: %v", err)
		}
		if st.size != 1234 {
			t.Errorf("size = %d, want 1234", st.size)
		}
		if in[n] != ' ' {
			t.Errorf("did not stop at space")
		}
	})

	t.Run("device node", func(t *testing.T) {
		st := &stat{mode: os.ModeCharDevice}
		// "        14,  3 2017-12-08 11:19 ."
		in := []byte("        14,  3 2017-12-08 11:19 .")
		_, err := st.parseSize(in)
		if err != nil {
			t.Fatalf("unexpected: %v", err)
		}
		// device nodes don't set size
		if st.size != 0 {
			t.Errorf("size = %d, want 0 for device", st.size)
		}
	})

	t.Run("no digits regular", func(t *testing.T) {
		st := &stat{}
		in := []byte("              x 2017-12-08 11:19 .")
		_, err := st.parseSize(in)
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("no digits device", func(t *testing.T) {
		st := &stat{mode: os.ModeDevice}
		in := []byte("              x 2017-12-08 11:19 .")
		_, err := st.parseSize(in)
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("device missing comma", func(t *testing.T) {
		st := &stat{mode: os.ModeCharDevice}
		in := []byte("            14 5 2017-12-08 11:19 .")
		_, err := st.parseSize(in)
		if err == nil {
			t.Fatal("expected error")
		}
	})
}

func TestParseTimeUTC(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		st := &stat{}
		n, err := st.parseTimeUTC([]byte("2017-12-08 11:19 ."))
		if err != nil {
			t.Fatalf("unexpected: %v", err)
		}
		if n != 16 {
			t.Errorf("n = %d, want 16", n)
		}
		want := time.Date(2017, 12, 8, 11, 19, 0, 0, time.UTC)
		if !st.mtime.Equal(want) {
			t.Errorf("mtime = %v, want %v", st.mtime, want)
		}
	})
	t.Run("bad", func(t *testing.T) {
		st := &stat{}
		_, err := st.parseTimeUTC([]byte("not-a-date-here ."))
		if err == nil {
			t.Fatal("expected error")
		}
	})
}

func TestParsePath(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
		err  bool
	}{
		{"root", ".", "/", false},
		{"nested", "./foo/bar", "/foo/bar", false},
		{"symlink form", "./link -> target", "/link -> target", false},
		{"bad", "foo", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			st := &stat{}
			_, err := st.parsePath([]byte(tc.in))
			if tc.err {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected: %v", err)
			}
			if st.path != tc.want {
				t.Errorf("path = %q, want %q", st.path, tc.want)
			}
		})
	}
}

func TestFromRaw(t *testing.T) {
	t.Run("regular file", func(t *testing.T) {
		// the exact column layout matches unsquashfs -lln output.
		line := []byte("-rwxr-xr-x root/root              1234 2017-12-08 11:19 ./hello")
		st, err := fromRaw(line)
		if err != nil {
			t.Fatalf("unexpected: %v", err)
		}
		if st.mode != 0755 {
			t.Errorf("mode = %o, want 0755", st.mode)
		}
		if st.size != 1234 {
			t.Errorf("size = %d, want 1234", st.size)
		}
		if st.user != "root" || st.group != "root" {
			t.Errorf("owner = %q/%q", st.user, st.group)
		}
		if st.path != "/hello" {
			t.Errorf("path = %q, want /hello", st.path)
		}
		// FileInfo-shaped methods
		if st.Name() != "hello" {
			t.Errorf("Name = %q", st.Name())
		}
		if st.Size() != 1234 {
			t.Errorf("Size = %d", st.Size())
		}
		if st.Mode() != 0755 {
			t.Errorf("Mode = %o", st.Mode())
		}
		if st.IsDir() {
			t.Error("IsDir true")
		}
		if st.Sys() != nil {
			t.Error("Sys not nil")
		}
		if st.Path() != "/hello" {
			t.Error("Path mismatch")
		}
		if !st.ModTime().Equal(time.Date(2017, 12, 8, 11, 19, 0, 0, time.UTC)) {
			t.Errorf("ModTime = %v", st.ModTime())
		}
	})

	t.Run("directory", func(t *testing.T) {
		line := []byte("drwxr-xr-x root/root                65 2017-12-08 11:19 .")
		st, err := fromRaw(line)
		if err != nil {
			t.Fatalf("unexpected: %v", err)
		}
		if !st.IsDir() {
			t.Error("not IsDir")
		}
		if st.path != "/" {
			t.Errorf("path = %q, want /", st.path)
		}
	})

	t.Run("symlink", func(t *testing.T) {
		line := []byte("lrwxrwxrwx root/root                 5 2017-12-08 11:19 ./link -> hello")
		st, err := fromRaw(line)
		if err != nil {
			t.Fatalf("unexpected: %v", err)
		}
		if st.mode&os.ModeSymlink == 0 {
			t.Error("symlink bit unset")
		}
		if st.path != "/link" {
			t.Errorf("path = %q, want /link (symlink target stripped)", st.path)
		}
	})

	t.Run("symlink missing arrow", func(t *testing.T) {
		// the path parses successfully but the symlink arrow split fails
		line := []byte("lrwxrwxrwx root/root                 5 2017-12-08 11:19 ./link-no-arrow")
		if _, err := fromRaw(line); err == nil {
			t.Fatal("expected error for symlink w/out ' -> '")
		}
	})

	t.Run("too short", func(t *testing.T) {
		if _, err := fromRaw([]byte("nope")); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("missing separator space", func(t *testing.T) {
		// no space between parts -- swap the space after mode for a tab-like junk byte
		line := []byte("-rwxr-xr-xXroot/root              1234 2017-12-08 11:19 ./hello")
		if _, err := fromRaw(line); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("device node", func(t *testing.T) {
		line := []byte("crw-rw---- root/audio           14,  3 2017-12-05 10:29 ./dev/dsp")
		st, err := fromRaw(line)
		if err != nil {
			t.Fatalf("unexpected: %v", err)
		}
		if st.mode&os.ModeCharDevice == 0 {
			t.Error("char device bit unset")
		}
		if st.path != "/dev/dsp" {
			t.Errorf("path = %q", st.path)
		}
	})
}

func TestStatErrorMessage(t *testing.T) {
	// each err* helper returns a statError; Error() must mention the part.
	cases := []struct {
		name string
		err  error
		part string
	}{
		{"line", errBadLine([]byte("x")), "line"},
		{"mode", errBadMode([]byte("x")), "mode"},
		{"owner", errBadOwner([]byte("x")), "owner"},
		{"node", errBadNode([]byte("x")), "node"},
		{"size", errBadSize([]byte("x")), "size"},
		{"time", errBadTime([]byte("x")), "time"},
		{"path", errBadPath([]byte("x")), "path"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			msg := tc.err.Error()
			if msg == "" {
				t.Fatal("empty error")
			}
			var se statError
			if !errors.As(tc.err, &se) {
				t.Fatalf("not a statError: %T", tc.err)
			}
			if se.part != tc.part {
				t.Errorf("part = %q, want %q", se.part, tc.part)
			}
		})
	}
}
