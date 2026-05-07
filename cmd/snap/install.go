// `snap install <name>` -- fetch from the store and unsquash to
// /snap/<name>/<rev>/. recursively installs the declared base
// (core24, core22, ...) when missing. wires up /snap/bin/<app>
// shims pointing back at this binary so `snap run <app>` works.
//
// no daemon, no state file. the on-disk layout is the only state:
// the presence of /snap/<name>/<rev>/meta/snap.yaml means it's
// installed. /snap/<name>/current symlinks the latest revision.

package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/snapcore/snapd/progress"
	"github.com/snapcore/snapd/snap"
	"github.com/snapcore/snapd/snap/squashfs"
	"github.com/snapcore/snapd/store"
)

func cmdInstall(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("install needs a snap name")
	}
	for _, name := range args {
		if err := installOne(name); err != nil {
			return fmt.Errorf("install %s: %w", name, err)
		}
	}
	return nil
}

// installOne does the fetch + unsquash + base-recursion + bin-wire dance
// for a single snap. idempotent on the snap name + revision: if the
// revision is already extracted under /snap/<name>/<rev>/, it skips
// the download but still re-runs the post-install steps.
func installOne(name string) error {
	st := store.New(nil, nil)
	ctx := context.Background()

	info, err := st.SnapInfo(ctx, store.SnapSpec{Name: name}, nil)
	if err != nil {
		return fmt.Errorf("fetch info: %w", err)
	}

	mountDir := filepath.Join("/snap", info.SnapName(), info.Revision.String())
	if _, statErr := os.Stat(filepath.Join(mountDir, "meta", "snap.yaml")); statErr == nil {
		fmt.Printf("%s %s already installed at %s\n", info.SnapName(), info.Revision, mountDir)
	} else {
		if err := download(ctx, st, info); err != nil {
			return err
		}
		if err := extractTo(snapDownloadPath(info), mountDir); err != nil {
			return err
		}
		fmt.Printf("installed %s %s -> %s\n", info.SnapName(), info.Revision, mountDir)
	}

	if err := updateCurrent(info); err != nil {
		return fmt.Errorf("update current symlink: %w", err)
	}

	// re-read snap.yaml from the extracted tree -- it has the apps
	// and the declared base (which info.Base from the store should
	// also report, but reading from the extracted file is the source
	// of truth once extracted).
	yamlBytes, err := os.ReadFile(filepath.Join(mountDir, "meta", "snap.yaml"))
	if err != nil {
		return fmt.Errorf("read snap.yaml: %w", err)
	}
	installedInfo, err := snap.InfoFromSnapYaml(yamlBytes)
	if err != nil {
		return fmt.Errorf("parse snap.yaml: %w", err)
	}

	if base := installedInfo.Base; base != "" && base != "none" && base != "bare" {
		if !isInstalled(base) {
			fmt.Printf("installing base %s\n", base)
			if err := installOne(base); err != nil {
				return fmt.Errorf("install base %s: %w", base, err)
			}
		}
	}

	if err := wireBins(installedInfo, mountDir); err != nil {
		return fmt.Errorf("wire /snap/bin shims: %w", err)
	}
	return nil
}

func download(ctx context.Context, st *store.Store, info *snap.Info) error {
	target := snapDownloadPath(info)
	if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
		return err
	}
	pbar := progress.MakeProgressBar(os.Stdout)
	defer pbar.Finished()
	if err := st.Download(ctx, info.SnapName(), target, &info.DownloadInfo, pbar, nil, nil); err != nil {
		return fmt.Errorf("download: %w", err)
	}
	return nil
}

func snapDownloadPath(info *snap.Info) string {
	return filepath.Join("/var/lib/snapd/snaps",
		fmt.Sprintf("%s_%s.snap", info.SnapName(), info.Revision))
}

func extractTo(snapPath, mountDir string) error {
	if err := os.MkdirAll(mountDir, 0755); err != nil {
		return err
	}
	sq := squashfs.New(snapPath)
	if err := sq.Unpack("*", mountDir); err != nil {
		return fmt.Errorf("unsquash: %w", err)
	}
	return nil
}

// updateCurrent points /snap/<name>/current at the just-installed rev.
// uses an atomic rename of a temp symlink so concurrent installs of
// the same snap don't see a half-broken pointer.
func updateCurrent(info *snap.Info) error {
	parent := filepath.Join("/snap", info.SnapName())
	cur := filepath.Join(parent, "current")
	tmp := filepath.Join(parent, ".current.new")
	_ = os.Remove(tmp)
	if err := os.Symlink(info.Revision.String(), tmp); err != nil {
		return err
	}
	return os.Rename(tmp, cur)
}

func isInstalled(name string) bool {
	_, err := os.Stat(filepath.Join("/snap", name, "current", "meta", "snap.yaml"))
	return err == nil
}

// wireBins makes /snap/bin/<app> -> /usr/bin/snap so that running the
// shim ends up in cmdRun, which sets up env and execs the real app.
// this binary itself must live at /usr/bin/snap; if the user installed
// it elsewhere, set SNAP_SELF to point at it.
func wireBins(info *snap.Info, _ string) error {
	if err := os.MkdirAll("/snap/bin", 0755); err != nil {
		return err
	}
	target := os.Getenv("SNAP_SELF")
	if target == "" {
		target = "/usr/bin/snap"
	}
	for appName := range info.Apps {
		var link string
		if appName == info.SnapName() {
			link = filepath.Join("/snap/bin", appName)
		} else {
			link = filepath.Join("/snap/bin", info.SnapName()+"."+appName)
		}
		// symlink races are easier to ignore than fight: rm first.
		_ = os.Remove(link)
		if err := os.Symlink(target, link); err != nil {
			return fmt.Errorf("symlink %s: %w", link, err)
		}
	}
	return nil
}

// io.Discard placeholder so the import isn't dropped if I add a
// Reader path later. keeps the file lint-clean for now.
var _ = io.Discard
