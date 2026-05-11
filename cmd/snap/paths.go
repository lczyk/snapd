package main

import (
	"os"
	"path/filepath"
)

// snapRoot is the filesystem root for all snap-managed paths. in production
// this is "/" (the real root). set SNAP_ROOT to redirect to a temp dir for
// tests -- no root permissions needed and the host filesystem is untouched.
var snapRoot = func() string {
	if r := os.Getenv("SNAP_ROOT"); r != "" {
		return r
	}
	return "/"
}()

var (
	snapMountDir    = filepath.Join(snapRoot, "snap")
	snapBinDir      = filepath.Join(snapRoot, "snap", "bin")
	snapDataDir     = filepath.Join(snapRoot, "var", "snap")
	snapDownloadDir = filepath.Join(snapRoot, "var", "lib", "snapd", "snaps")
	snapAssertsDir  = filepath.Join(snapRoot, "var", "lib", "snapd", "assertions")
	supervisorDir   = filepath.Join(snapRoot, "run", "snapd", "supervisors")
	daemonLogDir    = filepath.Join(snapRoot, "var", "log", "snapd")
)
