// chainsawed prototype: single-binary `snap` with `install` and `run`
// subcommands. install fetches a snap from the store, verifies its
// assertions, and unsquashes it under /snap/<name>/<rev>/. run reads
// the installed snap.yaml, sets up env (incl. LD_LIBRARY_PATH from
// the snap's declared base), and execs the app.
//
// no daemon, no state file, no refresh, no interfaces, no confinement.
// every snap is treated as classic; the container is the security
// boundary.

package main

import (
	"fmt"
	"os"

	"github.com/snapcore/snapd/snap"
)

func init() {
	// upstream daemon installs a real sanitiser via the interfaces
	// package; we have no interfaces, so just no-op it -- otherwise
	// any snap.InfoFromSnapYaml call panics.
	snap.SanitizePlugsSlots = func(*snap.Info) {}
}

const usage = `snap -- minimal install / run prototype

usage:
  snap install [--channel=<chan>] <name>...
                          fetch each <name> from the store, verify
                          assertions, extract to /snap/<name>/<rev>/
                          and wire up /snap/bin/<app> shims. without
                          --channel, tracks latest/stable.
  snap run <name>[.<app>] [args...]
                          run the named app from an installed snap
                          with the right env. invoking via the
                          /snap/bin/<x> shim does the same thing
                          implicitly.
  snap remove <name>...   delete /snap/<name>, /var/snap/<name>,
                          the cached download, and the shims
  snap refresh [--channel=<chan>] [<name>...]
                          re-install each <name> at the latest
                          revision; with no args, refresh every
                          installed snap. without --channel, stays
                          on the channel each snap was installed
                          from. old revisions are pruned automatically.
  snap list               show what's installed under /snap/
  snap info <name>...     print snap metadata; falls back to the
                          store if not installed locally
  snap find <query>...    search the store

every snap is treated as classic. the binary is meant to run inside
a container; nothing else (apparmor, seccomp, mount namespaces, ...)
is set up.
`

func main() {
	if err := run(os.Args[0], os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n", err)
		os.Exit(1)
	}
}

func run(invokedAs string, args []string) error {
	// shim mode: when invoked via a /snap/bin/<x> symlink, argv[0]
	// is the link path. translate to `run <x> <args...>` so the
	// user sees their installed snap, not the snap multitool.
	// the install path creates these symlinks pointing back at
	// this binary -- see cmd/snap/install.go::wireBins.
	if name := shimName(invokedAs); name != "" {
		return cmdRun(append([]string{name}, args...))
	}

	if len(args) == 0 {
		fmt.Print(usage)
		return nil
	}
	switch args[0] {
	case "install":
		return cmdInstall(args[1:])
	case "run":
		return cmdRun(args[1:])
	case "remove":
		return cmdRemove(args[1:])
	case "refresh":
		return cmdRefresh(args[1:])
	case "list":
		return cmdList(args[1:])
	case "info":
		return cmdInfo(args[1:])
	case "find":
		return cmdFind(args[1:])
	case "-h", "--help", "help":
		fmt.Print(usage)
		return nil
	default:
		return fmt.Errorf("unknown subcommand %q (try `snap help`)", args[0])
	}
}

// shimName returns the snap-name (or snap.app) component when the
// binary was invoked via a /snap/bin/<x> symlink, "" otherwise.
// matching is by directory rather than basename so renaming the
// real binary to `snap` doesn't get treated as a shim for itself.
func shimName(invokedAs string) string {
	dir, base := filepathSplit(invokedAs)
	if dir != "/snap/bin" {
		return ""
	}
	return base
}

// filepathSplit avoids importing path/filepath here just for a
// trivial split. argv[0] under /snap/bin is always /snap/bin/<x>
// (the symlinks the install path creates), so a manual rsplit on
// '/' is enough.
func filepathSplit(p string) (dir, base string) {
	for i := len(p) - 1; i >= 0; i-- {
		if p[i] == '/' {
			return p[:i], p[i+1:]
		}
	}
	return "", p
}
