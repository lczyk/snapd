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
)

const usage = `snap -- minimal install / run prototype

usage:
  snap install <name>    fetch <name> from the store, verify
                         assertions, extract to /snap/<name>/<rev>/
                         and wire up /snap/bin/<app> wrappers
  snap run <name>[.<app>]
                         run the named app from an installed snap
                         with the right env

every snap is treated as classic. the binary is meant to run inside
a container; nothing else (apparmor, seccomp, mount namespaces, ...)
is set up.
`

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		fmt.Print(usage)
		return nil
	}
	switch args[0] {
	case "install":
		return cmdInstall(args[1:])
	case "run":
		return cmdRun(args[1:])
	case "-h", "--help", "help":
		fmt.Print(usage)
		return nil
	default:
		return fmt.Errorf("unknown subcommand %q (try `snap help`)", args[0])
	}
}
