// `snap version` -- print snap and host info, matching the output of
// the real snap CLI:
//
//	snap          2.x.y
//	snapd         2.x.y
//	series        16
//	ubuntu        24.04
//	kernel        6.x.y-generic
//	architecture  amd64

package main

import (
	"bufio"
	"fmt"
	"os"
	"runtime"
	"strings"
	"text/tabwriter"

	"golang.org/x/sys/unix"
)

// snapVersion is the version string reported by `snap version`. it is
// intentionally distinct from real snapd so the two don't get confused
// in bug reports.
const snapVersion = "0.0+chainsawed"

func cmdVersion(_ []string) error {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintf(w, "snap\t%s\n", snapVersion)
	fmt.Fprintf(w, "snapd\t%s\n", snapVersion)
	fmt.Fprintf(w, "series\t16\n")
	fmt.Fprintf(w, "os\t%s\n", osReleasePretty())
	fmt.Fprintf(w, "kernel\t%s\n", unameRelease())
	fmt.Fprintf(w, "architecture\t%s\n", goarchToSnap(runtime.GOARCH))
	return w.Flush()
}

// osReleasePretty returns "ubuntu 24.04" (or equivalent) by reading
// /etc/os-release. falls back to "linux" on any error.
func osReleasePretty() string { return parseOsRelease("/etc/os-release") }

// parseOsRelease reads the given os-release file and returns "<id> <version_id>".
// extracted so tests can supply an arbitrary path.
func parseOsRelease(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return "linux"
	}
	defer f.Close()
	vals := map[string]string{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		v = strings.Trim(v, `"`)
		vals[k] = v
	}
	id := strings.ToLower(vals["ID"])
	ver := vals["VERSION_ID"]
	if id == "" {
		return "linux"
	}
	if ver == "" {
		return id
	}
	return id + " " + ver
}

// unameRelease returns the kernel release string (uname -r). falls
// back to "unknown" if the syscall fails.
func unameRelease() string {
	var u unix.Utsname
	if err := unix.Uname(&u); err != nil {
		return "unknown"
	}
	// Utsname.Release is [65]int8 on linux; convert to string.
	b := make([]byte, 0, len(u.Release))
	for _, c := range u.Release {
		if c == 0 {
			break
		}
		b = append(b, byte(c))
	}
	return string(b)
}

func goarchToSnap(arch string) string {
	switch arch {
	case "amd64":
		return "amd64"
	case "arm64":
		return "arm64"
	case "arm":
		return "armhf"
	case "386":
		return "i386"
	case "s390x":
		return "s390x"
	case "ppc64le":
		return "ppc64el"
	case "riscv64":
		return "riscv64"
	default:
		return arch
	}
}
